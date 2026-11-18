package pdfium

import (
	"github.com/loopsrage/gotools/lib/gen/nodes"
	"github.com/loopsrage/gotools/lib/gocvlib/gocvf"
	"github.com/loopsrage/gotools/lib/pdfpipeline/actionData"
	"github.com/loopsrage/gotools/lib/pdfpipeline/rectAction"
	"github.com/loopsrage/gotools/lib/queueController"
	"github.com/loopsrage/gotools/lib/settings"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/multi_threaded"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"gocv.io/x/gocv"
)

// Be sure to close pools/instances when you're done with them.
var pool pdfium.Pool

func init() {

	config := multi_threaded.Config{}
	err := settings.JsonUnmarshalSettingsTo("PDFium", &config)
	if err != nil {
		log.Fatal(err)
	}

	// Init the PDFium library and return the instance to open documents.
	// You can tweak these configs to your need. Be aware that workers can use quite some memory.
	pool = multi_threaded.Init(config)
}

func GetInstance() pdfium.Pdfium {
	var inst pdfium.Pdfium
	for inst == nil {
		var err error
		inst, err = pool.GetInstance(30 * time.Second)
		if err != nil {
			fmt.Println(err)
			continue
		}
	}
	return inst
}

func GetPageCount(fb []byte) (int, error) {
	instance := GetInstance()
	defer instance.Close()

	// Open the PDF using PDFium (and claim a worker)
	doc, err := instance.OpenDocument(&requests.OpenDocument{
		File: &fb,
	})
	if err != nil {
		return 0, err
	}

	// Always close the document, this will release its resources.
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
		Document: doc.Document,
	})

	cnt, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		return 0, err
	}

	return cnt.PageCount, nil
}

type PageRenderAction struct {
	wg  *sync.WaitGroup
	ctx context.Context
	*queueController.QueueController
	ra *rectAction.RectAction
}

func (p *PageRenderAction) QueueAction(on *nodes.QueueData) error {
	err := p.CheckContext()
	if err != nil {
		return err
	}

	args := &actionData.ActionData{}
	on.ReadAttributes(args)

	decodedBytes, err := base64.StdEncoding.DecodeString(string(args.PDFbytes))
	if err != nil {
		return err
	}

	pageText, pageRender, err := p.OpenAndRender(decodedBytes, args.Page, 300)
	if err != nil {
		return err
	}
	args.PageText = []byte(pageText.Text)

	f := bytes.NewBuffer(nil)
	err = png.Encode(f, pageRender.Result.Image)
	if err != nil {
		return err
	}

	imgMat, err := gocv.IMDecode(f.Bytes(), gocv.IMReadColor)
	if err != nil {
		return err
	}
	defer imgMat.Close()

	args.Image = actionData.NewImageLocker(&imgMat)

	binary, err := gocvf.ConvertToGrayScale(&imgMat)
	if err != nil {
		return err
	}
	defer binary.Close()

	err = gocvf.GetHorizontalAndVerticalLines(binary, 1)
	if err != nil {
		return err
	}

	contours := gocv.FindContours(*binary, gocv.RetrievalTree, gocv.ChainApproxTC89L1)
	defer contours.Close()

	rects := gocvf.ExtractCellBounds(contours)
	sorted := gocvf.SortAndFilterRects(rects, 900, -1)
	fmt.Printf("contours %d rects %d sorted %d\n", contours.Size(), len(rects), len(sorted))
	//rr := &sync.Map{}
	for i, v := range sorted {
		err := p.CheckContext()
		if err != nil {
			return err
		}
		der := on.CopyDerivative(strconv.Itoa(i)).(*nodes.QueueData)
		der.SetAttribute("Rect", v)
		der.SetAttribute("Image", args.Image)
		p.ra.SendSelf(der)
	}

	p.Done()

	return nil
}

func (p *PageRenderAction) Source() string {
	return "PageRenderAction"
}

func (p *PageRenderAction) OpenAndRender(fb []byte, page, dpi int) (*responses.GetPageText, *responses.RenderPageInDPI, error) {
	instance := GetInstance()
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		File: &fb,
	})
	if err != nil {
		return nil, nil, err
	}

	// Always close the document, this will release its resources.
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
		Document: doc.Document,
	})

	pageText, err := instance.GetPageText(&requests.GetPageText{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    page,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	pageRender, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI: dpi, // The DPI to render the page in.
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    page,
			},
		}, // The page to render, 0-indexed.
	})
	if err != nil {
		return nil, nil, err
	}

	fmt.Printf("[%s] starting render %d\n", time.Now().Format(time.RFC3339), page)
	return pageText, pageRender, nil
}

func NewPageRenderAction(ctx context.Context, wg *sync.WaitGroup, action *rectAction.RectAction) *nodes.QueueActionWrapper {
	pra := &PageRenderAction{ctx: ctx, wg: wg, ra: action}
	qa := nodes.NewQueueActionWrapper(ctx, wg, pra)
	pra.QueueController = qa.QueueController
	return qa
}
