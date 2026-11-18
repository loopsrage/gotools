// Package visionlib provides pure-Go PDFium WebAssembly rendering, input format detection,
// contour/region segmentation, adaptive denoising, and deterministic synthetic PDF/PNG fixtures.
package visionlib

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"
	"net/http"
	"sort"
	"strings"
)

// DenoisePreset selects the preprocessing filter chain applied prior to OCR.
type DenoisePreset string

const (
	PresetWordDenoise   DenoisePreset = "word_denoise"
	PresetWordIsolation DenoisePreset = "word_isolation"
	PresetPassthrough   DenoisePreset = "passthrough"
)

// ParseDenoisePreset normalizes a preset name, defaulting to PresetWordDenoise.
func ParseDenoisePreset(raw string) DenoisePreset {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "word_isolation", "isolation":
		return PresetWordIsolation
	case "passthrough", "none", "raw":
		return PresetPassthrough
	default:
		return PresetWordDenoise
	}
}

// InputKind classifies incoming document bytes.
type InputKind string

const (
	InputPDF   InputKind = "pdf"
	InputImage InputKind = "image"
)

// DetectInputKind inspects magic bytes and MIME headers to distinguish PDF vs Image uploads.
func DetectInputKind(data []byte, declaredMime string) (InputKind, string, error) {
	if len(data) < 4 {
		return "", "", errors.New("visionlib: input payload too short")
	}
	if bytes.HasPrefix(data, []byte("%PDF-")) || strings.EqualFold(strings.TrimSpace(declaredMime), "application/pdf") {
		return InputPDF, "application/pdf", nil
	}
	sniff := http.DetectContentType(data)
	if strings.HasPrefix(sniff, "image/") {
		return InputImage, sniff, nil
	}
	if (data[0] == 'I' && data[1] == 'I' && data[2] == 42 && data[3] == 0) ||
		(data[0] == 'M' && data[1] == 'M' && data[2] == 0 && data[3] == 42) {
		return InputImage, "image/tiff", nil
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err == nil && format != "" {
		return InputImage, "image/" + strings.ToLower(format), nil
	}
	return "", sniff, errors.New("visionlib: unsupported file format (expected PDF or image)")
}

// Processor defines the unified image segmentation and denoising contract shared by
// rendered PDF pages and direct image uploads.
type Processor interface {
	NormalizeToPNG(raw []byte) (img image.Image, pngBytes []byte, width int, height int, err error)
	SegmentRegions(pngBytes []byte, minArea, maxArea int) (preprocessedPNG []byte, boxes []image.Rectangle, err error)
	CropAndDenoise(pngBytes []byte, box image.Rectangle, preset DenoisePreset) (cropRawPNG []byte, cropDenoisedPNG []byte, err error)
}

// EncodePNG encodes an image.Image to PNG bytes.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SortAndFilterBoxes filters bounding boxes by area and orders them top-to-bottom, left-to-right.
func SortAndFilterBoxes(rects []image.Rectangle, bounds image.Rectangle, minArea, maxArea int) []image.Rectangle {
	filtered := make([]image.Rectangle, 0, len(rects))
	for _, r := range rects {
		clipped := r.Intersect(bounds)
		if clipped.Empty() {
			continue
		}
		area := clipped.Dx() * clipped.Dy()
		if area <= minArea {
			continue
		}
		if maxArea > 0 && area >= maxArea {
			continue
		}
		filtered = append(filtered, clipped)
	}

	sort.Slice(filtered, func(i, j int) bool {
		r1, r2 := filtered[i], filtered[j]
		if math.Abs(float64(r1.Min.Y-r2.Min.Y)) < 12 {
			return r1.Min.X < r2.Min.X
		}
		return r1.Min.Y < r2.Min.Y
	})
	return filtered
}
