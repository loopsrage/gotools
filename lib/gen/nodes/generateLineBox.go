package nodes

import (
	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"os"
	"os/exec"
	"sync"
	"time"
)

type GenerateLineBox struct {
	ctx context.Context
	*queueController.QueueController
}

func (g *GenerateLineBox) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)

	der := on.Derivative()
	if der != "" {
		err := fileslib.Copyfile(args.InitialGtTxt, args.IterationGtTxt(der))
		if err != nil {
			return err
		}
	}

	cmdArgs := []string{"/home/jarek/tess/tesstrain/generate_line_box.py",
		"-i", args.IterationTif(on.Derivative()), "-t", args.IterationGtTxt(on.Derivative())}

	toCtx, can := context.WithTimeout(g.ctx, time.Minute)
	defer can()

	cmd1 := exec.CommandContext(toCtx, "/home/jarek/tess/tesstrain/.venv/bin/python3", cmdArgs...)
	resp, err := cmd1.CombinedOutput()
	if err != nil {
		return err
	}

	outputFile := args.IterationBox(on.Derivative())
	on.StoreGenerated(Boxes, outputFile, struct{}{})

	return os.WriteFile(outputFile, resp, 0644)
}

func (g *GenerateLineBox) Source() string {
	return GenerateLineBoxNode.String()
}

func NewGenerateLineBox(ctx context.Context, wg *sync.WaitGroup) *QueueActionWrapper {
	glb := &GenerateLineBox{ctx: ctx}
	qa := NewQueueActionWrapper(ctx, wg, glb)
	glb.QueueController = qa.QueueController
	return qa
}
