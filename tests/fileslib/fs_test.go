package fileslib_test

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loopsrage/gotools/lib/fileslib"
)

func TestMD5NormalizationAndPathHelpers(t *testing.T) {
	payload := []byte("tariff_payload_123")
	sum := md5.Sum(payload)
	expectedHex := strings.ToLower(hex.EncodeToString(sum[:]))

	if got := fileslib.CalculateLocalMD5(payload); got != expectedHex {
		t.Fatalf("CalculateLocalMD5 = %q, want %q", got, expectedHex)
	}
	if got := fileslib.NormalizeMD5(""); got != "" {
		t.Fatalf("NormalizeMD5 empty = %q, want empty", got)
	}
	if got := fileslib.NormalizeMD5Bytes(sum[:]); got != expectedHex {
		t.Fatalf("NormalizeMD5Bytes = %q, want %q", got, expectedHex)
	}
	if got := fileslib.NormalizeMD5(`"` + strings.ToUpper(expectedHex) + `"`); got != expectedHex {
		t.Fatalf("NormalizeMD5 quoted hex = %q, want %q", got, expectedHex)
	}
	b64 := base64.StdEncoding.EncodeToString(sum[:])
	if got := fileslib.NormalizeMD5(b64); got != expectedHex {
		t.Fatalf("NormalizeMD5 base64 = %q, want %q", got, expectedHex)
	}
	if got := fileslib.GetFilePath("req-1", "report.csv", "raw", "2026"); got != "/req-1/raw/2026/report.csv" {
		t.Fatalf("GetFilePath = %q, want /req-1/raw/2026/report.csv", got)
	}
}

func TestLocalAndMemoryFSTransferSyncAndIndex(t *testing.T) {
	ctx := context.Background()
	mem := fileslib.NewMemoryFS()
	local := fileslib.NewLocalFS()

	fss := fileslib.NewFilesystemIndex()
	fss.RegisterFilesystems(map[string]fileslib.FS{
		"mem":   mem,
		"local": local,
	})

	memFS, err := fss.Filesystem("mem")
	if err != nil {
		t.Fatalf("Filesystem(mem) error: %v", err)
	}
	localFS, err := fss.Filesystem("local")
	if err != nil {
		t.Fatalf("Filesystem(local) error: %v", err)
	}

	stored, loaded, err := memFS.LoadOrStore(ctx, "/tariffs/item.txt", func() ([]byte, error) {
		return []byte("customs_data"), nil
	})
	if err != nil || loaded || string(stored) != "customs_data" {
		t.Fatalf("initial LoadOrStore = (%q, %v, %v), want (customs_data, false, nil)", string(stored), loaded, err)
	}

	stored2, loaded2, err := memFS.LoadOrStore(ctx, "/tariffs/item.txt", func() ([]byte, error) {
		return []byte("customs_data"), nil
	})
	if err != nil || !loaded2 || string(stored2) != "customs_data" {
		t.Fatalf("second LoadOrStore = (%q, %v, %v), want (customs_data, true, nil)", string(stored2), loaded2, err)
	}

	destDir := t.TempDir()
	if err := memFS.Transfer(ctx, localFS, "/tariffs/*", destDir); err != nil {
		t.Fatalf("Transfer error: %v", err)
	}
	diskBytes, err := os.ReadFile(filepath.Join(destDir, "item.txt"))
	if err != nil || string(diskBytes) != "customs_data" {
		t.Fatalf("transferred file = %q, err=%v", string(diskBytes), err)
	}

	if err := local.Index(destDir); err != nil {
		t.Fatalf("LocalFS.Index error: %v", err)
	}
	indexed := local.ListFiles()
	if len(indexed) != 1 || filepath.Base(indexed[0]) != "item.txt" {
		t.Fatalf("LocalFS.ListFiles = %v, want 1 item.txt", indexed)
	}
}
