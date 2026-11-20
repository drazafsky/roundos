// Package wifi integrates RoundOS Settings with Raspberry Pi OS NetworkManager.
// All nmcli calls run outside the shell UI loop. Passwords are never command
// arguments, logs, snapshots or persistent RoundOS state.
package wifi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Network struct {
	SSID      string
	Signal    int
	Security  string
	Connected bool
}
type Status struct {
	Interface, Current string
	Networks           []Network
}
type Runner interface {
	Run(context.Context, ...string) (string, error)
}
type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "nmcli", args...)
	// Do not expose command output in errors: some NetworkManager failures
	// can reveal connection identifiers or secret-file contents.
	b, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("NetworkManager command failed: %w", err)
	}
	return string(b), nil
}

type Client struct{ Runner Runner }

func New() *Client { return &Client{Runner: CommandRunner{}} }
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	return c.Runner.Run(ctx, args...)
}

// splitTerse respects nmcli's escaped colon and escaped backslash fields.
func splitTerse(line string) []string {
	var result []string
	var b strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == ':' {
			result = append(result, b.String())
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return append(result, b.String())
}
func (c *Client) Interface(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "-t", "-f", "DEVICE,TYPE,STATE", "device", "status")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		p := splitTerse(line)
		if len(p) >= 2 && p[1] == "wifi" {
			return p[0], nil
		}
	}
	return "", errors.New("no Wi-Fi interface found")
}
func (c *Client) Scan(ctx context.Context) (Status, error) {
	iface, err := c.Interface(ctx)
	if err != nil {
		return Status{}, err
	}
	out, err := c.run(ctx, "-t", "--escape", "yes", "-f", "IN-USE,SSID,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", "yes", "ifname", iface)
	if err != nil {
		return Status{Interface: iface}, err
	}
	result := Status{Interface: iface}
	best := map[string]Network{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		p := splitTerse(line)
		if len(p) < 4 {
			continue
		}
		ssid := p[1]
		if ssid == "" || ssid == "--" {
			continue
		}
		signal, _ := strconv.Atoi(p[2])
		if signal < 0 {
			signal = 0
		}
		if signal > 100 {
			signal = 100
		}
		n := Network{SSID: ssid, Signal: signal, Security: p[3], Connected: p[0] == "*"}
		if n.Connected {
			result.Current = ssid
		}
		if old, ok := best[ssid]; !ok || n.Connected || (!old.Connected && n.Signal > old.Signal) {
			best[ssid] = n
		}
	}
	for _, n := range best {
		result.Networks = append(result.Networks, n)
	}
	sort.Slice(result.Networks, func(i, j int) bool {
		a, b := result.Networks[i], result.Networks[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Signal != b.Signal {
			return a.Signal > b.Signal
		}
		return strings.ToLower(a.SSID) < strings.ToLower(b.SSID)
	})
	return result, nil
}
func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
func (c *Client) Connect(ctx context.Context, iface string, n Network, password string) error {
	if iface == "" || n.SSID == "" {
		return errors.New("Wi-Fi device and network are required")
	}
	if len([]byte(n.SSID)) > 32 {
		return errors.New("invalid network name")
	}
	secured := n.Security != "" && n.Security != "--"
	if strings.Contains(strings.ToUpper(n.Security), "802.1X") || strings.Contains(strings.ToUpper(n.Security), "EAP") {
		return errors.New("enterprise Wi-Fi requires an existing NetworkManager profile")
	}
	if !secured {
		_, err := c.run(ctx, "--wait", "40", "device", "wifi", "connect", n.SSID, "ifname", iface)
		return err
	}
	if strings.Contains(strings.ToUpper(n.Security), "WEP") {
		return errors.New("legacy WEP networks are not supported")
	}
	if len(password) < 8 || len(password) > 63 {
		return errors.New("Wi-Fi passphrase must have 8–63 characters")
	}
	uuid, err := randomUUID()
	if err != nil {
		return errors.New("could not create connection ID")
	}
	auth := "wpa-psk"
	if strings.Contains(strings.ToUpper(n.Security), "WPA3") || strings.Contains(strings.ToUpper(n.Security), "SAE") {
		auth = "sae"
	}
	_, err = c.run(ctx, "connection", "add", "type", "wifi", "ifname", iface, "con-name", "RoundOS "+n.SSID, "ssid", n.SSID, "connection.uuid", uuid, "wifi-sec.key-mgmt", auth)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = c.run(cleanCtx, "connection", "delete", "uuid", uuid)
		}
	}()
	// Use a temporary secret file in tmpfs where possible, never command-line
	// arguments. NM will store credentials in its standard protected profile.
	dir := "/dev/shm"
	if fi, e := os.Stat(dir); e != nil || !fi.IsDir() {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "roundos-wifi-*")
	if err != nil {
		return errors.New("could not prepare Wi-Fi credentials")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return errors.New("could not protect Wi-Fi credentials")
	}
	if _, err = fmt.Fprintln(f, "802-11-wireless-security.psk:"+password); err != nil {
		return errors.New("could not prepare Wi-Fi credentials")
	}
	if err = f.Sync(); err != nil {
		return errors.New("could not prepare Wi-Fi credentials")
	}
	_, err = c.run(ctx, "--wait", "40", "connection", "up", "uuid", uuid, "ifname", iface, "passwd-file", f.Name())
	if err != nil {
		return err
	}
	success = true
	return nil
}
func (c *Client) Disconnect(ctx context.Context, iface string) error {
	if iface == "" {
		return errors.New("Wi-Fi device is missing")
	}
	_, err := c.run(ctx, "device", "disconnect", iface)
	return err
}
