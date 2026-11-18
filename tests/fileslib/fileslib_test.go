package fileslib_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loopsrage/gotools/lib/fileslib"
)

func TestCopyfile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("hello gotools"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if err := fileslib.Copyfile(src, dst); err != nil {
		t.Fatalf("Copyfile error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "hello gotools" {
		t.Fatalf("copied content = %q, want %q", string(got), "hello gotools")
	}
}
