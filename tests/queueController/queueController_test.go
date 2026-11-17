package queueController_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golangApp/lib/concurr"
	"golangApp/lib/queueController"
)

type testAction struct {
	mu     sync.Mutex
	called int32
}

func (t *testAction) Action(on interface{}) {
	atomic.AddInt32(&t.called, 1)
}

func (t *testAction) QueueAction(on *queueController.QueueData) error {
	atomic.AddInt32(&t.called, 1)
	return nil
}

func TestNewQueueController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)
	if qc == nil {
		t.Fatal("NewQueueController() returned nil")
	}
}

func TestQueueControllerStartAndSendSelf(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)
	qc.Start(concurr.NewConfig(4))

	qc.SendSelf("test-item")
	time.Sleep(100 * time.Millisecond)
	cancel()

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&action.called); got < 1 {
		t.Fatalf("action called %d times, want at least 1", got)
	}
}

func TestQueueControllerSetNext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)
	next := queueController.NewQueueController(ctx, action, wg)

	qc.SetNext(next)
	cancel()
}

func TestQueueControllerSetHandleError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)

	called := false
	qc.SetHandleError(func(err error) bool {
		called = true
		return true
	})
	// Handler is invoked by HandleError only when err != nil
	if !qc.HandleError(errors.New("test")) {
		t.Fatal("HandleError with handler returning true should return true")
	}
	if !called {
		t.Fatal("SetHandleError handler should be invoked by HandleError")
	}
	cancel()
}

func TestQueueControllerCheckContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)

	if err := qc.CheckContext(); err != nil {
		t.Fatalf("CheckContext() on live ctx = %v, want nil", err)
	}

	cancel()
	if err := qc.CheckContext(); err == nil {
		t.Fatal("CheckContext() on cancelled ctx should return error")
	}
}

func TestQueueControllerHandleError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)

	// nil error returns true
	if !qc.HandleError(nil) {
		t.Fatal("HandleError(nil) should return true")
	}

	cancel()
}

func TestQueueControllerBroadcastNoBroadcastSet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	qc := queueController.NewQueueController(ctx, action, wg)

	// Broadcast with empty map should not panic
	qc.Broadcast(&queueController.QueueData{})
	cancel()
}

func TestNewQueueData(t *testing.T) {
	qd := queueController.NewQueueData()
	if qd == nil {
		t.Fatal("NewQueueData() returned nil")
	}
	if qd.Index == nil {
		t.Fatal("Index should be initialized")
	}
}

func TestQueueDataSetAndGetAttribute(t *testing.T) {
	qd := queueController.NewQueueData()
	qd.SetAttribute("name", "test-value")
	if got := qd.Attribute("name"); got != "test-value" {
		t.Fatalf("Attribute('name') = %v, want %v", got, "test-value")
	}
	if got := qd.Attribute("missing"); got != nil {
		t.Fatalf("Attribute('missing') = %v, want nil", got)
	}
}

func TestQueueDataSetAttributesAndAttributes(t *testing.T) {
	qd := queueController.NewQueueData()
	qd.SetAttributes(map[string]interface{}{
		"a": 1,
		"b": "hello",
		"c": true,
	})

	attrs := qd.Attributes()
	if len(attrs) != 3 {
		t.Fatalf("Attributes() = %d entries, want 3", len(attrs))
	}
	if attrs["a"] != 1 || attrs["b"] != "hello" || attrs["c"] != true {
		t.Fatalf("unexpected attributes: %v", attrs)
	}
}

func TestQueueDataInt64StringStringsBool(t *testing.T) {
	qd := queueController.NewQueueData()
	qd.SetAttribute("count", int64(42))
	qd.SetAttribute("name", "test")
	qd.SetAttribute("tags", []string{"a", "b"})
	qd.SetAttribute("active", true)

	if got := qd.Int64("count"); got != 42 {
		t.Fatalf("Int64('count') = %d, want 42", got)
	}
	if got := qd.Int64("missing"); got != -1 {
		t.Fatalf("Int64('missing') = %d, want -1", got)
	}
	if got := qd.String("name"); got != "test" {
		t.Fatalf("String('name') = %q, want %q", got, "test")
	}
	if got := qd.String("missing"); got != "" {
		t.Fatalf("String('missing') = %q, want %q", got, "")
	}
	if got := qd.Strings("tags"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Strings('tags') = %v, want [a b]", got)
	}
	if got := qd.Strings("missing"); got != nil {
		t.Fatalf("Strings('missing') = %v, want nil", got)
	}
	if got := qd.Bool("active"); got != true {
		t.Fatalf("Bool('active') = %v, want true", got)
	}
	if got := qd.Bool("missing"); got != false {
		t.Fatalf("Bool('missing') = %v, want false", got)
	}
}

func TestQueueDataAppendTraceAndTrace(t *testing.T) {
	qd := queueController.NewQueueData()
	qd.AppendTrace("source1")
	qd.AppendTrace("source2")

	trace := qd.Trace()
	if trace == nil {
		t.Fatal("Trace() should not be nil")
	}
	if got := trace.Count(); got != 2 {
		t.Fatalf("trace.Count() = %d, want 2", got)
	}
}

func TestQueueDataNewQueueData(t *testing.T) {
	qd := queueController.NewQueueData()
	qd.SetAttribute("key", "value")
	nd := qd.NewQueueData()
	if nd == nil {
		t.Fatal("NewQueueData() returned nil")
	}
	if nd.Index == qd.Index {
		t.Fatal("NewQueueData() should have its own Index")
	}
}

func TestQueueActionWrapper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	wrapper := queueController.NewQueueActionWrapper(ctx, wg, action)
	if wrapper == nil {
		t.Fatal("NewQueueActionWrapper() returned nil")
	}

	qd := queueController.NewQueueData()
	qd.SetAttribute("test", "data")
	wg.Add(1)
	wrapper.Action(qd)
	wg.Wait()

	if got := atomic.LoadInt32(&action.called); got != 1 {
		t.Fatalf("action called %d times, want 1", got)
	}
	cancel()
}

func TestQueueActionWrapperSetDebug(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	wrapper := queueController.NewQueueActionWrapper(ctx, wg, action)

	wrapper.SetDebug(false)
	wrapper.SetDebug(true)
	cancel()
}

func TestQueueActionWrapperUnexpectedTypePanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	action := &testAction{}
	wrapper := queueController.NewQueueActionWrapper(ctx, wg, action)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Action with unexpected type should panic")
		}
	}()
	wrapper.Action("not a queue data")
	cancel()
}
