package effects

import "gocv.io/x/gocv"

type Effect interface {
	Effect(input *gocv.Mat) (*gocv.Mat, error)
}
