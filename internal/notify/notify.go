// Package notify stores shell-owned notifications with adaptive presentation.
// The event loop is the sole owner; callers must serialize Store access.
package notify

import (
	"strings"
	"time"
)

type Priority string

const (
	Quiet      Priority = "quiet"
	Actionable Priority = "actionable"
	Critical   Priority = "critical"
)

type Request struct {
	Title    string   `json:"title"`
	Body     string   `json:"body,omitempty"`
	Priority Priority `json:"priority,omitempty"`
	Group    string   `json:"group,omitempty"`
	Action   string   `json:"action,omitempty"` // 'open' only in this checkpoint
}
type Item struct {
	ID      uint64
	App     string
	Request Request
	At      time.Time
	Count   int
	Read    bool
}
type Presentation string

const (
	Silent Presentation = "silent"
	Banner Presentation = "banner"
	Modal  Presentation = "modal"
)

type Result struct {
	Item         Item
	Presentation Presentation
	New          bool
}
type Store struct {
	next      uint64
	items     []Item
	quietMode bool
	perApp    map[string]Priority
}

func New() *Store                    { return &Store{perApp: make(map[string]Priority)} }
func (s *Store) SetQuietMode(v bool) { s.quietMode = v }
func (s *Store) QuietMode() bool     { return s.quietMode }

// SetAppPriority permits users to lower an app's requested priority. Critical
// system alerts are never exposed to application-provided priority overrides.
func (s *Store) SetAppPriority(app string, p Priority) {
	if p == Quiet || p == Actionable {
		s.perApp[app] = p
	} else {
		delete(s.perApp, app)
	}
}
func (s *Store) Emit(app string, req Request, at time.Time, fromSystem bool) Result {
	if strings.TrimSpace(req.Title) == "" {
		return Result{}
	}
	req.Title = clip(strings.TrimSpace(req.Title), 56)
	req.Body = clip(strings.TrimSpace(req.Body), 100)
	req.Group = clip(req.Group, 64)
	if req.Action != "open" {
		req.Action = ""
	}
	if req.Priority != Quiet && req.Priority != Actionable && req.Priority != Critical {
		req.Priority = Quiet
	}
	if !fromSystem && req.Priority == Critical {
		req.Priority = Actionable
	}
	if !fromSystem {
		if ceiling, ok := s.perApp[app]; ok && ceiling == Quiet {
			req.Priority = Quiet
		}
	}
	for i := range s.items {
		v := &s.items[i]
		if v.App == app && v.Request.Group == req.Group && v.Request.Title == req.Title && v.Request.Body == req.Body && v.Request.Priority == req.Priority && at.Sub(v.At) >= 0 && at.Sub(v.At) < 30*time.Second {
			v.Count++
			v.At = at
			// Same notification updates history, without showing another banner.
			return Result{Item: *v, Presentation: Silent, New: false}
		}
	}
	s.next++
	item := Item{ID: s.next, App: app, Request: req, At: at, Count: 1}
	s.items = append([]Item{item}, s.items...)
	if len(s.items) > 60 {
		s.items = s.items[:60]
	}
	result := Result{Item: item, New: true, Presentation: Silent}
	if req.Priority == Critical && fromSystem {
		result.Presentation = Modal
	} else if req.Priority == Actionable && !s.quietMode {
		result.Presentation = Banner
	}
	return result
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func (s *Store) Items() []Item { return append([]Item(nil), s.items...) }
func (s *Store) MarkRead(id uint64) bool {
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Read = true
			return true
		}
	}
	return false
}
func (s *Store) Dismiss(id uint64) bool {
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return true
		}
	}
	return false
}
func (s *Store) Clear() { s.items = nil }
