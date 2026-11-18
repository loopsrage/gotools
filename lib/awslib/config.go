package awslib

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// BuildAWSConfig loads an aws.Config using aws-sdk-go-v2's config chain, applying explicit region,
// static IAM credentials, and custom HTTPClient when provided.
func BuildAWSConfig(ctx context.Context, region, accessKeyID, secretAccessKey, sessionToken string, httpClient *http.Client) (aws.Config, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reg := strings.TrimSpace(region)
	if reg == "" {
		reg = strings.TrimSpace(os.Getenv("AWS_REGION"))
	}
	if reg == "" {
		reg = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if reg == "" {
		return aws.Config{}, errors.New("awslib: AWS region is required (RegionName or AWS_REGION)")
	}

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(reg))

	if strings.TrimSpace(accessKeyID) != "" || strings.TrimSpace(secretAccessKey) != "" {
		if strings.TrimSpace(accessKeyID) == "" || strings.TrimSpace(secretAccessKey) == "" {
			return aws.Config{}, errors.New("awslib: both AwsAccessKeyID and AwsSecretAccessKey must be provided together")
		}
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken),
		))
	}

	if httpClient != nil {
		opts = append(opts, awsconfig.WithHTTPClient(httpClient))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}

	// Verify credentials can be retrieved (fails fast when neither explicit nor ambient credentials exist).
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return aws.Config{}, err
	}
	return cfg, nil
}
