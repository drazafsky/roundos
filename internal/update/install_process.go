package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var errUpdateChildrenActive = errors.New("update subprocesses could not be stopped")

// A shell timeout must stop its install/mv/sync descendants before recovery
// touches the same files. systemd's cgroup remains a second cleanup boundary.
func runUpdateCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = time.Second
	if err := command.Start(); err != nil {
		return err
	}
	err := command.Wait()
	return errors.Join(err, stopUpdateGroup(command.Process.Pid))
}

func stopUpdateGroup(group int) error {
	if err := syscall.Kill(-group, syscall.SIGKILL); errors.Is(err, syscall.ESRCH) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: group %d: %v", errUpdateChildrenActive, group, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		active, err := updateGroupActive(group)
		if err != nil {
			return fmt.Errorf("%w: %v", errUpdateChildrenActive, err)
		}
		if !active {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: group %d", errUpdateChildrenActive, group)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func updateGroupActive(group int) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		end := strings.LastIndexByte(string(data), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(data[end+1:]))
		if len(fields) < 3 {
			continue
		}
		pgid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		// Zombies cannot modify files; PID 1 may reap an orphan asynchronously.
		if pgid == group && fields[0] != "Z" && fields[0] != "X" {
			return true, nil
		}
	}
	return false, nil
}
