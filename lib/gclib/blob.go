package gclib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/loopsrage/gotools/lib/fileslib"
)

// GcsBlobConfig configures Google Cloud Storage via cloud.google.com/go/storage.
type GcsBlobConfig struct {
	BucketName      string
	Project         string
	Token           string
	TokenSource     oauth2.TokenSource
	CredentialsJSON []byte
	EndpointURL     string
	HTTPClient      *http.Client
	ClientOptions   []option.ClientOption
	Extra           map[string]interface{}
}

// GcsBlob implements fileslib.FS backed by official cloud.google.com/go/storage.Client (or an injected FS backend).
type GcsBlob struct {
	BucketName string
	Config     GcsBlobConfig
	client     *storage.Client
	backend    fileslib.FS
}

// NewGcsBlob constructs a GcsBlob backed by official cloud.google.com/go/storage.Client.
func NewGcsBlob(cfg GcsBlobConfig) (*GcsBlob, error) {
	ctx := context.Background()
	var opts []option.ClientOption

	if ep := strings.TrimRight(cfg.EndpointURL, "/"); ep != "" {
		if !strings.Contains(ep, "/storage/v1") {
			ep += "/storage/v1/"
		} else if !strings.HasSuffix(ep, "/") {
			ep += "/"
		}
		opts = append(opts, option.WithEndpoint(ep))
	}

	if strings.TrimSpace(os.Getenv("STORAGE_EMULATOR_HOST")) != "" && cfg.TokenSource == nil && cfg.Token == "" && len(cfg.CredentialsJSON) == 0 {
		opts = append(opts, option.WithoutAuthentication())
		if cfg.HTTPClient != nil {
			opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
		}
		opts = append(opts, cfg.ClientOptions...)
	} else {
		authOpts, err := buildGoogleClientOptions(
			ctx,
			cfg.TokenSource,
			cfg.Token,
			cfg.CredentialsJSON,
			cfg.HTTPClient,
			nil,
			cfg.ClientOptions,
			storage.ScopeFullControl,
		)
		if err != nil {
			return nil, err
		}
		opts = append(opts, authOpts...)
	}

	opts = append(opts, storage.WithJSONReads())
	stClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &GcsBlob{
		BucketName: cfg.BucketName,
		Config:     cfg,
		client:     stClient,
	}, nil
}

// NewGcsBlobWithBackend constructs a GcsBlob delegating storage operations to backend (e.g. MemoryFS).
func NewGcsBlobWithBackend(bucketName string, backend fileslib.FS) *GcsBlob {
	return &GcsBlob{
		BucketName: bucketName,
		Config:     GcsBlobConfig{BucketName: bucketName},
		backend:    backend,
	}
}

// GcsClient returns the underlying *storage.Client.
func (g *GcsBlob) GcsClient() *storage.Client {
	return g.client
}

func (g *GcsBlob) Filesystem() string { return "gcs" }

// FullKey prefixes BucketName onto filePath if configured and not already prefixed.
func (g *GcsBlob) FullKey(filePath string) string {
	_, clean := fileslib.SplitProtocol(filePath)
	clean = strings.TrimLeft(clean, "/")
	if g.BucketName != "" && !strings.HasPrefix(clean, g.BucketName+"/") && clean != g.BucketName {
		return g.BucketName + "/" + clean
	}
	return clean
}

func (g *GcsBlob) splitBucketObject(filePath string) (string, string, error) {
	full := g.FullKey(filePath)
	idx := strings.IndexByte(full, '/')
	if idx <= 0 || idx >= len(full)-1 {
		return "", "", fmt.Errorf("gclib: invalid GCS bucket/object key %q", full)
	}
	return full[:idx], full[idx+1:], nil
}

