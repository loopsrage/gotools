package azurelib

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// AzureSecretsConfig configures Azure Key Vault Secrets via github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets.
type AzureSecretsConfig struct {
	VaultURL   string
	Credential azcore.TokenCredential
	HTTPClient *http.Client
}

// SecretProperties represents an Azure Key Vault secret item metadata entry.
type SecretProperties struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Enabled bool   `json:"enabled"`
}

// AzureSecrets wraps official azsecrets.Client.
type AzureSecrets struct {
	Config AzureSecretsConfig
	Client *azsecrets.Client
}

// NewAzureSecrets validates config and constructs an AzureSecrets client backed by azsecrets.Client.
func NewAzureSecrets(cfg AzureSecretsConfig) (*AzureSecrets, error) {
	if strings.TrimSpace(cfg.VaultURL) == "" {
		return nil, errors.New("azurelib: VaultURL is required")
	}
	cred, err := ResolveAzureCredential(cfg.Credential)
	if err != nil {
		return nil, err
	}

	opts := &azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			InsecureAllowCredentialWithHTTP: strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.VaultURL)), "http://"),
		},
		DisableChallengeResourceVerification: !strings.Contains(strings.ToLower(cfg.VaultURL), ".vault.azure.net"),
	}
	if cfg.HTTPClient != nil {
		opts.Transport = cfg.HTTPClient
	}

	client, err := azsecrets.NewClient(cfg.VaultURL, cred, opts)
	if err != nil {
		return nil, err
	}
	return &AzureSecrets{
		Config: cfg,
		Client: client,
	}, nil
}

// GetSecret fetches the latest version of secretID via azsecrets.Client.GetSecret.
func (s *AzureSecrets) GetSecret(ctx context.Context, secretID string) (string, error) {
	if strings.TrimSpace(secretID) == "" {
		return "", errors.New("azurelib: secretID cannot be empty")
	}
	resp, err := s.Client.GetSecret(ctx, secretID, "", nil)
	if err != nil {
		return "", err
	}
	if resp.Value == nil {
		return "", nil
	}
	return *resp.Value, nil
}

// PutSecret sets secretID to secretString via azsecrets.Client.SetSecret and returns {"name": ..., "version": ...}.
func (s *AzureSecrets) PutSecret(ctx context.Context, secretID, secretString string) (map[string]string, error) {
	if strings.TrimSpace(secretID) == "" {
		return nil, errors.New("azurelib: secretID cannot be empty")
	}
	resp, err := s.Client.SetSecret(ctx, secretID, azsecrets.SetSecretParameters{
		Value: &secretString,
	}, nil)
	if err != nil {
		return nil, err
	}
	name := secretID
	version := ""
	if resp.ID != nil {
		if resp.ID.Name() != "" {
			name = resp.ID.Name()
		}
		version = resp.ID.Version()
	}
	return map[string]string{
		"name":    name,
		"version": version,
	}, nil
}

// ListSecrets lists secret properties up to maxResults via azsecrets.Client.NewListSecretPropertiesPager.
func (s *AzureSecrets) ListSecrets(ctx context.Context, maxResults int) ([]SecretProperties, error) {
	if maxResults <= 0 {
		maxResults = 100
	}
	pager := s.Client.NewListSecretPropertiesPager(nil)
	var results []SecretProperties

	for pager.More() && len(results) < maxResults {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Value {
			if item == nil {
				continue
			}
			prop := SecretProperties{Enabled: true}
			if item.ID != nil {
				prop.ID = string(*item.ID)
				prop.Name = item.ID.Name()
				prop.Version = item.ID.Version()
			}
			if item.Attributes != nil && item.Attributes.Enabled != nil {
				prop.Enabled = *item.Attributes.Enabled
			}
			results = append(results, prop)
			if len(results) >= maxResults {
				break
			}
		}
	}

	if results == nil {
		results = []SecretProperties{}
	}
	return results, nil
}
