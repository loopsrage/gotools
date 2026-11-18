package standard

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

type ThinningEffect struct {
	*Standard
	ThinType contrib.ThinningType
}

func (t *ThinningEffect) Effect(input *gocv.Mat) (*gocv.Mat, error) {
	return gocvf.Thinning(input, t.ThinType)
}

func NewThinningEffect(thinType contrib.ThinningType) *ThinningEffect {
	return &ThinningEffect{
		Standard: NewStandardEffect(),
		ThinType: thinType,
	}
}
