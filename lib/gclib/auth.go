package gclib

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

const defaultCloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// TokenSource aliases oauth2.TokenSource from golang.org/x/oauth2.
type TokenSource = oauth2.TokenSource

// StaticTokenSource returns an oauth2.TokenSource yielding a fixed bearer access token.
func StaticTokenSource(token string) oauth2.TokenSource {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
}

// TokenSourceFromJSON parses a Google service_account or authorized_user JSON credential payload
// using golang.org/x/oauth2/google.CredentialsFromJSON.
func TokenSourceFromJSON(ctx context.Context, raw []byte, httpClient *http.Client, scopes ...string) (oauth2.TokenSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}
	if len(scopes) == 0 {
		scopes = []string{defaultCloudPlatformScope}
	}
	creds, err := google.CredentialsFromJSON(ctx, raw, scopes...)
	if err != nil {
		return nil, err
	}
	return creds.TokenSource, nil
}

// ResolveGoogleTokenSource resolves explicit TokenSource/Token, CredentialsJSON, GOOGLE_OAUTH_ACCESS_TOKEN,
// or Application Default Credentials via google.FindDefaultCredentials.
func ResolveGoogleTokenSource(
	ctx context.Context,
	explicit oauth2.TokenSource,
	explicitToken string,
	credentialsJSON []byte,
	httpClient *http.Client,
	scopes ...string,
) (oauth2.TokenSource, error) {
	if explicit != nil {
		return explicit, nil
	}
	if strings.TrimSpace(explicitToken) != "" {
		return StaticTokenSource(explicitToken), nil
	}
	if envTok := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN")); envTok != "" {
		return StaticTokenSource(envTok), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}
	if len(scopes) == 0 {
		scopes = []string{defaultCloudPlatformScope}
	}
	if len(credentialsJSON) > 0 {
		return TokenSourceFromJSON(ctx, credentialsJSON, httpClient, scopes...)
	}
	creds, err := google.FindDefaultCredentials(ctx, scopes...)
	if err != nil {
		return nil, err
	}
	if creds == nil || creds.TokenSource == nil {
		return nil, errors.New("gclib: Google Application Default Credentials returned nil TokenSource")
	}
	return creds.TokenSource, nil
}

func buildGoogleClientOptions(
	ctx context.Context,
	ts oauth2.TokenSource,
	token string,
	credsJSON []byte,
	httpClient *http.Client,
	grpcConn *grpc.ClientConn,
	extraOpts []option.ClientOption,
	scopes ...string,
) ([]option.ClientOption, error) {
	var opts []option.ClientOption
	if grpcConn != nil {
		opts = append(opts, option.WithGRPCConn(grpcConn))
		if ts != nil {
			opts = append(opts, option.WithTokenSource(ts))
		} else if token != "" {
			opts = append(opts, option.WithTokenSource(StaticTokenSource(token)))
		}
		opts = append(opts, extraOpts...)
		return opts, nil
	}

	resolvedTS, err := ResolveGoogleTokenSource(ctx, ts, token, credsJSON, httpClient, scopes...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, option.WithTokenSource(resolvedTS))
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	opts = append(opts, extraOpts...)
	return opts, nil
}
