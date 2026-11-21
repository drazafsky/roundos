package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"roundos/internal/rui/devtool"
)

func runDevCommand(command string, args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	session := flags.String("session", "", "private session.json path on the app machine")
	target := flags.String("ssh", "", "SSH target user@host")
	tool := flags.String("remote-tool", "/opt/roundos/current/bin/roundui", "absolute remote tool path")
	user := flags.String("sudo-user", "", "explicit remote non-root app user")
	// Accept the documented layout-first command as well as normal Go flags.
	if command == "dev" && len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		args = append(append([]string{}, args[1:]...), args[0])
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *session == "" {
		return fmt.Errorf("%s requires -session", command)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "dev-agent" {
		if flags.NArg() != 0 || *target != "" || *user != "" {
			return fmt.Errorf("dev-agent accepts only -session")
		}
		return devtool.Agent(ctx, *session, input, output)
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("dev requires one trusted layout path")
	}
	var sender devtool.Sender = devtool.LocalSender{Session: *session}
	if *target != "" {
		remote := &devtool.SSHSender{Target: *target, Tool: *tool, Session: *session, User: *user}
		if _, err := remote.RemoteCommand(); err != nil {
			return err
		}
		sender = remote
	} else if *user != "" {
		return fmt.Errorf("-sudo-user requires -ssh")
	}
	return devtool.Watch(ctx, flags.Arg(0), sender, output)
}
