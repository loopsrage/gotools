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
	return time.Duration(rand.Int64N(x)) * time.Millisecond
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

	// Acquire a semaphore slot. If the context is done no slot is taken and
	// Release must not be called.
	if !config.Acquire(ctx) {
		return
	}
	if ctx.Err() != nil {
		config.Release()
		return
	}

	// Sleep for stagger time (holds the slot while dispatching).
	time.Sleep(rndDur(config.StaggerMs()))

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer config.Release()
		select {
		case <-ctx.Done():
			return

		default:
			action.Action(on)
		}
	}()
}
