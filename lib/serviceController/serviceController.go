package serviceController

import (
	"context"
	"time"
)

type Controller interface {
	Ticker() *time.Ticker
	Reset(proceed bool)
}

type Action interface {
	ServiceLoop() (proceed bool)
}

func Service(ctx context.Context, controller Controller, action Action, notifyComplete ...Controller) (errs chan error) {
	errs = make(chan error, 1)
	go func() {
		defer close(errs)
		for {
			select {
			case <-ctx.Done():
				return
			case <-controller.Ticker().C:
				controller.Reset(false)
				controller.Reset(action.ServiceLoop())
				for _, v := range notifyComplete {
					if v == nil {
						continue
					}
					v.Reset(true)
				}
			}
		}
	}()
	return errs
}
