package gziplib_test

import (
	"bytes"
	"testing"

	"github.com/loopsrage/gotools/lib/gziplib"
)

func TestIsGzipNil(t *testing.T) {
	if gziplib.IsGzip(nil) {
		t.Fatal("IsGzip(nil) should return false")
	}
}

func TestIsGzipEmpty(t *testing.T) {
	if gziplib.IsGzip([]byte{}) {
		t.Fatal("IsGzip(empty) should return false")
	}
	if gziplib.IsGzip([]byte{0x1F}) {
		t.Fatal("IsGzip(single byte) should return false")
	}
}

func TestIsGzipTrue(t *testing.T) {
	if !gziplib.IsGzip([]byte{0x1F, 0x8B}) {
		t.Fatal("IsGzip(gzip header) should return true")
	}
}

func TestIsGzipFalse(t *testing.T) {
	if gziplib.IsGzip([]byte{0x1F, 0x8C}) {
		t.Fatal("IsGzip(non-gzip header) should return false")
	}
	if gziplib.IsGzip([]byte("hello")) {
		t.Fatal("IsGzip(text) should return false")
	}
}

func TestGzipRoundTrip(t *testing.T) {
	original := []byte("Hello, Gzip! This is some test data that should be compressed.")
	compressed, err := gziplib.Gzip(original)
	if err != nil {
		t.Fatalf("Gzip() error: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatal("Gzip() returned empty buffer")
	}
	if !gziplib.IsGzip(compressed) {
		t.Fatal("Gzip() output should start with gzip header")
	}

	// Decompress using EnsureGunzip
	buff := bytes.NewBuffer(compressed)
	gunzipped, err := gziplib.EnsureGunzip(buff)
	if err != nil {
		t.Fatalf("EnsureGunzip() error: %v", err)
	}
	got := gunzipped.Bytes()
	if string(got) != string(original) {
		t.Fatalf("EnsureGunzip() = %q, want %q", got, original)
	}
}

func TestEnsureGunzipNotGzip(t *testing.T) {
	input := []byte("plain text, not gzipped")
	buff := bytes.NewBuffer(input)
	out, err := gziplib.EnsureGunzip(buff)
	if err != nil {
		t.Fatalf("EnsureGunzip() on non-gzip error: %v", err)
	}
	if string(out.Bytes()) != string(input) {
		t.Fatalf("EnsureGunzip() = %q, want %q", out.Bytes(), input)
	}
}

func TestEnsureGunzipDoubleGzip(t *testing.T) {
	original := []byte("double gzip test")
	gz1, err := gziplib.Gzip(original)
	if err != nil {
		t.Fatalf("Gzip() error: %v", err)
	}
	gz2, err := gziplib.Gzip(gz1)
	if err != nil {
		t.Fatalf("second Gzip() error: %v", err)
	}

	buff := bytes.NewBuffer(gz2)
	out, err := gziplib.EnsureGunzip(buff)
	if err != nil {
		t.Fatalf("EnsureGunzip() double error: %v", err)
	}
	if string(out.Bytes()) != string(original) {
		t.Fatalf("double gunzip = %q, want %q", out.Bytes(), original)
	}
}

func TestEnsureGunzipRW(t *testing.T) {
	original := []byte("ReadWriter gunzip test")
	compressed, err := gziplib.Gzip(original)
	if err != nil {
		t.Fatalf("Gzip() error: %v", err)
	}

	var buf bytes.Buffer
	buf.Write(compressed)
	out, err := gziplib.EnsureGunzipRW(&buf)
	if err != nil {
		t.Fatalf("EnsureGunzipRW() error: %v", err)
	}
	if string(out.Bytes()) != string(original) {
		t.Fatalf("EnsureGunzipRW() = %q, want %q", out.Bytes(), original)
	}
}

func TestEnsureGunzipRWNotGzip(t *testing.T) {
	input := []byte("plain text")
	var buf bytes.Buffer
	buf.Write(input)
	out, err := gziplib.EnsureGunzipRW(&buf)
	if err != nil {
		t.Fatalf("EnsureGunzipRW() on non-gzip error: %v", err)
	}
	if string(out.Bytes()) != string(input) {
		t.Fatalf("EnsureGunzipRW() = %q, want %q", out.Bytes(), input)
	}
}

func TestGzipEmptyInput(t *testing.T) {
	compressed, err := gziplib.Gzip([]byte{})
	if err != nil {
		t.Fatalf("Gzip() on empty error: %v", err)
	}
	if !gziplib.IsGzip(compressed) {
		t.Fatal("Gzip(empty) output should still be gzip format")
	}
	buff := bytes.NewBuffer(compressed)
	out, err := gziplib.EnsureGunzip(buff)
	if err != nil {
		t.Fatalf("EnsureGunzip() error: %v", err)
	}
	if len(out.Bytes()) != 0 {
		t.Fatalf("EnsureGunzip(empty) = %d bytes, want 0", len(out.Bytes()))
	}
}
