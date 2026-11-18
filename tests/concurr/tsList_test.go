package concurr_test

import (
	"sync"
	"testing"

	"github.com/loopsrage/gotools/lib/concurr"
)

// ---------------- TsList tests ----------------

func TestTsListAddAndAt(t *testing.T) {
	l := concurr.NewTsList()
	pos := l.Add("a", "b", "c")
	if pos != 0 {
		t.Fatalf("expected Add to return start pos 0, got %d", pos)
	}
	if got := l.Count(); got != 3 {
		t.Fatalf("expected count 3, got %d", got)
	}
	if got := l.At(0); got != "a" {
		t.Fatalf("expected At(0)=a, got %v", got)
	}
	if got := l.At(1); got != "b" {
		t.Fatalf("expected At(1)=b, got %v", got)
	}
	if got := l.At(2); got != "c" {
		t.Fatalf("expected At(2)=c, got %v", got)
	}
}

func TestTsListAddEmpty(t *testing.T) {
	l := concurr.NewTsList()
	pos := l.Add()
	if pos != -1 {
		t.Fatalf("expected Add() to return -1, got %d", pos)
	}
	if got := l.Count(); got != 0 {
		t.Fatalf("expected count 0 after empty Add, got %d", got)
	}
}

func TestTsListSet(t *testing.T) {
	l := concurr.NewTsList()
	l.Add(1, 2, 3)
	l.Set(1, 100)
	if got := l.At(1); got != 100 {
		t.Fatalf("expected At(1)=100 after Set, got %v", got)
	}
	// Out of range: no-op, no panic.
	l.Set(100, 999)
	l.Set(-5, 999)
	if got := l.Count(); got != 3 {
		t.Fatalf("expected count unchanged 3, got %d", got)
	}
	if got := l.At(0); got != 1 {
		t.Fatalf("expected At(0)=1, got %v", got)
	}
	if got := l.At(2); got != 3 {
		t.Fatalf("expected At(2)=3, got %v", got)
	}
}

func TestTsListAll(t *testing.T) {
	l := concurr.NewTsList()
	l.Add("x", "y")
	all := l.All()
	if len(all) != 2 {
		t.Fatalf("expected All() len 2, got %d", len(all))
	}
	if all[0] != "x" || all[1] != "y" {
		t.Fatalf("unexpected All(): %v", all)
	}
	// Mutating the returned slice must not affect the list.
	all[0] = "mutated"
	if got := l.At(0); got != "x" {
		t.Fatalf("All() should return a copy, expected At(0)=x, got %v", got)
	}
	// Empty list All() returns non-nil empty slice.
	empty := concurr.NewTsList()
	if e := empty.All(); e == nil {
		t.Fatal("All() on empty list should return non-nil slice")
	}
}

func TestTsListNegativeIndex(t *testing.T) {
	l := concurr.NewTsList()
	l.Add(1)
	if got := l.At(-1); got != nil {
		t.Fatalf("expected At(-1)=nil, got %v", got)
	}
	if got := l.At(-100); got != nil {
		t.Fatalf("expected At(-100)=nil, got %v", got)
	}
}

func TestTsListSetNegativeAndOOB(t *testing.T) {
	l := concurr.NewTsList()
	l.Add(1, 2)
	l.Set(-1, 999)
	l.Set(100, 999)
	if got := l.Count(); got != 2 {
		t.Fatalf("expected count 2, got %d", got)
	}
	if got := l.At(0); got != 1 {
		t.Fatalf("expected At(0)=1, got %v", got)
	}
	if got := l.At(1); got != 2 {
		t.Fatalf("expected At(1)=2, got %v", got)
	}
}

func TestTsListConcurrentAdd(t *testing.T) {
	l := concurr.NewTsList()
	const workers = 30
	const perWorker = 500
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			vals := make([]interface{}, perWorker)
			for i := 0; i < perWorker; i++ {
				vals[i] = struct{ w, i int }{w, i}
			}
			l.Add(vals...)
		}(w)
	}
	wg.Wait()
	expected := int64(workers * perWorker)
	if got := l.Count(); got != expected {
		t.Fatalf("expected count %d, got %d", expected, got)
	}
}

func TestTsListConcurrentSetAndAt(t *testing.T) {
	l := concurr.NewTsList()
	for i := 0; i < 100; i++ {
		l.Add(i)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l.Set(int64(i), i*10)
		}(i)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = l.At(int64(i))
		}(i)
	}
	wg.Wait()
	if got := l.Count(); got != 100 {
		t.Fatalf("expected count 100, got %d", got)
	}
	for i := 0; i < 100; i++ {
		if got := l.At(int64(i)); got != i*10 {
			t.Fatalf("expected At(%d)=%d, got %v", i, i*10, got)
		}
	}
}

func TestTsListConcurrentAddAndCount(t *testing.T) {
	l := concurr.NewTsList()
	const workers = 20
	const perWorker = 250
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				l.Add(i)
			}
		}()
	}
	// Concurrent reads while adding.
	var readWg sync.WaitGroup
	for r := 0; r < 5; r++ {
		readWg.Add(1)
		go func() {
			defer readWg.Done()
			for i := 0; i < 1000; i++ {
				_ = l.Count()
			}
		}()
	}
	wg.Wait()
	readWg.Wait()
	expected := int64(workers * perWorker)
	if got := l.Count(); got != expected {
		t.Fatalf("expected count %d, got %d", expected, got)
	}
}
