package firstLastTime_test

import (
	"sync"
	"testing"
	"time"

	"github.com/loopsrage/gotools/lib/dataCollection/firstLastTime"
)

func TestFirstLastSetFirstTime(t *testing.T) {
	l := &firstLastTime.FirstLast{}
	t1 := time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC)
	l.SetFirstTime(t1)
	if got := l.FirstTime(); !got.Equal(t1) {
		t.Fatalf("FirstTime() = %v, want %v", got, t1)
	}

	// Earlier time should replace first time
	t0 := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	l.SetFirstTime(t0)
	if got := l.FirstTime(); !got.Equal(t0) {
		t.Fatalf("after SetFirstTime(earlier), FirstTime() = %v, want %v", got, t0)
	}

	// Later time should NOT replace first time
	t2 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	l.SetFirstTime(t2)
	if got := l.FirstTime(); !got.Equal(t0) {
		t.Fatalf("after SetFirstTime(later), FirstTime() = %v, want %v", got, t0)
	}
}

func TestFirstLastSetLastTime(t *testing.T) {
	l := &firstLastTime.FirstLast{}
	t1 := time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC)
	l.SetLastTime(t1)
	if got := l.LastTime(); !got.Equal(t1) {
		t.Fatalf("LastTime() = %v, want %v", got, t1)
	}

	// Later time should replace last time
	t2 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	l.SetLastTime(t2)
	if got := l.LastTime(); !got.Equal(t2) {
		t.Fatalf("after SetLastTime(later), LastTime() = %v, want %v", got, t2)
	}

	// Earlier time should NOT replace last time
	t0 := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	l.SetLastTime(t0)
	if got := l.LastTime(); !got.Equal(t2) {
		t.Fatalf("after SetLastTime(earlier), LastTime() = %v, want %v", got, t2)
	}
}

func TestFirstLastSetTimes(t *testing.T) {
	l := &firstLastTime.FirstLast{}
	t1 := time.Date(2020, 6, 15, 12, 30, 0, 0, time.UTC)
	l.SetTimes(t1)
	if got := l.FirstTime(); !got.Equal(t1) {
		t.Fatalf("FirstTime() = %v, want %v", got, t1)
	}
	if got := l.LastTime(); !got.Equal(t1) {
		t.Fatalf("LastTime() = %v, want %v", got, t1)
	}

	// SetTimes with a time between first and last should not change either
	tMid := time.Date(2020, 6, 15, 12, 30, 1, 0, time.UTC)
	l.SetTimes(tMid)
	if got := l.FirstTime(); !got.Equal(t1) {
		t.Fatalf("after mid SetTimes, FirstTime() = %v, want %v", got, t1)
	}
	if got := l.LastTime(); !got.Equal(tMid) {
		t.Fatalf("after mid SetTimes, LastTime() = %v, want %v", got, tMid)
	}
}

func TestFirstLastZeroInitial(t *testing.T) {
	l := &firstLastTime.FirstLast{}
	epoch := time.Unix(0, 0)
	if got := l.FirstTime(); !got.Equal(epoch) {
		t.Fatalf("FirstTime() on zero = %v, want Unix epoch", got)
	}
	if got := l.LastTime(); !got.Equal(epoch) {
		t.Fatalf("LastTime() on zero = %v, want Unix epoch", got)
	}
}

func TestFirstLastConcurrent(t *testing.T) {
	l := &firstLastTime.FirstLast{}
	const workers = 20
	const perWorker = 100
	var wg sync.WaitGroup
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				t := base.Add(time.Duration(w*perWorker+i) * time.Second)
				l.SetTimes(t)
			}
		}(w)
	}
	wg.Wait()
	// First should be base, last should be base + (workers*perWorker - 1) seconds
	expectedFirst := base
	expectedLast := base.Add(time.Duration(workers*perWorker-1) * time.Second)
	if got := l.FirstTime(); !got.Equal(expectedFirst) {
		t.Fatalf("concurrent FirstTime() = %v, want %v", got, expectedFirst)
	}
	if got := l.LastTime(); !got.Equal(expectedLast) {
		t.Fatalf("concurrent LastTime() = %v, want %v", got, expectedLast)
	}
}
