package azurelib

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// TokenCredential aliases azcore.TokenCredential from the official Azure SDK for Go.
type TokenCredential = azcore.TokenCredential

type staticAzCredential struct {
	token string
}

// StaticTokenCredential returns an azcore.TokenCredential backed by a fixed bearer token.
func StaticTokenCredential(token string) azcore.TokenCredential {
	return &staticAzCredential{token: token}
}

func (s *staticAzCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if s.token == "" {
		return azcore.AccessToken{}, errors.New("azurelib: static bearer token is empty")
	}
	return azcore.AccessToken{
		Token:     s.token,
		ExpiresOn: time.Now().UTC().Add(time.Hour),
	}, nil
}

// ResolveAzureCredential resolves an explicit azcore.TokenCredential, AZURE_ACCESS_TOKEN,
// or azidentity.NewDefaultAzureCredential.
func ResolveAzureCredential(explicit azcore.TokenCredential) (azcore.TokenCredential, error) {
	if explicit != nil {
		return explicit, nil
	}
	if envTok := strings.TrimSpace(os.Getenv("AZURE_ACCESS_TOKEN")); envTok != "" {
		return StaticTokenCredential(envTok), nil
	}
	return azidentity.NewDefaultAzureCredential(nil)
}
