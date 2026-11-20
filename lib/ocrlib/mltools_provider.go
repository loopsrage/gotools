package ocrlib

import (
	"context"
	"errors"

	"github.com/loopsrage/go_mltools/harnesses"
	"github.com/loopsrage/go_mltools/visionembed"
)

// MLToolsProvider adapts go_mltools/visionembed.SequenceRecognizer (or any harnesses.Inferrer)
// into a swappable OCR Provider.
type MLToolsProvider struct {
	rec *visionembed.SequenceRecognizer
}

// NewMLToolsProvider constructs the built-in go_mltools UnifiedCompositeOCR provider.
func NewMLToolsProvider(cfg MLToolsProviderConfig) (*MLToolsProvider, error) {
	rec, err := visionembed.NewSequenceRecognizer(visionembed.SequenceConfig{
		NumClasses: cfg.NumClasses,
		BeamSize:   cfg.BeamSize,
		Seed:       cfg.Seed,
		HeadType:   cfg.HeadType,
	})
	if err != nil {
		return nil, err
	}
	return &MLToolsProvider{rec: rec}, nil
}

// NewHarnessOCRProvider wraps any custom go_mltools harnesses.Inferrer into an ocrlib.Provider.
func NewHarnessOCRProvider(name, modelID string, inferrer harnesses.Inferrer, vocab []rune) (*MLToolsProvider, error) {
	rec, err := visionembed.NewHarnessSequenceRecognizer(name, modelID, inferrer, vocab)
	if err != nil {
		return nil, err
	}
	return &MLToolsProvider{rec: rec}, nil
}

func (m *MLToolsProvider) Name() string    { return m.rec.Name() }
func (m *MLToolsProvider) ModelID() string { return m.rec.ModelID() }
func (m *MLToolsProvider) Close() error    { return nil }

func (m *MLToolsProvider) Recognize(_ context.Context, req RecognizeRequest) (*RecognizeResult, error) {
	imgPNG := req.ImagePNG
	if len(req.RawImagePNG) > 0 && visionembed.IsCleanRenderPNG(req.RawImagePNG) {
		imgPNG = req.RawImagePNG
	}
	if len(imgPNG) == 0 {
		return nil, errors.New("ocrlib(mltools): empty ImagePNG")
	}
	res, err := m.rec.RecognizePNG(imgPNG)
	if err != nil {
		return nil, err
	}
	return &RecognizeResult{
		Provider:   res.Name,
		ModelID:    res.ModelID,
		Text:       res.Text,
		Confidence: res.Confidence,
		LatencyMs:  res.LatencyMs,
		Metadata:   res.Metadata,
	}, nil
}
