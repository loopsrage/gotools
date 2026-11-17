package serviceController_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"golangApp/lib/serviceController"
)

type mockController struct {
	mu         sync.Mutex
	ticker     *time.Ticker
	resetCalls int
}

func (m *mockController) Ticker() *time.Ticker {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ticker == nil {
		m.ticker = time.NewTicker(time.Millisecond)
	}
	return m.ticker
}

func (m *mockController) Reset(proceed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCalls++
}

type mockAction struct {
	mu        sync.Mutex
	callCount int
	proceed   bool
	err       error
}

func (m *mockAction) ServiceLoop() (proceed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	return m.proceed
}

func TestService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := &mockController{}
	action := &mockAction{proceed: true}

	errs := serviceController.Service(ctx, ctrl, action)
	if errs == nil {
		t.Fatal("Service() returned nil errs channel")
	}

	// Wait for at least one tick
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Drain errs channel
	select {
	case <-errs:
	case <-time.After(5 * time.Second):
		t.Fatal("errs channel was not closed after cancellation")
	}

	action.mu.Lock()
	defer action.mu.Unlock()
	if action.callCount < 1 {
		t.Fatalf("action.ServiceLoop() called %d times, want at least 1", action.callCount)
	}
	if ctrl.resetCalls < 1 {
		t.Fatalf("controller.Reset() called %d times, want at least 1", ctrl.resetCalls)
	}
}

func TestServiceActionProceedFalse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := &mockController{}
	action := &mockAction{proceed: false}

	serviceController.Service(ctx, ctrl, action)
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-time.After(5 * time.Second):
	}
}

func TestServiceNotifyComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := &mockController{}
	action := &mockAction{proceed: true}
	notify := &mockController{}

	serviceController.Service(ctx, ctrl, action, notify)
	time.Sleep(100 * time.Millisecond)
	cancel()

	notify.mu.Lock()
	defer notify.mu.Unlock()
	if notify.resetCalls < 1 {
		t.Fatalf("notify controller.Reset() called %d times, want at least 1", notify.resetCalls)
	}
}

func TestServiceNilNotifyComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := &mockController{}
	action := &mockAction{proceed: true}

	// Should not panic with nil notify
	serviceController.Service(ctx, ctrl, action, nil)
	time.Sleep(50 * time.Millisecond)
	cancel()
}
