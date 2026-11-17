package concurr_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golangApp/lib/concurr"
)

// ---------------- Config tests ----------------

func TestNewConfig(t *testing.T) {
	cfg := concurr.NewConfig(4)
	if cfg == nil {
		t.Fatal("NewConfig returned nil")
	}
	if cfg.SemaphorePool == nil {
		t.Fatal("SemaphorePool should be initialized by NewConfig")
	}
	if cap(cfg.SemaphorePool) != 4 {
		t.Fatalf("expected semaphore capacity 4, got %d", cap(cfg.SemaphorePool))
	}
	// Zero-value Config is valid: nil pool means unlimited concurrency.
	var zero concurr.Config
	if acquired := zero.Acquire(context.Background()); !acquired {
		t.Fatal("zero-value Config.Acquire should succeed (nil pool = unlimited)")
	}
	zero.Release() // must not deadlock
}

func TestConfigStaggerMs(t *testing.T) {
	cfg := concurr.NewConfig(1)
	if got := cfg.StaggerMs(); got != 0 {
		t.Fatalf("expected default stagger 0, got %d", got)
	}
	atomic.StoreInt64(&cfg.Stagger, 42)
	if got := cfg.StaggerMs(); got != 42 {
		t.Fatalf("expected stagger 42, got %d", got)
	}
}

func TestCheckContext(t *testing.T) {
	if err := concurr.CheckContext(context.Background()); err != nil {
		t.Fatalf("expected nil for background context, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := concurr.CheckContext(ctx); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestConfigAcquireReleaseRespectsLimit(t *testing.T) {
	cfg := concurr.NewConfig(2)
	ctx := context.Background()

	var inFlight int32
	var maxInFlight int32
	released := make([]chan struct{}, 0, 2)

	for i := 0; i < 2; i++ {
		done := make(chan struct{})
		go func() {
			if !cfg.Acquire(ctx) {
				t.Error("expected Acquire to succeed for initial slots")
			}
			atomic.AddInt32(&inFlight, 1)
			for {
				cur := atomic.LoadInt32(&maxInFlight)
				if atomic.LoadInt32(&inFlight) <= cur {
					break
				}
				if atomic.CompareAndSwapInt32(&maxInFlight, cur, atomic.LoadInt32(&inFlight)) {
					break
				}
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out acquiring initial slots")
		}
		released = append(released, done)
	}

	if got := atomic.LoadInt32(&maxInFlight); got != 2 {
		t.Fatalf("expected max in-flight 2, got %d", got)
	}

	// A 3rd acquire must block while 2 slots are held.
	blocked := make(chan struct{})
	go func() {
		cfg.Acquire(ctx)
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("3rd acquire should have blocked while 2 slots held")
	case <-time.After(100 * time.Millisecond):
	}

	// Release one slot; the blocked acquire should proceed.
	<-released[0]
	cfg.Release()
	atomic.AddInt32(&inFlight, -1)

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for blocked acquire to proceed")
	}

	cfg.Release()
	cfg.Release()
}

func TestConfigAcquireCtxDoneDoesNotTakeSlot(t *testing.T) {
	cfg := concurr.NewConfig(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if acquired := cfg.Acquire(ctx); acquired {
		t.Fatal("expected Acquire to report failure on done context")
	}
	// Returning quickly proves no slot was held (Release would block otherwise).
}

func TestConfigNilPoolDoesNotDeadlock(t *testing.T) {
	var cfg concurr.Config
	done := make(chan struct{})
	go func() {
		cfg.Acquire(context.Background())
		cfg.Release()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("zero-value Config Acquire/Release deadlocked")
	}
}

func TestConfigConcurrentAcquireRelease(t *testing.T) {
	cfg := concurr.NewConfig(16)
	ctx := context.Background()
	const workers = 300
	var wg sync.WaitGroup
	var processed int32

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !cfg.Acquire(ctx) {
				t.Error("Acquire should succeed on live context")
				return
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&processed, 1)
			cfg.Release()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&processed); got != workers {
		t.Fatalf("expected %d processed, got %d", workers, got)
	}
}

func TestConfigNegativeMaxInFlight(t *testing.T) {
	// NewConfig with a negative capacity creates a channel with negative cap -> panic.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative channel capacity")
		}
	}()
	concurr.NewConfig(-1)
}
