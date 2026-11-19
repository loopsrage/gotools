package appcontroller

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// AppIndex is a thread-safe named registry of application controllers / services.
type AppIndex struct {
	mu    sync.RWMutex
	items map[string]any
}

// NewAppIndex creates an empty AppIndex.
func NewAppIndex() *AppIndex {
	return &AppIndex{items: make(map[string]any)}
}

// Register binds name to svc.
func (a *AppIndex) Register(name string, svc any) {
	a.mu.Lock()
	a.items[name] = svc
	a.mu.Unlock()
}

// Get retrieves the named service.
func (a *AppIndex) Get(name string) (any, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	v, ok := a.items[name]
	return v, ok
}

// Names returns sorted registered names.
func (a *AppIndex) Names() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.items))
	for k := range a.items {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PeriodicProducer runs a callback periodically on a background goroutine until stopped.
type PeriodicProducer struct {
	Name     string
	Interval time.Duration
	Fn       func(ctx context.Context) error
	mu       sync.Mutex
	running  bool
	runs     int64
	lastErr  string
	lastRun  time.Time
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewPeriodicProducer constructs a PeriodicProducer.
func NewPeriodicProducer(name string, interval time.Duration, fn func(ctx context.Context) error) *PeriodicProducer {
	if interval <= 0 {
		interval = time.Minute
	}
	return &PeriodicProducer{Name: name, Interval: interval, Fn: fn}
}

// Start launches the periodic loop in a goroutine.
func (p *PeriodicProducer) Start(parent context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.running = true
	p.wg.Add(1)
	p.mu.Unlock()

	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if p.Fn != nil {
					err := p.Fn(ctx)
					p.mu.Lock()
					p.runs++
					p.lastRun = time.Now().UTC()
					if err != nil {
						p.lastErr = err.Error()
					} else {
						p.lastErr = ""
					}
					p.mu.Unlock()
				}
			}
		}
	}()
}

// Stop cancels the periodic loop and waits for completion.
func (p *PeriodicProducer) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	p.running = false
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	p.wg.Wait()
}

// Status reports current worker state.
func (p *PeriodicProducer) Status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]any{
		"name":         p.Name,
		"running":      p.running,
		"interval_sec": p.Interval.Seconds(),
		"runs":         p.runs,
		"last_error":   p.lastErr,
		"last_run":     p.lastRun.Format(time.RFC3339),
	}
}

// WebAppWithWorkers manages an HTTP server alongside registered background PeriodicProducers
// with SIGINT/SIGTERM graceful shutdown.
type WebAppWithWorkers struct {
	Server  *http.Server
	Workers []*PeriodicProducer
}

// Run starts all workers and serves HTTP until ctx cancels or SIGINT/SIGTERM arrives.
func (w *WebAppWithWorkers) Run(ctx context.Context) error {
	if w.Server == nil {
		return fmt.Errorf("appcontroller: nil http.Server")
	}
	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, wk := range w.Workers {
		wk.Start(runCtx)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := w.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-runCtx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = w.Server.Shutdown(shutCtx)
		for _, wk := range w.Workers {
			wk.Stop()
		}
		return nil
	case err := <-errCh:
		for _, wk := range w.Workers {
			wk.Stop()
		}
		return err
	}
}
