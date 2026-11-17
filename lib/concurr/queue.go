package concurr

import (
	"context"
	"sync"
)

func ActionQueue(ctx context.Context, wg *sync.WaitGroup, config *Config, action Action, queueChan <-chan interface{}) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case c, ok := <-queueChan:
				if !ok {
					return
				}
				runAction(ctx, wg, config, action, c)
			}
		}
	}()
}
