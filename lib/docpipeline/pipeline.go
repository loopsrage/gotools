package docpipeline

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/loopsrage/go_mltools/polyphonic/abacus"
	"github.com/loopsrage/go_mltools/proceduralphasespace/ppsclient"
	"github.com/loopsrage/go_mltools/visionembed"
	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/indexQueue"
	"github.com/loopsrage/gotools/lib/ocrlib"
	"github.com/loopsrage/gotools/lib/queueController"
	"github.com/loopsrage/gotools/lib/visionlib"
)

// Engine runs the unified 7-stage indexQueue pipeline for both PDF documents and direct images.
type Engine struct {
	cfg       Config
	ppsClient *ppsclient.Client
	proc      visionlib.Processor
	pdfRender *visionlib.PDFRenderer
	embedder  *visionembed.Embedder
	ocrReg    *ocrlib.Registry
	abacus    *abacus.Abacus

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	iq       *indexQueue.IndexQueue
	initOnce sync.Once
}

// NewEngine constructs and starts the indexQueue-backed OCR pipeline Engine.
func NewEngine(
	parentCtx context.Context,
	cfg Config,
	ppsClient *ppsclient.Client,
	proc visionlib.Processor,
	pdfRender *visionlib.PDFRenderer,
	embedder *visionembed.Embedder,
	ocrReg *ocrlib.Registry,
) (*Engine, error) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	if ppsClient == nil {
		ppsClient = ppsclient.NewClient(cfg.PhaseServer)
	}
	cfg.PhaseServer = ppsClient.Config()
	if proc == nil {
		proc = visionlib.NewProcessor()
	}
	if pdfRender == nil {
		pdfRender = visionlib.NewPDFRenderer(cfg.Pipeline.PDFiumMaxTotal)
	}
	if embedder == nil {
		var err error
		embedder, err = visionembed.NewEmbedder(ppsclient.DefaultManifoldDim, 42)
		if err != nil {
			return nil, err
		}
	}
	if ocrReg == nil {
		var err error
		ocrReg, err = ocrlib.NewDefaultRegistry(parentCtx, cfg.OCR)
		if err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(parentCtx)
	e := &Engine{
		cfg:       cfg,
		ppsClient: ppsClient,
		proc:      proc,
		pdfRender: pdfRender,
		embedder:  embedder,
		ocrReg:    ocrReg,
		abacus:    abacus.NewAbacus(),
		ctx:       ctx,
		cancel:    cancel,
	}

	stages := []indexQueue.StageSpec{
		indexQueue.NamedStage(StageIngest, e.runIngestStage),
		indexQueue.NamedStage(StagePDFRender, e.runPDFRenderStage),
		indexQueue.NamedStage(StageSegment, e.runSegmentStage),
		indexQueue.NamedStage(StageDenoise, e.runDenoiseStage),
		indexQueue.NamedStage(StageOCR, e.runOCRStage),
		indexQueue.NamedStage(StagePersist, e.runPersistStage),
		indexQueue.NamedStage(StageFinalize, e.runFinalizeStage),
	}

	actions := indexQueue.NewIndexQueueActions(ctx, &e.wg, 4, stages...)
	for stageName, actCfg := range actions {
		tuning, ok := cfg.Pipeline.Stages[stageName]
		if !ok {
			tuning = StageTuning{WorkerCount: 4, MaxQueueSize: 128, FlushIntervalMs: 50}
		}
		if tuning.WorkerCount <= 0 {
			tuning.WorkerCount = 4
		}
		if tuning.FlushIntervalMs <= 0 {
			tuning.FlushIntervalMs = 50
		}
		actCfg.WorkerCount = tuning.WorkerCount
		actCfg.Config = &indexQueue.WorkerServiceConfig{
			Identity:       stageName,
			WorkerCount:    tuning.WorkerCount,
			WorkerInterval: time.Duration(tuning.FlushIntervalMs) * time.Millisecond,
			StartNow:       true,
		}
	}

	e.iq = indexQueue.NewIndexQueue(ctx, actions)
	for stageName, tuning := range cfg.Pipeline.Stages {
		if tuning.MaxQueueSize > 0 {
			e.iq.SetMaxSize(stageName, tuning.MaxQueueSize)
		}
	}
	e.iq.Start()
	return e, nil
}

// OCRRegistry returns the active swappable OCR Registry.
func (e *Engine) OCRRegistry() *ocrlib.Registry { return e.ocrReg }

// PPSClient returns the proceduralphaseserver Client.
func (e *Engine) PPSClient() *ppsclient.Client { return e.ppsClient }

// Stats returns live Abacus counters and indexQueue telemetry.
func (e *Engine) Stats() map[string]any {
	stageInFlight := map[string]int64{}
	for _, s := range []string{StageIngest, StagePDFRender, StageSegment, StageDenoise, StageOCR, StagePersist, StageFinalize} {
		stageInFlight[s] = e.iq.InFlight(s)
	}
	counters := map[string]int64{}
	for _, name := range []string{
		"docs_submitted", "docs_completed", "docs_failed",
		"pdf_pages_rendered", "direct_images_ingested",
		"pages_segmented", "regions_denoised", "ocr_invocations", "pps_records_upserted",
	} {
		counters[name] = e.abacus.GetOrCreate(name, 0).Get()
	}
	return map[string]any{
		"queue_total_in_flight": e.iq.TotalInFlight(),
		"queue_completed_tasks": e.iq.Completed(),
		"queue_high_water":      e.iq.Total(),
		"stage_in_flight":       stageInFlight,
		"counters":              counters,
		"ocr_providers":         e.ocrReg.List(),
	}
}

