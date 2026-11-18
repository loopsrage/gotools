package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type AdaptiveThresholdEffect struct {
	*Standard
	Max                   float32
	C                     float32
	BlockSize             int
	ThresholdType         gocv.ThresholdType
	AdaptiveThresholdType gocv.AdaptiveThresholdType
}

func (a *AdaptiveThresholdEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	ms, err := gocvf.AdaptiveThreshhold(input, a.Max, a.C, a.BlockSize, a.ThresholdType, a.AdaptiveThresholdType)
	if err != nil {
		return nil, err
	}
	return ms, nil
}

func NewAdaptiveThresholdEffect(max, c float32, blockSize int, thresholdType gocv.ThresholdType, adaptiveThresholdType gocv.AdaptiveThresholdType) *AdaptiveThresholdEffect {
	return &AdaptiveThresholdEffect{
		Standard:              NewStandardEffect(),
		Max:                   max,
		C:                     c,
		BlockSize:             blockSize,
		ThresholdType:         thresholdType,
		AdaptiveThresholdType: adaptiveThresholdType,
	}
}
