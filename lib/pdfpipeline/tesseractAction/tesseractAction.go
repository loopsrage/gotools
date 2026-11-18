package tesseractAction

import (
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/gen/nodes"
	"github.com/loopsrage/gotools/lib/pdfpipeline/actionData"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"log"
	"sync"

	"github.com/otiai10/gosseract/v2"
)

type TesseractAction struct {
	*queueController.QueueController
	results *concurr.TsList
	cfg     *concurr.Config
}

func (t *TesseractAction) QueueAction(on *nodes.QueueData) error {
	err := t.CheckContext()
	if err != nil {
		return err
	}

	args := &actionData.ActionData{}
	on.ReadAttributes(args)

	client := gosseract.NewClient()
	defer client.Close()

	err = client.SetImageFromBytes(args.FileBytes.GetBytes())
	if err != nil {
		log.Fatal(err)
	}

	args.Output, err = client.Text()
	if err != nil {
		log.Fatal(err)
	}

	return nil
}

func (t *TesseractAction) Source() string {
	return "TesseractAction"
}

func NewTesseractAction(ctx context.Context, wg *sync.WaitGroup) *nodes.QueueActionWrapper {
	t2i := &TesseractAction{}
	qa := nodes.NewQueueActionWrapper(ctx, wg, t2i)
	t2i.QueueController = qa.QueueController
	return qa
}