// Close shuts down the indexQueue, PDFium pool, and OCR providers.
func (e *Engine) Close() error {
	e.cancel()
	if e.iq != nil {
		e.iq.Close()
	}
	_ = e.pdfRender.Close()
	return e.ocrReg.CloseAll()
}

// Process submits req into the indexQueue pipeline and blocks until all artifacts are persisted
// in proceduralphaseserver and finalization completes (or ctx expires).
func (e *Engine) Process(ctx context.Context, req SubmitRequest) (*ProcessResult, error) {
	if len(req.Data) == 0 {
		return nil, errors.New("docpipeline: empty file data")
	}
	providers, err := e.ocrReg.ResolveProviders(req.OCRProviders)
	if err != nil {
		return nil, err
	}
	provNames := make([]string, len(providers))
	for i, p := range providers {
		provNames[i] = p.Name()
	}

	e.initOnce.Do(func() {
		_ = e.ppsClient.EnsureCollection(ctx)
	})

	segment := e.cfg.Pipeline.SegmentByDefault
	if req.Segment != nil {
		segment = *req.Segment
	}
	dpi := req.DPI
	if dpi <= 0 {
		dpi = e.cfg.Pipeline.DefaultDPI
		if dpi <= 0 {
			dpi = 300
		}
	}
	presetStr := string(req.DenoisePreset)
	if presetStr == "" {
		presetStr = e.cfg.Pipeline.DefaultDenoisePreset
	}
	preset := visionlib.ParseDenoisePreset(presetStr)

	jobCtx, jobCancel := context.WithCancel(ctx)
	stopEngineLink := context.AfterFunc(e.ctx, jobCancel)
	defer stopEngineLink()
	job := &jobState{
		ctx:       jobCtx,
		cancel:    jobCancel,
		actor:     resolveAuditActor(e.cfg),
		req:       req,
		providers: providers,
		provNames: provNames,
		preset:    preset,
		segment:   segment,
		dpi:       dpi,
		startedAt: time.Now(),
		doneCh:    make(chan struct{}),
		pages:     make(map[int]*pageScratch),
	}
	defer jobCancel()

	e.abacus.GetOrCreate("docs_submitted", 0).Inc(1)

	job.pendingOps.Store(1)
	if _, err := e.iq.EnqueueContext(jobCtx, StageIngest, "", &ingestTask{job: job}); err != nil {
		e.abacus.GetOrCreate("docs_failed", 0).Inc(1)
		return nil, err
	}
	e.iq.Flush(StageIngest)

	select {
	case <-job.doneCh:
		if err := job.err(); err != nil {
			e.abacus.GetOrCreate("docs_failed", 0).Inc(1)
			return nil, err
		}
		e.abacus.GetOrCreate("docs_completed", 0).Inc(1)
		return job.result, nil
	case <-ctx.Done():
		job.recordError(ctx.Err())
		e.abacus.GetOrCreate("docs_failed", 0).Inc(1)
		return nil, ctx.Err()
	case <-e.ctx.Done():
		job.recordError(e.ctx.Err())
		e.abacus.GetOrCreate("docs_failed", 0).Inc(1)
		return nil, e.ctx.Err()
	}
}

func (e *Engine) enqueueChild(job *jobState, stage string, task any) error {
	job.pendingOps.Add(1)
	if _, err := e.iq.EnqueueContext(job.ctx, stage, "", task); err != nil {
		job.pendingOps.Add(-1)
		return err
	}
	e.iq.Flush(stage)
	return nil
}

func (e *Engine) finishStageWork(job *jobState, stageErr error) {
	if stageErr != nil {
		job.recordError(stageErr)
	}
	if rem := job.pendingOps.Add(-1); rem == 0 {
		if job.finalizing.CompareAndSwap(false, true) {
			if _, err := e.iq.EnqueueContext(e.ctx, StageFinalize, "", &finalizeTask{job: job}); err != nil {
				job.recordError(err)
				close(job.doneCh)
				return
			}
			e.iq.Flush(StageFinalize)
		}
	}
}

