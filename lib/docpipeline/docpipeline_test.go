package docpipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loopsrage/go_mltools/proceduralphasespace/ppsclient"
	"github.com/loopsrage/gotools/lib/docpipeline"
	"github.com/loopsrage/gotools/lib/gziplib"
	"github.com/loopsrage/gotools/lib/visionlib"
)

func TestEnginePDFAndDirectImagePipeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var mu sync.Mutex
	records := map[string]ppsclient.UpsertItem{}

	ppsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Encoding") == "gzip" {
			unz, err := gziplib.EnsureGunzip(bytes.NewBuffer(raw))
			if err == nil {
				raw = unz.Bytes()
			}
		}
		switch r.URL.Path {
		case "/v1/collections":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/integrate/frame":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"space_id":         "test",
				"frame_index":      1,
				"proper_time_tau":  0.42,
				"compaction_ratio": 1.0,
			})
		case "/v1/batch/upsert":
			var body struct {
				Items []ppsclient.UpsertItem `json:"items"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			for _, it := range body.Items {
				records[it.TopicPath+"#"+it.ID] = it
			}
			mu.Unlock()
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/sql/query":
			var q struct {
				Params []any `json:"params"`
			}
			_ = json.Unmarshal(raw, &q)
			prefix := ""
			if len(q.Params) > 0 {
				prefix, _ = q.Params[0].(string)
			}
			mu.Lock()
			var rows []map[string]any
			for _, it := range records {
				if it.TopicPath == prefix || strings.HasPrefix(it.TopicPath, prefix+".") {
					rows = append(rows, map[string]any{
						"id":         it.ID,
						"collection": it.Collection,
						"topic_path": it.TopicPath,
						"modality":   it.Modality,
						"content":    it.Content,
						"payload":    it.Payload,
					})
				}
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"row_maps": rows})
		case "/v1/batch/logs":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ppsSrv.Close()

	cfg := docpipeline.DefaultConfig()
	cfg.PhaseServer.BaseURL = ppsSrv.URL
	cfg.PhaseServer.OAuthToken = "unit-token"
	cfg.PhaseServer.ShipLogs = false

	sharedRend := visionlib.NewPDFRenderer(2)
	defer sharedRend.Close()

	eng, err := docpipeline.NewEngine(ctx, cfg, nil, nil, sharedRend, nil, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// 1. Multi-page PDF
	pdfBytes := visionlib.BuildMultiPagePDF([]string{"PIPELINE ONE", "STAGE 2026"})
	pdfRes, err := eng.Process(ctx, docpipeline.SubmitRequest{
		DocID:    "unit-pdf",
		FileName: "unit.pdf",
		Data:     pdfBytes,
		DPI:      150,
	})
	if err != nil {
		t.Fatalf("Process PDF: %v", err)
	}
	if pdfRes.PageCount != 2 || !strings.Contains(strings.ToUpper(pdfRes.FullText), "PIPELINE") || !strings.Contains(pdfRes.FullText, "2026") {
		t.Fatalf("unexpected PDF OCR result: pages=%d text=%q", pdfRes.PageCount, pdfRes.FullText)
	}

	// 2. Direct PNG image hitting unified post-render stage
	pageImg, err := sharedRend.RenderPage(visionlib.BuildMultiPagePDF([]string{"IMAGE 8844"}), 0, 180)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	imgRes, err := eng.Process(ctx, docpipeline.SubmitRequest{
		DocID:    "unit-img",
		FileName: "scan.png",
		Data:     pageImg.PNGBytes,
	})
	if err != nil {
		t.Fatalf("Process Image: %v", err)
	}
	if imgRes.SourceKind != "image" || !strings.Contains(imgRes.FullText, "8844") {
		t.Fatalf("unexpected direct image result: %+v", imgRes)
	}

	tree, err := eng.PPSClient().GetDocumentHierarchy(ctx, "unit-img", true)
	if err != nil {
		t.Fatalf("GetDocumentHierarchy: %v", err)
	}
	recovered, err := ppsclient.ReassembleSourceBytes(tree.Source, nil)
	if err != nil || !bytes.Equal(recovered, pageImg.PNGBytes) {
		t.Fatalf("source round-trip mismatch: err=%v", err)
	}
}
