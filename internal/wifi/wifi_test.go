package wifi

import (
	"context"
	"os"
	"strings"
	"testing"
)

type mockRunner struct{ calls [][]string }

func (r *mockRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, args)
	s := strings.Join(args, " ")
	if strings.Contains(s, "device status") {
		return "eth0:ethernet:connected\nwlan0:wifi:connected\n", nil
	}
	if strings.Contains(s, "wifi list") {
		return "*:Home\\:Desk:90:WPA2\n:Guest:55:--\n:Home\\:Desk:42:WPA2\n", nil
	}
	return "ok", nil
}
func TestTerseEscapedAndScan(t *testing.T) {
	c := &Client{Runner: &mockRunner{}}
	s, e := c.Scan(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if s.Interface != "wlan0" || s.Current != "Home:Desk" || len(s.Networks) != 2 || s.Networks[0].SSID != "Home:Desk" || !s.Networks[0].Connected {
		t.Fatalf("bad scan %+v", s)
	}
}
func TestNoPasswordInArguments(t *testing.T) {
	r := &mockRunner{}
	c := &Client{Runner: r}
	err := c.Connect(context.Background(), "wlan0", Network{SSID: "Cafe", Security: "WPA2"}, "super-secret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range r.calls {
		if strings.Contains(strings.Join(call, " "), "super-secret-passphrase") {
			t.Fatal("password leaked in process arguments")
		}
	}
}
func TestUnsupportedAndValidation(t *testing.T) {
	r := &mockRunner{}
	c := &Client{Runner: r}
	if err := c.Connect(context.Background(), "wlan0", Network{SSID: "Corp", Security: "WPA2 802.1X"}, "secret123"); err == nil {
		t.Fatal("expected enterprise rejection")
	}
	if err := c.Connect(context.Background(), "wlan0", Network{SSID: "Private", Security: "WPA2"}, "short"); err == nil {
		t.Fatal("short password accepted")
	}
	if len(r.calls) != 0 {
		t.Fatalf("unexpected command execution %+v", r.calls)
	}
}
func TestOpenNetwork(t *testing.T) {
	r := &mockRunner{}
	c := &Client{Runner: r}
	if e := c.Connect(context.Background(), "wlan0", Network{SSID: "Guest", Security: "--"}, ""); e != nil {
		t.Fatal(e)
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls=%d", len(r.calls))
	}
}

type credentialInspector struct {
	path  string
	mode  os.FileMode
	value string
}

func (r *credentialInspector) Run(_ context.Context, args ...string) (string, error) {
	for i, word := range args {
		if word == "passwd-file" && i+1 < len(args) {
			r.path = args[i+1]
			stat, err := os.Stat(r.path)
			if err != nil {
				return "", err
			}
			r.mode = stat.Mode().Perm()
			content, err := os.ReadFile(r.path)
			if err != nil {
				return "", err
			}
			r.value = string(content)
		}
	}
	return "ok", nil
}
func TestCredentialFileTemporaryAndPrivate(t *testing.T) {
	r := &credentialInspector{}
	c := &Client{Runner: r}
	if e := c.Connect(context.Background(), "wlan0", Network{SSID: "Desk", Security: "WPA2"}, "valid-password"); e != nil {
		t.Fatal(e)
	}
	if r.mode != 0600 {
		t.Fatalf("credential mode %o", r.mode)
	}
	if r.value != "802-11-wireless-security.psk:valid-password\n" {
		t.Fatal("missing protected secret")
	}
	if _, e := os.Stat(r.path); !os.IsNotExist(e) {
		t.Fatalf("secret file still exists: %v", e)
	}
}
