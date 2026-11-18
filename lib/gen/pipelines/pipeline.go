package pipelines

import (
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/gen/nodes"
	"github.com/loopsrage/gotools/lib/gocvlib/effects"
	"github.com/loopsrage/gotools/lib/gocvlib/effects/standard"
	"github.com/loopsrage/gotools/lib/index"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"errors"
	"image"
	"sync"

	"gocv.io/x/gocv"
)

const nodesKey = "Nodes"

type Pipeline struct {
	idx *index.Index
}

func Required(ctx context.Context, cfg *concurr.Config, wg *sync.WaitGroup) []*nodes.QueueActionWrapper {
	sts := nodes.NewAggregateStats(ctx, wg)
	vld := nodes.NewValidation(ctx, wg)

	racf := nodes.NewReadAndCompressFiles(ctx, wg)
	racf.SetNext(sts.QueueController)

	t2i := nodes.NewText2Image(ctx, wg)

	glb := nodes.NewGenerateLineBox(ctx, wg)
	t13 := nodes.NewTeseseract13LtsmTrain(ctx, wg)
	t13.SetNext(vld.QueueController)

	t2i.SetNext(glb.QueueController)
	glb.SetNext(t13.QueueController)

	// MAX ROUTINES FOR NODE
	t2i.Start(concurr.NewConfig(50))
	glb.Start(concurr.NewConfig(50))
	t13.Start(concurr.NewConfig(50))
	racf.Start(concurr.NewConfig(50))
	sts.Start(concurr.NewConfig(50))
	vld.Start(concurr.NewConfig(50))
	return []*nodes.QueueActionWrapper{
		t2i,
		glb,
		t13,
		racf,
		sts,
	}
}

func (p *Pipeline) LoadNode(source string) *nodes.QueueActionWrapper {
	x, _ := p.idx.LoadFromIndex(nodesKey, source)
	switch x := x.(type) {
	case *nodes.QueueActionWrapper:
		return x
	default:
		return nil
	}
}

func (p *Pipeline) RangeNodes(fn func(key, value interface{})) error {
	return p.idx.RangeIndex(nodesKey, fn)
}

func NoiseFunctions(ctx context.Context, cfg *concurr.Config, noiseSource *gocv.Mat, wg *sync.WaitGroup, next *nodes.QueueActionWrapper) (map[string]*queueController.QueueController, error) {
	if noiseSource == nil {
		return nil, errors.New("noise source is nil")
	}
	//Using the .gt.txt and the .tif files from above, create derived versions with noise / backgrounds
	//Generate images with a source background (random rect region of x by y)

	gnd, err := nodes.NewGenerateNoisyData(ctx, noiseSource, wg, nil)
	if err != nil {
		return nil, err
	}
	gnd.SetNext(next.QueueController)

	gndNs, err := nodes.NewGenerateNoisyData(ctx, nil, wg, nil)
	if err != nil {
		return nil, err
	}
	gndNs.SetNext(next.QueueController)

	gndCVC, err := nodes.NewGenerateNoisyData(ctx, noiseSource, wg, []effects.Effect{
		standard.NewMorphEffect(3, gocv.MorphClose),
		standard.NewGaussianBlurEffect(image.Point{X: 3, Y: 3}, 0, 0, gocv.BorderDefault),
		standard.NewMorphEffect(3, gocv.MorphOpen),

		standard.NewMorphEffect(5, gocv.MorphClose),
		standard.NewGaussianBlurEffect(image.Point{X: 5, Y: 5}, 0, 0, gocv.BorderDefault),
		standard.NewMorphEffect(5, gocv.MorphOpen),

		standard.NewMorphEffect(7, gocv.MorphClose),
		standard.NewGaussianBlurEffect(image.Point{X: 7, Y: 7}, 0, 0, gocv.BorderDefault),
		standard.NewMorphEffect(7, gocv.MorphOpen),

		standard.NewResizeEffect(image.Point{0, 0}, 2., 2., gocv.InterpolationLinear),
	})
	if err != nil {
		return nil, err
	}
	gndCVC.SetNext(next.QueueController)

	gnd.Start(concurr.NewConfig(50))
	gndNs.Start(concurr.NewConfig(50))
	gndCVC.Start(concurr.NewConfig(50))

	return map[string]*queueController.QueueController{
		"with_background": gnd.QueueController,
		"random_color":    gndNs.QueueController,
		"fx_chain":        gndCVC.QueueController,
	}, nil
}

func NewPipeline(ctx context.Context, cfg *concurr.Config, wg *sync.WaitGroup) *Pipeline {
	req := Required(ctx, cfg, wg)
	id := index.NewIndex()
	for _, v := range req {
		id.StoreInIndex("Nodes", v.Source(), v)
	}
	return &Pipeline{
		idx: id,
	}
}
