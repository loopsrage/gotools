package effects

import (
	"github.com/loopsrage/gotools/lib/gocvlib/effects/standard"
	"image"

	"gocv.io/x/gocv"
)

func WordDenoiseChain() []Effect {
	return []Effect{
		standard.NewGaussianBlurEffect(image.Point{X: 3, Y: 3}, 0, 0, gocv.BorderDefault),
		standard.NewGrayScaleEffect(),
		standard.NewAdaptiveThresholdEffect(255, .8, 3, gocv.ThresholdBinary, gocv.AdaptiveThresholdGaussian),
		standard.NewDilateEffect(3, 1),
		standard.NewErodeEffect(4, 3),
	}
}
