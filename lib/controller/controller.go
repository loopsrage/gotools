package controller

import (
	"github.com/loopsrage/gotools/lib/clsr"
	"sync/atomic"
	"time"
)

type Controller struct {
	Tk       *time.Ticker
	Interval int64
}

func (c *Controller) Close() error {
	c.Tk.Stop()
	return nil
}

func (c *Controller) Ticker() *time.Ticker {
	return c.Tk
}

func (c *Controller) Reset(proceed bool) {
	if proceed {
		c.Tk.Reset(1)
		return
	}
	c.Tk.Reset(time.Duration(atomic.LoadInt64(&c.Interval)))
}

func NewController(cc *clsr.CleanupComplete, interval time.Duration, startNow bool) *Controller {
	tk := time.NewTicker(1)
	if !startNow {
		tk.Reset(interval)
	}
	ctrl := &Controller{
		Tk:       tk,
		Interval: int64(interval),
	}
	clsr.SigClosure(nil, cc, ctrl)
	return ctrl
}
