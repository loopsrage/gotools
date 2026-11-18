package awslib

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// AwsSecretsConfig configures AWS Secrets Manager via aws-sdk-go-v2 service/secretsmanager.
type AwsSecretsConfig struct {
	RegionName         string
	AwsAccessKeyID     string
	AwsSecretAccessKey string
	AwsSessionToken    string
	EndpointURL        string
	HTTPClient         *http.Client
}

// AwsSecrets wraps official aws-sdk-go-v2 secretsmanager.Client.
type AwsSecrets struct {
	Config AwsSecretsConfig
	Client *secretsmanager.Client
}

// NewAwsSecrets constructs an AwsSecrets client backed by aws-sdk-go-v2 secretsmanager.Client.
func NewAwsSecrets(cfg AwsSecretsConfig) (*AwsSecrets, error) {
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

	smClient := secretsmanager.NewFromConfig(awsCfg, func(o *secretsmanager.Options) {
		if ep := strings.TrimSpace(cfg.EndpointURL); ep != "" {
			o.BaseEndpoint = aws.String(ep)
		}
	})

	return &AwsSecrets{
		Config: cfg,
		Client: smClient,
	}, nil
}

// GetSecret fetches SecretString for secretID via secretsmanager.Client.GetSecretValue.
func (s *AwsSecrets) GetSecret(ctx context.Context, secretID string) (string, error) {
	if strings.TrimSpace(secretID) == "" {
		return "", errors.New("awslib: secretID cannot be empty")
	}
	out, err := s.Client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return "", err
	}
	if out.SecretString == nil {
		return "", nil
	}
	return *out.SecretString, nil
}

// PutSecret updates secretID via secretsmanager.Client.PutSecretValue and returns metadata map.
func (s *AwsSecrets) PutSecret(ctx context.Context, secretID, secretString string) (map[string]interface{}, error) {
	if strings.TrimSpace(secretID) == "" {
		return nil, errors.New("awslib: secretID cannot be empty")
	}
	out, err := s.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretID),
		SecretString: aws.String(secretString),
	})
	if err != nil {
		return nil, err
	}
	res := make(map[string]interface{})
	if out.ARN != nil {
		res["ARN"] = *out.ARN
	}
	if out.Name != nil {
		res["Name"] = *out.Name
	}
	if out.VersionId != nil {
		res["VersionId"] = *out.VersionId
	}
	return res, nil
}

// ListSecrets lists secrets up to maxResults via secretsmanager.Client.ListSecrets.
func (s *AwsSecrets) ListSecrets(ctx context.Context, maxResults int) ([]map[string]interface{}, error) {
	if maxResults <= 0 {
		maxResults = 100
	}
	out, err := s.Client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{
		MaxResults: aws.Int32(int32(maxResults)),
	})
	if err != nil {
		return nil, err
	}
	list := make([]map[string]interface{}, 0, len(out.SecretList))
	for _, entry := range out.SecretList {
		m := make(map[string]interface{})
		if entry.Name != nil {
			m["Name"] = *entry.Name
		}
		if entry.ARN != nil {
			m["ARN"] = *entry.ARN
		}
		if entry.Description != nil {
			m["Description"] = *entry.Description
		}
		list = append(list, m)
	}
	return list, nil
}
