package fileslib

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/loopsrage/gotools/lib/index"
)

const directoriesIndexKey = "directories"

// FS defines the unified filesystem contract shared across local disk, in-memory,
// GCS, Azure Blob, and AWS S3 backends (matching toolset's FSBase + cloud blob methods).
type FS interface {
	Filesystem() string
	Write(ctx context.Context, filePath string, data []byte) error
	Read(ctx context.Context, filePath string) ([]byte, error)
	Exists(ctx context.Context, filePath string) (bool, error)
	List(ctx context.Context, pattern string) ([]string, error)
	GetRemoteMD5(ctx context.Context, filePath string) (string, error)
	WriteIfDifferent(ctx context.Context, filePath string, data []byte) (bool, error)
	LoadOrStore(ctx context.Context, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error)
	Transfer(ctx context.Context, to FS, pattern, targetDirectory string) error
	Sync(ctx context.Context, to FS, localPath, remotePath string) error
	Put(ctx context.Context, localPath, remotePath string, overwrite bool) error
	Close() error
}

// GetFilePath constructs a request-scoped path "/<requestID>[/<subDirs...>]/<fileName>".
func GetFilePath(requestID, fileName string, subDirs ...string) string {
	core := "/" + strings.Trim(requestID, "/")
	var cleanSubs []string
	for _, s := range subDirs {
		trimmed := strings.Trim(s, "/")
		if trimmed != "" {
			cleanSubs = append(cleanSubs, trimmed)
		}
	}
	if len(cleanSubs) == 0 {
		return core + "/" + strings.TrimLeft(fileName, "/")
	}
	return core + "/" + strings.Join(cleanSubs, "/") + "/" + strings.TrimLeft(fileName, "/")
}

