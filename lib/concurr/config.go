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

func (c *Config) Acquire(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case c.SemaphorePool <- struct{}{}:
		// Acquired the slot
	}
}

func (c *Config) Release() {
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
