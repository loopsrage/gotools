package nodes

import (
	"github.com/loopsrage/gotools/lib/clsr"
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/gziplib"
	"github.com/loopsrage/gotools/lib/queueController"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gocv.io/x/gocv"
)

type QueueData struct {
	*queueController.QueueData

	compressedBytes int64
	maxBytes        int64

	ctx context.Context
}

func (c *QueueData) StoreGenerated(indName GeneratedData, path string, meta interface{}) {
	c.Data.StoreInIndex(indName.String(), path, meta)
}

func (c *QueueData) DeleteGenerated(indName GeneratedData, path string) (err error) {
	return c.Data.DeleteFromIndex(indName.String(), path)
}

type Meta struct {
	Path  string
	Index GeneratedData
	Meta  interface{}
}

func (c *QueueData) LoadGenerated(indName GeneratedData, path string) *Meta {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return nil
	}

	x, _ := c.Data.LoadFromIndex(indName.String(), path)
	return &Meta{
		Path:  path,
		Index: indName,
		Meta:  x,
	}
}

func (c *QueueData) WriteCompressedData(outputPath string) error {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	return c.Data.RangeIndex(generatedFiles, func(key, value interface{}) {
		switch value := value.(type) {
		case []byte:

			err := concurr.CheckContext(c.ctx)
			if err != nil {
				file.Close()
				return
			}

			_, err = file.Write(value)
			if err != nil {
				fmt.Println(err)
				return
			}

			err = c.Data.DeleteFromIndex(generatedFiles, key)
			if err != nil {
				fmt.Println(err)
				return
			}

		}
	})
}

const generatedFiles = "generatedFiles"

func (c *QueueData) StoreGeneratedFile(file *GeneratedFile) (err error) {
	err = concurr.CheckContext(c.ctx)
	if err != nil {
		return err
	}

	ugzb, err := json.Marshal(file)
	if err != nil {
		return err
	}

	gzb, err := gziplib.Gzip(ugzb)
	if err != nil {
		return err
	}
	c.Data.StoreInIndex(generatedFiles, file.Path, gzb)

	atomic.AddInt64(&c.compressedBytes, int64(len(gzb)))
	if atomic.LoadInt64(&c.compressedBytes) >= atomic.LoadInt64(&c.maxBytes) {
		atomic.StoreInt64(&c.compressedBytes, 0)
		fn := fmt.Sprintf("/home/jarek/tdc/tdc-%d.gz", time.Now().Unix())
		return c.WriteCompressedData(fn)
	}
	return nil
}

const rtok = "RTOK"

func (c *QueueData) LoadReadTiffOnce(key string) *sync.Once {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return nil
	}

	x, _ := c.Data.LoadFromIndex(rtok, key)
	switch x := x.(type) {
	case *sync.Once:
		return x
	default:
		o := &sync.Once{}
		c.Data.StoreInIndex(rtok, key, o)
		return o
	}
}

func (c *QueueData) CopyInitialTif(key, path string, dst *gocv.Mat) (err error) {
	err = concurr.CheckContext(c.ctx)
	if err != nil {
		return err
	}

	tl, err := c.initialTif(key, path)
	if err != nil {
		return err
	}

	err = tl.NilOrEmpty()
	if err != nil {
		return err
	}
	return tl.CopyTo(dst)
}

const tiffLockers = "tiffLockers"

type TiffLocker struct {
	*sync.RWMutex
	*gocv.Mat
}

func (tl *TiffLocker) NilOrEmpty() (err error) {
	tl.RLock()
	defer tl.RUnlock()
	if tl.Mat == nil {
		return errors.New("nil mat")
	}

	if tl.Mat.Empty() {
		return errors.New("empty mat")
	}
	return nil
}

func (tl *TiffLocker) Close() error {
	tl.Lock()
	defer tl.Unlock()
	if tl.Mat != nil {
		tl.Mat.Close() // Free the C++ memory
		tl.Mat = nil
	}
	return nil
}

