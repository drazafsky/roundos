package update

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUpdateCommandCancellationStopsDescendantsBeforeReturning(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	late := filepath.Join(dir, "late")
	script := filepath.Join(dir, "installer.sh")
	if err := os.WriteFile(script, []byte("touch \"$1\"\n(sleep 0.2; printf late > \"$2\") &\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", script, started, late)
	result := make(chan error, 1)
	go func() { result <- runUpdateCommand(command) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("test installer did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled command reported success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("command descendants did not stop")
	}
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Stat(late); !os.IsNotExist(err) {
		t.Fatal("installer descendant wrote after command cancellation returned")
	}
}

func TestUpdateGroupScanToleratesProcessExitRaces(t *testing.T) {
	for _, exited := range []error{os.ErrNotExist, syscall.ESRCH} {
		active, err := updateGroupActiveUsing(4242, func(string) ([]byte, error) {
			return nil, &os.PathError{Op: "read", Path: "/proc/exited/stat", Err: exited}
		})
		if err != nil || active {
			t.Fatalf("exited process blocked recovery: active=%t, %v", active, err)
		}
	}
	active, err := updateGroupActiveUsing(4242, func(string) ([]byte, error) {
		return []byte("123 (installer child) S 1 4242 0"), nil
	})
	if err != nil || !active {
		t.Fatalf("live installer child was missed: active=%t, %v", active, err)
	}
	_, err = updateGroupActiveUsing(4242, func(string) ([]byte, error) { return nil, syscall.EACCES })
	if !errors.Is(err, syscall.EACCES) {
		t.Fatal("unverifiable process state was treated as safe")
	}
}
