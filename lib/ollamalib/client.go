package ollamalib

import (
	"context"
	"sync/atomic"

	"github.com/ollama/ollama/api"
)

type OllamaClient struct {
	ctx            context.Context
	client         *api.Client
	model          *atomic.Value
	embeddingModel *atomic.Value
}

func (cl *OllamaClient) Model() string {
	x, _ := cl.model.Load().(string)
	return x
}

func (cl *OllamaClient) EmbeddingModel() string {
	x, _ := cl.embeddingModel.Load().(string)
	return x
}

type GenerateResponseWrapper func(request *api.GenerateRequest) api.GenerateResponseFunc

func (cl *OllamaClient) Request(request *api.GenerateRequest, handle GenerateResponseWrapper) (err error) {
	if request.Model == "" {
		request.Model = cl.Model()
	}

	return cl.client.Generate(cl.ctx, request, handle(request))
}

func (cl *OllamaClient) Embeddings(request *api.EmbeddingRequest) ([]float64, error) {
	rexp, err := cl.client.Embeddings(cl.ctx, request)
	if err != nil {
		return nil, err
	}
	return rexp.Embedding, nil
}

func (cl *OllamaClient) NewRequest(prompt string, context []int, images []api.ImageData) *api.GenerateRequest {
	return &api.GenerateRequest{
		Model:   cl.Model(),
		Prompt:  prompt,
		Context: context,
		Images:  images,
	}
}

func NewOllamaClient(ctx context.Context, embeddingModel, model string) (*OllamaClient, error) {
	m := &atomic.Value{}
	m.Store(model)

	em := &atomic.Value{}
	em.Store(embeddingModel)

	ll := &OllamaClient{
		ctx:            ctx,
		model:          m,
		embeddingModel: em,
	}

	var err error
	ll.client, err = api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	return ll, nil
}
