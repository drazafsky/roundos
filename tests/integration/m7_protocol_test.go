package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"roundos/internal/timerstate"
	"roundos/internal/wire"
)

type timerTraffic struct {
	Bytes        int64  `json:"bytes"`
	Ticks        int    `json:"ticks"`
	FullTrees    int    `json:"full_trees"`
	BinarySHA256 string `json:"binary_sha256"`
}

type trafficReader struct {
	io.Reader
	bytes int64
}

func (r *trafficReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	r.bytes += int64(n)
	return n, err
}

type trafficWriter struct {
	io.Writer
	bytes int64
}

func (w *trafficWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	w.bytes += int64(n)
	return n, err
}

type timerTrafficMeter struct {
	modern, ready bool
	current       wire.View
	label         string
	result        timerTraffic
}

func (m *timerTrafficMeter) accept(message wire.Message, rawBytes int64) (bool, error) {
	wasReady := m.ready
	if wasReady {
		m.result.Bytes += rawBytes
	}
	if m.modern && message.Type == "view" && message.LayoutRevision == 0 {
		if wasReady {
			return false, fmt.Errorf("unrevisioned snapshot after initialization")
		}
		return false, nil
	}
	switch message.Type {
	case "view":
		if message.View == nil {
			return false, fmt.Errorf("missing Timer snapshot")
		}
		m.current = wire.CopyView(*message.View)
	case "widget_patch":
		var err error
		m.current, err = wire.ApplyProperties(m.current, message.Updates)
		if err != nil {
			return false, err
		}
	case "view_patch":
		if m.modern || !m.ready || message.Patch == nil {
			return false, fmt.Errorf("unexpected legacy patch")
		}
		m.current = message.Patch.Apply(m.current)
	default:
		return false, nil
	}
	if !wasReady {
		if message.Type != "view" || len(m.current.Screens) == 0 {
			return false, fmt.Errorf("missing initial layout")
		}
		m.ready = true
		m.label = m.current.Primary
		return true, nil
	}
	if message.Type == "view" && len(m.current.Screens) > 0 {
		m.result.FullTrees++
	}
	if m.current.Primary != m.label {
		m.result.Ticks++
		m.label = m.current.Primary
	}
	return true, nil
}

// Exercise the actual Timer executable and framed transport. Startup frames
// are excluded; every steady-state message and modern ACK is counted.
func measureTimerTraffic(t *testing.T, executable string, modern bool) timerTraffic {
	t.Helper()
	image, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	imageHash := sha256.Sum256(image)
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
	reader, writer := &trafficReader{Reader: connection}, &trafficWriter{Writer: connection}
	meter := timerTrafficMeter{modern: modern}
	draining := false
	for {
		before := reader.bytes
		message, err := wire.Read(reader)
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() && draining && reader.bytes == before {
				break
			}
			t.Fatal("Timer update", err)
		}
		wasReady := meter.ready
		stateFrame, err := meter.accept(message, reader.bytes-before)
		if err != nil {
			t.Fatal(err)
		}
		if modern && stateFrame {
			ack := wire.Message{Version: wire.Version, Type: "view_ack", LayoutRevision: message.LayoutRevision, NewRevision: message.NewRevision}
			before := writer.bytes
			if err := wire.Write(writer, ack); err != nil {
				t.Fatal(err)
			}
			if wasReady {
				meter.result.Bytes += writer.bytes - before
			}
		}
		// Include ACK-triggered trailing frames belonging to the fifth tick.
		if meter.result.Ticks >= 5 && !draining {
			draining = true
			connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		}
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(binary) != imageHash {
		t.Fatal("Timer executable changed during measurement")
	}
	meter.result.BinarySHA256 = fmt.Sprintf("%x", imageHash)
	return meter.result
}

func TestTimerMeterCountsRawAndAuxiliaryFrames(t *testing.T) {
	view := wire.View{Primary: "10", Screens: []wire.Screen{{ID: "root", Title: "Timer"}}}
	meter := timerTrafficMeter{modern: true}
	if _, err := meter.accept(wire.Message{Type: "view", View: &view, LayoutRevision: 1}, 0); err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	for _, body := range []string{
		`{ "version":7, "type":"diagnostic", "padding":"` + strings.Repeat("x", 500) + `" }`,
		`{ "version":7, "type":"widget_patch", "padding":"` + strings.Repeat("y", 500) + `", "updates":[{"property":"primary","string":"09"}] }`,
	} {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(body)))
		stream.Write(header[:])
		stream.WriteString(body)
	}
	want := int64(stream.Len())
	reader := &trafficReader{Reader: &stream}
	for stream.Len() > 0 {
		before := reader.bytes
		message, err := wire.Read(reader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := meter.accept(message, reader.bytes-before); err != nil {
			t.Fatal(err)
		}
	}
	if meter.result.Bytes != want || meter.result.Ticks != 1 {
		t.Fatal("raw or auxiliary frame bytes were dropped", meter.result, want)
	}
}

func TestTimerMeterRejectsPostStartupLegacySnapshots(t *testing.T) {
	view := wire.View{Primary: "10", Screens: []wire.Screen{{ID: "root", Title: "Timer"}}}
	meter := timerTrafficMeter{modern: true}
	legacy := wire.Message{Type: "view", View: &view}
	if _, err := meter.accept(legacy, 100); err != nil || meter.ready {
		t.Fatal("startup exemption failed", err)
	}
	modern := legacy
	modern.LayoutRevision = 1
	if _, err := meter.accept(modern, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := meter.accept(legacy, 100); err == nil {
		t.Fatal("post-startup full tree ignored")
	}
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
