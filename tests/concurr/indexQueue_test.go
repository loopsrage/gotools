package concurr_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/index"
)

// ---------------- BatchActionQueue tests ----------------

func TestBatchActionQueueDefaultBatchSize(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	if got := iq.BatchSize(); got != 1024 {
		t.Fatalf("expected default batch size 1024, got %d", got)
	}
}

func TestBatchActionQueueSetBatchSize(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	iq.SetBatchSize(3)
	if got := iq.BatchSize(); got != 3 {
		t.Fatalf("expected batch size 3, got %d", got)
	}
	iq.SetBatchSize(1)
	if got := iq.BatchSize(); got != 1 {
		t.Fatalf("expected batch size 1, got %d", got)
	}
}

func TestBatchActionQueueAddAndSize(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)

	iq.Add("k1", "v1")
	iq.Add("k2", "v2")
	if got := iq.Size(); got != 2 {
		t.Fatalf("expected size 2, got %d", got)
	}
}

func TestBatchActionQueueDuplicateKeyDoesNotInflateSize(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)

	for i := 0; i < 10; i++ {
		iq.Add("dup", i)
	}
	if got := iq.Size(); got != 1 {
		t.Fatalf("expected size 1 for duplicate keys, got %d", got)
	}
}

func TestBatchActionQueueMixedKeysSize(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)

	iq.Add("a", 1)
	iq.Add("b", 2)
	iq.Add("a", 3) // duplicate key 'a'
	iq.Add("c", 4)
	iq.Add("b", 5) // duplicate key 'b'
	if got := iq.Size(); got != 3 {
		t.Fatalf("expected size 3 (a, b, c), got %d", got)
	}
}

func TestBatchActionQueueBatchActionProcessesAll(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	iq.SetBatchSize(3)

	const items = 10
	for i := 0; i < items; i++ {
		iq.Add(i, i)
	}
	if got := iq.Size(); got != items {
		t.Fatalf("expected size %d before BatchAction, got %d", items, got)
	}

	iq.BatchAction()

	if got := action.Called(); got != items {
		t.Fatalf("expected %d action calls, got %d", items, got)
	}
	if got := iq.Size(); got != 0 {
		t.Fatalf("expected size 0 after BatchAction, got %d", got)
	}
}

func TestBatchActionQueueBatchActionSmallerThanBatch(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	iq.SetBatchSize(100) // larger than item count

	for i := 0; i < 5; i++ {
		iq.Add(i, i)
	}
	iq.BatchAction()
	if got := action.Called(); got != 5 {
		t.Fatalf("expected 5 action calls, got %d", got)
	}
}

func TestBatchActionQueueBatchActionEmpty(t *testing.T) {
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(4)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)

	start := time.Now()
	iq.BatchAction() // nothing queued
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("BatchAction on empty queue took too long: %v", elapsed)
	}
	if got := action.Called(); got != 0 {
		t.Fatalf("expected 0 calls on empty queue, got %d", got)
	}
}

func TestBatchActionQueueBatchActionNoInfiniteLoop(t *testing.T) {
	// Regression: duplicate keys must not cause BatchAction to spin forever.
	ctx := context.Background()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(4)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	for i := 0; i < 20; i++ {
		iq.Add("same", i)
	}

	done := make(chan struct{})
	go func() {
		iq.BatchAction()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("BatchAction appeared to loop forever on duplicate keys")
	}
}

func TestBatchActionQueueBatchActionWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(4)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	iq.SetBatchSize(2)

	for i := 0; i < 6; i++ {
		iq.Add(i, i)
	}

	cancel() // cancel before BatchAction runs

	done := make(chan struct{})
	go func() {
		iq.BatchAction()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("BatchAction did not return after context cancellation")
	}
}

func TestBatchActionQueueConcurrentAddAndBatchAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	action := &counterAction{items: concurr.NewTsList()}
	cfg := concurr.NewConfig(8)
	iq := concurr.NewBatchActionQueue(ctx, cfg, action)
	iq.SetBatchSize(10)

	var wg sync.WaitGroup
	const producers = 10
	const perProducer = 50
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				iq.Add(struct{ p, i int }{p, i}, i)
			}
		}(p)
	}
	wg.Wait()

	expected := int64(producers * perProducer)
	if got := iq.Size(); got != expected {
		t.Fatalf("expected size %d, got %d", expected, got)
	}

	iq.BatchAction()

	if got := action.Called(); got != int32(expected) {
		t.Fatalf("expected %d action calls, got %d", expected, got)
	}
	if got := iq.Size(); got != 0 {
		t.Fatalf("expected size 0 after BatchAction, got %d", got)
	}
}

func TestBatchActionQueueUsesIndexPackage(t *testing.T) {
	idx := index.NewIndex().New("Queue")
	idx.StoreInIndex("Queue", "k", "v")
	if got := idx.ListIndexes(); len(got) != 1 || got[0] != "Queue" {
		t.Fatalf("expected index 'Queue', got %v", got)
	}
	val, err := idx.LoadFromIndex("Queue", "k")
	if err != nil || val != "v" {
		t.Fatalf("expected LoadFromIndex Queue/k = v, got %v, %v", val, err)
	}
}
