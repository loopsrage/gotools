package ocrlib_test

import (
	"context"
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loopsrage/gotools/lib/ocrlib"
	"github.com/loopsrage/gotools/lib/visionlib"
)

func TestSwappableOCRProvidersAndTesseractRecognition(t *testing.T) {
	ctx := context.Background()

	pdfBytes := visionlib.BuildMultiPagePDF([]string{"SOVEREIGN 2026"})
	renderer := visionlib.NewPDFRenderer(1)
	defer renderer.Close()
	rp, err := renderer.RenderPage(pdfBytes, 0, 200)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}

	proc := visionlib.NewProcessor()
	_, boxes, err := proc.SegmentRegions(rp.PNGBytes, 200, -1)
	if err != nil || len(boxes) == 0 {
		t.Fatalf("SegmentRegions: %v", err)
	}
	_, denoisedPNG, err := proc.CropAndDenoise(rp.PNGBytes, boxes[0], visionlib.PresetWordDenoise)
	if err != nil {
		t.Fatalf("CropAndDenoise: %v", err)
	}

	reg := ocrlib.NewRegistry()

	tessProv, err := ocrlib.NewTesseractCLIProvider(ocrlib.TesseractProviderConfig{
		Enabled:  true,
		Language: "eng",
		PSM:      6,
	})
	if err != nil {
		t.Fatalf("NewTesseractCLIProvider: %v", err)
	}
	if err := reg.Register(tessProv); err != nil {
		t.Fatalf("Register tesseract: %v", err)
	}

	tessRes, err := tessProv.Recognize(ctx, ocrlib.RecognizeRequest{
		DocID:       "test-doc",
		PageIndex:   0,
		RegionIndex: 0,
		ImagePNG:    denoisedPNG,
		Bounds:      boxes[0],
	})
	if err != nil {
		t.Fatalf("tesseract Recognize: %v", err)
	}
	if !strings.Contains(strings.ToUpper(tessRes.Text), "SOVEREIGN") || !strings.Contains(tessRes.Text, "2026") {
		t.Fatalf("tesseract text = %q, expected SOVEREIGN 2026", tessRes.Text)
	}
	if tessRes.Confidence <= 0.5 {
		t.Fatalf("tesseract confidence = %f, expected > 0.5", tessRes.Confidence)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body["image_b64"] == "" {
			http.Error(w, "missing image_b64", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model_id":   "trocr-large-printed",
			"text":       "SOVEREIGN 2026",
			"confidence": 0.99,
			"tokens": []ocrlib.TokenBox{
				{Text: "SOVEREIGN", Confidence: 0.99, Box: image.Rect(0, 0, 120, 40)},
				{Text: "2026", Confidence: 0.99, Box: image.Rect(130, 0, 200, 40)},
			},
		})
	}))
	defer srv.Close()

	httpProv, err := ocrlib.NewHTTPProvider(ocrlib.HTTPProviderConfig{
		Enabled:  true,
		Name:     "trocr_http",
		Endpoint: srv.URL,
		ModelID:  "trocr-large-printed",
	})
	if err != nil {
		t.Fatalf("NewHTTPProvider: %v", err)
	}
	_ = reg.Register(httpProv)

	if err := reg.SetDefault("trocr_http"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	resolved, err := reg.ResolveProviders(nil)
	if err != nil || len(resolved) != 1 || resolved[0].Name() != "trocr_http" {
		t.Fatalf("ResolveProviders(nil) = %v, %v", resolved, err)
	}

	httpRes, err := resolved[0].Recognize(ctx, ocrlib.RecognizeRequest{
		DocID:    "test-doc",
		ImagePNG: denoisedPNG,
		Bounds:   boxes[0],
	})
	if err != nil || httpRes.Text != "SOVEREIGN 2026" || httpRes.ModelID != "trocr-large-printed" {
		t.Fatalf("httpProv Recognize = %+v, err=%v", httpRes, err)
	}

	mlProv, err := ocrlib.NewMLToolsProvider(ocrlib.MLToolsProviderConfig{
		Enabled:    true,
		NumClasses: 95,
		BeamSize:   4,
		Seed:       42,
	})
	if err != nil {
		t.Fatalf("NewMLToolsProvider: %v", err)
	}
	_ = reg.Register(mlProv)
	mlRes, err := mlProv.Recognize(ctx, ocrlib.RecognizeRequest{
		DocID:    "test-doc",
		ImagePNG: denoisedPNG,
		Bounds:   boxes[0],
	})
	if err != nil || mlRes.Provider != "mltools" {
		t.Fatalf("mlProv Recognize = %+v, err=%v", mlRes, err)
	}
}
