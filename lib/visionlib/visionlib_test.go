package visionlib_test

import (
	"strings"
	"testing"

	"github.com/loopsrage/gotools/lib/visionlib"
)

func TestProcessorAndPDFRenderer(t *testing.T) {
	pngBytes, err := visionlib.RenderBannerPNG([]string{"PHASE 2026"}, 6)
	if err != nil {
		t.Fatalf("RenderBannerPNG: %v", err)
	}

	kind, mime, err := visionlib.DetectInputKind(pngBytes, "")
	if err != nil || kind != visionlib.InputImage || mime != "image/png" {
		t.Fatalf("DetectInputKind(png) = (%q, %q, %v)", kind, mime, err)
	}

	proc := visionlib.NewProcessor()
	prepPNG, boxes, err := proc.SegmentRegions(pngBytes, 200, -1)
	if err != nil || len(prepPNG) == 0 || len(boxes) == 0 {
		t.Fatalf("SegmentRegions: boxes=%d err=%v", len(boxes), err)
	}

	cropRaw, cropDen, err := proc.CropAndDenoise(pngBytes, boxes[0], visionlib.PresetWordDenoise)
	if err != nil || len(cropRaw) == 0 || len(cropDen) == 0 {
		t.Fatalf("CropAndDenoise: raw=%d den=%d err=%v", len(cropRaw), len(cropDen), err)
	}

	pdfBytes := visionlib.BuildMultiPagePDF([]string{"ALPHA PAGE ONE", "BETA PAGE TWO"})
	pKind, _, err := visionlib.DetectInputKind(pdfBytes, "")
	if err != nil || pKind != visionlib.InputPDF {
		t.Fatalf("DetectInputKind(pdf) = (%q, %v)", pKind, err)
	}

	renderer := visionlib.NewPDFRenderer(2)
	defer renderer.Close()

	pc, err := renderer.PageCount(pdfBytes)
	if err != nil || pc != 2 {
		t.Fatalf("PageCount = (%d, %v), want 2", pc, err)
	}

	rp0, err := renderer.RenderPage(pdfBytes, 0, 150)
	if err != nil {
		t.Fatalf("RenderPage(0): %v", err)
	}
	if rp0.Width <= 0 || rp0.Height <= 0 || len(rp0.PNGBytes) == 0 {
		t.Fatalf("unexpected rendered page 0: %dx%d bytes=%d", rp0.Width, rp0.Height, len(rp0.PNGBytes))
	}
	if !strings.Contains(rp0.NativeText, "ALPHA PAGE ONE") {
		t.Fatalf("NativeText = %q, want substring ALPHA PAGE ONE", rp0.NativeText)
	}
}
