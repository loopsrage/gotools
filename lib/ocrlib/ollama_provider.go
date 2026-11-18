package ocrlib

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/loopsrage/gotools/lib/ollamalib"
	"github.com/ollama/ollama/api"
)

// OllamaProvider executes multimodal vision OCR using gotools/lib/ollamalib.
type OllamaProvider struct {
	client *ollamalib.OllamaClient
	model  string
	prompt string
}

// NewOllamaProvider constructs an OllamaProvider wrapping gotools/lib/ollamalib.
func NewOllamaProvider(ctx context.Context, cfg OllamaProviderConfig) (*OllamaProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, errors.New("ocrlib(ollama): Model is required")
	}
	prompt := strings.TrimSpace(cfg.Prompt)
	if prompt == "" {
		prompt = "Extract all visible text verbatim without commentary."
	}
	cl, err := ollamalib.NewOllamaClient(ctx, cfg.EmbeddingModel, model)
	if err != nil {
		return nil, err
	}
	return &OllamaProvider{
		client: cl,
		model:  model,
		prompt: prompt,
	}, nil
}

func (o *OllamaProvider) Name() string    { return "ollama" }
func (o *OllamaProvider) ModelID() string { return "ollama:" + o.model }
func (o *OllamaProvider) Close() error    { return nil }

func (o *OllamaProvider) Recognize(_ context.Context, req RecognizeRequest) (*RecognizeResult, error) {
	if len(req.ImagePNG) == 0 {
		return nil, errors.New("ocrlib(ollama): empty ImagePNG")
	}
	start := time.Now()
	genReq := o.client.NewRequest(o.prompt, nil, []api.ImageData{api.ImageData(req.ImagePNG)})
	streamFalse := false
	genReq.Stream = &streamFalse

	var out strings.Builder
	err := o.client.Request(genReq, func(_ *api.GenerateRequest) api.GenerateResponseFunc {
		return func(resp api.GenerateResponse) error {
			out.WriteString(resp.Response)
			return nil
		}
	})
	if err != nil {
		return nil, err
	}

	return &RecognizeResult{
		Provider:   o.Name(),
		ModelID:    o.ModelID(),
		Text:       strings.TrimSpace(out.String()),
		Confidence: 1.0,
		LatencyMs:  float64(time.Since(start).Microseconds()) / 1000.0,
	}, nil
}
