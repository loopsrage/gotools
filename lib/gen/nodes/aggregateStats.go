package nodes

import (
	"github.com/loopsrage/gotools/lib/index"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"sync"
)

type AggregateStats struct {
	ctx context.Context
	*queueController.QueueController

	Data *QueueData
	idx  *index.Index
}

func (g *AggregateStats) Source() string {
	return AggregateStatsNode.String()
}

func (g *AggregateStats) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)

	on.DoOnDerivativeFiles(args.IterationName(on.Derivative()), func(meta *Meta) error {
		g.Data.StoreGenerated(meta.Index, meta.Path, meta)
		return nil
	})

	return on.Data.RangeIndex(generatedFiles, func(key, value interface{}) {
		switch key := key.(type) {
		case string:
			switch value := value.(type) {
			case []byte:
				g.Data.Data.StoreInIndex(generatedFiles, key, value)
			}
		}
	})
}

func NewAggregateStats(ctx context.Context, wg *sync.WaitGroup) *QueueActionWrapper {
	glb := &AggregateStats{ctx: ctx, Data: NewQueueData(ctx)}
	qa := NewQueueActionWrapper(ctx, wg, glb)
	glb.QueueController = qa.QueueController
	return qa
}