func (e *Engine) runIngestStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) (retErr error) {
	task, ok := qd.Attribute("obj").(*ingestTask)
	if !ok || task == nil {
		return errors.New("docpipeline(ingest): invalid task payload")
	}
	job := task.job
	defer func() { e.finishStageWork(job, retErr) }()

	if err := job.ctx.Err(); err != nil {
		return err
	}

	kind, mime, err := visionlib.DetectInputKind(job.req.Data, job.req.MimeType)
	if err != nil {
		return err
	}

	shaHex := ppsclient.SHA256Hex(job.req.Data)
	md5Hex := fileslib.CalculateLocalMD5(job.req.Data)

	fileName := strings.TrimSpace(job.req.FileName)
	if fileName == "" {
		if kind == visionlib.InputPDF {
			fileName = "document.pdf"
		} else {
			fileName = "image.png"
		}
	}
	docID := strings.TrimSpace(job.req.DocID)
	if docID == "" {
		stem := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
		shortHash := shaHex
		if len(shortHash) > 12 {
			shortHash = shortHash[:12]
		}
		docID = ppsclient.SanitizeSegment(stem) + "-" + shortHash
	} else {
		docID = ppsclient.SanitizeSegment(docID)
	}

	job.docID = docID
	job.fileName = fileName
	job.mimeType = mime
	job.sourceKind = string(kind)
	job.sha256Hex = shaHex
	job.md5Hex = md5Hex

	rootTopic := e.cfg.PhaseServer.RootTopic
	coll := e.cfg.PhaseServer.Collection
	docTopic := ppsclient.DocTopic(rootTopic, docID)

	job.appendAudit("ingest", docTopic, map[string]interface{}{
		"file_name":   fileName,
		"mime_type":   mime,
		"source_kind": string(kind),
		"byte_size":   len(job.req.Data),
		"sha256":      shaHex,
		"md5":         md5Hex,
	})

	switch kind {
	case visionlib.InputPDF:
		pageCount, err := e.pdfRender.PageCount(job.req.Data)
		if err != nil {
			return err
		}
		if pageCount <= 0 {
			return errors.New("docpipeline(ingest): PDF contains 0 pages")
		}
		job.mu.Lock()
		job.pageCount = pageCount
		job.mu.Unlock()

		initItems := e.buildInitialIngestRecords(job, pageCount, coll, rootTopic)
		if err := e.enqueueChild(job, StagePersist, &persistTask{
			job:         job,
			items:       initItems,
			metaItemIdx: -1,
		}); err != nil {
			return err
		}

		for p := 0; p < pageCount; p++ {
			if err := e.enqueueChild(job, StagePDFRender, &renderTask{
				job:       job,
				pageIndex: p,
			}); err != nil {
				return err
			}
		}
		return nil

	case visionlib.InputImage:
		_, pngBytes, w, h, err := e.proc.NormalizeToPNG(job.req.Data)
		if err != nil {
			return fmt.Errorf("docpipeline(ingest): decode image: %w", err)
		}
		job.mu.Lock()
		job.pageCount = 1
		job.mu.Unlock()
		e.abacus.GetOrCreate("direct_images_ingested", 0).Inc(1)

		initItems := e.buildInitialIngestRecords(job, 1, coll, rootTopic)
		if err := e.enqueueChild(job, StagePersist, &persistTask{
			job:         job,
			items:       initItems,
			metaItemIdx: -1,
		}); err != nil {
			return err
		}

		return e.enqueueRenderedPageImage(job, 0, "direct_image", w, h, job.dpi, "", pngBytes)

	default:
		return fmt.Errorf("docpipeline(ingest): unsupported input kind %q", kind)
	}
}

func (e *Engine) buildInitialIngestRecords(job *jobState, pageCount int, coll, rootTopic string) []ppsclient.UpsertItem {
	docTopic := ppsclient.DocTopic(rootTopic, job.docID)
	manifestItem := ppsclient.UpsertItem{
		ID:         "manifest",
		Collection: coll,
		TopicPath:  docTopic,
		Modality:   "document",
		Content:    fmt.Sprintf("%s (%s)", job.fileName, job.sourceKind),
		Payload: map[string]any{
			"doc_id":         job.docID,
			"artifact_type":  "manifest",
			"file_name":      job.fileName,
			"mime_type":      job.mimeType,
			"source_kind":    job.sourceKind,
			"sha256":         job.sha256Hex,
			"md5":            job.md5Hex,
			"byte_size":      len(job.req.Data),
			"page_count":     pageCount,
			"ocr_providers":  job.provNames,
			"denoise_preset": string(job.preset),
			"status":         "processing",
			"created_at":     job.startedAt.UTC().Format(time.RFC3339Nano),
		},
	}
	srcItems := ppsclient.EncodeSourceRecords(coll, rootTopic, job.docID, job.fileName, job.mimeType, job.req.Data, e.cfg.PhaseServer.ChunkBytes)
	out := make([]ppsclient.UpsertItem, 0, 1+len(srcItems))
	out = append(out, manifestItem)
	out = append(out, srcItems...)
	return out
}

func (e *Engine) runPDFRenderStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) (retErr error) {
	task, ok := qd.Attribute("obj").(*renderTask)
	if !ok || task == nil {
		return errors.New("docpipeline(pdf_render): invalid task payload")
	}
	job := task.job
	defer func() { e.finishStageWork(job, retErr) }()

	if err := job.ctx.Err(); err != nil {
		return err
	}

	rp, err := e.pdfRender.RenderPage(job.req.Data, task.pageIndex, job.dpi)
	if err != nil {
		return err
	}
	e.abacus.GetOrCreate("pdf_pages_rendered", 0).Inc(1)

	return e.enqueueRenderedPageImage(job, rp.PageIndex, "pdf_render", rp.Width, rp.Height, rp.DPI, rp.NativeText, rp.PNGBytes)
}

