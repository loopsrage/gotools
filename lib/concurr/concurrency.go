package concurr

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

type Action interface {
	Action(on interface{})
}

func rndDur(x int64) time.Duration {
	x = int64(math.Max(float64(x), float64(1)))
	return time.Duration(rand.Int64N(x))
}

// ActionOn performs action.Action(on) for each instance provided
func ActionOn(ctx context.Context, config *Config, action Action, on ...interface{}) {
	wg := &sync.WaitGroup{}
	for _, x := range on {
		if ctx.Err() != nil {
			fmt.Println("[ActionOn] Context is already done. Stopping loop iteration early.")
			break
		}
		runAction(ctx, wg, config, action, x)
	}
	wg.Wait()
}

func runAction(ctx context.Context, wg *sync.WaitGroup, config *Config, action Action, on interface{}) {
	err := CheckContext(ctx)
	if err != nil {
		return
	}

	// Acquire a semaphore slot (this will block if maxInFlight is reached)
	config.Acquire(ctx)

	if ctx.Err() != nil {
		return
	}

	// Sleep for stagger time
	time.Sleep(rndDur(config.StaggerMs()))

	wg.Go(func() {
		defer config.Release()
		select {
		case <-ctx.Done():
			return

		default:
			action.Action(on)
		}
	})
}
