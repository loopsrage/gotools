package nodes

import (
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"sync"
	"sync/atomic"
)

type QueueActionWrapper struct {
	*queueController.QueueController
	Wraps QueueAction
	wg    *sync.WaitGroup
	count int64
}

func (q *QueueActionWrapper) Count() int64 {
	return atomic.LoadInt64(&q.count)
}

func (q *QueueActionWrapper) Source() string {
	return q.Wraps.Source()
}

func (q *QueueActionWrapper) Action(on interface{}) {
	switch on := on.(type) {
	case *QueueData:
		err := q.Wraps.QueueAction(on)
		proceed := q.HandleError(err)
		if !proceed {
			return
		}
		q.Broadcast(on)
		q.SendNext(on)
		atomic.AddInt64(&q.count, 1)
	default:
		panic("unexpected type")
	}
}

type QueueAction interface {
	QueueAction(on *QueueData) error
	Source() string
}

func NewQueueActionWrapper(ctx context.Context, wg *sync.WaitGroup, wrap QueueAction) *QueueActionWrapper {
	qar := &QueueActionWrapper{Wraps: wrap, wg: wg}
	qc := queueController.NewQueueController(ctx, qar, wg)
	qar.QueueController = qc
	return qar
}
