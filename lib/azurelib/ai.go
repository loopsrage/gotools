package azurelib

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"
)

const defaultAzureOpenAIAPIVersion = "2024-02-01"

// AzureAiConfig configures Azure OpenAI endpoint, authentication, API version, and default deployment.
type AzureAiConfig struct {
	AzureEndpoint   string
	APIKey          string
	APIVersion      string
	AzureDeployment string
	Credential      azcore.TokenCredential
	HTTPClient      *http.Client
}

// ChatMessage represents a single role/content turn in an Azure OpenAI chat completion request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AzureAi wraps the official openai-go/v3 Client configured with openai-go/v3/azure options.
type AzureAi struct {
	Config AzureAiConfig
	Client openai.Client
}

// NewAzureAi validates configuration and constructs an AzureAi client backed by openai-go/v3/azure.
func NewAzureAi(cfg AzureAiConfig) (*AzureAi, error) {
	if strings.TrimSpace(cfg.AzureEndpoint) == "" {
		return nil, errors.New("azurelib: AzureEndpoint is required")
	}
	ver := strings.TrimSpace(cfg.APIVersion)
	if ver == "" {
		ver = defaultAzureOpenAIAPIVersion
	}

	opts := []option.RequestOption{
		azure.WithEndpoint(strings.TrimRight(cfg.AzureEndpoint, "/"), ver),
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		opts = append(opts, azure.WithAPIKey(cfg.APIKey))
	} else if cfg.Credential != nil {
		opts = append(opts, azure.WithTokenCredential(cfg.Credential))
	} else {
		cred, err := ResolveAzureCredential(nil)
		if err != nil {
			return nil, err
		}
		opts = append(opts, azure.WithTokenCredential(cred))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}

	client := openai.NewClient(opts...)
	return &AzureAi{
		Config: cfg,
		Client: client,
	}, nil
}

// Chat executes a chat completion request against model (or Config.AzureDeployment when model is empty).
func (a *AzureAi) Chat(ctx context.Context, messages []ChatMessage, model string, temperature float64, maxTokens int) (string, error) {
	deployment := strings.TrimSpace(model)
	if deployment == "" {
		deployment = strings.TrimSpace(a.Config.AzureDeployment)
	}
	if deployment == "" {
		return "", errors.New("azurelib: model or AzureDeployment must be specified")
	}
	if maxTokens <= 0 {
		maxTokens = 512
	}

	msgParams := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch strings.ToLower(m.Role) {
		case "system":
			msgParams = append(msgParams, openai.SystemMessage(m.Content))
		case "assistant":
			msgParams = append(msgParams, openai.AssistantMessage(m.Content))
		default:
			msgParams = append(msgParams, openai.UserMessage(m.Content))
		}
	}

	resp, err := a.Client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(deployment),
		Messages:    msgParams,
		Temperature: openai.Float(temperature),
		MaxTokens:   openai.Int(int64(maxTokens)),
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("azurelib: Azure OpenAI returned empty choices")
	}
	return resp.Choices[0].Message.Content, nil
}

// Embed computes an embedding vector for inputText using model (or Config.AzureDeployment when empty).
func (a *AzureAi) Embed(ctx context.Context, inputText, model string) ([]float64, error) {
	deployment := strings.TrimSpace(model)
	if deployment == "" {
		deployment = strings.TrimSpace(a.Config.AzureDeployment)
	}
	if deployment == "" {
		return nil, errors.New("azurelib: model or AzureDeployment must be specified")
	}

	resp, err := a.Client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(deployment),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(inputText),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("azurelib: Azure OpenAI returned empty embedding data")
	}
	return resp.Data[0].Embedding, nil
}
