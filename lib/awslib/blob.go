package awslib

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/loopsrage/gotools/lib/fileslib"
)

// AwsBlobConfig configures AWS S3 access via aws-sdk-go-v2 service/s3.
type AwsBlobConfig struct {
	BucketName         string
	RegionName         string
	AwsAccessKeyID     string
	AwsSecretAccessKey string
	AwsSessionToken    string
	EndpointURL        string
	ForcePathStyle     bool
	HTTPClient         *http.Client
	Extra              map[string]interface{}
}

// AwsBlob implements fileslib.FS backed by official aws-sdk-go-v2 s3.Client (or an injected FS backend).
type AwsBlob struct {
	BucketName string
	Config     AwsBlobConfig
	client     *s3.Client
	backend    fileslib.FS
}

// NewAwsBlob constructs an AwsBlob backed by aws-sdk-go-v2 s3.Client.
func NewAwsBlob(cfg AwsBlobConfig) (*AwsBlob, error) {
	awsCfg, err := BuildAWSConfig(
		context.Background(),
		cfg.RegionName,
		cfg.AwsAccessKeyID,
		cfg.AwsSecretAccessKey,
		cfg.AwsSessionToken,
		cfg.HTTPClient,
	)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if ep := strings.TrimSpace(cfg.EndpointURL); ep != "" {
			o.BaseEndpoint = aws.String(ep)
			o.UsePathStyle = true
		}
		if cfg.ForcePathStyle {
			o.UsePathStyle = true
		}
	})

	return &AwsBlob{
		BucketName: cfg.BucketName,
		Config:     cfg,
		client:     s3Client,
	}, nil
}

// NewAwsBlobWithBackend constructs an AwsBlob delegating storage operations to backend (e.g. MemoryFS).
func NewAwsBlobWithBackend(bucketName string, backend fileslib.FS) *AwsBlob {
	return &AwsBlob{
		BucketName: bucketName,
		Config:     AwsBlobConfig{BucketName: bucketName},
		backend:    backend,
	}
}

// S3Client returns the underlying aws-sdk-go-v2 *s3.Client.
func (s *AwsBlob) S3Client() *s3.Client {
	return s.client
}

func (s *AwsBlob) Filesystem() string { return "s3" }

// FullKey prefixes BucketName onto filePath when BucketName is configured and not already prefixed.
func (s *AwsBlob) FullKey(filePath string) string {
	_, clean := fileslib.SplitProtocol(filePath)
	clean = strings.TrimLeft(clean, "/")
	if s.BucketName != "" && !strings.HasPrefix(clean, s.BucketName+"/") && clean != s.BucketName {
		return s.BucketName + "/" + clean
	}
	return clean
}

func (s *AwsBlob) splitBucketKey(filePath string) (string, string, error) {
	full := s.FullKey(filePath)
	idx := strings.IndexByte(full, '/')
	if idx <= 0 || idx >= len(full)-1 {
		return "", "", fmt.Errorf("awslib: invalid S3 bucket/key %q", full)
	}
	return full[:idx], full[idx+1:], nil
}

func (s *AwsBlob) Write(ctx context.Context, filePath string, data []byte) error {
	if s.backend != nil {
		return s.backend.Write(ctx, s.FullKey(filePath), data)
	}
	bucket, key, err := s.splitBucketKey(filePath)
	if err != nil {
		return err
	}
	sum := md5.Sum(data)
	contentMD5 := base64.StdEncoding.EncodeToString(sum[:])
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentMD5:    aws.String(contentMD5),
	})
	return err
}

func (s *AwsBlob) Read(ctx context.Context, filePath string) ([]byte, error) {
	if s.backend != nil {
		return s.backend.Read(ctx, s.FullKey(filePath))
	}
	bucket, key, err := s.splitBucketKey(filePath)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *AwsBlob) Exists(ctx context.Context, filePath string) (bool, error) {
	if s.backend != nil {
		return s.backend.Exists(ctx, s.FullKey(filePath))
	}
	md5Hex, err := s.GetRemoteMD5(ctx, filePath)
	if err != nil {
		return false, nil
	}
	return md5Hex != "", nil
}

func (s *AwsBlob) GetRemoteMD5(ctx context.Context, filePath string) (string, error) {
	if s.backend != nil {
		return s.backend.GetRemoteMD5(ctx, s.FullKey(filePath))
	}
	bucket, key, err := s.splitBucketKey(filePath)
	if err != nil {
		return "", err
	}
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", err
	}
	if out.ETag == nil || *out.ETag == "" {
		return "", errors.New("awslib: S3 HeadObject response missing ETag")
	}
	return fileslib.NormalizeMD5(*out.ETag), nil
}

func (s *AwsBlob) List(ctx context.Context, pattern string) ([]string, error) {
	fullPattern := s.FullKey(pattern)
	if s.backend != nil {
		items, err := s.backend.List(ctx, fullPattern)
		if err != nil {
			return nil, err
		}
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = "s3://" + strings.TrimLeft(it, "/")
		}
		return out, nil
	}

	idx := strings.IndexByte(fullPattern, '/')
	bucket := s.BucketName
	subPattern := fullPattern
	if idx > 0 {
		bucket = fullPattern[:idx]
		subPattern = fullPattern[idx+1:]
	}
	if bucket == "" {
		return nil, errors.New("awslib: S3 bucket name required for List")
	}

	prefix := fileslib.PrefixFromGlob(subPattern)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var matched []string
	paginator := s3.NewListObjectsV2Paginator(s.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			if fileslib.MatchGlob(subPattern, *obj.Key) {
				matched = append(matched, fmt.Sprintf("s3://%s/%s", bucket, *obj.Key))
			}
		}
	}

	sort.Strings(matched)
	return matched, nil
}

func (s *AwsBlob) WriteIfDifferent(ctx context.Context, filePath string, data []byte) (bool, error) {
	return fileslib.WriteIfDifferentDefault(ctx, s, filePath, data)
}

func (s *AwsBlob) LoadOrStore(ctx context.Context, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error) {
	return fileslib.LoadOrStoreDefault(ctx, s, filePath, getBytes)
}

func (s *AwsBlob) Transfer(ctx context.Context, to fileslib.FS, pattern, targetDirectory string) error {
	return fileslib.TransferFS(ctx, s, to, pattern, targetDirectory)
}

func (s *AwsBlob) Sync(ctx context.Context, to fileslib.FS, localPath, remotePath string) error {
	return fileslib.SyncFS(ctx, s, to, localPath, remotePath)
}

func (s *AwsBlob) Put(ctx context.Context, localPath, remotePath string, overwrite bool) error {
	if !overwrite {
		if exists, _ := s.Exists(ctx, remotePath); exists {
			return fmt.Errorf("awslib: remote S3 object already exists: %s", remotePath)
		}
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return s.Write(ctx, remotePath, data)
}

func (s *AwsBlob) Close() error {
	if s.backend != nil {
		return s.backend.Close()
	}
	return nil
}
