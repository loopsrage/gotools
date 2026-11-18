//go:build gocv

package visionlib

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/loopsrage/gotools/lib/gocvlib/effects"
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"
	"gocv.io/x/gocv"
)

type gocvProcessor struct{}

// NewProcessor returns the OpenCV4-accelerated Processor when compiled with -tags gocv.
func NewProcessor() Processor {
	return &gocvProcessor{}
}

func (p *gocvProcessor) NormalizeToPNG(raw []byte) (image.Image, []byte, int, int, error) {
	mat, err := gocv.IMDecode(raw, gocv.IMReadColor)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	defer mat.Close()
	if mat.Empty() {
		return nil, nil, 0, 0, errors.New("visionlib(gocv): empty decoded Mat")
	}
	buf, err := gocv.IMEncode(".png", mat)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	defer buf.Close()
	pngBytes := append([]byte(nil), buf.GetBytes()...)
	img, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, nil, 0, 0, err
	}
	return img, pngBytes, mat.Cols(), mat.Rows(), nil
}

func (p *gocvProcessor) SegmentRegions(pngBytes []byte, minArea, maxArea int) ([]byte, []image.Rectangle, error) {
	mat, err := gocv.IMDecode(pngBytes, gocv.IMReadColor)
	if err != nil {
		return nil, nil, err
	}
	defer mat.Close()
	if mat.Empty() {
		return nil, nil, errors.New("visionlib(gocv): empty Mat in SegmentRegions")
	}

	ec := effects.NewEffectChain()
	isolated, err := ec.Chain(&mat, effects.WordIsolationChain())
	if err != nil {
		return nil, nil, err
	}
	defer isolated.Close()

	rects := gocvf.FindContours(isolated, minArea, maxArea, gocv.RetrievalTree, gocv.ChainApproxTC89L1)
	if len(rects) == 0 {
		rects = []image.Rectangle{image.Rect(0, 0, mat.Cols(), mat.Rows())}
	}

	buf, err := gocv.IMEncode(".png", *isolated)
	if err != nil {
		return nil, nil, err
	}
	defer buf.Close()
	return append([]byte(nil), buf.GetBytes()...), rects, nil
}

func (p *gocvProcessor) CropAndDenoise(pngBytes []byte, box image.Rectangle, preset DenoisePreset) ([]byte, []byte, error) {
	mat, err := gocv.IMDecode(pngBytes, gocv.IMReadColor)
	if err != nil {
		return nil, nil, err
	}
	defer mat.Close()
	full := image.Rect(0, 0, mat.Cols(), mat.Rows())
	clipped := box.Intersect(full)
	if clipped.Empty() {
		clipped = full
	}

	region := mat.Region(clipped)
	defer region.Close()
	crop := region.Clone()
	defer crop.Close()

	rawBuf, err := gocv.IMEncode(".png", crop)
	if err != nil {
		return nil, nil, err
	}
	defer rawBuf.Close()
	rawPNG := append([]byte(nil), rawBuf.GetBytes()...)

	if preset == PresetPassthrough {
		return rawPNG, rawPNG, nil
	}

	chain := effects.WordDenoiseChain()
	if preset == PresetWordIsolation {
		chain = effects.WordIsolationChain()
	}
	ec := effects.NewEffectChain()
	den, err := ec.Chain(&crop, chain)
	if err != nil {
		return nil, nil, err
	}
	defer den.Close()

	denBuf, err := gocv.IMEncode(".png", *den)
	if err != nil {
		return nil, nil, err
	}
	defer denBuf.Close()
	return rawPNG, append([]byte(nil), denBuf.GetBytes()...), nil
}
