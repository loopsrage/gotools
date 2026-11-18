package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type ThresholdBinaryEffect struct {
	*Standard
	Min, Max float32
}

func (t ThresholdBinaryEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	return gocvf.ThresholdBinary(input, t.Min, t.Max), nil
}

func NewThresholdBinaryEffect(min, max float32) *ThresholdBinaryEffect {
	return &ThresholdBinaryEffect{
		Standard: NewStandardEffect(),
		Min:      min,
		Max:      max,
	}
}
