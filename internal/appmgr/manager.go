// Package appmgr launches, authenticates, and supervises external RoundOS apps.
package appmgr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"roundos/internal/touch"
	"roundos/internal/wire"
)

type Event struct {
	App          string
	Kind         string // connected, view, disconnected, error, stopped
	View         wire.View
	Detail       string
	Notification *wire.Notification
	TextRequest  *wire.TextRequest
}

type endpoint struct {
	app         string
	conn        net.Conn
	send        chan wire.Message
	raw         chan wire.Message
	closed      chan struct{}
	rawContacts bool
	once        sync.Once
}

func (e *endpoint) close() { e.once.Do(func() { close(e.closed); e.conn.Close() }) }
func (e *endpoint) enqueue(m wire.Message) bool {
	select {
	case <-e.closed:
		return false
	case e.send <- m:
		return true
	default:
		return false
	}
}

type Manager struct {
	mu          sync.Mutex
	eventMu     sync.Mutex // orders peer events with connection removal and exit
	ctx         context.Context
	cancel      context.CancelFunc
	listener    net.Listener
	directory   string
	socket      string
	executables map[string]string
	tokens      map[string]string
	peers       map[string]*endpoint
	pids        map[string]int
	active      string
	events      chan Event
	credentials *appCredentials
	dev         *DevConfig
	wg          sync.WaitGroup
}

// New starts a private IPC socket and launches both apps. Paths must be trusted
// local executables. This prototype does not yet sandbox app processes.
func New(parent context.Context, clockExe, timerExe, stateDir string) (*Manager, error) {
	return NewWithApps(parent, clockExe, timerExe, stateDir, nil)
}

// NewWithApps starts built-in apps and additional trusted executables.
func NewWithApps(parent context.Context, clockExe, timerExe, stateDir string, extra []AppSpec) (*Manager, error) {
	return NewWithAppsRuntime(parent, clockExe, timerExe, stateDir, extra, nil)
}

// NewWithAppsRuntime starts apps with an explicit restricted account when
// runtime is non-nil. The shell owns the IPC socket and credentials are never
// included in inherited root environment variables.
func NewWithAppsRuntime(parent context.Context, clockExe, timerExe, stateDir string, extra []AppSpec, runtime *Runtime) (*Manager, error) {
	return NewWithAppsRuntimeDev(parent, clockExe, timerExe, stateDir, extra, runtime, nil)
}

// NewWithAppsRuntimeDev enables development only for an explicit manual launch.
func NewWithAppsRuntimeDev(parent context.Context, clockExe, timerExe, stateDir string, extra []AppSpec, runtime *Runtime, dev *DevConfig) (*Manager, error) {
	if dev != nil {
		value := *dev
		dev = &value
	}
	credentials, err := validateRuntime(runtime)
	if err != nil {
		return nil, err
	}
	executables := map[string]string{wire.ClockID: clockExe, wire.TimerID: timerExe}
	for _, app := range extra {
		if !validAppID.MatchString(app.ID) || app.ID == wire.ClockID || app.ID == wire.TimerID || app.Exec == "" {
			return nil, fmt.Errorf("invalid extra app %q", app.ID)
		}
		if _, exists := executables[app.ID]; exists {
			return nil, fmt.Errorf("duplicate app %q", app.ID)
		}
		executables[app.ID] = app.Exec
	}
	if err := validateDev(dev, credentials, executables); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "roundos-ipc-")
	if err != nil {
		return nil, err
	}
	if credentials != nil {
		// Directory needs only traversal by the app group; root retains
		// exclusive ownership. Socket requires group connect permissions.
		if err = os.Chown(directory, 0, int(credentials.gid)); err != nil {
			os.RemoveAll(directory)
			return nil, err
		}
		err = os.Chmod(directory, 0710)
	} else {
		err = os.Chmod(directory, 0700)
	}
	if err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	path := filepath.Join(directory, "roundd.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	if credentials != nil {
		if err = os.Chown(path, 0, int(credentials.gid)); err != nil {
			listener.Close()
			os.RemoveAll(directory)
			return nil, err
		}
		err = os.Chmod(path, 0660)
	} else {
		err = os.Chmod(path, 0600)
	}
	if err != nil {
		listener.Close()
		os.RemoveAll(directory)
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{ctx: ctx, cancel: cancel, listener: listener, directory: directory, socket: path,
		executables: executables, credentials: credentials, dev: dev,
		tokens: map[string]string{}, peers: map[string]*endpoint{}, pids: map[string]int{}, events: make(chan Event, 256)}
	m.wg.Add(1)
	go func() { defer m.wg.Done(); m.acceptLoop() }()
	ids := make([]string, 0, len(executables))
	for id := range executables {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if m.executables[id] == "" {
			m.Close()
			return nil, fmt.Errorf("missing executable for %s", id)
		}
		m.wg.Add(1)
		go func(appID string) { defer m.wg.Done(); m.supervise(appID, stateDir) }(id)
	}
	return m, nil
}
func (m *Manager) Events() <-chan Event { return m.events }
func (m *Manager) Socket() string       { return m.socket }
func (m *Manager) report(ev Event) {
	select {
	case m.events <- ev:
	case <-m.ctx.Done():
	}
}

