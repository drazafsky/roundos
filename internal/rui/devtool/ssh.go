package devtool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"roundos/internal/rui/devproto"
)

type SSHSender struct {
	Target, Tool, Session, User string
	cmd                         *exec.Cmd
	input                       io.WriteCloser
	output                      io.ReadCloser
}

func quote(argument string) string { return "'" + strings.ReplaceAll(argument, "'", "'\"'\"'") + "'" }

// RemoteCommand quotes each POSIX argument independently. Bundle data and
// session secrets are never interpreted as command text.
func (s *SSHSender) RemoteCommand() (string, error) {
	if s.Target == "" || strings.HasPrefix(s.Target, "-") || strings.ContainsAny(s.Target, " \r\n\t") {
		return "", fmt.Errorf("invalid SSH target")
	}
	if !filepath.IsAbs(s.Tool) || !filepath.IsAbs(s.Session) {
		return "", fmt.Errorf("remote tool and session paths must be absolute")
	}
	arguments := []string{s.Tool, "dev-agent", "-session", s.Session}
	if s.User != "" {
		if strings.HasPrefix(s.User, "-") || strings.ContainsAny(s.User, " \r\n\t") {
			return "", fmt.Errorf("invalid sudo user")
		}
		arguments = append([]string{"sudo", "-n", "-u", s.User, "--"}, arguments...)
	}
	for i := range arguments {
		arguments[i] = quote(arguments[i])
	}
	return strings.Join(arguments, " "), nil
}

func (s *SSHSender) start(ctx context.Context) error {
	command, err := s.RemoteCommand()
	if err != nil {
		return err
	}
	s.cmd = exec.CommandContext(ctx, "ssh", "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=1", "--", s.Target, command)
	s.input, err = s.cmd.StdinPipe()
	if err != nil {
		s.cmd = nil
		return err
	}
	s.output, err = s.cmd.StdoutPipe()
	if err != nil {
		s.input.Close()
		s.cmd = nil
		return err
	}
	if err = s.cmd.Start(); err != nil {
		s.input.Close()
		s.output.Close()
		s.cmd = nil
		return err
	}
	return nil
}

func (s *SSHSender) Close() error {
	if s.cmd == nil {
		return nil
	}
	s.input.Close()
	s.output.Close()
	s.cmd.Process.Kill()
	err := s.cmd.Wait()
	s.cmd = nil
	return err
}

func (s *SSHSender) Send(ctx context.Context, data []byte) (bool, error) {
	// The request context bounds a stalled SSH process. Healthy requests keep
	// the connection alive across saves; no persistent process outlives Close.
	if s.cmd == nil {
		if err := s.start(context.WithoutCancel(ctx)); err != nil {
			return false, err
		}
	}
	type result struct {
		response devproto.Response
		err      error
	}
	done := make(chan result, 1)
	go func() {
		var response devproto.Response
		err := devproto.Write(s.input, devproto.Request{Bundle: data})
		if err == nil {
			err = devproto.Read(s.output, &response)
		}
		done <- result{response, err}
	}()
	select {
	case <-ctx.Done():
		s.Close()
		<-done
		return false, ctx.Err()
	case r := <-done:
		if r.err != nil {
			s.Close()
			return false, r.err
		}
		if r.response.Error != "" {
			return r.response.Retry, errors.New(r.response.Error)
		}
		return false, nil
	}
}
