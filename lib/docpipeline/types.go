// Package docpipeline implements a 7-stage indexQueue document and image OCR processing engine
// with Poincare-ball visual embeddings, swappable OCR providers, SHA-256 audit chains, and
// hierarchical JSON + Base64 persistence in proceduralphaseserver.
package docpipeline

import (
	"context"
	"image"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loopsrage/go_mltools/proceduralphasespace/ppsclient"
	"github.com/loopsrage/gotools/lib/auditlib"
	"github.com/loopsrage/gotools/lib/ocrlib"
	"github.com/loopsrage/gotools/lib/visionlib"
)

// Stage names registered in indexQueue.IndexQueue.
const (
	StageIngest    = "ingest"
	StagePDFRender = "pdf_render"
	StageSegment   = "segment"
	StageDenoise   = "denoise"
	StageOCR       = "ocr"
	StagePersist   = "persist"
	StageFinalize  = "finalize"
)

// StageTuning configures WorkerCount, MaxQueueSize, and ticker flush interval for an indexQueue stage.
type StageTuning struct {
	WorkerCount     int64 `json:"WorkerCount" yaml:"WorkerCount"`
	MaxQueueSize    int64 `json:"MaxQueueSize" yaml:"MaxQueueSize"`
	FlushIntervalMs int64 `json:"FlushIntervalMs" yaml:"FlushIntervalMs"`
}

// PipelineConfig configures unified PDF + direct-image processing across indexQueue stages.
type PipelineConfig struct {
	ServiceName          string                 `json:"ServiceName,omitempty" yaml:"ServiceName,omitempty"`
	DefaultDPI           int                    `json:"DefaultDPI" yaml:"DefaultDPI"`
	MinContourArea       int                    `json:"MinContourArea" yaml:"MinContourArea"`
	MaxContourArea       int                    `json:"MaxContourArea" yaml:"MaxContourArea"`
	DefaultDenoisePreset string                 `json:"DefaultDenoisePreset" yaml:"DefaultDenoisePreset"`
	SegmentByDefault     bool                   `json:"SegmentByDefault" yaml:"SegmentByDefault"`
	PDFiumMaxTotal       int                    `json:"PDFiumMaxTotal" yaml:"PDFiumMaxTotal"`
	Stages               map[string]StageTuning `json:"Stages" yaml:"Stages"`
}

// Config aggregates proceduralphaseserver, indexQueue stage, and OCR provider settings for Engine.
type Config struct {
	PhaseServer ppsclient.Config `json:"PhaseServer" yaml:"PhaseServer"`
	Pipeline    PipelineConfig   `json:"Pipeline" yaml:"Pipeline"`
	OCR         ocrlib.Config    `json:"OCR" yaml:"OCR"`
}

// DefaultConfig returns canonical defaults for the 7-stage indexQueue document OCR pipeline.
func DefaultConfig() Config {
	ppsCfg := ppsclient.DefaultConfig()
	ppsCfg.Collection = "ocr_artifacts"
	ppsCfg.RootTopic = "ocr.documents"
	ppsCfg.ServiceName = "docpipeline"
	ppsCfg.ServiceEmail = "docpipeline@gmail.com"
	return Config{
		PhaseServer: ppsCfg,
		Pipeline: PipelineConfig{
			ServiceName:          "docpipeline",
			DefaultDPI:           300,
			MinContourArea:       900,
			MaxContourArea:       -1,
			DefaultDenoisePreset: "word_denoise",
			SegmentByDefault:     true,
			PDFiumMaxTotal:       4,
			Stages: map[string]StageTuning{
				StageIngest:    {WorkerCount: 4, MaxQueueSize: 64, FlushIntervalMs: 50},
				StagePDFRender: {WorkerCount: 4, MaxQueueSize: 64, FlushIntervalMs: 50},
				StageSegment:   {WorkerCount: 8, MaxQueueSize: 128, FlushIntervalMs: 50},
				StageDenoise:   {WorkerCount: 8, MaxQueueSize: 256, FlushIntervalMs: 50},
				StageOCR:       {WorkerCount: 8, MaxQueueSize: 256, FlushIntervalMs: 50},
				StagePersist:   {WorkerCount: 8, MaxQueueSize: 256, FlushIntervalMs: 50},
				StageFinalize:  {WorkerCount: 4, MaxQueueSize: 64, FlushIntervalMs: 50},
			},
		},
		OCR: ocrlib.DefaultConfig(),
	}
}

// SubmitRequest describes a PDF or Image processing job submitted to Engine.
type SubmitRequest struct {
	DocID         string                  `json:"doc_id,omitempty"`
	FileName      string                  `json:"file_name,omitempty"`
	MimeType      string                  `json:"mime_type,omitempty"`
	Data          []byte                  `json:"-"`
	OCRProviders  []string                `json:"ocr_providers,omitempty"`
	DenoisePreset visionlib.DenoisePreset `json:"denoise_preset,omitempty"`
	Segment       *bool                   `json:"segment,omitempty"`
	DPI           int                     `json:"dpi,omitempty"`
	Language      string                  `json:"language,omitempty"`
	FlushPT       bool                    `json:"flush_pt,omitempty"`
}

