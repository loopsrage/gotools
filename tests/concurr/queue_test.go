package concurr_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golangApp/lib/concurr"
)

// sinkAction records items passed to Action in order.
type sinkAction struct {
	mu    sync.Mutex
	items []interface{}
}

func (s *sinkAction) Action(on interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, on)
}

func (s *sinkAction) Items() []interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]interface{}, len(s.items))
	copy(out, s.items)
	return out
}

func (s *sinkAction) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// waitForItems blocks until the sink has at least n items or timeout expires.
func waitForItems(s *sinkAction, n int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.Len() >= n {
			return true
		}
		time.Sleep(1 * time.Millisecond)
	}
	return s.Len() >= n
}

// ---------------- ActionQueue tests ----------------

func TestActionQueueProcessesAllItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	action := &sinkAction{}
	cfg := concurr.NewConfig(8)
	ch := make(chan interface{}, 10)

	concurr.ActionQueue(ctx, &wg, cfg, action, ch)

	for i := 0; i < 12; i++ {
		ch <- i
	}
	close(ch)

	if !waitForItems(action, 12, 10*time.Second) {
		t.Fatalf("expected 12 items processed, got %d", action.Len())
	}
	wg.Wait()

	if got := action.Len(); got != 12 {
		t.Fatalf("expected 12 items processed, got %d", got)
	}
}

func TestActionQueueStopsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	action := &sinkAction{}
	cfg := concurr.NewConfig(8)
	ch := make(chan interface{}, 5)

	concurr.ActionQueue(ctx, &wg, cfg, action, ch)

	for i := 0; i < 5; i++ {
		ch <- i
	}
	cancel() // stop the queue goroutine
	close(ch)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	_ = action.Items()
}

func TestActionQueueClosedChannelDoesNotSpin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	action := &sinkAction{}
	cfg := concurr.NewConfig(4)
	ch := make(chan interface{})

	concurr.ActionQueue(ctx, &wg, cfg, action, ch)
	close(ch)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Millisecond)
	}
	_ = action.Items()
}

func TestActionQueueZeroConfigUnlimited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	action := &sinkAction{}
	cfg := &concurr.Config{} // nil pool = unlimited concurrency
	ch := make(chan interface{}, 10)

	concurr.ActionQueue(ctx, &wg, cfg, action, ch)

	for i := 0; i < 12; i++ {
		ch <- i
	}
	close(ch)

	if !waitForItems(action, 12, 10*time.Second) {
		t.Fatalf("expected 12 items processed, got %d", action.Len())
	}
	wg.Wait()

	if got := action.Len(); got != 12 {
		t.Fatalf("expected 12 items processed, got %d", got)
	}
}

func TestActionQueueConcurrentProducers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	action := &sinkAction{}
	cfg := concurr.NewConfig(16)
	ch := make(chan interface{}, 100)

	concurr.ActionQueue(ctx, &wg, cfg, action, ch)

	const producers = 20
	const perProducer = 100
	var sent int32
	var pWg sync.WaitGroup
	for p := 0; p < producers; p++ {
		pWg.Add(1)
		go func(p int) {
			defer pWg.Done()
			for i := 0; i < perProducer; i++ {
				ch <- struct{ p, i int }{p, i}
				atomic.AddInt32(&sent, 1)
			}
		}(p)
	}
	pWg.Wait()
	close(ch)

	expected := int(atomic.LoadInt32(&sent))
	if !waitForItems(action, expected, 15*time.Second) {
		t.Fatalf("expected %d items processed, got %d", expected, action.Len())
	}
	wg.Wait()

	if got := action.Len(); got != expected {
		t.Fatalf("expected %d items processed, got %d", expected, got)
	}
}

func TestActionQueueAllItemsProcessedWithUnlimited(t *testing.T) {
	// With a nil semaphore pool, all items are dispatched without limiting.
	// Order of completion is non-deterministic; only the count matters.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	action := &sinkAction{}
	cfg := &concurr.Config{} // unlimited
	ch := make(chan interface{}, 50)

	concurr.ActionQueue(ctx, &wg, cfg, action, ch)

	for i := 0; i < 20; i++ {
		ch <- i
	}
	close(ch)

	if !waitForItems(action, 20, 10*time.Second) {
		t.Fatalf("expected 20 items processed, got %d", action.Len())
	}
	wg.Wait()

	if got := action.Len(); got != 20 {
		t.Fatalf("expected 20 items processed, got %d", got)
	}
}
