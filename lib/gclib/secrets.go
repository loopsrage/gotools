package gclib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GcpSecretsConfig configures Google Cloud Secret Manager via cloud.google.com/go/secretmanager/apiv1.
type GcpSecretsConfig struct {
	ProjectID       string
	EndpointURL     string
	Token           string
	TokenSource     oauth2.TokenSource
	CredentialsJSON []byte
	HTTPClient      *http.Client
	GRPCConn        *grpc.ClientConn
	ClientOptions   []option.ClientOption
}

// GcpSecrets wraps official cloud.google.com/go/secretmanager/apiv1.Client.
type GcpSecrets struct {
	Config GcpSecretsConfig
	Client *secretmanager.Client
}

// NewGcpSecrets initializes a GcpSecrets client backed by cloud.google.com/go/secretmanager/apiv1.
func NewGcpSecrets(cfg GcpSecretsConfig) (*GcpSecrets, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, errors.New("gclib: ProjectID is required")
	}
	ctx := context.Background()

	// Use NewRESTClient when an HTTP endpoint override or custom HTTPClient is provided,
	// and standard gRPC NewClient otherwise (or when GRPCConn is provided).
	useREST := cfg.GRPCConn == nil && (strings.HasPrefix(cfg.EndpointURL, "http://") || strings.HasPrefix(cfg.EndpointURL, "https://") || cfg.HTTPClient != nil)

	var hc *http.Client
	if useREST {
		hc = cfg.HTTPClient
	}
	opts, err := buildGoogleClientOptions(
		ctx,
		cfg.TokenSource,
		cfg.Token,
		cfg.CredentialsJSON,
		hc,
		cfg.GRPCConn,
		cfg.ClientOptions,
		secretmanager.DefaultAuthScopes()...,
	)
	if err != nil {
		return nil, err
	}

	if ep := strings.TrimSpace(cfg.EndpointURL); ep != "" && cfg.GRPCConn == nil {
		opts = append([]option.ClientOption{option.WithEndpoint(strings.TrimRight(ep, "/"))}, opts...)
	}

	var smClient *secretmanager.Client
	if useREST {
		smClient, err = secretmanager.NewRESTClient(ctx, opts...)
	} else {
		smClient, err = secretmanager.NewClient(ctx, opts...)
	}
	if err != nil {
		return nil, err
	}

	return &GcpSecrets{
		Config: cfg,
		Client: smClient,
	}, nil
}

// SecretPath returns "projects/<project_id>/secrets/<secret_id>".
func (s *GcpSecrets) SecretPath(secretID string) string {
	return fmt.Sprintf("projects/%s/secrets/%s", s.Config.ProjectID, secretID)
}

// GetSecret accesses the latest version of secretID via Client.AccessSecretVersion.
func (s *GcpSecrets) GetSecret(ctx context.Context, secretID string) (string, error) {
	if strings.TrimSpace(secretID) == "" {
		return "", errors.New("gclib: secretID cannot be empty")
	}
	resp, err := s.Client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: s.SecretPath(secretID) + "/versions/latest",
	})
	if err != nil {
		return "", err
	}
	return string(resp.GetPayload().GetData()), nil
}

// PutSecret creates secretID (if absent) with automatic replication and appends a new secret version.
func (s *GcpSecrets) PutSecret(ctx context.Context, secretID, secretString string) (map[string]string, error) {
	if strings.TrimSpace(secretID) == "" {
		return nil, errors.New("gclib: secretID cannot be empty")
	}
	parent := fmt.Sprintf("projects/%s", s.Config.ProjectID)
	secretName := s.SecretPath(secretID)

	created, err := s.Client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   parent,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	})
	if err == nil && created.GetName() != "" {
		secretName = created.GetName()
	} else if err != nil && status.Code(err) != codes.AlreadyExists && !strings.Contains(err.Error(), "409") {
		return nil, err
	}

	_, err = s.Client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: secretName,
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(secretString),
		},
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"name": secretName}, nil
}

// ListSecrets lists secrets in Config.ProjectID up to maxResults via Client.ListSecrets.
func (s *GcpSecrets) ListSecrets(ctx context.Context, maxResults int) ([]map[string]interface{}, error) {
	if maxResults <= 0 {
		maxResults = 100
	}
	parent := fmt.Sprintf("projects/%s", s.Config.ProjectID)
	it := s.Client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{
		Parent:   parent,
		PageSize: int32(maxResults),
	})

	var results []map[string]interface{}
	for len(results) < maxResults {
		sec, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"name": sec.GetName(),
		})
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	return results, nil
}

// Close closes the underlying Secret Manager client.
func (s *GcpSecrets) Close() error {
	if s.Client != nil {
		return s.Client.Close()
	}
	return nil
}
