package catchPanic_test

import (
	"errors"
	"testing"
)

import "golangApp/lib/catchPanic"

func TestCatchPanicNormalExecution(t *testing.T) {
	var called bool
	catchPanic.CatchPanic(func() { called = true }, func(interface{}) {})
	if !called {
		t.Fatal("function was not called")
	}
}

func TestCatchPanicErrorPanic(t *testing.T) {
	var received interface{}
	err := errors.New("boom")
	catchPanic.CatchPanic(func() { panic(err) }, func(v interface{}) { received = v })
	if received != err {
		t.Fatalf("expected panic value %v, got %v", err, received)
	}
}

func TestCatchPanicStringPanic(t *testing.T) {
	var received interface{}
	catchPanic.CatchPanic(func() { panic("string panic") }, func(v interface{}) { received = v })
	if received != "string panic" {
		t.Fatalf("expected 'string panic', got %v", received)
	}
}

func TestCatchPanicIntPanic(t *testing.T) {
	var received interface{}
	catchPanic.CatchPanic(func() { panic(42) }, func(v interface{}) { received = v })
	if received != 42 {
		t.Fatalf("expected 42, got %v", received)
	}
}

func TestCatchPanicCbNotCalledOnNoPanic(t *testing.T) {
	cbCalled := false
	catchPanic.CatchPanic(func() {}, func(interface{}) { cbCalled = true })
	if cbCalled {
		t.Fatal("callback should not be called when no panic occurs")
	}
}
