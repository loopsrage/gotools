package onceler_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"golangApp/lib/onceler"
)

func TestOncelerStoreOnceExecutesOnce(t *testing.T) {
	o := onceler.NewOnceler()
	var calls int
	var mu sync.Mutex
	err := o.StoreOnce("idx", "key", func() error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("StoreOnce() error: %v", err)
	}
	// Second call with same key should not execute do()
	err = o.StoreOnce("idx", "key", func() error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("second StoreOnce() error: %v", err)
	}
	if got := calls; got != 1 {
		t.Fatalf("do() called %d times, want 1", got)
	}
}

func TestOncelerStoreOnceDifferentKeys(t *testing.T) {
	o := onceler.NewOnceler()
	var calls1, calls2 int
	var mu sync.Mutex
	o.StoreOnce("idx", "key1", func() error {
		mu.Lock()
		calls1++
		mu.Unlock()
		return nil
	})
	o.StoreOnce("idx", "key2", func() error {
		mu.Lock()
		calls2++
		mu.Unlock()
		return nil
	})
	if calls1 != 1 || calls2 != 1 {
		t.Fatalf("calls1=%d calls2=%d, want 1 and 1", calls1, calls2)
	}
}

func TestOncelerStoreOnceError(t *testing.T) {
	o := onceler.NewOnceler()
	expectedErr := errors.New("do error")
	err := o.StoreOnce("idx", "key", func() error {
		return expectedErr
	})
	if err == nil {
		t.Fatal("expected error from do()")
	}
	if err != expectedErr {
		t.Fatalf("err = %v, want %v", err, expectedErr)
	}
	// After an error, the sync.Once has been consumed; subsequent calls return nil
	// because once.Do does not re-execute the function.
	err = o.StoreOnce("idx", "key", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil on second call (once consumed), got %v", err)
	}
}

func TestOncelerStoreOnceConcurrent(t *testing.T) {
	o := onceler.NewOnceler()
	var calls int32
	var wg sync.WaitGroup
	const workers = 50
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.StoreOnce("idx", "key", func() error {
				atomic.AddInt32(&calls, 1)
				return nil
			})
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("do() called %d times under concurrency, want 1", got)
	}
}

func TestOncelerStoreOnceMultipleIndexes(t *testing.T) {
	o := onceler.NewOnceler()
	var callsA, callsB int
	var mu sync.Mutex
	o.StoreOnce("idxA", "key", func() error {
		mu.Lock()
		callsA++
		mu.Unlock()
		return nil
	})
	o.StoreOnce("idxB", "key", func() error {
		mu.Lock()
		callsB++
		mu.Unlock()
		return nil
	})
	if callsA != 1 || callsB != 1 {
		t.Fatalf("callsA=%d callsB=%d, want 1 and 1", callsA, callsB)
	}
}
