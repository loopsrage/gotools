package toggle_test

import (
	"sync"
	"testing"

	"github.com/loopsrage/gotools/lib/toggle"
)

func TestToggleNewInitial(t *testing.T) {
	toggles := []struct {
		initial bool
		want    bool
	}{
		{true, true},
		{false, false},
	}
	for _, tt := range toggles {
		tg := toggle.NewToggle(tt.initial)
		if tg.Load() != tt.want {
			t.Fatalf("NewToggle(%v).Load() = %v, want %v", tt.initial, tg.Load(), tt.want)
		}
	}
}

func TestToggleSet(t *testing.T) {
	tg := toggle.NewToggle(false)
	tg.Set(true)
	if tg.Load() != true {
		t.Fatalf("after Set(true), Load() = %v, want true", tg.Load())
	}
	tg.Set(false)
	if tg.Load() != false {
		t.Fatalf("after Set(false), Load() = %v, want false", tg.Load())
	}
}

func TestToggleToggle(t *testing.T) {
	tg := toggle.NewToggle(false)
	if got := tg.Toggle(); got != true {
		t.Fatalf("Toggle() = %v, want true", got)
	}
	if tg.Load() != true {
		t.Fatalf("after Toggle(), Load() = %v, want true", tg.Load())
	}
	if got := tg.Toggle(); got != false {
		t.Fatalf("Toggle() = %v, want false", got)
	}
	if tg.Load() != false {
		t.Fatalf("after second Toggle(), Load() = %v, want false", tg.Load())
	}
}

func TestToggleConcurrent(t *testing.T) {
	tg := toggle.NewToggle(false)
	const workers = 20
	const iterations = 100
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tg.Toggle()
			}
		}()
	}
	wg.Wait()
	// Just ensure no panic/data race; final value is nondeterministic.
	_ = tg.Load()
}
