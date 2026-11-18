package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
)

type MorphEffect struct {
	*Standard
	RectKernalSize int
	Type           gocv.MorphType
}

func (m *MorphEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	mrph, err := gocvf.Morph(input, m.RectKernalSize, m.Type)
	if err != nil {
		return nil, err
	}
	return mrph, nil
}

func NewMorphEffect(rectKernalSize int, mt gocv.MorphType) *MorphEffect {
	return &MorphEffect{
		Standard:       NewStandardEffect(),
		RectKernalSize: rectKernalSize,
		Type:           mt,
	}
}
