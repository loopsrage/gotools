package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type CannyEffect struct {
	*Standard
	Threshold float32
	Max       float32
}

func (c *CannyEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	edges, err := gocvf.CannyEdgeDetection(input, c.Threshold, c.Max)
	if err != nil {
		return nil, err
	}
	return edges, nil
}

func NewCannyEffect(threshold, max float32) *CannyEffect {
	return &CannyEffect{
		Standard:  NewStandardEffect(),
		Threshold: threshold,
		Max:       max,
	}
}
