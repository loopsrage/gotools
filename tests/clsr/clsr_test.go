package clsr_test

import (
	"sync"
	"testing"
	"time"

	"golangApp/lib/clsr"
)

func TestNewCleanupComplete(t *testing.T) {
	can := func() {}
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(can, wg)
	if cc == nil {
		t.Fatal("NewCleanupComplete() returned nil")
	}
}

func TestCleanupCompleteCompleteBlocks(t *testing.T) {
	can := func() {}
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(can, wg)

	done := make(chan struct{})
	go func() {
		cc.Complete()
		close(done)
	}()

	// Should block initially since nothing closes the complete channel
	select {
	case <-done:
		t.Fatal("Complete() should block when complete channel is not closed")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSigClosureNilCompletePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("SigClosure with nil complete should panic")
		}
	}()
	clsr.SigClosure(nil, nil)
}
