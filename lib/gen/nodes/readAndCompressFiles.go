package nodes

import (
	"github.com/loopsrage/gotools/lib/gziplib"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"os"
	"sync"
)

type ReadAndCompressFiles struct {
	ctx context.Context
	*queueController.QueueController
}

func (t *ReadAndCompressFiles) Source() string {
	return ReadAndCompressFilesNode.String()
}

type GeneratedFile struct {
	*Meta
	Data []byte `json:"Data"`
}

func NewGeneratedFile(mt *Meta, data []byte) *GeneratedFile {
	return &GeneratedFile{
		Meta: mt,
		Data: data,
	}
}

func m(args *Args, on *QueueData, meta *Meta) error {
	if on.CountDerivativeFiles(args.IterationName(on.Derivative())) != 4 {
		on.DeleteDerivativeFiles(args.IterationName(on.Derivative()))
		return nil
	}

	fb, err := os.ReadFile(meta.Path)
	if err != nil {
		return err
	}

	gfb, err := gziplib.Gzip(fb)
	if err != nil {
		return err
	}

	gf := NewGeneratedFile(meta, gfb)
	err = on.StoreGeneratedFile(gf)
	if err != nil {
		return err
	}
	return nil
}

func (t *ReadAndCompressFiles) QueueAction(on *QueueData) error {
	args := &Args{}
	on.ReadAttributes(args)

	on.DoOnDerivativeFiles(args.IterationName(on.Derivative()),
		func(met *Meta) error { return m(args, on, met) })

	return nil
}

func NewReadAndCompressFiles(ctx context.Context, wg *sync.WaitGroup) *QueueActionWrapper {
	racf := &ReadAndCompressFiles{ctx: ctx}
	qa := NewQueueActionWrapper(ctx, wg, racf)
	racf.QueueController = qa.QueueController
	return qa
}
