package awslib

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// AwsAiConfig configures AWS Bedrock Runtime and Bedrock control-plane clients via aws-sdk-go-v2.
type AwsAiConfig struct {
	RegionName              string
	AwsAccessKeyID          string
	AwsSecretAccessKey      string
	AwsSessionToken         string
	EndpointURL             string
	ControlPlaneEndpointURL string
	HTTPClient              *http.Client
}

// AwsAi wraps official aws-sdk-go-v2 bedrockruntime.Client and bedrock.Client.
type AwsAi struct {
	Config        AwsAiConfig
	RuntimeClient *bedrockruntime.Client
	BedrockClient *bedrock.Client
}

// NewAwsAi constructs an AwsAi client backed by aws-sdk-go-v2 Bedrock & BedrockRuntime SDKs.
func NewAwsAi(cfg AwsAiConfig) (*AwsAi, error) {
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

	rtClient := bedrockruntime.NewFromConfig(awsCfg, func(o *bedrockruntime.Options) {
		if ep := strings.TrimSpace(cfg.EndpointURL); ep != "" {
			o.BaseEndpoint = aws.String(ep)
		}
	})

	ctrlEndpoint := strings.TrimSpace(cfg.ControlPlaneEndpointURL)
	if ctrlEndpoint == "" {
		ctrlEndpoint = strings.TrimSpace(cfg.EndpointURL)
	}
	ctrlClient := bedrock.NewFromConfig(awsCfg, func(o *bedrock.Options) {
		if ctrlEndpoint != "" {
			o.BaseEndpoint = aws.String(ctrlEndpoint)
		}
	})

	return &AwsAi{
		Config:        cfg,
		RuntimeClient: rtClient,
		BedrockClient: ctrlClient,
	}, nil
}

// InvokeModel invokes a Bedrock model via bedrockruntime.Client.InvokeModel.
func (a *AwsAi) InvokeModel(ctx context.Context, modelID, prompt string, maxTokens int, temperature float64, extra map[string]interface{}) (string, error) {
	if strings.TrimSpace(modelID) == "" {
		return "", errors.New("awslib: modelID cannot be empty")
	}
	if maxTokens <= 0 {
		maxTokens = 512
	}
	bodyMap := map[string]interface{}{
		"prompt":               prompt,
		"max_tokens_to_sample": maxTokens,
		"temperature":          temperature,
	}
	for k, v := range extra {
		bodyMap[k] = v
	}
	payload, err := json.Marshal(bodyMap)
	if err != nil {
		return "", err
	}

	out, err := a.RuntimeClient.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		Body:        payload,
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", err
	}
	return string(out.Body), nil
}

// ListModels returns foundation model summaries via bedrock.Client.ListFoundationModels.
func (a *AwsAi) ListModels(ctx context.Context) ([]map[string]interface{}, error) {
	out, err := a.BedrockClient.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, err
	}
	summaries := make([]map[string]interface{}, 0, len(out.ModelSummaries))
	for _, s := range out.ModelSummaries {
		m := make(map[string]interface{})
		if s.ModelId != nil {
			m["modelId"] = *s.ModelId
		}
		if s.ModelName != nil {
			m["modelName"] = *s.ModelName
		}
		if s.ProviderName != nil {
			m["providerName"] = *s.ProviderName
		}
		if s.ModelArn != nil {
			m["modelArn"] = *s.ModelArn
		}
		summaries = append(summaries, m)
	}
	return summaries, nil
}
