package devtool

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSSHArgumentsRemainLiteral(t *testing.T) {
	s := &SSHSender{Target: "user@pi", Tool: "/path with spaces/round'ui", Session: "/tmp/a'$(touch SHOULD_NOT_EXIST);x/session.json", User: "roundos-app"}
	command, err := s.RemoteCommand()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-c", "set -- "+command+"; printf '%s\\n' \"$@\"").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sudo", "-n", "-u", "roundos-app", "--", s.Tool, "dev-agent", "-session", s.Session}
	if got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments changed: %#v", got)
	}
	s.Target = "-oProxyCommand=bad"
	if _, err := s.RemoteCommand(); err == nil {
		t.Fatal("SSH option injection accepted")
	}
}

type recordingSender struct{ sent chan []byte }

func (s *recordingSender) Send(_ context.Context, data []byte) (bool, error) {
	s.sent <- bytes.Clone(data)
	return false, nil
}
func (*recordingSender) Close() error { return nil }

func TestWatchDebouncesAndRetainsLastGood(t *testing.T) {
	source := []byte("schema: roundos.rui/v1\napplication:\n  id: org.example.test\nscreens:\n  - id: root\n    title: First\n    layout: list\n    groups:\n      - id: one\n        widgets:\n          - id: label\n            type: label\n            label: Hello\n")
	path := filepath.Join(t.TempDir(), "app.rui.yaml")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{sent: make(chan []byte, 10)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Watch(ctx, path, sender, io.Discard) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	receive := func() []byte {
		t.Helper()
		select {
		case data := <-sender.sent:
			return data
		case <-time.After(3 * time.Second):
			t.Fatal("reload not sent")
			return nil
		}
	}
	first := receive()
	if err := os.WriteFile(path, []byte("invalid source"), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(450 * time.Millisecond)
	select {
	case <-sender.sent:
		t.Fatal("invalid source sent")
	default:
	}
	second := bytes.Replace(source, []byte("First"), []byte("Second"), 1)
	if err := os.WriteFile(path, second, 0600); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, receive()) {
		t.Fatal("valid edit did not produce new bundle")
	}
}

func TestSSHStreamPersistsAndCancellationReapsProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "launches")
	mock := `#!/usr/bin/env python3
import json,struct,sys,time,os
with open(os.environ['RUI_SSH_MARKER'],'a') as f: f.write('start\n')
while True:
 h=sys.stdin.buffer.read(4)
 if not h: break
 n=struct.unpack('>I',h)[0];sys.stdin.buffer.read(n)
 if os.environ.get('RUI_SSH_HANG'): time.sleep(30)
 data=b'{}';sys.stdout.buffer.write(struct.pack('>I',len(data))+data);sys.stdout.buffer.flush()
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(mock), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RUI_SSH_MARKER", marker)
	sender := &SSHSender{Target: "user@pi", Tool: "/bin/roundui", Session: "/private/session.json"}
	defer sender.Close()
	for range 2 {
		if _, err := sender.Send(context.Background(), []byte("compiled")); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "start\n" {
		t.Fatal("SSH was not reused", err, string(data))
	}
	sender.Close()
	t.Setenv("RUI_SSH_HANG", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := sender.Send(ctx, []byte("compiled")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("stalled SSH not canceled", err)
	}
	if sender.cmd != nil {
		t.Fatal("SSH process was not reaped")
	}
}