// CalculateLocalMD5 computes the lowercase 32-character hex MD5 digest of data.
func CalculateLocalMD5(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// NormalizeMD5Bytes formats a raw 16-byte MD5 digest as lowercase hex.
func NormalizeMD5Bytes(digest []byte) string {
	if len(digest) == 0 {
		return ""
	}
	return strings.ToLower(hex.EncodeToString(digest))
}

// NormalizeMD5 normalizes quoted ETags, hex MD5 strings, or Base64 Content-MD5 headers
// into a lowercase 32-character hex string. Returns "" when input is empty.
func NormalizeMD5(md5Val string) string {
	if md5Val == "" {
		return ""
	}
	cleanVal := strings.Trim(md5Val, "\"")
	if cleanVal == "" {
		return ""
	}
	if len(cleanVal) == 32 && isHex(cleanVal) {
		return strings.ToLower(cleanVal)
	}
	if decoded, err := base64.StdEncoding.DecodeString(cleanVal); err == nil && len(decoded) == 16 {
		return strings.ToLower(hex.EncodeToString(decoded))
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(cleanVal); err == nil && len(decoded) == 16 {
		return strings.ToLower(hex.EncodeToString(decoded))
	}
	return strings.ToLower(cleanVal)
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// SplitProtocol separates a URI scheme ("gs://bucket/a" -> "gs", "bucket/a").
func SplitProtocol(remotePath string) (string, string) {
	if idx := strings.Index(remotePath, "://"); idx > 0 {
		return remotePath[:idx], remotePath[idx+3:]
	}
	return "", remotePath
}

// MatchGlob matches slash-normalized paths against a glob pattern supporting '*' and '?'.
func MatchGlob(pattern, candidate string) bool {
	_, cleanPattern := SplitProtocol(pattern)
	_, cleanCandidate := SplitProtocol(candidate)
	cleanPattern = strings.TrimPrefix(path.Clean("/"+cleanPattern), "/")
	cleanCandidate = strings.TrimPrefix(path.Clean("/"+cleanCandidate), "/")
	if cleanPattern == "" || cleanPattern == "." || cleanPattern == "*" {
		return true
	}
	matched, err := path.Match(cleanPattern, cleanCandidate)
	if err == nil && matched {
		return true
	}
	// Prefix wildcard fallback when pattern ends with "/*" and candidate has deeper nesting
	if strings.HasSuffix(cleanPattern, "/*") {
		prefix := strings.TrimSuffix(cleanPattern, "*")
		if strings.HasPrefix(cleanCandidate, prefix) {
			rest := strings.TrimPrefix(cleanCandidate, prefix)
			if !strings.Contains(rest, "/") && rest != "" {
				return true
			}
		}
	}
	return false
}

// PrefixFromGlob extracts the static key prefix prior to any glob metacharacters.
func PrefixFromGlob(pattern string) string {
	_, clean := SplitProtocol(pattern)
	clean = strings.TrimLeft(clean, "/")
	for i := 0; i < len(clean); i++ {
		switch clean[i] {
		case '*', '?', '[':
			return clean[:i]
		}
	}
	return clean
}

// WriteIfDifferentDefault compares local MD5 against remote MD5 and writes only on mismatch.
func WriteIfDifferentDefault(ctx context.Context, target FS, filePath string, data []byte) (bool, error) {
	localMD5 := CalculateLocalMD5(data)
	remoteMD5, err := target.GetRemoteMD5(ctx, filePath)
	if err == nil && remoteMD5 != "" && localMD5 == remoteMD5 {
		return false, nil
	}
	if err := target.Write(ctx, filePath, data); err != nil {
		return false, err
	}
	return true, nil
}

// LoadOrStoreDefault implements MD5-verified LoadOrStore semantics:
//  1. Computes local bytes via getBytes().
//  2. Queries GetRemoteMD5(filePath).
//  3. If remote exists and MD5 matches, returns (localData, true, nil) without writing.
//  4. If missing or MD5 differs, writes new bytes and returns (localData, false, nil).
func LoadOrStoreDefault(ctx context.Context, target FS, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error) {
	if getBytes == nil {
		return nil, false, errors.New("fileslib: getBytes callback cannot be nil")
	}
	localData, err := getBytes()
	if err != nil {
		return nil, false, err
	}
	localMD5 := CalculateLocalMD5(localData)
	remoteMD5, err := target.GetRemoteMD5(ctx, filePath)
	if err == nil && remoteMD5 != "" && remoteMD5 == localMD5 {
		return localData, true, nil
	}
	if _, err := target.WriteIfDifferent(ctx, filePath, localData); err != nil {
		return nil, false, err
	}
	return localData, false, nil
}

// TransferFS copies files matching pattern from src into targetDirectory on dst via LoadOrStore.
func TransferFS(ctx context.Context, src, dst FS, pattern, targetDirectory string) error {
	if dst == nil {
		return errors.New("fileslib: destination FS cannot be nil")
	}
	if strings.TrimSpace(targetDirectory) == "" {
		return errors.New("fileslib: targetDirectory cannot be empty")
	}
	if dst.Filesystem() == "file" {
		if err := os.MkdirAll(targetDirectory, 0o755); err != nil {
			return err
		}
	}
	paths, err := src.List(ctx, pattern)
	if err != nil {
		return err
	}
	for _, remotePath := range paths {
		_, withoutProto := SplitProtocol(remotePath)
		cleanFilename := path.Base(filepath.ToSlash(withoutProto))
		if cleanFilename == "" || cleanFilename == "." || cleanFilename == "/" {
			continue
		}
		var destPath string
		if dst.Filesystem() == "file" {
			destPath = filepath.Join(targetDirectory, cleanFilename)
		} else {
			destPath = strings.TrimRight(targetDirectory, "/") + "/" + cleanFilename
		}
		rp := remotePath
		_, _, err := dst.LoadOrStore(ctx, destPath, func() ([]byte, error) {
			return src.Read(ctx, rp)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// SyncFS performs bidirectional transfer between src (localPath) and dst (remotePath).
func SyncFS(ctx context.Context, src, dst FS, localPath, remotePath string) error {
	localTrim := strings.TrimRight(localPath, "/")
	remoteTrim := strings.TrimRight(remotePath, "/")
	if err := src.Transfer(ctx, dst, localTrim+"/*", remoteTrim+"/"); err != nil {
		return err
	}
	return dst.Transfer(ctx, src, remoteTrim+"/*", localTrim)
}

// LocalFS implements FS backed by the local operating system filesystem.
type LocalFS struct {
	idx *index.Index
}

func NewLocalFS() *LocalFS {
	idx := index.NewIndex()
	idx.New(directoriesIndexKey)
	return &LocalFS{idx: idx}
}

func (l *LocalFS) Filesystem() string { return "file" }

func (l *LocalFS) Write(_ context.Context, filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(filePath, data, 0o644)
}

func (l *LocalFS) Append(_ context.Context, filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func (l *LocalFS) MakeDirs(dirPath string, existOK bool) error {
	if !existOK {
		if _, err := os.Stat(dirPath); err == nil {
			return fmt.Errorf("fileslib: directory already exists: %s", dirPath)
		}
	}
	return os.MkdirAll(dirPath, 0o755)
}

func (l *LocalFS) Read(_ context.Context, filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

func (l *LocalFS) Exists(_ context.Context, filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (l *LocalFS) List(_ context.Context, pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr == nil && !info.IsDir() {
			files = append(files, m)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (l *LocalFS) GetRemoteMD5(_ context.Context, filePath string) (string, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return CalculateLocalMD5(b), nil
}

func (l *LocalFS) WriteIfDifferent(ctx context.Context, filePath string, data []byte) (bool, error) {
	return WriteIfDifferentDefault(ctx, l, filePath, data)
}

func (l *LocalFS) LoadOrStore(ctx context.Context, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error) {
	return LoadOrStoreDefault(ctx, l, filePath, getBytes)
}

func (l *LocalFS) Transfer(ctx context.Context, to FS, pattern, targetDirectory string) error {
	return TransferFS(ctx, l, to, pattern, targetDirectory)
}

func (l *LocalFS) Sync(ctx context.Context, to FS, localPath, remotePath string) error {
	return SyncFS(ctx, l, to, localPath, remotePath)
}

func (l *LocalFS) Put(ctx context.Context, localPath, remotePath string, overwrite bool) error {
	if !overwrite {
		if exists, _ := l.Exists(ctx, remotePath); exists {
			return fmt.Errorf("fileslib: destination already exists: %s", remotePath)
		}
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return l.Write(ctx, remotePath, data)
}

func (l *LocalFS) Index(rootPath string) error {
	byDir := make(map[string][]string)
	err := filepath.WalkDir(rootPath, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			dir := filepath.Dir(p)
			byDir[dir] = append(byDir[dir], d.Name())
		}
		return nil
	})
	if err != nil {
		return err
	}
	for dir, files := range byDir {
		sort.Strings(files)
		l.idx.StoreInIndex(directoriesIndexKey, dir, files)
	}
	return nil
}

func (l *LocalFS) ListFiles() []string {
	var out []string
	_ = l.idx.RangeIndex(directoriesIndexKey, func(key, value interface{}) {
		root, ok1 := key.(string)
		files, ok2 := value.([]string)
		if !ok1 || !ok2 {
			return
		}
		for _, f := range files {
			out = append(out, filepath.Join(root, f))
		}
	})
	sort.Strings(out)
	return out
}

func (l *LocalFS) Close() error { return nil }

// MemoryFS implements a thread-safe in-memory FS backed by index.Index.
type MemoryFS struct {
	store *index.Index
}

const memFilesBucket = "files"

func NewMemoryFS() *MemoryFS {
	idx := index.NewIndex()
	idx.New(memFilesBucket)
	return &MemoryFS{store: idx}
}

func normalizeMemKey(p string) string {
	_, without := SplitProtocol(p)
	return strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(without)), "/")
}

func (m *MemoryFS) Filesystem() string { return "memory" }

func (m *MemoryFS) Write(_ context.Context, filePath string, data []byte) error {
	cp := bytes.Clone(data)
	m.store.StoreInIndex(memFilesBucket, normalizeMemKey(filePath), cp)
	return nil
}

func (m *MemoryFS) Read(_ context.Context, filePath string) ([]byte, error) {
	val, err := m.store.LoadFromIndex(memFilesBucket, normalizeMemKey(filePath))
	if err != nil || val == nil {
		return nil, fmt.Errorf("fileslib: file not found: %s", filePath)
	}
	b, ok := val.([]byte)
	if !ok {
		return nil, fmt.Errorf("fileslib: invalid stored type for %s", filePath)
	}
	return bytes.Clone(b), nil
}

func (m *MemoryFS) Exists(_ context.Context, filePath string) (bool, error) {
	val, err := m.store.LoadFromIndex(memFilesBucket, normalizeMemKey(filePath))
	return err == nil && val != nil, nil
}

func (m *MemoryFS) List(_ context.Context, pattern string) ([]string, error) {
	var matches []string
	_ = m.store.RangeIndex(memFilesBucket, func(key, _ interface{}) {
		k, ok := key.(string)
		if !ok {
			return
		}
		if MatchGlob(pattern, k) {
			matches = append(matches, k)
		}
	})
	sort.Strings(matches)
	return matches, nil
}

func (m *MemoryFS) GetRemoteMD5(ctx context.Context, filePath string) (string, error) {
	b, err := m.Read(ctx, filePath)
	if err != nil {
		return "", err
	}
	return CalculateLocalMD5(b), nil
}

func (m *MemoryFS) WriteIfDifferent(ctx context.Context, filePath string, data []byte) (bool, error) {
	return WriteIfDifferentDefault(ctx, m, filePath, data)
}

func (m *MemoryFS) LoadOrStore(ctx context.Context, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error) {
	return LoadOrStoreDefault(ctx, m, filePath, getBytes)
}

func (m *MemoryFS) Transfer(ctx context.Context, to FS, pattern, targetDirectory string) error {
	return TransferFS(ctx, m, to, pattern, targetDirectory)
}

func (m *MemoryFS) Sync(ctx context.Context, to FS, localPath, remotePath string) error {
	return SyncFS(ctx, m, to, localPath, remotePath)
}

func (m *MemoryFS) Put(ctx context.Context, localPath, remotePath string, overwrite bool) error {
	if !overwrite {
		if exists, _ := m.Exists(ctx, remotePath); exists {
			return fmt.Errorf("fileslib: destination already exists: %s", remotePath)
		}
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return m.Write(ctx, remotePath, data)
}

func (m *MemoryFS) Close() error { return nil }

// FilesystemIndex provides a thread-safe named registry of FS implementations.
type FilesystemIndex struct {
	idx *index.Index
}

const fsRegistryBucket = "filesystems"

func NewFilesystemIndex() *FilesystemIndex {
	idx := index.NewIndex()
	idx.New(fsRegistryBucket)
	return &FilesystemIndex{idx: idx}
}

func (fi *FilesystemIndex) Register(name string, f FS) {
	fi.idx.StoreInIndex(fsRegistryBucket, name, f)
}

func (fi *FilesystemIndex) RegisterFilesystems(filesystems map[string]FS) {
	for name, f := range filesystems {
		fi.Register(name, f)
	}
}

func (fi *FilesystemIndex) Filesystem(name string) (FS, error) {
	v, err := fi.idx.LoadFromIndex(fsRegistryBucket, name)
	if err != nil || v == nil {
		return nil, fmt.Errorf("fileslib: filesystem %q is not registered", name)
	}
	fsImpl, ok := v.(FS)
	if !ok {
		return nil, fmt.Errorf("fileslib: entry %q does not implement FS", name)
	}
	return fsImpl, nil
}