// RegionOutput summarizes OCR results across 1..K providers for a single bounding-box region.
type RegionOutput struct {
	RegionIndex int                                `json:"region_index"`
	TopicPath   string                             `json:"topic_path"`
	Bounds      image.Rectangle                    `json:"bounds"`
	Results     map[string]*ocrlib.RecognizeResult `json:"results"`
}

// PageProviderSummary holds reconstructed reading-order text and row cells for one provider on one page.
type PageProviderSummary struct {
	Provider       string     `json:"provider"`
	ModelID        string     `json:"model_id"`
	FullText       string     `json:"full_text"`
	Rows           [][]string `json:"rows"`
	RegionCount    int        `json:"region_count"`
	MeanConfidence float64    `json:"mean_confidence"`
}

// PageOutput summarizes all regions and reconstructed text for a single page.
type PageOutput struct {
	PageIndex  int                             `json:"page_index"`
	TopicPath  string                          `json:"topic_path"`
	Source     string                          `json:"source"`
	Width      int                             `json:"width"`
	Height     int                             `json:"height"`
	DPI        int                             `json:"dpi"`
	NativeText string                          `json:"native_text,omitempty"`
	Regions    []*RegionOutput                 `json:"regions"`
	Summary    map[string]*PageProviderSummary `json:"summary"`
}

// ProcessResult is returned when a document completes all indexQueue stages and is persisted in proceduralphaseserver.
type ProcessResult struct {
	DocID         string         `json:"doc_id"`
	RootTopic     string         `json:"root_topic"`
	Collection    string         `json:"collection"`
	FileName      string         `json:"file_name"`
	MimeType      string         `json:"mime_type"`
	SourceKind    string         `json:"source_kind"`
	SHA256        string         `json:"sha256"`
	MD5           string         `json:"md5"`
	ByteSize      int            `json:"byte_size"`
	PageCount     int            `json:"page_count"`
	RegionCount   int            `json:"region_count"`
	OCRProviders  []string       `json:"ocr_providers"`
	DenoisePreset string         `json:"denoise_preset"`
	FullText      string         `json:"full_text"`
	AuditHeadHash string         `json:"audit_head_hash"`
	DurationMs    float64        `json:"duration_ms"`
	Pages         []*PageOutput  `json:"pages"`
	TimingsMs     map[string]any `json:"timings_ms,omitempty"`
}

type pageScratch struct {
	pageIndex  int
	source     string
	width      int
	height     int
	dpi        int
	nativeText string
	regions    map[int]*RegionOutput
}

type jobState struct {
	ctx       context.Context
	cancel    context.CancelFunc
	actor     string
	req       SubmitRequest
	providers []ocrlib.Provider
	provNames []string
	preset    visionlib.DenoisePreset
	segment   bool
	dpi       int
	startedAt time.Time

	docID      string
	fileName   string
	mimeType   string
	sourceKind string
	sha256Hex  string
	md5Hex     string

	pendingOps atomic.Int64
	finalizing atomic.Bool
	doneCh     chan struct{}

	mu         sync.Mutex
	firstErr   error
	pageCount  int
	pages      map[int]*pageScratch
	auditChain []*auditlib.AuditEntry
	result     *ProcessResult
}

func resolveAuditActor(cfg Config) string {
	svc := strings.TrimSpace(cfg.Pipeline.ServiceName)
	if svc == "" {
		svc = strings.TrimSpace(cfg.PhaseServer.ServiceName)
	}
	if svc == "" {
		return "docpipeline"
	}
	if strings.HasSuffix(svc, ".pipeline") {
		return svc
	}
	return svc + ".pipeline"
}

func (j *jobState) appendAudit(action, resource string, details map[string]interface{}) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	var prev *string
	if n := len(j.auditChain); n > 0 {
		h := j.auditChain[n-1].CurrentHash
		prev = &h
	}
	entry := auditlib.NewAuditEntry(j.actor, action, resource, details, auditlib.WithPreviousHash(prev))
	j.auditChain = append(j.auditChain, entry)
	return entry.CurrentHash
}

func (j *jobState) recordError(err error) {
	if err == nil {
		return
	}
	j.mu.Lock()
	if j.firstErr == nil {
		j.firstErr = err
		j.cancel()
	}
	j.mu.Unlock()
}

func (j *jobState) err() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.firstErr
}

func (j *jobState) ensurePageLocked(pageIdx int) *pageScratch {
	ps, ok := j.pages[pageIdx]
	if !ok {
		ps = &pageScratch{
			pageIndex: pageIdx,
			regions:   make(map[int]*RegionOutput),
		}
		j.pages[pageIdx] = ps
	}
	return ps
}

type ingestTask struct {
	job *jobState
}

type renderTask struct {
	job       *jobState
	pageIndex int
}

type segmentTask struct {
	job        *jobState
	pageIndex  int
	source     string
	width      int
	height     int
	dpi        int
	nativeText string
	pagePNG    []byte
}

type denoiseTask struct {
	job         *jobState
	pageIndex   int
	regionIndex int
	bounds      image.Rectangle
	pagePNG     []byte
}

type ocrTask struct {
	job          *jobState
	pageIndex    int
	regionIndex  int
	bounds       image.Rectangle
	denoisedPNG  []byte
	rawPNG       []byte
	providerName string
}

type persistTask struct {
	job         *jobState
	items       []ppsclient.UpsertItem
	frameSpace  string
	frameSignal []float64
	metaItemIdx int
}

type finalizeTask struct {
	job *jobState
}
