package standard

import (
	"gocv.io/x/gocv"
)

type Standard struct {
	original *gocv.Mat
	cpy      *gocv.Mat
}

func (s *Standard) Copy(from *gocv.Mat) (err error) {
	return from.CopyTo(s.cpy)
}

func NewStandardEffect() *Standard {
	return &Standard{}
}
