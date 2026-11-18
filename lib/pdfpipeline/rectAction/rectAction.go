package rectAction

import (
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/gen/nodes"
	"github.com/loopsrage/gotools/lib/gocvlib/effects"
	"github.com/loopsrage/gotools/lib/pdfpipeline/actionData"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/otiai10/gosseract/v2"
	"gocv.io/x/gocv"
)

type RectAction struct {
	*queueController.QueueController
	ctx context.Context
}

func (r *RectAction) QueueAction(on *nodes.QueueData) error {
	err := concurr.CheckContext(r.ctx)
	if err != nil {
		return err
	}

	args := &actionData.ActionData{}
	on.ReadAttributes(args)

	cellImg := args.Image.Region(args.Rect)
	if cellImg.Empty() {
		return errors.New("empty image")
	}
	defer cellImg.Close()

	cp := gocv.NewMat()
	defer cp.Close()

	err = cellImg.CopyTo(&cp)
	if err != nil {
		return err
	}

	ec := effects.NewEffectChain()
	out2, err := ec.Chain(&cp, effects.WordDenoiseChain())
	if err != nil {
		return err
	}
	defer out2.Close()

	if out2 == nil || out2.Empty() {
		return errors.New("empty image")
	}

	buffer, err := gocv.IMEncode(".png", *out2)
	defer buffer.Close()
	if err != nil {
		return err
	}

	nbl := actionData.NewNativeBufferLocker(buffer)

	if nbl.Len() <= 0 {
		return err
	}

	client := gosseract.NewClient()
	defer client.Close()

	err = client.SetImageFromBytes(nbl.GetBytes())
	if err != nil {
		return err
	}

	output, err := client.Text()
	if err != nil {
		return err
	}

	if output == "" {
		return nil
	}
	fmt.Println(output)
	//ev, _ := args.RectResult.Load(args.Rect.Dy())
	//switch ev := ev.(type) {
	//case nil:
	//	ev = []interface{}{output}
	//	args.RectResult.Store(args.Rect.Dy(), ev)
	//case []interface{}:
	//	ev = append(ev, output)
	//	args.RectResult.Store(args.Rect.Dy(), ev)
	//}
	//on.SetAttribute("RectResult", args.RectResult)
	on.SetAttribute("FileBytes", nbl)
	return nil
}

func (r *RectAction) Source() string {
	return "RectAction"
}

func NewRectAction(ctx context.Context, wg *sync.WaitGroup) (*nodes.QueueActionWrapper, *RectAction) {
	pra := &RectAction{ctx: ctx}
	qa := nodes.NewQueueActionWrapper(ctx, wg, pra)
	pra.QueueController = qa.QueueController
	return qa, pra
}