// reportPeer drops messages from retired connections and orders accepted
// messages before any subsequent disconnect/process-exit event. Never hold mu
// while waiting for the shell to drain events: the shell calls manager APIs.
func (m *Manager) reportPeer(e *endpoint, ev Event) {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	m.mu.Lock()
	current := m.peers[e.app] == e
	m.mu.Unlock()
	if current {
		select {
		case <-e.closed:
			return
		default:
		}
		m.report(ev)
	}
}

func (m *Manager) processExited(id, detail string) {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	m.mu.Lock()
	delete(m.pids, id)
	if e := m.peers[id]; e != nil {
		delete(m.peers, id)
		e.close()
	}
	delete(m.tokens, id)
	m.mu.Unlock()
	m.report(Event{App: id, Kind: "error", Detail: detail})
}
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (m *Manager) supervise(id, stateDir string) {
	failures := 0
	for m.ctx.Err() == nil {
		token, err := newToken()
		if err != nil {
			m.report(Event{App: id, Kind: "error", Detail: err.Error()})
			return
		}
		m.mu.Lock()
		m.tokens[id] = token
		m.mu.Unlock()
		cmd := exec.CommandContext(m.ctx, m.executables[id])
		if m.credentials != nil {
			cmd.Env = m.credentials.restrictedEnv(m.socket, id, token)
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
				Uid: m.credentials.uid, Gid: m.credentials.gid, Groups: []uint32{},
			}}
		} else {
			cmd.Env = append(os.Environ(), "ROUNDOS_SOCKET="+m.socket, "ROUNDOS_APP_ID="+id, "ROUNDOS_TOKEN="+token, "ROUNDOS_STATE_DIR="+stateDir)
		}
		// Never inherit the development opt-in from a service environment.
		filtered := cmd.Env[:0]
		for _, value := range cmd.Env {
			if !strings.HasPrefix(value, "ROUNDOS_RUI_DEV_DIR=") {
				filtered = append(filtered, value)
			}
		}
		cmd.Env = filtered
		if m.dev != nil && m.dev.AppID == id {
			cmd.Env = append(cmd.Env, "ROUNDOS_RUI_DEV_DIR="+m.dev.Directory)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		started := time.Now()
		err = cmd.Start()
		if err == nil {
			m.mu.Lock()
			m.pids[id] = cmd.Process.Pid
			m.mu.Unlock()
			err = cmd.Wait()
		}
		if m.ctx.Err() != nil {
			return
		}
		msg := "exited"
		if err != nil {
			msg = err.Error()
		}
		m.processExited(id, msg)
		if time.Since(started) > 30*time.Second {
			failures = 0
		}
		failures++
		if failures >= 5 {
			m.report(Event{App: id, Kind: "stopped", Detail: "crash loop; restart limit reached"})
			return
		}
		delay := time.Duration(1<<(failures-1)) * 200 * time.Millisecond
		select {
		case <-time.After(delay):
		case <-m.ctx.Done():
			return
		}
	}
}
func (m *Manager) acceptLoop() {
	for {
		c, err := m.listener.Accept()
		if err != nil {
			if m.ctx.Err() == nil {
				m.report(Event{Kind: "error", Detail: err.Error()})
			}
			return
		}
		m.wg.Add(1)
		go func() { defer m.wg.Done(); m.handle(c) }()
	}
}
func (m *Manager) handle(c net.Conn) {
	defer c.Close()
	if m.credentials != nil {
		uid, err := peerUserID(c)
		if err != nil || uid != m.credentials.uid {
			return
		}
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, err := wire.Read(c)
	_ = c.SetReadDeadline(time.Time{})
	if err != nil || hello.Type != "hello" {
		return
	}
	m.eventMu.Lock()
	m.mu.Lock()
	token, known := m.tokens[hello.App]
	if !known || token != hello.Token || token == "" || m.peers[hello.App] != nil {
		m.mu.Unlock()
		m.eventMu.Unlock()
		return
	}
	e := &endpoint{app: hello.App, conn: c, send: make(chan wire.Message, 64), raw: make(chan wire.Message, 1), closed: make(chan struct{})}
	m.peers[hello.App] = e
	active := m.active == hello.App
	e.enqueue(wire.Message{Version: wire.Version, Type: "visibility", Visible: active})
	m.mu.Unlock()
	m.report(Event{App: hello.App, Kind: "connected"})
	m.eventMu.Unlock()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			var msg wire.Message
			// Prioritize control/gesture traffic over opt-in raw snapshots.
			select {
			case <-e.closed:
				return
			case msg = <-e.send:
			default:
				select {
				case <-e.closed:
					return
				case msg = <-e.send:
				case msg = <-e.raw:
				}
			}
			_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if wire.Write(c, msg) != nil {
				e.close()
				return
			}
		}
	}()
	// Each connection owns its reconstruction baseline. A restart must publish
	// a full view before patches can be accepted.
	var current *wire.View
	for {
		msg, err := wire.Read(c)
		if err != nil {
			break
		}
		switch {
		case msg.Type == "view" && msg.View != nil:
			v := *msg.View
			current = &v
			m.reportPeer(e, Event{App: hello.App, Kind: "view", View: v})
		case msg.Type == "notify" && msg.Notification != nil:
			n := *msg.Notification
			m.reportPeer(e, Event{App: hello.App, Kind: "notify", Notification: &n})
		case msg.Type == "text_request" && msg.TextRequest != nil:
			req := *msg.TextRequest
			m.reportPeer(e, Event{App: hello.App, Kind: "text_request", TextRequest: &req})
		case msg.Type == "subscribe_contacts":
			m.mu.Lock()
			e.rawContacts = msg.RawContacts
			m.mu.Unlock()
		case msg.Type == "view_patch" && msg.Patch != nil && current != nil:
			v := msg.Patch.Apply(*current)
			current = &v
			m.reportPeer(e, Event{App: hello.App, Kind: "view", View: v})
		}
	}
	e.close()
	<-writerDone
	m.eventMu.Lock()
	m.mu.Lock()
	wasCurrent := m.peers[hello.App] == e
	if wasCurrent {
		delete(m.peers, hello.App)
	}
	m.mu.Unlock()
	if wasCurrent {
		m.report(Event{App: hello.App, Kind: "disconnected"})
	}
	m.eventMu.Unlock()
}
func (m *Manager) SetForeground(id string) error {
	if id != "" {
		if _, ok := m.executables[id]; !ok {
			return errors.New("unknown app")
		}
	}
	m.mu.Lock()
	old := m.active
	m.active = id
	if old != id {
		if e := m.peers[old]; e != nil {
			e.enqueue(wire.Message{Version: wire.Version, Type: "visibility", Visible: false})
		}
		if e := m.peers[id]; e != nil {
			e.enqueue(wire.Message{Version: wire.Version, Type: "visibility", Visible: true})
		}
	}
	m.mu.Unlock()
	return nil
}

