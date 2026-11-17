package concurr

import (
	"context"
	"golangApp/lib/index"
	"sync/atomic"
)

type BatchActionQueue struct {
	*Config
	Ctx    context.Context
	Action Action

	batchSize int64
	size      int64
	index     *index.Index
}

func (iq *BatchActionQueue) Add(key, value interface{}) {
	_, loaded := iq.index.LoadOrStoreInIndex("Queue", key, value)
	if !loaded {
		atomic.AddInt64(&iq.size, 1)
	}
}

func (iq *BatchActionQueue) BatchAction() {
	var batch []interface{}
	for iq.Size() > 0 {

		iq.index.LoadIndex("Queue").Range(func(current, value any) bool {
			iq.index.DeleteFromIndex("Queue", current)
			iq.addSize(-1)

			batch = append(batch, map[interface{}]interface{}{
				current: value,
			})

			if int64(len(batch)) >= iq.BatchSize() {
				ActionOn(iq.Ctx, iq.Config, iq.Action, batch...)
				batch = nil
			}
			return true
		})

		if len(batch) > 0 {
			ActionOn(iq.Ctx, iq.Config, iq.Action, batch...)
			batch = nil
		}
	}
}

func (iq *BatchActionQueue) SetBatchSize(size int64) *BatchActionQueue {
	atomic.StoreInt64(&iq.batchSize, size)
	return iq
}

func (iq *BatchActionQueue) BatchSize() int64 {
	return atomic.LoadInt64(&iq.batchSize)
}

func (iq *BatchActionQueue) addSize(x int64) {
	atomic.AddInt64(&iq.size, x)
}

func (iq *BatchActionQueue) Size() int64 { return atomic.LoadInt64(&iq.size) }

func NewBatchActionQueue(ctx context.Context, config *Config, action Action) *BatchActionQueue {
	iq := &BatchActionQueue{
		Ctx:    ctx,
		Config: config,
		index:  index.NewIndex().New("Queue"),
		Action: action,
	}
	iq.SetBatchSize(1024)
	return iq
}
