package clsr

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type CleanupComplete struct {
	can      context.CancelFunc
	wg       *sync.WaitGroup
	complete chan struct{}
	once     *sync.Once
}

func (c *CleanupComplete) Complete() {
	<-c.complete
}

func NewCleanupComplete(can context.CancelFunc, wg *sync.WaitGroup) *CleanupComplete {
	return &CleanupComplete{
		can:      can,
		wg:       wg,
		complete: make(chan struct{}),
		once:     &sync.Once{},
	}
}

func SigClosure(signals []os.Signal, complete *CleanupComplete, after ...io.Closer) {

	if complete == nil {
		panic("nil complete")
	}
	go func() {
		x := make(chan os.Signal, 1)

		sigs := []os.Signal{syscall.SIGTERM, os.Interrupt}
		if len(signals) > 0 {
			sigs = append(sigs, signals...)
		}
		signal.Notify(x, sigs...)

		// Loop forever, waiting for signals
		for sig := range x {
			// once.Do() executes the function argument only the first time Do is called.
			// Subsequent calls do nothing, regardless of which signal triggered the call.
			complete.once.Do(func() {
				fmt.Printf("Received signal %s. Starting cleanup...\n", sig)

				// Use a local WaitGroup for parallel cleanup
				var cleanupWG sync.WaitGroup

				for _, f := range after {
					cleanupWG.Add(1)
					go func(closer io.Closer) {
						defer cleanupWG.Done()
						if err := closer.Close(); err != nil {
							fmt.Printf("Error during cleanup: %v\n", err)
						}
					}(f)
				}

				// Wait for all concurrent cleanup to finish before proceeding
				cleanupWG.Wait()

				close(complete.complete)
				signal.Stop(x)
			})
		}
	}()
}
