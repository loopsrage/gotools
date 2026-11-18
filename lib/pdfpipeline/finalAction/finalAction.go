package finalAction

import (
	"github.com/loopsrage/gotools/lib/gen/nodes"
	"github.com/loopsrage/gotools/lib/gziplib"
	"github.com/loopsrage/gotools/lib/pdfpipeline/actionData"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"encoding/json"
	"sync"
)

type FinalAction struct {
	*queueController.QueueController
	responses *sync.Map
}

func (f *FinalAction) StoreResponse(uuid string, rs chan interface{}) {
	f.responses.Store(uuid, rs)
}

func (f *FinalAction) LoadResponse(uuid string) chan interface{} {
	x, _ := f.responses.Load(uuid)
	switch x := x.(type) {
	case chan interface{}:
		return x
	default:
		return nil
	}
}

func (f *FinalAction) QueueAction(on *nodes.QueueData) error {
	err := f.CheckContext()
	if err != nil {
		return err
	}

	args := &actionData.ActionData{}
	on.ReadAttributes(args)

	jb, err := json.Marshal(args.PageText)
	if err != nil {
		return err
	}

	tb, err := json.Marshal(args.TableData)
	if err != nil {
		return err
	}

	args.PageText, err = gziplib.Gzip(jb)
	if err != nil {
		return err
	}

	args.TableData, err = gziplib.Gzip(tb)
	if err != nil {
		return err
	}

	f.LoadResponse(args.RequestId) <- args
	return nil
}

func (f *FinalAction) Source() string {
	return "FinalAction"
}

func NewFinalAction(ctx context.Context, wg *sync.WaitGroup) (*nodes.QueueActionWrapper, *FinalAction) {
	fa := &FinalAction{responses: &sync.Map{}}
	qa := nodes.NewQueueActionWrapper(ctx, wg, fa)
	fa.QueueController = qa.QueueController
	return qa, fa
}
