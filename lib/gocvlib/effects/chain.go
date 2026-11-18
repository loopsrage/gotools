package effects

import (
	"github.com/loopsrage/gotools/lib/gocvlib/gocvd"

	"gocv.io/x/gocv"
)

type EffectChain struct {
	Debug bool
}

func (e *EffectChain) Chain(img *gocv.Mat, order []Effect) (*gocv.Mat, error) {
	var p *gocv.Mat
	for _, v := range order {
		if p == nil {
			p = img
		}

		out, err := v.Effect(p)
		if err != nil {
			return nil, err
		}
		if e.Debug {
			gocvd.DebugWindow(out, nil)
		}
		p = out
	}
	return p, nil
}

func NewEffectChain() *EffectChain {
	return &EffectChain{}
}
