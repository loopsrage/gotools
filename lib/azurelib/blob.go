package azurelib

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"

	"github.com/loopsrage/gotools/lib/fileslib"
)

// AzureBlobConfig configures Azure Blob Storage via github.com/Azure/azure-sdk-for-go/sdk/storage/azblob.
type AzureBlobConfig struct {
	AccountName      string
	AccountKey       string
	ContainerName    string
	EndpointURL      string
	SSL              *bool
	ConnectionVerify *bool
	Credential       azcore.TokenCredential
	HTTPClient       *http.Client
	Extra            map[string]interface{}
}

// AzureIndexAgentSearchConfig wraps AzureBlobConfig under Storage, matching toolset's AzureIndexAgentSearchConfig.
type AzureIndexAgentSearchConfig struct {
	Storage AzureBlobConfig
	Extra   map[string]interface{}
}

// CalculateLocalMD5 computes the lowercase hex MD5 digest of data.
func CalculateLocalMD5(data []byte) string {
	return fileslib.CalculateLocalMD5(data)
}

// NormalizeMD5 normalizes quoted ETags, hex MD5 strings, or Base64 Content-MD5 headers into lowercase hex.
func NormalizeMD5(md5Val string) string {
	return fileslib.NormalizeMD5(md5Val)
}

// NormalizeMD5Bytes normalizes a raw MD5 digest byte slice into lowercase hex.
func NormalizeMD5Bytes(digest []byte) string {
	return fileslib.NormalizeMD5Bytes(digest)
}

// AzureBlob implements fileslib.FS backed by official azblob.Client (or an injected FS backend).
type AzureBlob struct {
	Config  AzureBlobConfig
	Client  *azblob.Client
	backend fileslib.FS
}

// NewAzureBlob constructs an AzureBlob client backed by the official azblob SDK.
func NewAzureBlob(cfg AzureBlobConfig) (*AzureBlob, error) {
	serviceURL := strings.TrimRight(cfg.EndpointURL, "/")
	if serviceURL == "" {
		if strings.TrimSpace(cfg.AccountName) == "" {
			return nil, errors.New("azurelib: AccountName or EndpointURL is required")
		}
		scheme := "https"
		if cfg.SSL != nil && !*cfg.SSL {
			scheme = "http"
		}
		serviceURL = fmt.Sprintf("%s://%s.blob.core.windows.net", scheme, cfg.AccountName)
	}
	if !strings.HasSuffix(serviceURL, "/") {
		serviceURL += "/"
	}

	clientOpts := &azblob.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			InsecureAllowCredentialWithHTTP: strings.HasPrefix(strings.ToLower(serviceURL), "http://"),
		},
	}
	if cfg.HTTPClient != nil {
		clientOpts.Transport = cfg.HTTPClient
	}

	var (
		azClient *azblob.Client
		err      error
	)
	if strings.TrimSpace(cfg.AccountKey) != "" {
		acct := cfg.AccountName
		if acct == "" {
			acct = "devstoreaccount1"
		}
		skCred, credErr := azblob.NewSharedKeyCredential(acct, cfg.AccountKey)
		if credErr != nil {
			return nil, credErr
		}
		azClient, err = azblob.NewClientWithSharedKeyCredential(serviceURL, skCred, clientOpts)
	} else {
		cred, credErr := ResolveAzureCredential(cfg.Credential)
		if credErr != nil {
			return nil, credErr
		}
		azClient, err = azblob.NewClient(serviceURL, cred, clientOpts)
	}
	if err != nil {
		return nil, err
	}

	return &AzureBlob{
		Config: cfg,
		Client: azClient,
	}, nil
}

// NewAzureBlobWithBackend constructs an AzureBlob delegating storage operations to backend (e.g. MemoryFS).
func NewAzureBlobWithBackend(backend fileslib.FS) *AzureBlob {
	return &AzureBlob{backend: backend}
}

func (b *AzureBlob) Filesystem() string { return "abfs" }

func (b *AzureBlob) splitContainerBlob(filePath string) (string, string, error) {
	_, clean := fileslib.SplitProtocol(filePath)
	clean = strings.TrimLeft(clean, "/")
	if b.Config.ContainerName != "" && !strings.HasPrefix(clean, b.Config.ContainerName+"/") {
		return b.Config.ContainerName, clean, nil
	}
	idx := strings.IndexByte(clean, '/')
	if idx <= 0 || idx >= len(clean)-1 {
		return "", "", fmt.Errorf("azurelib: expected <container>/<blob> path, got %q", filePath)
	}
	return clean[:idx], clean[idx+1:], nil
}

