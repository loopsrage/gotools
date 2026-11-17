package concurr_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golangApp/lib/concurr"
)

// counterAction records invocation count, peak concurrent executions, and items seen.
type counterAction struct {
	mu        sync.Mutex
	called    int32
	active    int32
	maxActive int32
	items     *concurr.TsList
}

func (c *counterAction) Action(on interface{}) {
	atomic.AddInt32(&c.active, 1)
	for {
		cur := atomic.LoadInt32(&c.maxActive)
		if atomic.LoadInt32(&c.active) <= cur {
			break
		}
		if atomic.CompareAndSwapInt32(&c.maxActive, cur, atomic.LoadInt32(&c.active)) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	c.items.Add(on)
	atomic.AddInt32(&c.called, 1)
	atomic.AddInt32(&c.active, -1)
}

func (c *counterAction) Called() int32        { return atomic.LoadInt32(&c.called) }
func (c *counterAction) MaxActive() int32     { return atomic.LoadInt32(&c.maxActive) }
func (c *counterAction) Items() []interface{} { return c.items.All() }

func waitOrFail(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("timed out waiting for work to complete")
	}
}

// ---------------- ActionOn tests ----------------

func TestActionOnRunsAllItems(t *testing.T) {
	items := []interface{}{1, 2, 3, 4, 5}
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(10)

	concurr.ActionOn(context.Background(), cfg, action, items...)

	if got := action.Called(); got != int32(len(items)) {
		t.Fatalf("expected %d calls, got %d", len(items), got)
	}
	if got := len(action.Items()); got != len(items) {
		t.Fatalf("expected %d items recorded, got %d", len(items), got)
	}
}

func TestActionOnEmpty(t *testing.T) {
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(4)
	start := time.Now()
	concurr.ActionOn(context.Background(), cfg, action) // no items
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("ActionOn with no items took too long: %v", elapsed)
	}
	if got := action.Called(); got != 0 {
		t.Fatalf("expected 0 calls, got %d", got)
	}
}

func TestActionOnCtxDoneBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(4)
	concurr.ActionOn(ctx, cfg, action, 1, 2, 3)
	if got := action.Called(); got != 0 {
		t.Fatalf("expected 0 calls with cancelled ctx, got %d", got)
	}
}

func TestActionOnCtxDoneMidRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(1)
	atomic.StoreInt64(&cfg.Stagger, 0)

	items := make([]interface{}, 50)
	for i := range items {
		items[i] = i
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	concurr.ActionOn(ctx, cfg, action, items...)

	if got := action.Called(); got < 1 {
		t.Fatalf("expected at least 1 call before cancellation, got %d", got)
	}
	if got := action.Called(); got > int32(len(items)) {
		t.Fatalf("expected at most %d calls, got %d", len(items), got)
	}
}

func TestActionOnRespectsMaxInFlight(t *testing.T) {
	const items = 40
	const maxInFlight = 4
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(maxInFlight)

	vals := make([]interface{}, items)
	for i := range vals {
		vals[i] = i
	}

	concurr.ActionOn(context.Background(), cfg, action, vals...)

	if got := action.Called(); got != items {
		t.Fatalf("expected %d calls, got %d", items, got)
	}
	if got := action.MaxActive(); got > maxInFlight {
		t.Fatalf("max in-flight exceeded: %d > %d", got, maxInFlight)
	}
}

func TestActionOnStaggerSerializesWithOneSlot(t *testing.T) {
	// StaggerMs=0 removes randomness; maxInFlight=1 forces strict serialization.
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(1)
	atomic.StoreInt64(&cfg.Stagger, 0)

	items := []interface{}{1, 2, 3}
	start := time.Now()
	concurr.ActionOn(context.Background(), cfg, action, items...)
	elapsed := time.Since(start)

	if got := action.Called(); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
	if elapsed < 12*time.Millisecond { // 3 items * ~5ms action sleep
		t.Fatalf("expected serialized execution >= ~15ms, got %v", elapsed)
	}
}

func TestActionOnActionIsCalledExactlyOncePerItem(t *testing.T) {
	const items = 25
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(7)
	vals := make([]interface{}, items)
	for i := range vals {
		vals[i] = i
	}
	concurr.ActionOn(context.Background(), cfg, action, vals...)
	if got := action.Called(); got != items {
		t.Fatalf("expected %d calls, got %d", items, got)
	}
	seen := make(map[interface{}]bool)
	for _, it := range action.Items() {
		seen[it] = true
	}
	if len(seen) != items {
		t.Fatalf("expected %d unique items, got %d", items, len(seen))
	}
}

// ---------------- Benchmark ----------------

func BenchmarkActionOn(b *testing.B) {
	const numItems = 1000
	concurrencyLevels := []int{1, 10, 50, 100, 200, 500}
	for _, max := range concurrencyLevels {
		b.Run(fmt.Sprintf("max_in_flight=%d", max), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			action := &noopAction{}
			cfg := concurr.NewConfig(int64(max))
			items := make([]interface{}, numItems)
			for j := 0; j < numItems; j++ {
				items[j] = j
			}
			for i := 0; i < b.N; i++ {
				concurr.ActionOn(context.Background(), cfg, action, items...)
			}
		})
	}
}

type noopAction struct{}

func (noopAction) Action(interface{}) {}
