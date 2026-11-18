package concurr_test

import (
	"sync"
	"testing"

	"github.com/loopsrage/gotools/lib/concurr"
)

// ---------------- Scanner tests ----------------

func TestScannerAddNextDataOrder(t *testing.T) {
	s := concurr.NewScanner()
	s.Add("a")
	s.Add("b")
	s.Add("c")

	if got := s.Count(); got != 3 {
		t.Fatalf("expected count 3, got %d", got)
	}

	var seen []interface{}
	for s.Next() {
		pos, data := s.Data()
		seen = append(seen, data)
		_ = pos
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 values, got %d: %v", len(seen), seen)
	}
	if seen[0] != "a" || seen[1] != "b" || seen[2] != "c" {
		t.Fatalf("unexpected order: %v", seen)
	}
}

func TestScannerEmpty(t *testing.T) {
	s := concurr.NewScanner()
	if s.Next() {
		t.Fatal("Next() should be false on empty scanner")
	}
	if got := s.Count(); got != 0 {
		t.Fatalf("expected count 0, got %d", got)
	}
	if got := s.At(0); got != nil {
		t.Fatalf("expected nil At(0), got %v", got)
	}
	if got := s.At(-1); got != nil {
		t.Fatalf("expected nil At(-1), got %v", got)
	}
}

func TestScannerAt(t *testing.T) {
	s := concurr.NewScanner()
	s.Add(10)
	s.Add(20)
	if got := s.At(0); got != 10 {
		t.Fatalf("expected At(0)=10, got %v", got)
	}
	if got := s.At(1); got != 20 {
		t.Fatalf("expected At(1)=20, got %v", got)
	}
	if got := s.At(2); got != nil {
		t.Fatalf("expected At(2)=nil, got %v", got)
	}
	if got := s.At(-1); got != nil {
		t.Fatalf("expected At(-1)=nil, got %v", got)
	}
}

func TestScannerDataIncrementsPosition(t *testing.T) {
	s := concurr.NewScanner()
	s.Add("first")
	s.Add("second")

	_, d1 := s.Data()
	if d1 != "first" {
		t.Fatalf("expected Data() first='first', got %v", d1)
	}
	_, d2 := s.Data()
	if d2 != "second" {
		t.Fatalf("expected Data() second='second', got %v", d2)
	}
	if got := s.Count(); got != 2 {
		t.Fatalf("expected count 2, got %d", got)
	}
	if s.Next() {
		t.Fatal("Next() should be false after consuming all via Data()")
	}
}

func TestScannerReset(t *testing.T) {
	s := concurr.NewScanner()
	s.Add(1)
	s.Add(2)
	s.Reset()
	if s.Next() {
		t.Fatal("Next() should be false after Reset")
	}
	if got := s.Count(); got != 0 {
		t.Fatalf("expected count 0 after Reset, got %d", got)
	}
	// Can add again after reset.
	s.Add(99)
	if s.Next() {
		if _, data := s.Data(); data != 99 {
			t.Fatalf("expected 99 after reset+add, got %v", data)
		}
	}
	if got := s.Count(); got != 1 {
		t.Fatalf("expected count 1 after reset+add, got %d", got)
	}
}

func TestScannerConcurrentAdd(t *testing.T) {
	s := concurr.NewScanner()
	const workers = 50
	const perWorker = 100
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				s.Add(struct{ w, i int }{w, i})
			}
		}(w)
	}
	wg.Wait()
	expected := int64(workers * perWorker)
	if got := s.Count(); got != expected {
		t.Fatalf("expected count %d, got %d", expected, got)
	}
	// All added values should be retrievable via Next/Data.
	found := 0
	for s.Next() {
		_, _ = s.Data()
		found++
	}
	if found != int(expected) {
		t.Fatalf("expected to iterate %d items, found %d", expected, found)
	}
}
