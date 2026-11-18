package queueController

import (
	"context"
	"errors"
	"fmt"
	"github.com/loopsrage/gotools/lib/concurr"
	"sync"
	"time"
)

type QueueController struct {
	ctx context.Context
	wg  *sync.WaitGroup
	wg2 *sync.WaitGroup

	*sync.RWMutex

	queue     chan interface{}
	nextQueue chan interface{}
	prevQueue chan interface{}

	broadCast map[string]*QueueController

	wraps       concurr.Action
	handleError func(error) bool
}

func (q *QueueController) CheckContext() error {
	select {
	case <-q.ctx.Done():
		return q.ctx.Err()
	default:
		return nil
	}
}

func (q *QueueController) SetHandleError(handleErr func(err error) bool) {
	q.Lock()
	defer q.Unlock()

	q.handleError = handleErr
}

func (q *QueueController) SetNext(queueController *QueueController) {
	q.Lock()
	defer q.Unlock()

	q.nextQueue = queueController.queue
	queueController.prevQueue = q.queue
}

func (q *QueueController) SendSelf(on interface{}) {
	q.wg2.Add(1)
	q.queue <- on
}

func (q *QueueController) Done() {
	q.wg2.Done()
}

func (q *QueueController) SendNext(on interface{}) {
	q.RLock() // Acquire read lock
	nextQ := q.nextQueue
	q.RUnlock()

	if nextQ == nil {
		q.wg2.Done()
		return
	}
	nextQ <- on
}

func (q *QueueController) SetBroadcast(to map[string]*QueueController) {
	q.Lock()
	defer q.Unlock()

	q.broadCast = to
}

func (q *QueueController) Broadcast(x QueueD) {
	q.RLock()
	for dr, qc := range q.broadCast {
		go func(der string, to *QueueController, y QueueD) {
			err := q.CheckContext()
			if err != nil {
				return
			}
			to.SendSelf(y.CopyDerivative(der))
		}(dr, qc, x)
	}
	q.RUnlock()
}

func (q *QueueController) Start(cfg *concurr.Config) {
	concurr.ActionQueue(q.ctx, q.wg, cfg, q.wraps, q.queue)
}

func (q *QueueController) HandleError(err error) bool {
	if err != nil {
		q.RLock()
		handler := q.handleError
		q.RUnlock()

		proceed := handler(errors.New(fmt.Sprintf("[%s] Command failed: %s.", time.Now().Format(time.RFC3339), err.Error())))
		if !proceed {
			q.wg2.Done()
			return false
		}
		return true
	}
	return true
}

func NewQueueController(ctx context.Context, wraps concurr.Action, wg *sync.WaitGroup) *QueueController {
	return &QueueController{
		ctx:     ctx,
		wg:      &sync.WaitGroup{},
		wg2:     wg,
		queue:   make(chan interface{}, 1),
		wraps:   wraps,
		RWMutex: &sync.RWMutex{},
		handleError: func(err error) bool {
			return true
		},
	}
}
