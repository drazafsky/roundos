// Package devtool compiles a trusted workspace on the developer machine and
// delivers only compiled bundles to an explicitly enabled app session.
package devtool

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"roundos/internal/rui/compile"
	"roundos/internal/rui/devproto"
	"roundos/sdk/rui"
)

type Sender interface {
	Send(context.Context, []byte) (bool, error)
	Close() error
}

type LocalSender struct{ Session string }

func (s LocalSender) Send(ctx context.Context, data []byte) (bool, error) {
	return rui.SendDevBundle(ctx, s.Session, data)
}
func (s LocalSender) Close() error { return nil }

// Agent bridges a persistent SSH stdio stream to a same-user private app
// session. Neither this process nor the remote app parses developer YAML.
func Agent(ctx context.Context, session string, input io.Reader, output io.Writer) error {
	// Production uses stdin/stdout files. Closing their descriptors unblocks
	// an idle frame read or stalled response write when SIGTERM cancels ctx.
	stop := context.AfterFunc(ctx, func() {
		if closer, ok := input.(io.Closer); ok {
			closer.Close()
		}
		if closer, ok := output.(io.Closer); ok {
			closer.Close()
		}
	})
	defer stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		var request devproto.Request
		if err := devproto.Read(input, &request); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if request.Token != "" {
			return fmt.Errorf("session tokens must come from the private remote session file")
		}
		retry, err := rui.SendDevBundle(ctx, session, request.Bundle)
		response := devproto.Response{Retry: retry}
		if err != nil {
			response.Error = err.Error()
		}
		if err := devproto.Write(output, response); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func readSource(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("layout must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 128<<10+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 128<<10 {
		return nil, fmt.Errorf("layout exceeds source budget")
	}
	return data, nil
}

// Watch samples every 100 ms and waits 150 ms after the last content change.
// Identical/invalid saves are coalesced; retryable editor/connection failures
// retain only the latest compiled candidate, never an unbounded update queue.
func Watch(ctx context.Context, path string, sender Sender, diagnostics io.Writer) error {
	defer sender.Close()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var observed, compiled [32]byte
	var haveObserved, haveCompiled bool
	var stableSince, retryAt time.Time
	var source, pending []byte
	var lastDiagnostic string
	report := func(message string) {
		if message != lastDiagnostic {
			fmt.Fprintln(diagnostics, message)
			lastDiagnostic = message
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			data, err := readSource(path)
			if err != nil {
				pending = nil
				haveObserved = false
				haveCompiled = false
				report("layout read failed: " + err.Error())
				continue
			}
			hash := sha256.Sum256(data)
			if !haveObserved || hash != observed {
				observed, haveObserved, stableSince, source = hash, true, now, data
				haveCompiled = false
				pending = nil
				continue
			}
			if now.Sub(stableSince) < 150*time.Millisecond {
				continue
			}
			if !haveCompiled || compiled != observed {
				compiled, haveCompiled = observed, true
				pending, _, err = compile.Compile(source)
				if err != nil {
					report(fmt.Sprintf("%s: %v", path, err))
					continue
				}
				retryAt = time.Time{}
			}
			if pending == nil || now.Before(retryAt) {
				continue
			}
			requestCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			retry, err := sender.Send(requestCtx, pending)
			cancel()
			if err != nil {
				report("reload rejected or unavailable: " + err.Error())
				if retry {
					retryAt = now.Add(100 * time.Millisecond)
				} else {
					retryAt = now.Add(time.Second)
				}
				continue
			}
			pending = nil
			report("layout accepted; application state retained")
		}
	}
}
