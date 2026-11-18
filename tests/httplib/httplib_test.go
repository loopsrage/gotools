package httplib_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loopsrage/gotools/lib/httplib"
)

func TestWriteCorsResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	msg := []byte(`{"ok":true}`)
	status := http.StatusCreated

	httplib.WriteCorsResponse(rec, &msg, &status)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS header missing: %v", rec.Header())
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body = %q, want %q", rec.Body.String(), `{"ok":true}`)
	}
}