func (e *Engine) enqueueRenderedPageImage(
	job *jobState,
	pageIdx int,
	source string,
	width, height, dpi int,
	nativeText string,
	pngBytes []byte,
) error {
	job.mu.Lock()
	ps := job.ensurePageLocked(pageIdx)
	ps.source = source
	ps.width = width
	ps.height = height
	ps.dpi = dpi
	ps.nativeText = nativeText
	job.mu.Unlock()

	rootTopic := e.cfg.PhaseServer.RootTopic
	coll := e.cfg.PhaseServer.Collection
	docTopic := ppsclient.DocTopic(rootTopic, job.docID)
	pageTopic := ppsclient.PageTopic(rootTopic, job.docID, pageIdx)
	pageImgTopic := ppsclient.PageImagesTopic(rootTopic, job.docID, pageIdx)

	rawVec, err := e.embedder.EmbedPNG(pngBytes)
	if err != nil {
		rawVec = ppsclient.DeterministicHashVector(fmt.Sprintf("%s:p%d:raw", job.docID, pageIdx), ppsclient.DefaultManifoldDim)
	}
	pngSHA := ppsclient.SHA256Hex(pngBytes)

	pageContent := strings.TrimSpace(nativeText)
	if pageContent == "" {
		pageContent = fmt.Sprintf("page:%s:%d", job.docID, pageIdx)
	}

	items := []ppsclient.UpsertItem{
		{
			ID:         "page_meta",
			Collection: coll,
			TopicPath:  pageTopic,
			Modality:   "page",
			Content:    pageContent,
			Payload: map[string]any{
				"doc_id":        job.docID,
				"artifact_type": "page_meta",
				"page_index":    pageIdx,
				"source":        source,
				"dpi":           dpi,
				"width":         width,
				"height":        height,
				"has_pdf_text":  strings.TrimSpace(nativeText) != "",
			},
		},
		{
			ID:         "raw",
			Collection: coll,
			TopicPath:  pageImgTopic,
			Modality:   "image",
			Content:    fmt.Sprintf("image:%s:p%d:raw", job.docID, pageIdx),
			Vector:     rawVec,
			Payload: map[string]any{
				"doc_id":        job.docID,
				"artifact_type": "page_image",
				"variant":       "raw",
				"page_index":    pageIdx,
				"mime_type":     "image/png",
				"width":         width,
				"height":        height,
				"byte_size":     len(pngBytes),
				"sha256":        pngSHA,
				"image_b64":     base64.StdEncoding.EncodeToString(pngBytes),
			},
		},
	}

	job.appendAudit("page_ready", pageTopic, map[string]interface{}{
		"page_index": pageIdx,
		"source":     source,
		"width":      width,
		"height":     height,
		"raw_sha256": pngSHA,
	})

	if err := e.enqueueChild(job, StagePersist, &persistTask{
		job:         job,
		items:       items,
		frameSpace:  docTopic,
		frameSignal: rawVec,
		metaItemIdx: 0,
	}); err != nil {
		return err
	}

	if !job.segment {
		return e.enqueueChild(job, StageDenoise, &denoiseTask{
			job:         job,
			pageIndex:   pageIdx,
			regionIndex: 0,
			bounds:      image.Rect(0, 0, width, height),
			pagePNG:     pngBytes,
		})
	}

	return e.enqueueChild(job, StageSegment, &segmentTask{
		job:        job,
		pageIndex:  pageIdx,
		source:     source,
		width:      width,
		height:     height,
		dpi:        dpi,
		nativeText: nativeText,
		pagePNG:    pngBytes,
	})
}

