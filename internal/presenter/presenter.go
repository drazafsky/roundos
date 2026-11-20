// Package presenter isolates potentially blocking DRM page flips and rasterization
// from the latency-sensitive input/gesture event loop. A one-element mailbox
// keeps only the latest immutable shell snapshot: intermediate motion frames are
// deliberately coalesced without losing any input events or gesture transitions.
package presenter

import (
	"image"
	"sync"
	"sync/atomic"
	"time"

	"roundos/internal/perf"
	"roundos/internal/shell"
)

type Display interface{ Present(*image.RGBA) error }

type Request struct {
	Scene   *shell.Model
	InputAt time.Time
}

type Presenter struct {
	pending   chan Request
	errors    chan error
	stop      chan struct{}
	done      chan struct{}
	once      sync.Once
	coalesced atomic.Uint64
}

func New(display Display, stats *perf.Stats) *Presenter {
	p := &Presenter{
		pending: make(chan Request, 1), errors: make(chan error, 1),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go func() {
		defer close(p.done)
		framebuffer := image.NewRGBA(image.Rect(0, 0, shell.Width, shell.Height))
		for {
			select {
			case <-p.stop:
				return
			case req := <-p.pending:
				// If the shell changed state again before we began work,
				// render the newest scene rather than stale touch positions.
				for draining := true; draining; {
					select {
					case newer := <-p.pending:
						req = newer
					default:
						draining = false
					}
				}
				began := time.Now()
				shell.DrawInto(framebuffer, req.Scene, began)
				if err := display.Present(framebuffer); err != nil {
					select {
					case p.errors <- err:
					default:
					}
					return
				}
				var latency time.Duration
				if !req.InputAt.IsZero() {
					latency = time.Since(req.InputAt)
				}
				stats.Frame(time.Since(began), latency)
			}
		}
	}()
	return p
}

// Submit never blocks for a frame. Every input event still reaches the shell;
// only redundant intermediate *images* are dropped when DRM is slower.
func (p *Presenter) Submit(scene *shell.Model, inputAt time.Time) {
	req := Request{Scene: scene.Snapshot(), InputAt: inputAt}
	select {
	case <-p.done:
		return
	default:
	}
	select {
	case p.pending <- req:
		return
	default:
	}
	select {
	case <-p.pending:
		p.coalesced.Add(1)
	default:
	}
	select {
	case p.pending <- req:
	default:
	}
}

// Coalesced reports intermediate images skipped, not touch events lost.
func (p *Presenter) Coalesced() uint64    { return p.coalesced.Load() }
func (p *Presenter) Errors() <-chan error { return p.errors }
func (p *Presenter) Close() {
	p.once.Do(func() { close(p.stop); <-p.done })
}
