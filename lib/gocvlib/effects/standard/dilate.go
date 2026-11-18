package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type DilateEffect struct {
	*Standard
	X, Y int
}

func (e *DilateEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	dil, err := gocvf.Dilate(input, gocvf.NewStructuringElement(e.X, e.Y))
	if err != nil {
		return nil, err
	}
	return dil, nil
}

func NewDilateEffect(x, y int) *DilateEffect {
	return &DilateEffect{
		Standard: NewStandardEffect(),
		X:        x,
		Y:        y,
	}
}
