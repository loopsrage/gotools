package controller_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"golangApp/lib/clsr"
	"golangApp/lib/controller"
)

func TestNewController(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(func() {}, wg)

	ctrl := controller.NewController(cc, time.Second, true)
	if ctrl == nil {
		t.Fatal("NewController() returned nil")
	}
	if ctrl.Ticker() == nil {
		t.Fatal("Ticker() should not be nil")
	}
	if ctrl.Interval != int64(time.Second) {
		t.Fatalf("Interval = %d, want %d", ctrl.Interval, int64(time.Second))
	}

	ctrl.Close()
}

func TestNewControllerStartNowFalse(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(func() {}, wg)

	ctrl := controller.NewController(cc, time.Second, false)
	if ctrl == nil {
		t.Fatal("NewController() returned nil")
	}
	ctrl.Close()
}

func TestControllerClose(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(func() {}, wg)

	ctrl := controller.NewController(cc, time.Second, true)
	err := ctrl.Close()
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

func TestControllerResetProceed(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(func() {}, wg)

	ctrl := controller.NewController(cc, time.Second, true)
	ctrl.Reset(true)
	// Reset(true) resets to 1 second interval
	ctrl.Close()
}

func TestControllerResetNotProceed(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	cc := clsr.NewCleanupComplete(func() {}, wg)

	ctrl := controller.NewController(cc, time.Second, true)
	ctrl.Reset(false)
	ctrl.Close()
}
