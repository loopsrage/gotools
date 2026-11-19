package fileslib

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// FileInfo holds portable file metadata returned by Stat.
type FileInfo struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
}

// MutableFS extends FS with Delete, IsDir, and Stat operations.
type MutableFS interface {
	FS
	Delete(ctx context.Context, filePath string) error
	IsDir(ctx context.Context, filePath string) (bool, error)
	Stat(ctx context.Context, filePath string) (FileInfo, error)
}

// Delete removes a file or directory on LocalFS.
func (l *LocalFS) Delete(_ context.Context, filePath string) error {
	return os.RemoveAll(filePath)
}

// IsDir reports whether filePath is a directory on LocalFS.
func (l *LocalFS) IsDir(_ context.Context, filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// Stat returns FileInfo for filePath on LocalFS.
func (l *LocalFS) Stat(_ context.Context, filePath string) (FileInfo, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Path:    filePath,
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime().UTC(),
	}, nil
}

// Delete removes a key from MemoryFS.
func (m *MemoryFS) Delete(_ context.Context, filePath string) error {
	m.store.DeleteFromIndex(memFilesBucket, normalizeMemKey(filePath))
	return nil
}

// IsDir reports whether filePath is a directory prefix in MemoryFS.
func (m *MemoryFS) IsDir(_ context.Context, filePath string) (bool, error) {
	prefix := strings.TrimRight(normalizeMemKey(filePath), "/") + "/"
	found := false
	_ = m.store.RangeIndex(memFilesBucket, func(key, _ interface{}) {
		if k, ok := key.(string); ok && strings.HasPrefix(k, prefix) {
			found = true
		}
	})
	return found, nil
}

// Stat returns FileInfo for filePath in MemoryFS.
func (m *MemoryFS) Stat(ctx context.Context, filePath string) (FileInfo, error) {
	b, err := m.Read(ctx, filePath)
	if err == nil {
		return FileInfo{
			Path:    filePath,
			Size:    int64(len(b)),
			IsDir:   false,
			ModTime: time.Now().UTC(),
		}, nil
	}
	if isDir, _ := m.IsDir(ctx, filePath); isDir {
		return FileInfo{Path: filePath, IsDir: true, ModTime: time.Now().UTC()}, nil
	}
	return FileInfo{}, fmt.Errorf("fileslib: not found: %s", filePath)
}

// Delete removes filePath if target implements MutableFS.
func Delete(ctx context.Context, target FS, filePath string) error {
	if m, ok := target.(MutableFS); ok {
		return m.Delete(ctx, filePath)
	}
	return fmt.Errorf("fileslib: filesystem %s does not support Delete", target.Filesystem())
}

// Stat retrieves FileInfo if target implements MutableFS.
func Stat(ctx context.Context, target FS, filePath string) (FileInfo, error) {
	if m, ok := target.(MutableFS); ok {
		return m.Stat(ctx, filePath)
	}
	b, err := target.Read(ctx, filePath)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Path: filePath, Size: int64(len(b)), IsDir: false}, nil
}
