package rui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"roundos/internal/rui/devproto"
	"roundos/sdk/rui/bundle"
)

// DevSession is opt-in and must never be started by a production application.
// Its one-slot request queue has no SDK event reader and invokes no handlers.
type DevSession struct {
	Path      string
	directory string
	listener  net.Listener
	updates   chan *DevUpdate
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type DevUpdate struct {
	data  []byte
	ctx   context.Context
	reply chan error
	used  atomic.Bool
}

// Apply belongs on the application's Dispatch/Flush loop. Acknowledgement is
// sent only after staging succeeds; a secure editor causes an explicit retry.
func (u *DevUpdate) Apply(runtime *Runtime) error {
	if !u.used.CompareAndSwap(false, true) {
		return fmt.Errorf("development update already consumed")
	}
	if err := u.ctx.Err(); err != nil {
		return err
	}
	err := runtime.ReplaceContext(u.ctx, u.data)
	u.reply <- err
	return err
}

type devAddress struct {
	Socket string `json:"socket"`
	Token  string `json:"token"`
}

// ListenDev creates a private session under an existing, same-user mode-0700
// directory. Applications enable this only in an explicit developer launch.
func ListenDev(parent context.Context, directory string) (*DevSession, error) {
	if os.Getenv("ROUNDOS_RUI_DEV_DIR") != directory || directory == "" {
		return nil, fmt.Errorf("development session requires the manual supervisor opt-in")
	}
	if os.Geteuid() == 0 {
		return nil, fmt.Errorf("development sessions must run as a non-root app user")
	}
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("development directory must be absolute")
	}
	if err := privatePath(directory, true); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(directory, "rui-")
	if err != nil {
		return nil, err
	}
	socket := filepath.Join(dir, "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		listener.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	address := devAddress{Socket: socket, Token: hex.EncodeToString(secret[:])}
	data, err := json.Marshal(address)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "session.json"), data, 0600)
	}
	if err != nil {
		listener.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	context.AfterFunc(ctx, func() { listener.Close() })
	s := &DevSession{Path: filepath.Join(dir, "session.json"), directory: dir, listener: listener, updates: make(chan *DevUpdate, 1), ctx: ctx, cancel: cancel}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(s.updates)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.serve(conn, address.Token)
			conn.Close()
		}
	}()
	return s, nil
}

func (s *DevSession) Updates() <-chan *DevUpdate { return s.updates }
func (s *DevSession) Close() error {
	s.cancel()
	s.listener.Close()
	s.wg.Wait()
	return os.RemoveAll(s.directory)
}

func (s *DevSession) serve(conn net.Conn, token string) {
	stop := context.AfterFunc(s.ctx, func() { conn.Close() })
	defer stop()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	var request devproto.Request
	if err := devproto.Read(conn, &request); err != nil {
		return
	}
	if subtle.ConstantTimeCompare([]byte(request.Token), []byte(token)) != 1 {
		return
	}
	if len(request.Bundle) > bundle.MaxPayload+40 {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Second)
	defer cancel()
	update := &DevUpdate{data: request.Bundle, ctx: ctx, reply: make(chan error, 1)}
	select {
	case s.updates <- update:
	case <-ctx.Done():
		return
	}
	var err error
	select {
	case err = <-update.reply:
	case <-ctx.Done():
		return
	}
	response := devproto.Response{Retry: errors.Is(err, ErrReloadDeferred)}
	if err != nil {
		response.Error = err.Error()
	}
	devproto.Write(conn, response)
}

func privatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return privateInfo(info, directory)
}

func privateInfo(info os.FileInfo, directory bool) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	if !ok || st.Uid != uint32(os.Geteuid()) || info.Mode().Perm() != mode || info.IsDir() != directory || info.Mode()&os.ModeSymlink != 0 || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("development path must be private and owned by the current user")
	}
	return nil
}

// SendDevBundle delivers only compiled data. The session secret is read from a
// private file and is never accepted on the command line or printed in logs.
func SendDevBundle(ctx context.Context, sessionPath string, data []byte) (bool, error) {
	if !filepath.IsAbs(sessionPath) {
		return false, fmt.Errorf("session path must be absolute")
	}
	if len(data) > bundle.MaxPayload+40 {
		return false, fmt.Errorf("compiled layout exceeds budget")
	}
	if err := privatePath(filepath.Dir(sessionPath), true); err != nil {
		return false, err
	}
	if err := privatePath(sessionPath, false); err != nil {
		return false, err
	}
	file, err := os.OpenFile(sessionPath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if err = privateInfo(info, false); err != nil {
		return false, err
	}
	encoded, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return false, err
	}
	if len(encoded) > 4096 {
		return false, fmt.Errorf("development session exceeds budget")
	}
	var address devAddress
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&address); err != nil {
		return false, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return false, fmt.Errorf("trailing session data")
	}
	if len(address.Token) != 64 || filepath.Dir(address.Socket) != filepath.Dir(sessionPath) {
		return false, fmt.Errorf("invalid development session")
	}
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "unix", address.Socket)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	if err := devproto.Write(conn, devproto.Request{Token: address.Token, Bundle: data}); err != nil {
		return false, err
	}
	var response devproto.Response
	if err := devproto.Read(conn, &response); err != nil {
		return false, err
	}
	if response.Error != "" {
		return response.Retry, errors.New(response.Error)
	}
	return false, nil
}
