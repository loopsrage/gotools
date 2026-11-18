package nodes

import (
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Text2Image struct {
	ctx context.Context
	*queueController.QueueController
}

func (t *Text2Image) Source() string {
	return Text2ImageNode.String()
}

func (t *Text2Image) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)

	initial := args.OutputBase + "/" + args.GeneratedAsin
	initialText := initial + ".gt.txt"
	initialTif := initial + ".tif"

	on.SetAttribute("Initial", initial)
	on.SetAttribute("InitialTif", initialTif)
	on.SetAttribute("InitialGtTxt", initialText)

	on.StoreGenerated(GtTxts, initialText, struct{}{})
	on.StoreGenerated(Images, initialTif, struct{}{})

	err := os.WriteFile(initialText, []byte(args.GeneratedAsin), 0644)
	if err != nil {
		fmt.Println(err)
		return err
	}

	cmdArgs := []string{
		fmt.Sprintf("--text=%s", initialText),
		fmt.Sprintf("--outputbase=%s", initial),
		fmt.Sprintf("--font=%s", args.FontName),
		fmt.Sprintf("--fonts_dir=%s", args.FontDir),
		"--ptsize=12", "--ysize=300", "--xsize=600",
	}

	toCtx, can := context.WithTimeout(t.ctx, time.Minute)
	defer can()

	cmd2 := exec.CommandContext(toCtx, "/usr/local/bin/text2image", cmdArgs...)
	_, err = cmd2.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func NewText2Image(ctx context.Context, wg *sync.WaitGroup) *QueueActionWrapper {
	t2i := &Text2Image{ctx: ctx}
	qa := NewQueueActionWrapper(ctx, wg, t2i)
	t2i.QueueController = qa.QueueController
	return qa
}
