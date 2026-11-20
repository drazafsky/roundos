package appmgr

import (
	"context"
	"net"
	"testing"
	"time"

	"roundos/internal/wire"
)

func TestViewAndExitReportsStayOrderedWithoutBlockingManagerAPIs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, other := net.Pipe()
	defer conn.Close()
	defer other.Close()
	e := &endpoint{app: wire.TimerID, conn: conn, closed: make(chan struct{})}
	m := &Manager{ctx: ctx, events: make(chan Event, 1), peers: map[string]*endpoint{wire.TimerID: e},
		pids: map[string]int{wire.TimerID: 123}, tokens: map[string]string{wire.TimerID: "token"}}
	m.events <- Event{Kind: "fill queue"}
	viewDone := make(chan struct{})
	go func() { m.reportPeer(e, Event{App: wire.TimerID, Kind: "view"}); close(viewDone) }()
	// Hold event backpressure while the view owns lifecycle ordering.
	until := time.Now().Add(2 * time.Second)
	for m.eventMu.TryLock() {
		m.eventMu.Unlock()
		if time.Now().After(until) {
			t.Fatal("view reporter did not start")
		}
		time.Sleep(time.Millisecond)
	}
	exitDone := make(chan struct{})
	go func() { m.processExited(wire.TimerID, "exited"); close(exitDone) }()
	apiDone := make(chan struct{})
	go func() { _ = m.PIDs(); close(apiDone) }()
	select {
	case <-apiDone:
	case <-time.After(2 * time.Second):
		t.Fatal("event backpressure blocked manager API mutex")
	}
	<-m.events
	receive := func() Event {
		t.Helper()
		select {
		case event := <-m.events:
			return event
		case <-time.After(2 * time.Second):
			t.Fatal("lifecycle report stalled")
			return Event{}
		}
	}
	if event := receive(); event.Kind != "view" {
		t.Fatalf("exit overtook an accepted view: %+v", event)
	}
	if event := receive(); event.Kind != "error" {
		t.Fatalf("missing process exit: %+v", event)
	}
	<-viewDone
	<-exitDone
	m.reportPeer(e, Event{App: wire.TimerID, Kind: "view", Detail: "late stale view"})
	select {
	case event := <-m.events:
		t.Fatalf("retired peer reported after exit: %+v", event)
	default:
	}
}

func TestReplacedPeerCannotPublishStartupView(t *testing.T) {
	old := &endpoint{app: wire.ClockID}
	current := &endpoint{app: wire.ClockID}
	m := &Manager{ctx: context.Background(), events: make(chan Event, 2),
		peers: map[string]*endpoint{wire.ClockID: current}}
	m.reportPeer(old, Event{App: wire.ClockID, Kind: "view", Detail: "stale"})
	m.reportPeer(current, Event{App: wire.ClockID, Kind: "view", Detail: "current"})
	if event := <-m.events; event.Detail != "current" {
		t.Fatalf("replaced connection published a view: %+v", event)
	}
	if len(m.events) != 0 {
		t.Fatal("unexpected stale connection event")
	}
}

func TestClosedCurrentPeerCannotPublishStartupView(t *testing.T) {
	e := &endpoint{app: wire.TimerID, closed: make(chan struct{})}
	close(e.closed)
	m := &Manager{ctx: context.Background(), events: make(chan Event, 1),
		peers: map[string]*endpoint{wire.TimerID: e}}
	m.reportPeer(e, Event{App: wire.TimerID, Kind: "view"})
	if len(m.events) != 0 {
		t.Fatal("closed connection published startup data before cleanup")
	}
}
