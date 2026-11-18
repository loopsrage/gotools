package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type GrayScaleEffect struct {
	*Standard
}

func (s *GrayScaleEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	gsi, err := gocvf.ConvertToGrayScale(input)
	if err != nil {
		return nil, err
	}
	return gsi, nil
}

func NewGrayScaleEffect() *GrayScaleEffect {
	return &GrayScaleEffect{
		Standard: NewStandardEffect(),
	}
}