func (tl *TiffLocker) CopyTo(dst *gocv.Mat) error {
	tl.RLock()
	defer tl.RUnlock()
	if tl.Mat == nil {
		return errors.New("nil mat")
	}
	return tl.Mat.CopyTo(dst)
}

func NewTiffLocker() *TiffLocker {
	return &TiffLocker{RWMutex: &sync.RWMutex{}}
}

func (c *QueueData) initialTif(key, path string) (*TiffLocker, error) {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return nil, err
	}

	x, _ := c.Data.LoadFromIndex(tiffLockers, path)
	switch x := x.(type) {
	case *TiffLocker:
		return x, nil
	case nil:
		c.LoadReadTiffOnce(key).Do(func() {
			args := &Args{}
			c.ReadAttributes(args)

			tif := gocv.IMRead(args.InitialTif, gocv.IMReadColor)
			defer tif.Close()

			if tif.Empty() {
				tif.Close()
				return
			}

			cl := tif.Clone()
			tl := NewTiffLocker()
			tl.Mat = &cl
			c.Data.StoreInIndex(tiffLockers, path, tl)
			clsr.SigClosure(nil, clsr.NewCleanupComplete(nil, nil), tl)
		})
	}

	x, err = c.Data.LoadFromIndex(tiffLockers, path)
	if err != nil {
		return nil, err
	}

	switch x := x.(type) {
	case *TiffLocker:
		return x, nil
	default:
		return nil, errors.New("no tiff locker")
	}
}

func (c *QueueData) CopyDerivative(derivative string) queueController.QueueD {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return nil
	}

	return &QueueData{
		QueueData:       c.QueueData.CopyDerivative(derivative).(*queueController.QueueData),
		compressedBytes: atomic.LoadInt64(&c.compressedBytes),
		maxBytes:        atomic.LoadInt64(&c.maxBytes),
		ctx:             c.ctx,
	}
}

func (c *QueueData) CountDerivativeFiles(iterationName string) int64 {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return -1
	}

	count := int64(0)
	c.DoOnDerivativeFiles(iterationName, func(meta *Meta) error {
		mt := c.LoadGenerated(meta.Index, iterationName+meta.Index.Extention())
		if mt == nil {
			return errors.New("no meta")
		}
		atomic.AddInt64(&count, 1)
		return nil
	})
	return count
}

func (c *QueueData) DeleteDerivativeFiles(iterationName string) {
	err := concurr.CheckContext(c.ctx)
	if err != nil {
		return
	}

	c.DoOnDerivativeFiles(iterationName, func(meta *Meta) error {
		err := c.DeleteGenerated(meta.Index, iterationName)
		if err != nil {
			return err
		}

		return os.Remove(iterationName + meta.Index.Extention())
	})
}

func (c *QueueData) DoOnDerivativeFiles(derivative string, do func(meta *Meta) error) {
	// Remove local WaitGroup, use channels for coordination instead
	errChan := make(chan error, len(AllGeneratedData()))

	for _, v := range AllGeneratedData() {
		v := v // capture loop variable
		go func(ind GeneratedData) {
			select {
			case <-c.ctx.Done():
				errChan <- c.ctx.Err()
				return
			default:
				mt := c.LoadGenerated(ind, derivative+ind.Extention())
				if mt != nil {
					errChan <- do(mt)
				} else {
					errChan <- nil
				}
			}
		}(v)
	}

	// Collect all results with timeout
	for i := 0; i < len(AllGeneratedData()); i++ {
		select {
		case err := <-errChan:
			if err != nil {
				fmt.Println(err)
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func NewQueueData(ctx context.Context) *QueueData {
	qd := &QueueData{
		ctx:       ctx,
		QueueData: queueController.NewQueueData(),
		maxBytes:  1000000,
	}
	return qd
}
