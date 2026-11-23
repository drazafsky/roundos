package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"roundos/internal/timerstate"
	"roundos/internal/wire"
)

type timerTraffic struct {
	Bytes        int    `json:"bytes"`
	Ticks        int    `json:"ticks"`
	FullTrees    int    `json:"full_trees"`
	BinarySHA256 string `json:"binary_sha256"`
}

// Exercise the actual Timer executable and framed transport. Startup frames
// are excluded; every steady-state message and modern ACK is counted.
func measureTimerTraffic(t *testing.T, executable string, modern bool) timerTraffic {
	t.Helper()
	directory := t.TempDir()
	state := &timerstate.Timer{Path: filepath.Join(directory, "timer.json"), Remaining: 20 * time.Second, Deadline: time.Now().Add(20 * time.Second), Running: true}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(directory, "ipc"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.SetDeadline(time.Now().Add(10 * time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable)
	command.Env = append(os.Environ(), "ROUNDOS_SOCKET="+listener.Addr().String(), "ROUNDOS_APP_ID="+wire.TimerID, "ROUNDOS_TOKEN=m7-isolated-fixture", "ROUNDOS_STATE_DIR="+directory, "ROUNDOS_RUI_DEV_DIR=")
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); command.Wait() }()
	connection, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(15 * time.Second))
	hello, err := wire.Read(connection)
	if err != nil || hello.Type != "hello" || hello.App != wire.TimerID || hello.Token != "m7-isolated-fixture" {
		t.Fatal("invalid isolated Timer handshake", err)
	}
	visibility := wire.Message{Version: wire.Version, Type: "visibility", Visible: true}
	if modern {
		visibility.Capabilities = wire.Capabilities()
	}
	if err := wire.Write(connection, visibility); err != nil {
		t.Fatal(err)
	}
	bytesOnWire := func(message wire.Message) int {
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		return 4 + len(data)
	}
	var result timerTraffic
	var current wire.View
	ready := false
	label := ""
	for result.Ticks < 5 {
		message, err := wire.Read(connection)
		if err != nil {
			t.Fatal("Timer update", err)
		}
		if modern && message.Type == "view" && message.LayoutRevision == 0 {
			continue
		}
		switch message.Type {
		case "view":
			if message.View == nil {
				t.Fatal("missing Timer snapshot")
			}
			current = wire.CopyView(*message.View)
		case "widget_patch":
			current, err = wire.ApplyProperties(current, message.Updates)
			if err != nil {
				t.Fatal(err)
			}
		default:
			continue
		}
		ackBytes := 0
		if modern {
			ack := wire.Message{Version: wire.Version, Type: "view_ack", LayoutRevision: message.LayoutRevision, NewRevision: message.NewRevision}
			if err := wire.Write(connection, ack); err != nil {
				t.Fatal(err)
			}
			ackBytes = bytesOnWire(ack)
		}
		if !ready {
			ready = true
			label = current.Primary
			continue
		}
		result.Bytes += bytesOnWire(message) + ackBytes
		if message.Type == "view" && len(current.Screens) > 0 {
			result.FullTrees++
		}
		if current.Primary != label {
			result.Ticks++
			label = current.Primary
		}
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	result.BinarySHA256 = fmt.Sprintf("%x", sha256.Sum256(binary))
	return result
}

func TestM7ActualTimerTrafficAgainstV050(t *testing.T) {
	if testing.Short() {
		t.Skip("measures ten seconds of real Timer processes")
	}
	legacy := legacySource(t)
	current, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	oldExecutable := build(t, "./cmd/round-timer", "old-timer", legacy)
	newExecutable := os.Getenv("ROUNDOS_M7_TIMER_BINARY")
	if newExecutable == "" {
		newExecutable = build(t, "./cmd/round-timer", "new-timer", current)
	}
	old := measureTimerTraffic(t, oldExecutable, false)
	next := measureTimerTraffic(t, newExecutable, true)
	if next.FullTrees != 0 || next.Bytes*2 > old.Bytes {
		t.Fatalf("Timer traffic budget failed: baseline=%+v candidate=%+v", old, next)
	}
	report := struct {
		Architecture                string       `json:"architecture"`
		BaselineSourceArchiveSHA256 string       `json:"baseline_source_archive_sha256"`
		Baseline                    timerTraffic `json:"baseline"`
		Candidate                   timerTraffic `json:"candidate"`
		ReductionPercent            float64      `json:"reduction_percent"`
	}{runtime.GOARCH, "06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75", old, next, 100 * (1 - float64(next.Bytes)/float64(old.Bytes))}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("M7_TIMER_PROTOCOL %s", data)
}
