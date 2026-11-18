package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type ErodeEffect struct {
	*Standard
	X, Y int
}

func (e *ErodeEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	dil, err := gocvf.Erode(input, gocvf.NewStructuringElement(e.X, e.Y))
	if err != nil {
		return nil, err
	}
	return dil, nil
}

func NewErodeEffect(x, y int) *ErodeEffect {
	return &ErodeEffect{
		Standard: NewStandardEffect(),
		X:        x,
		Y:        y,
	}
}
