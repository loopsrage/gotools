package visionlib

import (
	"fmt"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// RenderedPage contains the rendered PNG bytes, dimensions, and embedded text of a PDF page.
type RenderedPage struct {
	PageIndex  int
	Width      int
	Height     int
	DPI        int
	PNGBytes   []byte
	NativeText string
}

// PDFRenderer manages a pure-Go wazero PDFium WebAssembly worker pool.
type PDFRenderer struct {
	maxTotal int
	initOnce sync.Once
	pool     pdfium.Pool
	initErr  error
}

// NewPDFRenderer constructs a lazy-initialized pure-Go PDFium renderer pool.
func NewPDFRenderer(maxTotal int) *PDFRenderer {
	if maxTotal <= 0 {
		maxTotal = 4
	}
	return &PDFRenderer{maxTotal: maxTotal}
}

func (r *PDFRenderer) ensurePool() (pdfium.Pool, error) {
	r.initOnce.Do(func() {
		r.pool, r.initErr = webassembly.Init(webassembly.Config{
			MinIdle:  1,
			MaxIdle:  r.maxTotal,
			MaxTotal: r.maxTotal,
		})
	})
	return r.pool, r.initErr
}

// PageCount returns the number of pages in pdfBytes.
func (r *PDFRenderer) PageCount(pdfBytes []byte) (int, error) {
	pool, err := r.ensurePool()
	if err != nil {
		return 0, fmt.Errorf("pdfium init: %w", err)
	}
	inst, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		return 0, fmt.Errorf("pdfium instance: %w", err)
	}
	defer inst.Close()

	fb := pdfBytes
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &fb})
	if err != nil {
		return 0, fmt.Errorf("pdfium open document: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	cnt, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return 0, fmt.Errorf("pdfium page count: %w", err)
	}
	return cnt.PageCount, nil
}

// RenderPage renders 0-indexed pageIndex at dpi and extracts native PDF text.
func (r *PDFRenderer) RenderPage(pdfBytes []byte, pageIndex, dpi int) (*RenderedPage, error) {
	if dpi <= 0 {
		dpi = 300
	}
	pool, err := r.ensurePool()
	if err != nil {
		return nil, fmt.Errorf("pdfium init: %w", err)
	}
	inst, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("pdfium instance: %w", err)
	}
	defer inst.Close()

	fb := pdfBytes
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &fb})
	if err != nil {
		return nil, fmt.Errorf("pdfium open document: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	var nativeText string
	if pt, err := inst.GetPageText(&requests.GetPageText{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    pageIndex,
			},
		},
	}); err == nil && pt != nil {
		nativeText = pt.Text
	}

	rendered, err := inst.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI: dpi,
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    pageIndex,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("pdfium render page %d: %w", pageIndex, err)
	}

	pngBytes, err := EncodePNG(rendered.Result.Image)
	if err != nil {
		return nil, fmt.Errorf("pdfium encode page %d png: %w", pageIndex, err)
	}
	b := rendered.Result.Image.Bounds()
	return &RenderedPage{
		PageIndex:  pageIndex,
		Width:      b.Dx(),
		Height:     b.Dy(),
		DPI:        dpi,
		PNGBytes:   pngBytes,
		NativeText: nativeText,
	}, nil
}

// Close releases the underlying PDFium WebAssembly pool if initialized.
func (r *PDFRenderer) Close() error {
	if r.pool != nil {
		return r.pool.Close()
	}
	return nil
}
