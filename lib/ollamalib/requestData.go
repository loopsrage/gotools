package ollamalib

import (
	"context"
	"github.com/loopsrage/gotools/lib/concurr"
	"strings"
	"sync"

	"github.com/ollama/ollama/api"
)

type RequestBlock struct {
	Request  *api.GenerateRequest
	Response []*api.GenerateResponse
	Index    int64
}
type RequestData struct {
	ctx context.Context

	*sync.RWMutex
	ollamaContext *concurr.TsList
	*concurr.TsList

	respInd *sync.Map
}

func (r *RequestData) Context() (out []int) {
	all := r.ollamaContext.All()
	out = make([]int, len(all))
	for i, v := range all {
		switch v := v.(type) {
		case int:
			out[i] = v
		}
	}
	return out
}

func (r *RequestData) Requests() (out []*RequestBlock) {
	all := r.All()
	out = make([]*RequestBlock, len(all))
	for i, v := range all {
		switch v := v.(type) {
		case *RequestBlock:
			out[i] = v
		}
	}
	return out
}

func (r *RequestData) PrefixContext(data []int) {
	var newCtx []interface{}
	for _, v := range data {
		newCtx = append(newCtx, v)
	}
	newCtx = append(newCtx, r.ollamaContext.All()...)

	nl := concurr.NewTsList()
	nl.Add(newCtx...)

	r.Lock()
	r.ollamaContext = nl
	r.Unlock()
}

func (r *RequestData) LoadRequestBlock(request *api.GenerateRequest) *RequestBlock {
	x, _ := r.respInd.Load(request)
	switch v := x.(type) {
	case *RequestBlock:
		return v
	default:
		rb := &RequestBlock{
			Request: request,
		}
		ind := r.Add(rb)
		rb.Index = ind
		r.respInd.Store(request, rb)
		return rb
	}
}

func (r *RequestData) HandleRequestResponse(request *api.GenerateRequest) api.GenerateResponseFunc {
	return func(response api.GenerateResponse) error {
		err := concurr.CheckContext(r.ctx)
		if err != nil {
			return err
		}

		rb := r.LoadRequestBlock(request)
		rb.Response = append(rb.Response, &response)

		opc := make([]interface{}, len(response.Context))
		for i := 0; i < len(opc); i++ {
			opc[i] = response.Context[i]
		}
		r.ollamaContext.Add(opc...)
		return nil
	}
}

func (r *RequestData) Response() string {
	out := r.Requests()
	op := strings.Builder{}
	for _, v := range out {
		bl := strings.Builder{}
		for _, v2 := range v.Response {
			bl.WriteString(v2.Response)
		}
		op.WriteString(bl.String() + "\n")
	}
	return op.String()
}

func NewRequestData(ctx context.Context) *RequestData {
	return &RequestData{
		ctx:           ctx,
		ollamaContext: concurr.NewTsList(),
		TsList:        concurr.NewTsList(),
		respInd:       &sync.Map{},
	}
}

func DoEmbeddingsRequest(client *OllamaClient, prompt string) ([]float64, error) {
	return client.Embeddings(&api.EmbeddingRequest{
		Model:  client.EmbeddingModel(),
		Prompt: prompt,
	})
}

func DoRequest(client *OllamaClient, data *RequestData, prompt string, images ...api.ImageData) error {
	req := client.NewRequest(prompt, data.Context(), images)
	return client.Request(req, data.HandleRequestResponse)
}
