package nodes

import (
	"github.com/loopsrage/gotools/lib/clsr"
	"github.com/loopsrage/gotools/lib/gocvlib/effects"
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"
	"github.com/loopsrage/gotools/lib/gocvlib/utils"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"errors"
	"image"
	"sync"
	"sync/atomic"

	"gocv.io/x/gocv"
)

type GenerateNoisyData struct {
	*sync.RWMutex
	source *gocv.Mat

	csOnce *sync.Once
	ups    *atomic.Bool

	*queueController.QueueController

	cvChain []effects.Effect
}

func (g *GenerateNoisyData) Source() string {
	return "GenerateNoisyData"
}

func (g *GenerateNoisyData) Close() error {
	return g.source.Close()
}

func (g *GenerateNoisyData) Dimensions(img *gocv.Mat) image.Point {
	if g.csOnce == nil {
		g.csOnce = &sync.Once{}
	}
	g.RLock()
	g.csOnce.Do(func() {
		if !g.ups.Load() {
			s := gocv.NewMatWithSize(img.Rows(), img.Cols(), gocv.MatTypeCV8UC3)
			g.source = &s
		}
	})
	srcWidth := g.source.Cols()
	srcHeight := g.source.Rows()
	g.RUnlock()

	if !g.ups.Load() {
		cols := utils.RandColorFloat64()
		g.Lock()
		g.source.SetTo(gocv.NewScalar(cols[0], cols[1], cols[2], 0.0))
		g.Unlock()
	}

	return image.Point{X: srcWidth, Y: srcHeight}
}

func (g *GenerateNoisyData) CopyInitialTif(from *QueueData) (*gocv.Mat, error) {
	img := gocv.NewMat()

	args := &Args{}
	from.ReadAttributes(args)

	err := from.CopyInitialTif(tiffLockers, args.InitialTif, &img)
	if err != nil {
		img.Close()
		return nil, err
	}
	return &img, nil
}

func (g *GenerateNoisyData) RectFromSource(rect image.Rectangle) *gocv.Mat {
	g.RLock()
	cellImg := g.source.Region(rect)
	g.RUnlock()
	return &cellImg
}

func (g *GenerateNoisyData) CVPlan() []effects.Effect {
	return g.cvChain
}

func (g *GenerateNoisyData) ApplyCVChain(src *gocv.Mat, target *gocv.Mat) (err error) {
	ec := effects.NewEffectChain()
	out2, err := ec.Chain(src, g.CVPlan())
	if err != nil {
		return err
	}
	defer out2.Close()

	return out2.CopyTo(target)
}

func (g *GenerateNoisyData) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)

	img, err := g.CopyInitialTif(on)
	if err != nil {
		return err
	}
	defer img.Close()

	pt := g.Dimensions(img)
	rect := utils.RandomRect(img, pt)

	cpy := g.RectFromSource(rect)
	defer cpy.Close()

	if cpy.Empty() {
		return errors.New("cpy is empty")
	}

	dst, err := gocvf.AddWeighted(img, cpy, .3, .5, .5)
	if err != nil {
		return err
	}
	defer dst.Close()

	if g.CVPlan() != nil {
		err := g.ApplyCVChain(img, dst)
		if err != nil {
			return err
		}
	}

	itn := args.IterationTif(on.Derivative())
	ok := gocv.IMWrite(itn, *dst)
	if !ok {
		return errors.New("gocv.IMWrite() failed")
	}

	on.StoreGenerated(Images, itn, struct{}{})
	return nil
}

func NewGenerateNoisyData(ctx context.Context, source *gocv.Mat, wg *sync.WaitGroup, cvChain []effects.Effect) (*QueueActionWrapper, error) {
	ups := &atomic.Bool{}
	ups.Store(source != nil)

	var sourceCopy gocv.Mat
	if source != nil {
		sourceCopy = gocv.NewMat()
		err := source.CopyTo(&sourceCopy)
		if err != nil {
			return nil, err
		}
	}

	gnd := &GenerateNoisyData{
		RWMutex: &sync.RWMutex{},
		source:  &sourceCopy,
		ups:     ups,
		cvChain: cvChain,
	}
	clsr.SigClosure(nil, clsr.NewCleanupComplete(nil, nil), gnd)
	qa := NewQueueActionWrapper(ctx, wg, gnd)
	gnd.QueueController = qa.QueueController
	return qa, nil
}
