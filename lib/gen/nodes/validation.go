package nodes

import (
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"sync"
)

type Validation struct {
	ctx context.Context
	*queueController.QueueController
}

func (v *Validation) Source() string {
	return "Validation"
}

func (v *Validation) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)

	tl, _ := on.initialTif(tiffLockers, args.InitialTif)
	if tl != nil {
		tl.Close()
	}

	on.Data.StoreInIndex(tiffLockers, args.InitialTif, nil)
	on.Data.StoreInIndex(rtok, args.InitialTif, nil)

	if on.CountDerivativeFiles(args.IterationName(on.Derivative())) != 4 {
		on.DeleteDerivativeFiles(args.IterationName(on.Derivative()))
		return nil
	}

	on.DoOnDerivativeFiles(args.IterationName(on.Derivative()), func(meta *Meta) error {
		return on.DeleteGenerated(meta.Index, meta.Path)
	})

	return nil
}

func NewValidation(ctx context.Context, wg *sync.WaitGroup) *QueueActionWrapper {
	t2i := &Validation{ctx: ctx}
	qa := NewQueueActionWrapper(ctx, wg, t2i)
	t2i.QueueController = qa.QueueController
	return qa
}
