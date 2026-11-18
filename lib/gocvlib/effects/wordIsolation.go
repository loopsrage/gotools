package effects

import (
	"github.com/loopsrage/gotools/lib/gocvlib/effects/standard"
	"image"

	"gocv.io/x/gocv"
)

func WordIsolationChain() []Effect {
	return []Effect{
		standard.NewGrayScaleEffect(),
		standard.NewGaussianBlurEffect(image.Point{X: 5, Y: 5}, 0, 0, gocv.BorderDefault),
		standard.NewAdaptiveThresholdEffect(255, 3, 3, gocv.ThresholdBinary, gocv.AdaptiveThresholdGaussian),
		standard.NewErodeEffect(15, 3),
	}
}