func (b *AzureBlob) Write(ctx context.Context, filePath string, data []byte) error {
	if b.backend != nil {
		return b.backend.Write(ctx, filePath, data)
	}
	container, blobName, err := b.splitContainerBlob(filePath)
	if err != nil {
		return err
	}
	sum := md5.Sum(data)
	_, err = b.Client.UploadBuffer(ctx, container, blobName, data, &azblob.UploadBufferOptions{
		HTTPHeaders: &blob.HTTPHeaders{
			BlobContentMD5: sum[:],
		},
	})
	return err
}

func (b *AzureBlob) Read(ctx context.Context, filePath string) ([]byte, error) {
	if b.backend != nil {
		return b.backend.Read(ctx, filePath)
	}
	container, blobName, err := b.splitContainerBlob(filePath)
	if err != nil {
		return nil, err
	}
	resp, err := b.Client.DownloadStream(ctx, container, blobName, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (b *AzureBlob) Exists(ctx context.Context, filePath string) (bool, error) {
	if b.backend != nil {
		return b.backend.Exists(ctx, filePath)
	}
	md5Hex, err := b.GetRemoteMD5(ctx, filePath)
	if err != nil {
		return false, nil
	}
	return md5Hex != "", nil
}

func (b *AzureBlob) GetRemoteMD5(ctx context.Context, filePath string) (string, error) {
	if b.backend != nil {
		return b.backend.GetRemoteMD5(ctx, filePath)
	}
	container, blobName, err := b.splitContainerBlob(filePath)
	if err != nil {
		return "", err
	}
	blobClient := b.Client.ServiceClient().NewContainerClient(container).NewBlobClient(blobName)
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		return "", err
	}
	if len(props.ContentMD5) > 0 {
		return NormalizeMD5Bytes(props.ContentMD5), nil
	}
	if props.ETag != nil && string(*props.ETag) != "" {
		return NormalizeMD5(string(*props.ETag)), nil
	}
	return "", errors.New("azurelib: blob properties missing ContentMD5 and ETag")
}

func (b *AzureBlob) List(ctx context.Context, pattern string) ([]string, error) {
	if b.backend != nil {
		items, err := b.backend.List(ctx, pattern)
		if err != nil {
			return nil, err
		}
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = "abfs://" + strings.TrimLeft(it, "/")
		}
		return out, nil
	}

	_, cleanPattern := fileslib.SplitProtocol(pattern)
	cleanPattern = strings.TrimLeft(cleanPattern, "/")
	container := b.Config.ContainerName
	subPattern := cleanPattern
	if container == "" {
		idx := strings.IndexByte(cleanPattern, '/')
		if idx <= 0 {
			return nil, fmt.Errorf("azurelib: container name required in pattern %q", pattern)
		}
		container = cleanPattern[:idx]
		subPattern = cleanPattern[idx+1:]
	} else if strings.HasPrefix(cleanPattern, container+"/") {
		subPattern = strings.TrimPrefix(cleanPattern, container+"/")
	}

	prefix := fileslib.PrefixFromGlob(subPattern)
	var listOpts *azblob.ListBlobsFlatOptions
	if prefix != "" {
		listOpts = &azblob.ListBlobsFlatOptions{Prefix: &prefix}
	}

	var matched []string
	pager := b.Client.NewListBlobsFlatPager(container, listOpts)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if page.Segment == nil {
			continue
		}
		for _, item := range page.Segment.BlobItems {
			if item.Name == nil {
				continue
			}
			if fileslib.MatchGlob(subPattern, *item.Name) {
				matched = append(matched, fmt.Sprintf("abfs://%s/%s", container, *item.Name))
			}
		}
	}

	sort.Strings(matched)
	return matched, nil
}

func (b *AzureBlob) WriteIfDifferent(ctx context.Context, filePath string, data []byte) (bool, error) {
	return fileslib.WriteIfDifferentDefault(ctx, b, filePath, data)
}

func (b *AzureBlob) LoadOrStore(ctx context.Context, filePath string, getBytes func() ([]byte, error)) ([]byte, bool, error) {
	return fileslib.LoadOrStoreDefault(ctx, b, filePath, getBytes)
}

func (b *AzureBlob) Transfer(ctx context.Context, to fileslib.FS, pattern, targetDirectory string) error {
	return fileslib.TransferFS(ctx, b, to, pattern, targetDirectory)
}

func (b *AzureBlob) Sync(ctx context.Context, to fileslib.FS, localPath, remotePath string) error {
	return fileslib.SyncFS(ctx, b, to, localPath, remotePath)
}

func (b *AzureBlob) Put(ctx context.Context, localPath, remotePath string, overwrite bool) error {
	if !overwrite {
		if exists, _ := b.Exists(ctx, remotePath); exists {
			return fmt.Errorf("azurelib: remote blob already exists: %s", remotePath)
		}
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	return b.Write(ctx, remotePath, data)
}

func (b *AzureBlob) Close() error {
	if b.backend != nil {
		return b.backend.Close()
	}
	return nil
}
