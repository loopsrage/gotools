package pipeline

import (
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/gen/nodes"
	"github.com/loopsrage/gotools/lib/pdfpipeline/finalAction"
	"github.com/loopsrage/gotools/lib/pdfpipeline/pdfium"
	"github.com/loopsrage/gotools/lib/pdfpipeline/rectAction"
	"github.com/loopsrage/gotools/lib/pdfpipeline/tesseractAction"
	"context"
	"sync"
)

type Queues struct {
	Rect        *nodes.QueueActionWrapper
	TessAction  *nodes.QueueActionWrapper
	PdfAction   *nodes.QueueActionWrapper
	FinalAction *nodes.QueueActionWrapper

	Final *finalAction.FinalAction
}

func NewPipeline(ctx context.Context, wg *sync.WaitGroup, maxInFlight int64) *Queues {
	ra, ri := rectAction.NewRectAction(ctx, wg)
	pa := pdfium.NewPageRenderAction(ctx, wg, ri)
	ta := tesseractAction.NewTesseractAction(ctx, wg)
	fa, fi := finalAction.NewFinalAction(ctx, wg)

	pa.SetNext(ra.QueueController)
	ra.SetNext(ta.QueueController)
	ta.SetNext(fa.QueueController)

	ra.Start(concurr.NewConfig(maxInFlight))
	pa.Start(concurr.NewConfig(maxInFlight))
	ta.Start(concurr.NewConfig(maxInFlight))
	fa.Start(concurr.NewConfig(maxInFlight))

	return &Queues{
		Rect:        ra,
		TessAction:  ta,
		PdfAction:   pa,
		FinalAction: fa,
		Final:       fi,
	}
}
