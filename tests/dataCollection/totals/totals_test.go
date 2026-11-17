package totals_test

import (
	"sync"
	"testing"

	"golangApp/lib/dataCollection/totals"
)

func TestTotalsAddAndTotal(t *testing.T) {
	o := totals.NewTotals()
	o.AddTotal("a", 10)
	o.AddTotal("a", 5)
	if got := o.Total("a"); got != 15 {
		t.Fatalf("Total('a') = %d, want 15", got)
	}

	o.AddTotal("b", 7)
	if got := o.Total("b"); got != 7 {
		t.Fatalf("Total('b') = %d, want 7", got)
	}
	if got := o.Total("a"); got != 15 {
		t.Fatalf("Total('a') after adding 'b' = %d, want 15", got)
	}
	if got := o.Total("missing"); got != 0 {
		t.Fatalf("Total('missing') = %d, want 0", got)
	}
}

func TestTotalsAddNegative(t *testing.T) {
	o := totals.NewTotals()
	o.AddTotal("a", 100)
	o.AddTotal("a", -30)
	if got := o.Total("a"); got != 70 {
		t.Fatalf("Total('a') = %d, want 70", got)
	}
}

func TestTotalsReset(t *testing.T) {
	o := totals.NewTotals()
	o.AddTotal("a", 42)
	o.Reset("a")
	if got := o.Total("a"); got != 0 {
		t.Fatalf("after Reset('a'), Total('a') = %d, want 0", got)
	}
	// Reset non-existent prefix should not panic
	o.Reset("missing")
	if got := o.Total("missing"); got != 0 {
		t.Fatalf("after Reset('missing'), Total('missing') = %d, want 0", got)
	}
}

func TestTotalsRange(t *testing.T) {
	o := totals.NewTotals()
	o.AddTotal("x", 1)
	o.AddTotal("y", 2)
	o.AddTotal("z", 3)

	seen := make(map[string]int64)
	o.Range(func(prefix string, total int64) {
		seen[prefix] = total
	})
	if len(seen) != 3 {
		t.Fatalf("Range() saw %d entries, want 3: %v", len(seen), seen)
	}
	if seen["x"] != 1 || seen["y"] != 2 || seen["z"] != 3 {
		t.Fatalf("unexpected range values: %v", seen)
	}
}

func TestTotalsRangeEmpty(t *testing.T) {
	o := totals.NewTotals()
	count := 0
	o.Range(func(prefix string, total int64) {
		count++
	})
	if count != 0 {
		t.Fatalf("Range on empty Totals called %d times, want 0", count)
	}
}

func TestTotalsConcurrent(t *testing.T) {
	o := totals.NewTotals()
	const workers = 20
	const perWorker = 100
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				o.AddTotal("shared", 1)
				o.AddTotal("perWorker", int64(w))
			}
		}(w)
	}
	wg.Wait()
	if got := o.Total("shared"); got != int64(workers*perWorker) {
		t.Fatalf("Total('shared') = %d, want %d", got, workers*perWorker)
	}
	// perWorker: each worker w adds int64(w) perWorker times
	// total = perWorker * (0 + 1 + ... + (workers-1)) = perWorker * workers*(workers-1)/2
	expectedPerWorker := int64(perWorker) * int64(workers) * int64(workers-1) / 2
	if got := o.Total("perWorker"); got != expectedPerWorker {
		t.Fatalf("Total('perWorker') = %d, want %d", got, expectedPerWorker)
	}
}
