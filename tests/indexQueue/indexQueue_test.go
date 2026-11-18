package indexQueue_test

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopsrage/gotools/lib/indexQueue"
	"github.com/loopsrage/gotools/lib/queueController"
)

func TestIndexQueueProcessingAndCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var (
		mu        sync.Mutex
		processed []string
		wg        sync.WaitGroup
	)

	stageOne := indexQueue.NamedStage("stage_one", func(iq *indexQueue.IndexQueue, qd *queueController.QueueData) error {
		mu.Lock()
		processed = append(processed, qd.String("key")+":"+qd.String("obj"))
		mu.Unlock()
		return nil
	})

	actions := indexQueue.NewIndexQueueActions(ctx, &wg, 2, stageOne)
	iq := indexQueue.NewIndexQueue(ctx, actions)
	defer iq.Close()
	iq.Start()

	cfg := iq.LoadStageAction("stage_one")
	if cfg == nil {
		t.Fatal("LoadStageAction('stage_one') returned nil")
	}

	if _, err := iq.Enqueue("stage_one", "k1", "v1"); err != nil {
		t.Fatalf("Enqueue k1 failed: %v", err)
	}
	if _, err := iq.Enqueue("stage_one", "k2", "v2"); err != nil {
		t.Fatalf("Enqueue k2 failed: %v", err)
	}

	cfg.Flush()

	if err := iq.WaitForCompletions(ctx); err != nil {
		t.Fatalf("WaitForCompletions failed: %v", err)
	}
	wg.Wait()

	mu.Lock()
	sort.Strings(processed)
	got := append([]string(nil), processed...)
	mu.Unlock()

	if len(got) != 2 || got[0] != "k1:v1" || got[1] != "k2:v2" {
		t.Fatalf("processed = %v, want [k1:v1 k2:v2]", got)
	}
	if iq.TotalInFlight() != 0 {
		t.Fatalf("TotalInFlight = %d, want 0", iq.TotalInFlight())
	}
	if iq.Completed() != 2 {
		t.Fatalf("Completed = %d, want 2", iq.Completed())
	}
}

func TestIndexQueueMultiStageChaining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []string
	)

	s1 := indexQueue.NamedStage("s1", func(iq *indexQueue.IndexQueue, qd *queueController.QueueData) error {
		val := qd.String("obj") + "-step2"
		if _, err := iq.Enqueue("s2", qd.String("key"), val); err != nil {
			return err
		}
		iq.Flush("s2")
		return nil
	})

	s2 := indexQueue.NamedStage("s2", func(iq *indexQueue.IndexQueue, qd *queueController.QueueData) error {
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		results = append(results, qd.String("obj"))
		mu.Unlock()
		return nil
	})

	actions := indexQueue.NewIndexQueueActions(ctx, &wg, 2, s1, s2)
	iq := indexQueue.NewIndexQueue(ctx, actions)
	defer iq.Close()
	iq.Start()

	for _, item := range []string{"alpha", "beta", "gamma"} {
		if _, err := iq.Enqueue("s1", "", item); err != nil {
			t.Fatalf("Enqueue %s failed: %v", item, err)
		}
	}
	iq.Flush("s1")

	if err := iq.WaitForCompletions(ctx); err != nil {
		t.Fatalf("WaitForCompletions failed: %v", err)
	}
	wg.Wait()

	mu.Lock()
	sort.Strings(results)
	mu.Unlock()

	if len(results) != 3 || results[0] != "alpha-step2" || results[1] != "beta-step2" || results[2] != "gamma-step2" {
		t.Fatalf("multi-stage results = %v", results)
	}
}

func TestIndexQueueBackpressureAndContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	gate := make(chan struct{})
	var wg sync.WaitGroup

	slow := indexQueue.NamedStage("slow", func(iq *indexQueue.IndexQueue, qd *queueController.QueueData) error {
		<-gate
		return nil
	})

	actions := indexQueue.NewIndexQueueActions(ctx, &wg, 2, slow)
	iq := indexQueue.NewIndexQueue(ctx, actions)
	defer iq.Close()
	iq.SetMaxSize("slow", 2)
	iq.Start()

	if _, err := iq.Enqueue("slow", "1", "a"); err != nil {
		t.Fatalf("Enqueue 1: %v", err)
	}
	if _, err := iq.Enqueue("slow", "2", "b"); err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}
	iq.Flush("slow")

	// Third enqueue with short timeout should time out while capacity=2 is full.
	shortCtx, shortCancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer shortCancel()
	if _, err := iq.EnqueueContext(shortCtx, "slow", "3", "c"); err == nil {
		t.Fatal("expected EnqueueContext to block and fail on full backpressure window")
	}

	// Unblock workers and verify third enqueue succeeds immediately.
	var enqueuedThird atomic.Bool
	doneThird := make(chan error, 1)
	go func() {
		_, err := iq.EnqueueContext(ctx, "slow", "3", "c")
		if err == nil {
			enqueuedThird.Store(true)
			iq.Flush("slow")
		}
		doneThird <- err
	}()

	close(gate)
	if err := <-doneThird; err != nil {
		t.Fatalf("third Enqueue after release failed: %v", err)
	}
	if !enqueuedThird.Load() {
		t.Fatal("expected third item to enqueue once backpressure cleared")
	}
	if err := iq.WaitForCompletions(ctx); err != nil {
		t.Fatalf("WaitForCompletions error: %v", err)
	}
}

func TestIndexQueuePanicSafetyAndPeriodicProducer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	panicky := indexQueue.NamedStage("panicky", func(iq *indexQueue.IndexQueue, qd *queueController.QueueData) error {
		if qd.String("obj") == "boom" {
			panic("simulated stage fault")
		}
		return nil
	})

	actions := indexQueue.NewIndexQueueActions(ctx, &wg, 2, panicky)
	actions["panicky"].Config = &indexQueue.WorkerServiceConfig{
		Identity:       "panicky",
		WorkerCount:    2,
		WorkerInterval: 20 * time.Millisecond,
		StartNow:       true,
	}

	iq := indexQueue.NewIndexQueue(ctx, actions)
	defer iq.Close()
	iq.Start()

	k1, err := iq.Enqueue("panicky", "", "boom")
	if err != nil || len(k1) != 32 {
		t.Fatalf("auto-key Enqueue = (%q, %v), want 32-char hex UUID", k1, err)
	}
	if _, err := iq.Enqueue("panicky", "", "ok"); err != nil {
		t.Fatalf("Enqueue ok failed: %v", err)
	}

	// Rely on periodic producer ticker (20ms) without manual Flush()
	if err := iq.WaitForCompletions(ctx); err != nil {
		t.Fatalf("WaitForCompletions timed out: %v", err)
	}
	if iq.TotalInFlight() != 0 || iq.Completed() != 2 {
		t.Fatalf("post-panic stats: inFlight=%d completed=%d", iq.TotalInFlight(), iq.Completed())
	}
}
