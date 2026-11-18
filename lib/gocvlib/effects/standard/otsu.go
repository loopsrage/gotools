package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type ThresholdOtsuEffect struct {
	*Standard
}

func (t ThresholdOtsuEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	th := gocvf.ThresholdOtsu(input)
	return th, nil
}

func NewOtsuEffect() ThresholdOtsuEffect {
	return ThresholdOtsuEffect{
		Standard: NewStandardEffect(),
	}
}