func (e *Engine) runSegmentStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) (retErr error) {
	task, ok := qd.Attribute("obj").(*segmentTask)
	if !ok || task == nil {
		return errors.New("docpipeline(segment): invalid task payload")
	}
	job := task.job
	defer func() { e.finishStageWork(job, retErr) }()

	if err := job.ctx.Err(); err != nil {
		return err
	}

	prepPNG, boxes, err := e.proc.SegmentRegions(task.pagePNG, e.cfg.Pipeline.MinContourArea, e.cfg.Pipeline.MaxContourArea)
	if err != nil {
		return err
	}
	if len(boxes) == 0 {
		boxes = []image.Rectangle{image.Rect(0, 0, task.width, task.height)}
	}
	e.abacus.GetOrCreate("pages_segmented", 0).Inc(1)

	rootTopic := e.cfg.PhaseServer.RootTopic
	coll := e.cfg.PhaseServer.Collection
	pageImgTopic := ppsclient.PageImagesTopic(rootTopic, job.docID, task.pageIndex)

	prepVec, err := e.embedder.EmbedPNG(prepPNG)
	if err != nil {
		prepVec = ppsclient.DeterministicHashVector(fmt.Sprintf("%s:p%d:preprocessed", job.docID, task.pageIndex), ppsclient.DefaultManifoldDim)
	}
	prepSHA := ppsclient.SHA256Hex(prepPNG)

	if err := e.enqueueChild(job, StagePersist, &persistTask{
		job: job,
		items: []ppsclient.UpsertItem{
			{
				ID:         "preprocessed",
				Collection: coll,
				TopicPath:  pageImgTopic,
				Modality:   "image",
				Content:    fmt.Sprintf("image:%s:p%d:preprocessed", job.docID, task.pageIndex),
				Vector:     prepVec,
				Payload: map[string]any{
					"doc_id":        job.docID,
					"artifact_type": "page_image",
					"variant":       "preprocessed",
					"page_index":    task.pageIndex,
					"region_count":  len(boxes),
					"mime_type":     "image/png",
					"byte_size":     len(prepPNG),
					"sha256":        prepSHA,
					"image_b64":     base64.StdEncoding.EncodeToString(prepPNG),
				},
			},
		},
		metaItemIdx: -1,
	}); err != nil {
		return err
	}

	for rIdx, box := range boxes {
		if err := e.enqueueChild(job, StageDenoise, &denoiseTask{
			job:         job,
			pageIndex:   task.pageIndex,
			regionIndex: rIdx,
			bounds:      box,
			pagePNG:     task.pagePNG,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) runDenoiseStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) (retErr error) {
	task, ok := qd.Attribute("obj").(*denoiseTask)
	if !ok || task == nil {
		return errors.New("docpipeline(denoise): invalid task payload")
	}
	job := task.job
	defer func() { e.finishStageWork(job, retErr) }()

	if err := job.ctx.Err(); err != nil {
		return err
	}

	cropRawPNG, cropDenoisedPNG, err := e.proc.CropAndDenoise(task.pagePNG, task.bounds, job.preset)
	if err != nil {
		return err
	}
	e.abacus.GetOrCreate("regions_denoised", 0).Inc(1)

	rootTopic := e.cfg.PhaseServer.RootTopic
	coll := e.cfg.PhaseServer.Collection
	regTopic := ppsclient.RegionTopic(rootTopic, job.docID, task.pageIndex, task.regionIndex)

	job.mu.Lock()
	ps := job.ensurePageLocked(task.pageIndex)
	ro, exists := ps.regions[task.regionIndex]
	if !exists {
		ro = &RegionOutput{
			RegionIndex: task.regionIndex,
			TopicPath:   regTopic,
			Bounds:      task.bounds,
			Results:     make(map[string]*ocrlib.RecognizeResult),
		}
		ps.regions[task.regionIndex] = ro
	}
	job.mu.Unlock()

	rawVec, err := e.embedder.EmbedPNG(cropRawPNG)
	if err != nil {
		rawVec = ppsclient.DeterministicHashVector(regTopic+":crop_raw", ppsclient.DefaultManifoldDim)
	}
	denVec, err := e.embedder.EmbedPNG(cropDenoisedPNG)
	if err != nil {
		denVec = ppsclient.DeterministicHashVector(regTopic+":crop_denoised", ppsclient.DefaultManifoldDim)
	}

	bboxMap := map[string]int{
		"x": task.bounds.Min.X,
		"y": task.bounds.Min.Y,
		"w": task.bounds.Dx(),
		"h": task.bounds.Dy(),
	}

	if err := e.enqueueChild(job, StagePersist, &persistTask{
		job: job,
		items: []ppsclient.UpsertItem{
			{
				ID:         "crop_raw",
				Collection: coll,
				TopicPath:  regTopic,
				Modality:   "image",
				Content:    fmt.Sprintf("region:%s:p%d:r%d:crop_raw", job.docID, task.pageIndex, task.regionIndex),
				Vector:     rawVec,
				Payload: map[string]any{
					"doc_id":        job.docID,
					"artifact_type": "region_image",
					"variant":       "crop_raw",
					"page_index":    task.pageIndex,
					"region_index":  task.regionIndex,
					"bbox":          bboxMap,
					"mime_type":     "image/png",
					"byte_size":     len(cropRawPNG),
					"sha256":        ppsclient.SHA256Hex(cropRawPNG),
					"image_b64":     base64.StdEncoding.EncodeToString(cropRawPNG),
				},
			},
			{
				ID:         "crop_denoised",
				Collection: coll,
				TopicPath:  regTopic,
				Modality:   "image",
				Content:    fmt.Sprintf("region:%s:p%d:r%d:crop_denoised", job.docID, task.pageIndex, task.regionIndex),
				Vector:     denVec,
				Payload: map[string]any{
					"doc_id":         job.docID,
					"artifact_type":  "region_image",
					"variant":        "crop_denoised",
					"page_index":     task.pageIndex,
					"region_index":   task.regionIndex,
					"bbox":           bboxMap,
					"denoise_preset": string(job.preset),
					"mime_type":      "image/png",
					"byte_size":      len(cropDenoisedPNG),
					"sha256":         ppsclient.SHA256Hex(cropDenoisedPNG),
					"image_b64":      base64.StdEncoding.EncodeToString(cropDenoisedPNG),
				},
			},
		},
		metaItemIdx: -1,
	}); err != nil {
		return err
	}

	for _, prov := range job.providers {
		if err := e.enqueueChild(job, StageOCR, &ocrTask{
			job:          job,
			pageIndex:    task.pageIndex,
			regionIndex:  task.regionIndex,
			bounds:       task.bounds,
			denoisedPNG:  cropDenoisedPNG,
			rawPNG:       cropRawPNG,
			providerName: prov.Name(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) runOCRStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) (retErr error) {
	task, ok := qd.Attribute("obj").(*ocrTask)
	if !ok || task == nil {
		return errors.New("docpipeline(ocr): invalid task payload")
	}
	job := task.job
	defer func() { e.finishStageWork(job, retErr) }()

	if err := job.ctx.Err(); err != nil {
		return err
	}

	prov, ok := e.ocrReg.Get(task.providerName)
	if !ok {
		return fmt.Errorf("docpipeline(ocr): provider %q not found", task.providerName)
	}

	res, err := prov.Recognize(job.ctx, ocrlib.RecognizeRequest{
		DocID:       job.docID,
		PageIndex:   task.pageIndex,
		RegionIndex: task.regionIndex,
		ImagePNG:    task.denoisedPNG,
		RawImagePNG: task.rawPNG,
		Bounds:      task.bounds,
		Language:    job.req.Language,
	})
	if err != nil {
		return err
	}
	e.abacus.GetOrCreate("ocr_invocations", 0).Inc(1)

	rootTopic := e.cfg.PhaseServer.RootTopic
	coll := e.cfg.PhaseServer.Collection
	regTopic := ppsclient.RegionTopic(rootTopic, job.docID, task.pageIndex, task.regionIndex)

	job.mu.Lock()
	ps := job.ensurePageLocked(task.pageIndex)
	if ro, exists := ps.regions[task.regionIndex]; exists {
		ro.Results[res.Provider] = res
	}
	job.mu.Unlock()

	job.appendAudit("ocr_recognize", regTopic, map[string]interface{}{
		"page_index":   task.pageIndex,
		"region_index": task.regionIndex,
		"provider":     res.Provider,
		"model_id":     res.ModelID,
		"confidence":   res.Confidence,
		"text_len":     len(res.Text),
		"latency_ms":   res.LatencyMs,
	})

	ocrContent := res.Text
	if strings.TrimSpace(ocrContent) == "" {
		ocrContent = fmt.Sprintf("empty_ocr:%s:p%d:r%d:%s", job.docID, task.pageIndex, task.regionIndex, res.Provider)
	}

	return e.enqueueChild(job, StagePersist, &persistTask{
		job: job,
		items: []ppsclient.UpsertItem{
			{
				ID:         "ocr." + res.Provider,
				Collection: coll,
				TopicPath:  regTopic,
				Modality:   "ocr_result",
				Content:    ocrContent,
				Vector:     res.Vector,
				Payload: map[string]any{
					"doc_id":        job.docID,
					"artifact_type": "region_ocr",
					"page_index":    task.pageIndex,
					"region_index":  task.regionIndex,
					"bbox": map[string]int{
						"x": task.bounds.Min.X,
						"y": task.bounds.Min.Y,
						"w": task.bounds.Dx(),
						"h": task.bounds.Dy(),
					},
					"provider":   res.Provider,
					"model_id":   res.ModelID,
					"text":       res.Text,
					"confidence": res.Confidence,
					"tokens":     res.Tokens,
					"latency_ms": res.LatencyMs,
					"metadata":   res.Metadata,
				},
			},
		},
		metaItemIdx: -1,
	})
}

func (e *Engine) runPersistStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) (retErr error) {
	task, ok := qd.Attribute("obj").(*persistTask)
	if !ok || task == nil {
		return errors.New("docpipeline(persist): invalid task payload")
	}
	job := task.job
	defer func() { e.finishStageWork(job, retErr) }()

	if err := job.ctx.Err(); err != nil {
		return err
	}

	if task.frameSpace != "" && len(task.frameSignal) > 0 {
		if frameRes, err := e.ppsClient.IntegrateFrame(job.ctx, task.frameSpace, task.frameSignal); err == nil && frameRes != nil {
			if task.metaItemIdx >= 0 && task.metaItemIdx < len(task.items) {
				task.items[task.metaItemIdx].Payload["pinhole_tau"] = frameRes.ProperTimeTau
				task.items[task.metaItemIdx].Payload["compaction_ratio"] = frameRes.CompactionRatio
				task.items[task.metaItemIdx].Payload["frame_index"] = frameRes.FrameIndex
			}
		}
	}

	if err := e.ppsClient.BatchUpsert(job.ctx, task.items); err != nil {
		return err
	}
	e.abacus.GetOrCreate("pps_records_upserted", 0).Inc(int64(len(task.items)))
	return nil
}

func (e *Engine) runFinalizeStage(_ *indexQueue.IndexQueue, qd *queueController.QueueData) error {
	task, ok := qd.Attribute("obj").(*finalizeTask)
	if !ok || task == nil {
		return errors.New("docpipeline(finalize): invalid task payload")
	}
	job := task.job
	defer close(job.doneCh)

	if err := job.err(); err != nil {
		e.ppsClient.ShipLog(e.ctx, "ERROR", "document pipeline failed", map[string]any{
			"doc_id": job.docID,
			"error":  err.Error(),
		})
		return err
	}

	rootTopic := e.cfg.PhaseServer.RootTopic
	coll := e.cfg.PhaseServer.Collection
	docTopic := ppsclient.DocTopic(rootTopic, job.docID)

	job.mu.Lock()
	pageIndices := make([]int, 0, len(job.pages))
	for idx := range job.pages {
		pageIndices = append(pageIndices, idx)
	}
	sort.Ints(pageIndices)

	pagesOut := make([]*PageOutput, 0, len(pageIndices))
	totalRegions := 0
	var docTextParts []string
	var finalItems []ppsclient.UpsertItem

	for _, pIdx := range pageIndices {
		ps := job.pages[pIdx]
		regIndices := make([]int, 0, len(ps.regions))
		for rIdx := range ps.regions {
			regIndices = append(regIndices, rIdx)
		}
		sort.Ints(regIndices)

		orderedRegions := make([]*RegionOutput, 0, len(regIndices))
		for _, rIdx := range regIndices {
			orderedRegions = append(orderedRegions, ps.regions[rIdx])
		}
		totalRegions += len(orderedRegions)

		summaries := make(map[string]*PageProviderSummary, len(job.providers))
		pageOCRTopic := ppsclient.PageOCRTopic(rootTopic, job.docID, pIdx)

		for _, prov := range job.providers {
			pName := prov.Name()
			summary := buildPageProviderSummary(pName, prov.ModelID(), orderedRegions)
			summaries[pName] = summary
			if pName == job.provNames[0] && strings.TrimSpace(summary.FullText) != "" {
				docTextParts = append(docTextParts, summary.FullText)
			}

			ocrPageContent := summary.FullText
			if strings.TrimSpace(ocrPageContent) == "" {
				ocrPageContent = fmt.Sprintf("page_ocr:%s:p%d:%s", job.docID, pIdx, pName)
			}
			finalItems = append(finalItems, ppsclient.UpsertItem{
				ID:         "page_ocr." + pName,
				Collection: coll,
				TopicPath:  pageOCRTopic,
				Modality:   "ocr_page",
				Content:    ocrPageContent,
				Payload: map[string]any{
					"doc_id":          job.docID,
					"artifact_type":   "page_ocr",
					"page_index":      pIdx,
					"provider":        pName,
					"model_id":        summary.ModelID,
					"full_text":       summary.FullText,
					"rows":            summary.Rows,
					"region_count":    summary.RegionCount,
					"mean_confidence": summary.MeanConfidence,
				},
			})
		}

		pageContent := strings.TrimSpace(ps.nativeText)
		if pageContent == "" && len(job.provNames) > 0 && summaries[job.provNames[0]] != nil && summaries[job.provNames[0]].FullText != "" {
			pageContent = summaries[job.provNames[0]].FullText
		}
		if pageContent == "" {
			pageContent = fmt.Sprintf("page:%s:%d", job.docID, pIdx)
		}
		finalItems = append(finalItems, ppsclient.UpsertItem{
			ID:         "page_meta",
			Collection: coll,
			TopicPath:  ppsclient.PageTopic(rootTopic, job.docID, pIdx),
			Modality:   "page",
			Content:    pageContent,
			Payload: map[string]any{
				"doc_id":        job.docID,
				"artifact_type": "page_meta",
				"page_index":    pIdx,
				"source":        ps.source,
				"dpi":           ps.dpi,
				"width":         ps.width,
				"height":        ps.height,
				"region_count":  len(orderedRegions),
				"has_pdf_text":  strings.TrimSpace(ps.nativeText) != "",
			},
		})

		pagesOut = append(pagesOut, &PageOutput{
			PageIndex:  pIdx,
			TopicPath:  ppsclient.PageTopic(rootTopic, job.docID, pIdx),
			Source:     ps.source,
			Width:      ps.width,
			Height:     ps.height,
			DPI:        ps.dpi,
			NativeText: ps.nativeText,
			Regions:    orderedRegions,
			Summary:    summaries,
		})
	}
	job.mu.Unlock()

	fullDocText := strings.TrimSpace(strings.Join(docTextParts, "\n\n"))
	durationMs := float64(time.Since(job.startedAt).Microseconds()) / 1000.0

	headHash := job.appendAudit("finalize", docTopic, map[string]interface{}{
		"page_count":   len(pagesOut),
		"region_count": totalRegions,
		"providers":    job.provNames,
		"duration_ms":  durationMs,
	})

	job.mu.Lock()
	auditEntries := append([]interface{}(nil), toAnySlice(job.auditChain)...)
	job.mu.Unlock()

	finalItems = append(finalItems, ppsclient.UpsertItem{
		ID:         "audit_chain",
		Collection: coll,
		TopicPath:  ppsclient.AuditTopic(rootTopic, job.docID),
		Modality:   "audit",
		Content:    fmt.Sprintf("audit:%s:%s", job.docID, headHash),
		Payload: map[string]any{
			"doc_id":        job.docID,
			"artifact_type": "audit_chain",
			"entry_count":   len(auditEntries),
			"head_hash":     headHash,
			"entries":       auditEntries,
		},
	})

	manifestContent := fullDocText
	if manifestContent == "" {
		manifestContent = fmt.Sprintf("%s (%s)", job.fileName, job.sourceKind)
	}
	completedAt := time.Now().UTC().Format(time.RFC3339Nano)
	timings := map[string]any{"total_ms": durationMs}

	finalItems = append(finalItems, ppsclient.UpsertItem{
		ID:         "manifest",
		Collection: coll,
		TopicPath:  docTopic,
		Modality:   "document",
		Content:    manifestContent,
		Payload: map[string]any{
			"doc_id":          job.docID,
			"artifact_type":   "manifest",
			"file_name":       job.fileName,
			"mime_type":       job.mimeType,
			"source_kind":     job.sourceKind,
			"sha256":          job.sha256Hex,
			"md5":             job.md5Hex,
			"byte_size":       len(job.req.Data),
			"page_count":      len(pagesOut),
			"region_count":    totalRegions,
			"ocr_providers":   job.provNames,
			"denoise_preset":  string(job.preset),
			"full_text":       fullDocText,
			"audit_head_hash": headHash,
			"status":          "completed",
			"timings_ms":      timings,
			"created_at":      job.startedAt.UTC().Format(time.RFC3339Nano),
			"completed_at":    completedAt,
		},
	})

	if err := e.ppsClient.BatchUpsert(job.ctx, finalItems); err != nil {
		job.recordError(err)
		return err
	}
	e.abacus.GetOrCreate("pps_records_upserted", 0).Inc(int64(len(finalItems)))

	if job.req.FlushPT || e.cfg.PhaseServer.SavePTOnFinalize {
		_, _ = e.ppsClient.SavePT(job.ctx)
	}

	e.ppsClient.ShipLog(e.ctx, "INFO", "document pipeline finalized", map[string]any{
		"doc_id":          job.docID,
		"source_kind":     job.sourceKind,
		"page_count":      len(pagesOut),
		"region_count":    totalRegions,
		"providers":       job.provNames,
		"duration_ms":     durationMs,
		"audit_head_hash": headHash,
	})

	job.result = &ProcessResult{
		DocID:         job.docID,
		RootTopic:     docTopic,
		Collection:    coll,
		FileName:      job.fileName,
		MimeType:      job.mimeType,
		SourceKind:    job.sourceKind,
		SHA256:        job.sha256Hex,
		MD5:           job.md5Hex,
		ByteSize:      len(job.req.Data),
		PageCount:     len(pagesOut),
		RegionCount:   totalRegions,
		OCRProviders:  job.provNames,
		DenoisePreset: string(job.preset),
		FullText:      fullDocText,
		AuditHeadHash: headHash,
		DurationMs:    durationMs,
		Pages:         pagesOut,
		TimingsMs:     timings,
	}
	return nil
}

func buildPageProviderSummary(providerName, defaultModelID string, regions []*RegionOutput) *PageProviderSummary {
	type cell struct {
		bounds image.Rectangle
		text   string
	}
	var cells []cell
	var confSum float64
	confCnt := 0
	modelID := defaultModelID

	for _, reg := range regions {
		res, ok := reg.Results[providerName]
		if !ok || res == nil {
			continue
		}
		if res.ModelID != "" {
			modelID = res.ModelID
		}
		confSum += res.Confidence
		confCnt++
		if trimmed := strings.TrimSpace(res.Text); trimmed != "" {
			cells = append(cells, cell{bounds: reg.Bounds, text: trimmed})
		}
	}

	sort.Slice(cells, func(i, j int) bool {
		tol := max(12, min(cells[i].bounds.Dy(), cells[j].bounds.Dy())/2)
		if math.Abs(float64(cells[i].bounds.Min.Y-cells[j].bounds.Min.Y)) <= float64(tol) {
			return cells[i].bounds.Min.X < cells[j].bounds.Min.X
		}
		return cells[i].bounds.Min.Y < cells[j].bounds.Min.Y
	})

	var rows [][]string
	var rowY int
	for i, c := range cells {
		if i == 0 {
			rows = append(rows, []string{c.text})
			rowY = c.bounds.Min.Y
			continue
		}
		tol := max(12, c.bounds.Dy()/2)
		if math.Abs(float64(c.bounds.Min.Y-rowY)) <= float64(tol) {
			rows[len(rows)-1] = append(rows[len(rows)-1], c.text)
		} else {
			rows = append(rows, []string{c.text})
			rowY = c.bounds.Min.Y
		}
	}

	lineStrs := make([]string, len(rows))
	for i, r := range rows {
		lineStrs[i] = strings.Join(r, "\t")
	}

	var meanConf float64
	if confCnt > 0 {
		meanConf = confSum / float64(confCnt)
	}

	return &PageProviderSummary{
		Provider:       providerName,
		ModelID:        modelID,
		FullText:       strings.TrimSpace(strings.Join(lineStrs, "\n")),
		Rows:           rows,
		RegionCount:    confCnt,
		MeanConfidence: meanConf,
	}
}

func toAnySlice[T any](in []T) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
