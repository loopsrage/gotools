package nodes

import (
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"os/exec"
	"sync"
	"time"
)

type Tesseract13Lstm struct {
	ctx context.Context
	*queueController.QueueController
}

func (t *Tesseract13Lstm) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)
	on.StoreGenerated(Lstms, args.IterationLtsm(on.Derivative()), struct{}{})
	cmd := []string{args.IterationTif(on.Derivative()), args.IterationName(on.Derivative()), "--psm", "13", "lstm.train"}
	toCtx, can := context.WithTimeout(t.ctx, time.Minute)
	defer can()

	cmd2 := exec.CommandContext(toCtx, "tesseract", cmd...)
	_, err := cmd2.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func (t *Tesseract13Lstm) Source() string {
	return Tesseract13LstmTrainNode.String()
}

func NewTeseseract13LtsmTrain(ctx context.Context, wg *sync.WaitGroup) *QueueActionWrapper {
	t13 := &Tesseract13Lstm{ctx: ctx}
	qa := NewQueueActionWrapper(ctx, wg, t13)
	t13.QueueController = qa.QueueController
	return qa
}
