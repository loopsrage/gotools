package clsr

import (
	"errors"
	"fmt"
	"testing"
)

type clsr struct {
	x chan struct{}
}

func (c *clsr) Close() error {
	go func() {
		c.x <- struct{}{}
	}()
	return errors.New("closing test clsr")
}

func (c *clsr) Run() {
	fmt.Println(<-c.x)
}

func TestClsr(t *testing.T) {
	cs := &clsr{x: make(chan struct{})}
	t.Run("Testing closure", func(t *testing.T) {
		cs.Run()
	})
}