func (g *GcsBlob) Write(ctx context.Context, filePath string, data []byte) error {
	if g.backend != nil {
		return g.backend.Write(ctx, g.FullKey(filePath), data)
	}
	bucket, obj, err := g.splitBucketObject(filePath)
	if err != nil {
		return err
	}
	w := g.client.Bucket(bucket).Object(obj).NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (g *GcsBlob) Read(ctx context.Context, filePath string) ([]byte, error) {
	if g.backend != nil {
		return g.backend.Read(ctx, g.FullKey(filePath))
	}
	bucket, obj, err := g.splitBucketObject(filePath)
	if err != nil {
		return nil, err
	}
	r, err := g.client.Bucket(bucket).Object(obj).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (g *GcsBlob) Exists(ctx context.Context, filePath string) (bool, error) {
	if g.backend != nil {
		return g.backend.Exists(ctx, g.FullKey(filePath))
	}
	md5Hex, err := g.GetRemoteMD5(ctx, filePath)
	if err != nil {
		return false, nil
	}
	return md5Hex != "", nil
}

func (g *GcsBlob) GetRemoteMD5(ctx context.Context, filePath string) (string, error) {
	if g.backend != nil {
		return g.backend.GetRemoteMD5(ctx, g.FullKey(filePath))
	}
	bucket, obj, err := g.splitBucketObject(filePath)
	if err != nil {
		return "", err
	}
	attrs, err := g.client.Bucket(bucket).Object(obj).Attrs(ctx)
	if err != nil {
		return "", err
	}
	if len(attrs.MD5) > 0 {
		return fileslib.NormalizeMD5Bytes(attrs.MD5), nil
	}
	if attrs.Etag != "" {
		return fileslib.NormalizeMD5(attrs.Etag), nil
	}
	return "", errors.New("gclib: GCS object attributes missing MD5 and Etag")
}

func (g *GcsBlob) List(ctx context.Context, pattern string) ([]string, error) {
	fullPattern := g.FullKey(pattern)
	if g.backend != nil {
		items, err := g.backend.List(ctx, fullPattern)
		if err != nil {
			return nil, err
		}
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = "gs://" + strings.TrimLeft(it, "/")
		}
		return out, nil
	}

	idx := strings.IndexByte(fullPattern, '/')
	bucket := g.BucketName
	subPattern := fullPattern
	if idx > 0 {
		bucket = fullPattern[:idx]
		subPattern = fullPattern[idx+1:]
	}
	if bucket == "" {
		return nil, errors.New("gclib: bucket name required for List")
	}

	prefix := fileslib.PrefixFromGlob(subPattern)
	it := g.client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})

	var matched []string
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if fileslib.MatchGlob(subPattern, attrs.Name) {
			matched = append(matched, fmt.Sprintf("gs://%s/%s", bucket, attrs.Name))
		}
	}
	sort.Strings(matched)
	return matched, nil
}

func (g *GcsBlob) WriteIfDifferent(ctx context.Context, filePath string, data []byte) (bool, error) {
	return fileslib.WriteIfDifferentDefault(ctx, g, filePath, data)
}

func (g *GcsBlob) LoadOrStore(ctx context.Context, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error) {
	return fileslib.LoadOrStoreDefault(ctx, g, filePath, getBytes)
}

func (g *GcsBlob) Transfer(ctx context.Context, to fileslib.FS, pattern, targetDirectory string) error {
	return fileslib.TransferFS(ctx, g, to, pattern, targetDirectory)
}

func (g *GcsBlob) Sync(ctx context.Context, to fileslib.FS, localPath, remotePath string) error {
	return fileslib.SyncFS(ctx, g, to, localPath, remotePath)
}

func (g *GcsBlob) Put(ctx context.Context, localPath, remotePath string, overwrite bool) error {
	if !overwrite {
		if exists, _ := g.Exists(ctx, remotePath); exists {
			return fmt.Errorf("gclib: remote object already exists: %s", remotePath)
		}
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return g.Write(ctx, remotePath, data)
}

func (g *GcsBlob) Close() error {
	if g.backend != nil {
		return g.backend.Close()
	}
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}
