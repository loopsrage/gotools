//go:build gosseract

package ocrlib

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/loopsrage/gotools/lib/gosseractlib"
	"github.com/otiai10/gosseract/v2"
)

// GosseractProvider wraps a bounded pool of CGo gosseract.Client instances.
type GosseractProvider struct {
	language string
	pool     chan *gosseract.Client
}

// NewGosseractProvider creates a pooled CGo GosseractProvider when compiled with -tags gosseract.
func NewGosseractProvider(tessDataPath, language string, poolSize int) (*GosseractProvider, error) {
	if poolSize <= 0 {
		poolSize = 4
	}
	if language == "" {
		language = "eng"
	}
	ch := make(chan *gosseract.Client, poolSize)
	for i := 0; i < poolSize; i++ {
		cl, err := gosseractlib.GosseractClient(tessDataPath, language)
		if err != nil {
			for len(ch) > 0 {
				c := <-ch
				_ = c.Close()
			}
			return nil, err
		}
		ch <- cl
	}
	return &GosseractProvider{language: language, pool: ch}, nil
}

func (g *GosseractProvider) Name() string    { return "gosseract" }
func (g *GosseractProvider) ModelID() string { return "gosseract-cgo:" + g.language }

func (g *GosseractProvider) Recognize(ctx context.Context, req RecognizeRequest) (*RecognizeResult, error) {
	if len(req.ImagePNG) == 0 {
		return nil, errors.New("ocrlib(gosseract): empty ImagePNG")
	}
	start := time.Now()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case cl := <-g.pool:
		defer func() { g.pool <- cl }()
		if err := cl.SetImageFromBytes(req.ImagePNG); err != nil {
			return nil, err
		}
		txt, err := cl.Text()
		if err != nil {
			return nil, err
		}
		return &RecognizeResult{
			Provider:   g.Name(),
			ModelID:    g.ModelID(),
			Text:       strings.TrimSpace(txt),
			Confidence: 1.0,
			LatencyMs:  float64(time.Since(start).Microseconds()) / 1000.0,
		}, nil
	}
}

func (g *GosseractProvider) Close() error {
	var firstErr error
	close(g.pool)
	for cl := range g.pool {
		if err := cl.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
