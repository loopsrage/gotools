package concurr

import (
	"context"
	"sync"
	"sync/atomic"
)

// Config provides settings for ActionOn
type Config struct {
	WaitGroup *sync.WaitGroup

	// StaggerMs controls the time.Sleep(ms) between running
	Stagger int64 `json:"Stagger"`

	//
	SemaphorePool chan struct{}
}

// Acquire takes a semaphore slot. It returns false when the context is done
// before a slot could be taken (no slot is held, so Release must not be called).
// A nil SemaphorePool means unlimited concurrency and always succeeds.
func (c *Config) Acquire(ctx context.Context) bool {
	if c.SemaphorePool == nil {
		return true
	}
	// Fast path: never take a slot when the context is already done.
	select {
	case <-ctx.Done():
		return false
	default:
	}
	select {
	case <-ctx.Done():
		return false
	case c.SemaphorePool <- struct{}{}:
		// Acquired the slot
		return true
	}
}

func (c *Config) Release() {
	if c.SemaphorePool == nil {
		return
	}
	<-c.SemaphorePool
}

func (c *Config) StaggerMs() int64 { return atomic.LoadInt64(&c.Stagger) }

func NewConfig(maxInFlight int64) *Config {
	return &Config{
		SemaphorePool: make(chan struct{}, maxInFlight),
	}
}

func CheckContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
