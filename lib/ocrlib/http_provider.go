package ocrlib

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPProvider invokes any external HTTP OCR model server (supporting both direct OCR JSON
// schemas and OpenAI-compatible /v1/chat/completions vision endpoints).
type HTTPProvider struct {
	name       string
	endpoint   string
	modelID    string
	apiKey     string
	httpClient *http.Client
}

// NewHTTPProvider constructs a swappable HTTP OCR provider.
func NewHTTPProvider(cfg HTTPProviderConfig) (*HTTPProvider, error) {
	ep := strings.TrimSpace(cfg.Endpoint)
	if ep == "" {
		return nil, errors.New("ocrlib(http): Endpoint is required")
	}
	name := strings.ToLower(strings.TrimSpace(cfg.Name))
	if name == "" {
		name = "http"
	}
	modelID := strings.TrimSpace(cfg.ModelID)
	if modelID == "" {
		modelID = "remote-ocr"
	}
	timeout := 15 * time.Second
	if cfg.TimeoutMs > 0 {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}
	return &HTTPProvider{
		name:       name,
		endpoint:   ep,
		modelID:    modelID,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (h *HTTPProvider) Name() string    { return h.name }
func (h *HTTPProvider) ModelID() string { return h.modelID }
func (h *HTTPProvider) Close() error    { return nil }

func (h *HTTPProvider) Recognize(ctx context.Context, req RecognizeRequest) (*RecognizeResult, error) {
	if len(req.ImagePNG) == 0 {
		return nil, errors.New("ocrlib(http): empty ImagePNG")
	}
	start := time.Now()
	b64 := base64.StdEncoding.EncodeToString(req.ImagePNG)

	var reqBody map[string]any
	isChatCompletions := strings.HasSuffix(strings.TrimRight(h.endpoint, "/"), "/chat/completions")
	if isChatCompletions {
		reqBody = map[string]any{
			"model": h.modelID,
			"messages": []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{"type": "text", "text": "Extract all visible text verbatim without commentary."},
						{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + b64}},
					},
				},
			},
			"temperature": 0,
		}
	} else {
		reqBody = map[string]any{
			"doc_id":       req.DocID,
			"page_index":   req.PageIndex,
			"region_index": req.RegionIndex,
			"model_id":     h.modelID,
			"language":     req.Language,
			"mime_type":    "image/png",
			"image_b64":    b64,
			"bounds": map[string]int{
				"x": req.Bounds.Min.X,
				"y": req.Bounds.Min.Y,
				"w": req.Bounds.Dx(),
				"h": req.Bounds.Dy(),
			},
			"params": req.Params,
		}
	}

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ocrlib(%s): request failed: %w", h.name, err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ocrlib(%s): HTTP %d: %s", h.name, resp.StatusCode, string(respBytes))
	}

	if isChatCompletions {
		var chatResp struct {
			Model   string `json:"model"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(respBytes, &chatResp); err != nil {
			return nil, fmt.Errorf("ocrlib(%s): decode chat response: %w", h.name, err)
		}
		var text string
		if len(chatResp.Choices) > 0 {
			text = strings.TrimSpace(chatResp.Choices[0].Message.Content)
		}
		modelID := h.modelID
		if chatResp.Model != "" {
			modelID = chatResp.Model
		}
		return &RecognizeResult{
			Provider:   h.name,
			ModelID:    modelID,
			Text:       text,
			Confidence: 1.0,
			LatencyMs:  float64(time.Since(start).Microseconds()) / 1000.0,
		}, nil
	}

	var parsed struct {
		ModelID    string         `json:"model_id"`
		Text       string         `json:"text"`
		Confidence float64        `json:"confidence"`
		Tokens     []TokenBox     `json:"tokens"`
		Vector     []float64      `json:"vector"`
		Metadata   map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("ocrlib(%s): decode JSON response: %w", h.name, err)
	}
	modelID := h.modelID
	if parsed.ModelID != "" {
		modelID = parsed.ModelID
	}
	return &RecognizeResult{
		Provider:   h.name,
		ModelID:    modelID,
		Text:       strings.TrimSpace(parsed.Text),
		Confidence: parsed.Confidence,
		Tokens:     parsed.Tokens,
		Vector:     parsed.Vector,
		LatencyMs:  float64(time.Since(start).Microseconds()) / 1000.0,
		Metadata:   parsed.Metadata,
	}, nil
}
