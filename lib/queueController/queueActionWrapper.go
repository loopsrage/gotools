package queueController

import (
	"context"
	"fmt"
	"golangApp/lib/toggle"
	"sync"
)

type QueueActionWrapper struct {
	*QueueController
	Wraps QueueAction

	debug *toggle.Toggle
}

func (q *QueueActionWrapper) SetDebug(to bool) {
	q.debug.Set(to)
}

func (q *QueueActionWrapper) Action(on interface{}) {
	switch on := on.(type) {
	case *QueueData:

		err := q.Wraps.QueueAction(on)
		if q.debug.Load() && err != nil {
			fmt.Println(on.Trace().All(), err)
		}

		proceed := q.HandleError(err)
		if !proceed {
			return
		}
		q.Broadcast(on)
		q.SendNext(on)
	default:
		panic("unexpected type")
	}
}

type QueueAction interface {
	QueueAction(on *QueueData) error
}

func NewQueueActionWrapper(ctx context.Context, wg *sync.WaitGroup, wrap QueueAction) *QueueActionWrapper {
	qar := &QueueActionWrapper{Wraps: wrap, debug: toggle.NewToggle(true)}
	qc := NewQueueController(ctx, qar, wg)
	qar.QueueController = qc
	return qar
}
