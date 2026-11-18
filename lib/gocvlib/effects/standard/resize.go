package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"
	"image"

	"gocv.io/x/gocv"
)

type ResizeEffect struct {
	*Standard
	Sz   image.Point
	Fy   float64
	Fx   float64
	Flag gocv.InterpolationFlags
}

func (r *ResizeEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	rz, err := gocvf.Resize(input, r.Sz, r.Fx, r.Fy, r.Flag)
	if err != nil {
		return nil, err
	}
	return rz, nil
}

func NewResizeEffect(sz image.Point, fx, fy float64, flag gocv.InterpolationFlags) *ResizeEffect {
	return &ResizeEffect{
		Standard: NewStandardEffect(),
		Sz:       sz,
		Fy:       fy,
		Fx:       fx,
		Flag:     flag,
	}
}
