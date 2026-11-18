package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"
	"image"

	"gocv.io/x/gocv"
)

type GaussianBlurEffect struct {
	*Standard
	Kernel     image.Point
	SigX       float64
	SigY       float64
	BorderType gocv.BorderType
}

func (g *GaussianBlurEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	gbl, err := gocvf.GaussianBlur(input, g.Kernel, g.SigX, g.SigY, g.BorderType)
	if err != nil {
		return nil, err
	}
	return gbl, nil
}

func NewGaussianBlurEffect(kernel image.Point, sigX, sigY float64, border gocv.BorderType) *GaussianBlurEffect {
	return &GaussianBlurEffect{
		Standard:   NewStandardEffect(),
		Kernel:     kernel,
		SigX:       sigX,
		SigY:       sigY,
		BorderType: border,
	}
}