// TextResult delivers a completed/canceled editor result back to the requesting
// app. Applications never receive secret text in logs or shared state.
func (m *Manager) TextResult(id string, result wire.TextResult) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.peers[id]
	if e == nil {
		return false
	}
	if !e.enqueue(wire.Message{Version: wire.Version, Type: "text_result", TextResult: &result}) {
		// Dropping this terminal result would leave a live client waiting
		// forever. Retire the connection so normal app recovery can run.
		e.close()
		return false
	}
	return true
}
func (m *Manager) Input(id, action string, delta int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.peers[id]
	if e == nil {
		return false
	}
	return e.enqueue(wire.Message{Version: wire.Version, Type: "input", Action: action, Delta: delta})
}

// Gesture delivers a recognized gesture only to the foreground app. Delivery
// is bounded and nonblocking so an unresponsive app cannot delay input.
func (m *Manager) Gesture(id string, g wire.Gesture) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" || id != m.active {
		return false
	}
	e := m.peers[id]
	if e == nil {
		return false
	}
	return e.enqueue(wire.Message{Version: wire.Version, Type: "gesture", Gesture: &g})
}

// Contacts forwards raw tracking snapshots only to the subscribed foreground
// application. This never blocks the input loop or the DRM presenter.
func (m *Manager) Contacts(id string, contacts []touch.Contact) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" || id != m.active {
		return false
	}
	e := m.peers[id]
	if e == nil || !e.rawContacts {
		return false
	}
	data := make([]wire.Contact, 0, len(contacts))
	for _, c := range contacts {
		data = append(data, wire.Contact{ID: c.ID, Slot: c.Slot, X: c.X, Y: c.Y})
	}
	msg := wire.Message{Version: wire.Version, Type: "contacts", Contacts: data}
	select {
	case <-e.closed:
		return false
	case e.raw <- msg:
		return true
	default:
	}
	// Latest-wins raw-contact mailbox; no input events are blocked or lost.
	select {
	case <-e.raw:
	default:
	}
	select {
	case <-e.closed:
		return false
	case e.raw <- msg:
		return true
	default:
		return false
	}
}

func (m *Manager) Close() error {
	m.cancel()
	m.listener.Close()
	m.mu.Lock()
	for _, e := range m.peers {
		e.close()
	}
	m.mu.Unlock()
	m.wg.Wait()
	os.RemoveAll(m.directory)
	return nil
}
func (m *Manager) Log() { log.Printf("RoundOS 0.3 IPC socket %s", m.socket) }

// PIDs returns a safe snapshot for on-demand diagnostics, without polling.
func (m *Manager) PIDs() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := make(map[string]int, len(m.pids))
	for id, pid := range m.pids {
		snapshot[id] = pid
	}
	return snapshot
}
