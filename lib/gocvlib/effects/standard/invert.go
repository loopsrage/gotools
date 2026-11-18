package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type InvertEffect struct {
	*Standard
}

func (i *InvertEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	inv, err := gocvf.InvertImage(input)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func NewInvertEffect() *InvertEffect {
	return &InvertEffect{
		Standard: NewStandardEffect(),
	}
}
