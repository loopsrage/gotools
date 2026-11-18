package queueController

import (
	"context"
	"github.com/loopsrage/gotools/lib/concurr"
)

func QueueDataBackgroundQueue(ctx context.Context, queueData *[]*QueueData, to *QueueController) {
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			for _, qd := range *queueData {
				err := concurr.CheckContext(ctx)
				if err != nil {
					return
				}
				to.SendSelf(qd)
			}
		}
	}()
}
