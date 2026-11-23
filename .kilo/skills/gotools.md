# Kilo Skill: gotools — Go helper library collection (`lib/`)

Applies to: `lib/` in module `github.com/loopsrage/gotools` (repo root `/Users/jarek/IdeaProjects/gotools`).

`gotools` is a collection of ~55 pure-Go and dependency-backed helper libraries under `lib/`. It is designed as a shared toolkit for cloud, AI/LLM, OCR/document pipelines, web services, settings, databases, and concurrency primitives. Go 1.27+ is required (`go.mod: go 1.27`).

Module: `github.com/loopsrage/gotools` → import path `github.com/loopsrage/gotools/lib/<pkg>`.

---

## Table of contents

1. [Repo layout & conventions](#repo-layout--conventions)
2. [Core concurrency & helpers](#core-concurrency--helpers)
3. [Cloud (AWS, Azure, secrets)](#cloud-aws-azure-secrets)
4. [AI / LLM / OCR / vision](#ai-llm-ocr--vision)
5. [Document / OCR pipelines](#document--ocr-pipelines)
6. [Web / HTTP](#web--http)
7. [Databases](#databases)
8. [Settings & configuration](#settings--configuration)
9. [Feature flags](#feature-flags)
10. [Service lifecycle & controllers](#service-lifecycle--controllers)
11. [Resilience](#resilience)
12. [MCP server](#mcp-server)
13. [Data / indexing / reflection](#data-indexing--reflection)
14. [Indexing / queues](#indexing--queues)
15. [Small utilities](#small-utilities)
16. [Full package index](#full-package-index)
17. [Verification commands](#verification-commands)

---

## Repo layout & conventions

```
gotools/
  go.mod                  module github.com/loopsrage/gotools, go 1.27
  lib/                    ~55 packages (one import path per package)
    appcontroller/        app-level controller wiring
    auditlib/             audit trail helpers
    awslib/               AWS: Bedrock AI, S3 blob, secrets, SQL
    azurelib/             Azure: SQL, blob, secrets, auth
    benchmarkledger/      benchmark recording ledger
    catchPanic/           panic recovery with callback
    clsr/                 signal-triggered cleanup (SIGTERM/SIGINT)
    concurr/              semaphore-limited concurrency + thread-safe collections
    configBase/           FeatureController + ConfigBase wiring
    controller/           time.Ticker wrapper with reset + clsr cleanup
    dataCollection/       data collection primitives
    dataWrapper/          data wrapper types
    docpipeline/          7-stage indexQueue OCR pipeline (PDF + images)
    emaillib/             email helpers
    etllib/               ETL helpers
    featureflags/         thread-safe feature flag store + rollout evaluation
    featureSettings/      feature settings aggregation
    fileslib/             file helpers (md5, etc.)
    fuzzy/                fuzzy matching
    gclib/                GC helpers
    gen/                  code generation nodes / queue action wrappers
    gmaillib/             Gmail API v1 client (OAuth2, rate limiting)
    gocvlib/              OpenCV (gocv) effects / filters / utils
    gosseractlib/         Tesseract CGo bindings
    gziplib/              gzip compress/decompress with auto-detection
    httplib/              minimal HTTP response writers
    idlib/                ID helpers
    index/                sync.Map-backed named sub-indexes
    indexQueue/           multi-stage concurrent work queue
    informationalindex/   informational index types
    jsonContainers/       JSON container tree builder + searcher
    jsonIndexer/          JSON indexer
    mcplib/               JSON-RPC 2.0 MCP server (HTTP + stdio)
    measureTime/          timing measurement
    mysqllib/             MySQL proxy helpers
    oauthsession/         OAuth2 session helpers
    ocrlib/               swappable OCR provider registry (Tesseract/HTTP/Ollama/MLTools)
    ollamalib/            Ollama client wrapper
    onceler/              sync.Once-per-key execution
    pdfpipeline/          PDFium render + tesseract + finalize action chain
    queueController/      queue data + controller wiring
    quickTimeFormats/     quick time format helpers
    reflectStruct/        struct field population from flattened maps
    reflectTags/          struct tag reflection
    resilience/           circuit breaker, token bucket rate limiter, retry
    scraperlib/           web scraping helpers
    secretslib/           multi-source secret store (mem → env → NAME_FILE → .env)
    serviceController/    ticker-driven service loop
    settings/             hierarchical YAML/JSON SettingsStore (dotted paths)
    sqllib/               reflection-based SQL queries (sql.Null generics)
    timelib/              time-window iteration
    toggle/               atomic boolean switch
    visionlib/            image/PDF processing (gocv + pure Go)
    webhooklib/           webhook helpers
    weblib/               JSON/CORS/WebSocket/SPA HTTP helpers
  tests/                  per-package tests mirroring lib/
```

Conventions observed across packages:
- Package doc comment on the first non-blank line (e.g. `// Package ocrlib provides ...`).
- Constructors named `New<Thing>()`, config structs with `json`/`yaml` tags, `DefaultConfig()` where applicable.
- Thread-safe via `sync.RWMutex` or `sync/atomic`; context-first signatures (`ctx context.Context` first param).
- Errors returned, not panicked (except `configBase.NewFeatureController` which panics on settings load failure).
- Cloud clients accept an optional `*http.Client` for injection/testing.
- `settings` package `init()` auto-loads `env.yaml` from CWD or `$ENVIRONMENT_FILE`.

---

## Core concurrency & helpers

Documented in existing skill `.kilo/skills/concurr-helper-libs.md`: `concurr`, `index`, `onceler`, `toggle`, `timelib`, `controller`, `catchPanic`, `clsr`, `gziplib`.

Quick recap of the most used:

```go
import "github.com/loopsrage/gotools/lib/concurr"

cfg := concurr.NewConfig(8)          // semaphore: max 8 in flight
concurr.ActionOn(ctx, cfg, myAction, a, b, c)

tl := concurr.NewTsList()            // thread-safe []interface{}
tl.Add(1, 2, 3); tl.At(0); tl.All(); tl.Count()

s := concurr.NewScanner()            // thread-safe index scanner (sync.Map)
s.Add(data); s.Data(); s.Next(); s.At(pos); s.Reset()
```

```go
import "github.com/loopsrage/gotools/lib/index"

idx := index.NewIndex()
idx.StoreInIndex("metrics", "loss", 0.42)
val, err := idx.LoadFromIndex("metrics", "loss")
idx.RangeIndex("metrics", func(k, v interface{}) { ... })
idx.ListIndexes(); idx.DeleteFromIndex("metrics", "loss"); idx.DeleteIndex("metrics")
```

```go
import "github.com/loopsrage/gotools/lib/onceler"
o := onceler.NewOnceler()
o.StoreOnce("idx", "key", func() error { ... })  // runs once per (index, key)
```

```go
import "github.com/loopsrage/gotools/lib/toggle"
t := toggle.NewToggle(false)
t.Set(true); t.Toggle(); t.Load()
```

```go
import "github.com/loopsrage/gotools/lib/clsr"
cc := clsr.NewCleanupComplete(cancelFunc, waitGroup)
clsr.SigClosure(nil, cc, closer1, closer2)  // on SIGINT/SIGTERM run closers then close cc
cc.Complete()
```

```go
import "github.com/loopsrage/gotools/lib/gziplib"
gziplib.IsGzip(buf)
buf, err := gziplib.EnsureGunzip(buf)
data, err := gziplib.Gzip(input)
```

---

## Cloud (AWS, Azure, secrets)

### awslib — AWS SDK v2 clients

Files: `config.go`, `ai.go`, `blob.go`, `secrets.go`, `sql.go`.

```go
import "github.com/loopsrage/gotools/lib/awslib"

cfg, err := awslib.BuildAWSConfig(ctx, region, accessKey, secretKey, sessionToken, httpClient)
// region falls back to AWS_REGION / AWS_DEFAULT_REGION env

ai := &awslib.AwsAi{
    RegionName: "us-east-1",
    AwsAccessKeyID: key,
    AwsSecretAccessKey: secret,
    EndpointURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
}
client, err := awslib.NewAwsAi(*ai)
text, err := client.InvokeModel(ctx, "anthropic.claude-...", "prompt", 512, 0.7, nil)
models, err := client.ListModels(ctx)
```

`blob.go` / `secrets.go` / `sql.go` wrap S3, Secrets Manager, and RDS/Data respectively using the same `BuildAWSConfig` chain.

### azurelib — Azure SDK clients

Files: `auth.go`, `ai.go`, `blob.go`, `secrets.go`, `sql.go`.

```go
import "github.com/loopsrage/gotools/lib/azurelib"

cred, err := azurelib.ResolveAzureCredential(nil)  // explicit → AZURE_ACCESS_TOKEN → DefaultAzureCredential
static := azurelib.StaticTokenCredential("Bearer ...")
```

### secretslib — multi-source secret store

```go
import "github.com/loopsrage/gotools/lib/secretslib"

store := secretslib.NewStore(".env")   // optional .env fallback paths
val := store.Get("API_KEY", "default") // mem cache → env → NAME_FILE → .env → default
store.Set("API_KEY", "v")
store.Delete("API_KEY")
store.Has("API_KEY")
keys := store.ListKeys()   // sorted names, values never exposed
```

---

## AI / LLM / OCR / vision

### ollamalib — Ollama client

```go
import "github.com/loopsrage/gotools/lib/ollamalib"

cl, err := ollamalib.NewOllamaClient(ctx, "nomic-embed-text", "llama3")
resp, err := cl.Embeddings(&api.EmbeddingRequest{Model: cl.EmbeddingModel(), Input: "text"})
req := cl.NewRequest("prompt", nil, nil)
err = cl.Request(req, func(r *api.GenerateRequest) api.GenerateResponseFunc { ... })
cl.Model(); cl.EmbeddingModel()
```

### gmaillib — Gmail API v1 client

Wraps `google.golang.org/api/gmail/v1.Service` with OAuth2 token persistence and rate limiting.

```go
import "github.com/loopsrage/gotools/lib/gmaillib"

c := gmaillib.NewGmailClient(&gmaillib.GmailConfig{UserID: "me"})
svc, err := c.Service(ctx)
msgs, err := c.ListMessages(ctx, "is:unread", gmaillib.IntPtr(50))
ids, err := c.MessageIDs(ctx, "from:boss", nil)
msg, err := c.GetMessage(ctx, id, "full")
_, err = c.Archive(ctx, id); _, err = c.Trash(ctx, id)
_, err = c.MarkRead(ctx, id); _, err = c.MarkUnread(ctx, id)
labelID, _, err := c.LabelID(ctx, "Work")
id, err := c.CreateLabel(ctx, "Work", "labelShow", "show")
```

### ocrlib — swappable OCR provider registry

Providers: Tesseract CLI/TSV, CGo gosseract, remote HTTP/OpenAI vision, Ollama multimodal, go_mltools neural. Thread-safe registry with hot-swapping.

```go
import (
    "github.com/loopsrage/gotools/lib/ocrlib"
)

cfg := ocrlib.DefaultConfig()   // Tesseract default, eng, PSM 6
reg, err := ocrlib.NewDefaultRegistry(ctx, cfg)

reg.Register(myProvider)                       // first registered becomes default
reg.SetDefault("tesseract")
p, ok := reg.Get("tesseract")                  // "" → default
names := reg.Names(); infos := reg.List()      // discovery metadata
err = reg.CloseAll()

res, err := p.Recognize(ctx, ocrlib.RecognizeRequest{
    DocID: "doc1", PageIndex: 0, RegionIndex: 0,
    ImagePNG: pngBytes, Bounds: image.Rect(...),
})
// res.Provider, res.ModelID, res.Text, res.Confidence, res.Tokens, res.Vector, res.LatencyMs
```

Provider configs live in `ocrlib.Config` (`DefaultProvider`, `Tesseract`, `HTTP`, `Ollama`, `MLTools`) with `json`/`yaml` tags.

### visionlib — image & PDF processing

Files: `processor.go`, `processor_gocv.go`, `processor_purego.go`, `pdfrender.go`, `docgen.go`. Dual backend: gocv (OpenCV) and pure Go.

```go
import "github.com/loopsrage/gotools/lib/visionlib"

proc := visionlib.NewProcessor()
png, w, h, err := proc.NormalizeToPNG(data)
boxes, err := proc.SegmentRegions(png, minArea, maxArea)
crop, den, err := proc.CropAndDenoise(png, bounds, preset)
kind, mime, err := visionlib.DetectInputKind(data, mimeType)  // InputPDF / InputImage
rend := visionlib.NewPDFRenderer(maxTotal)
page, err := rend.RenderPage(pdfBytes, pageIndex, dpi)
count, err := rend.PageCount(pdfBytes)
```

### gocvlib — OpenCV effects

Sub-packages: `effects/`, `gocvd/`, `gocvf/`, `utils/` (uses `gocv.io/x/gocv`).

### gosseractlib — Tesseract CGo

Tesseract OCR bindings (`otiai10/gosseract/v2`).

---

## Document / OCR pipelines

### docpipeline — 7-stage indexQueue pipeline

`lib/docpipeline/pipeline.go` (1184 lines). Unified engine for PDF documents and direct images.

Stages: `Ingest → PDFRender → Segment → Denoise → OCR → Persist → Finalize`.

```go
import (
    "github.com/loopsrage/gotools/lib/docpipeline"
    "github.com/loopsrage/gotools/lib/ocrlib"
)

engine, err := docpipeline.NewEngine(parentCtx, cfg, ppsClient, proc, pdfRender, embedder, ocrReg)
defer engine.Close()

result, err := engine.Process(ctx, docpipeline.SubmitRequest{
    Data: pdfBytes, FileName: "scan.pdf", MimeType: "application/pdf",
    OCRProviders: []string{"tesseract", "ollama"}, Segment: &trueVal, DPI: 300,
    FlushPT: true,
})
// result.DocID, result.FullText, result.PageCount, result.RegionCount, result.Pages, result.AuditHeadHash
stats := engine.Stats()   // queue telemetry + abacus counters + ocr_providers
```

Depends on `go_mltools` (`proceduralphasespace/ppsclient`, `visionembed`, `polyphonic/abacus`).

### pdfpipeline — PDFium + tesseract action chain

`lib/pdfpipeline/pipeline/pipeline.go`. Chains `rectAction → pdfium → tesseractAction → finalAction`, each started with `concurr.NewConfig(maxInFlight)`.

```go
import "github.com/loopsrage/gotools/lib/pdfpipeline/pipeline"

queues := pipeline.NewPipeline(ctx, wg, maxInFlight)
// queues.Rect, queues.TessAction, queues.PdfAction, queues.FinalAction, queues.Final
```

Sub-packages: `tesseractAction/`, `rectAction/`, `pdfium/` (incl. `pdfium/worker/main.go`), `finalAction/`, `actionData/`.

---

## Web / HTTP

### weblib — JSON, CORS, WebSocket, SPA

```go
import "github.com/loopsrage/gotools/lib/weblib"

weblib.WriteJSON(w, 200, v)                          // JSON response
weblib.WriteError(w, 400, "bad request")            // {"detail":..., "error":...}
err := weblib.ReadJSON(r, &dst)                     // 16 MiB bound
handler := weblib.CORSMiddleware(origins, next)     // credentials, OPTIONS preflight
ws, err := weblib.AcceptWebSocket(w, r)             // coder/websocket
ws.ReadJSON(ctx, &dst); ws.WriteJSON(ctx, v); ws.Close()
spa := weblib.SPAHandler("./dist", "/ui")           // static + index.html fallback
```

### httplib — minimal response writers

```go
import "github.com/loopsrage/gotools/lib/httplib"

httplib.WriteResponse(w, &body, &status)
httplib.WriteCorsResponse(w, &body, &status)  // JSON + CORS headers
```

### webhooklib, scraperlib, emaillib

Smaller helpers for webhooks, scraping (chromedp-backed), and email.

---

## Databases

### sqllib — reflection-based SQL queries

Uses `database/sql` + `reflectTags` to map rows into structs via `sql.Null[T]` generics. SQL Server-style `SELECT TOP %d`.

```go
import "github.com/loopsrage/gotools/lib/sqllib"

type Row struct { ID int64; Name string; Created time.Time }
var out []interface{}
err := sqllib.SqlQuery(ctx, db, "users", "db", func() interface{} { return &Row{} }, 100)
// note: SqlQuery appends into an internal output; SqlQueryRow fills a single struct
err := sqllib.SqlQueryRow(ctx, db, "users", "db", &row, 1)
```

### mysqllib — MySQL proxy

`lib/mysqllib/mysqlProxy/mysqlProxy.go` — MySQL proxy helpers.

### awslib.sql / azurelib.sql

Cloud RDS / Azure SQL wrappers.

---

## Settings & configuration

### settings — hierarchical YAML/JSON store

Two layers: package-level global `appIndex` (app-name → settings map) and `SettingsStore` (dotted-path YAML/JSON file store).

```go
import "github.com/loopsrage/gotools/lib/settings"

// Global app settings (init() auto-loads env.yaml from CWD or $ENVIRONMENT_FILE)
settings.Add("myapp", configMap)
val := settings.LoadAppSettingString("myapp", "server.port")
settings.RestoreFromFile("env.yaml")
yamlBytes, _ := settings.Yaml("myapp")
envBytes, _ := settings.Env("myapp")

// SettingsStore (dotted paths)
store, err := settings.LoadSettingsStoreFromFile("config.yaml")
port := store.GetInt("server.port", 8080)
host := store.GetString("server.host", "localhost")
ratio := store.GetFloat64("rag.chunk_overlap_ratio", 0.1)
flags := store.GetStringSlice("features.enabled", nil)
store.Set("server.port", 9000)
store.WriteYAMLFile("config.yaml")
raw := store.RawAll(); j, _ := store.ToJSON()
```

`settings.JsonUnmarshalSettingsTo(appName, appConfig)` populates a struct from the global app index (used by `configBase`).

### configBase — FeatureController + ConfigBase

```go
import "github.com/loopsrage/gotools/lib/configBase"

type MyFeature struct { ... }
fc := configBase.NewFeatureController(myFeature)
// fc.Controller (ticker), fc.ConfigBase (feature settings + enabled flag)
// settings auto-loaded from JsonUnmarshalSettingsTo by feature source name
```

### featureSettings

Feature settings aggregation used by `configBase`.

---

## Feature flags

```go
import "github.com/loopsrage/gotools/lib/featureflags"

store := featureflags.NewStore()
store.Set(featureflags.FeatureFlag{
    Name: "new-ui", Enabled: true, RolloutPct: 50,
    TargetUsers: []string{"alice"}, TargetAttrs: map[string]string{"tier": "pro"},
})
active := store.Evaluate("new-ui", "alice", map[string]string{"tier": "pro"})
flag, ok := store.Get("new-ui")
store.List(); store.Delete("new-ui")
```

Evaluation: target users → target attrs → rollout % (deterministic SHA-256 bucket of `flagName:userID`).

---

## Service lifecycle & controllers

### serviceController — ticker-driven service loop

```go
import "github.com/loopsrage/gotools/lib/serviceController"

errs := serviceController.Service(ctx, controller, action, notifyComplete...)
// loops on controller.Ticker().C, calls action.ServiceLoop(), Reset on each tick
```

### controller — time.Ticker with reset + clsr cleanup

```go
import (
    "github.com/loopsrage/gotools/lib/clsr"
    "github.com/loopsrage/gotools/lib/controller"
    "time"
)

cc := clsr.NewCleanupComplete(cancelFunc, waitGroup)
ctrl := controller.NewController(cc, 100*time.Millisecond, false)
ctrl.Ticker(); ctrl.Reset(proceed); ctrl.Close()
```

### appcontroller, queueController, onceler

App-level wiring, queue data/controller, and `sync.Once`-per-key execution (see core helpers).

---

## Resilience

```go
import "github.com/loopsrage/gotools/lib/resilience"

cb := resilience.NewCircuitBreaker("payments", 5, 30*time.Second)
err := cb.Execute(func() error { return riskyCall() })
cb.State(); cb.RecordFailure(); cb.RecordSuccess(); cb.Reset(); cb.Status()

rl := resilience.NewRateLimiter("api", 10.0, 20)
ok := rl.Allow(); rl.Status()

err := resilience.Retry(ctx, resilience.RetryConfig{MaxAttempts: 3, BaseDelay: 50*time.Millisecond, MaxDelay: 5*time.Second}, fn)

// Combined: rate limiter → circuit breaker → retry
err := resilience.Do(ctx, rl, cb, &retry, fn)
```

States: `closed` → `open` (after FailureThreshold) → `half_open` (after RecoveryTimeout) → `closed` on success.

---

## MCP server

`lib/mcplib/mcplib.go` — JSON-RPC 2.0 Model Context Protocol server over HTTP (single + batch) and stdio. Protocol version `2024-11-05`.

```go
import (
    "net/http"
    "github.com/loopsrage/gotools/lib/mcplib"
)

srv := mcplib.NewServer("my-server", "1.0.0")
srv.RegisterTool(mcplib.Tool{
    Name: "echo", Description: "echo back text",
    InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
    Handler: func(ctx context.Context, args map[string]any) (any, error) {
        return map[string]any{"echo": args["text"]}, nil
    },
})
srv.RegisterResource(mcplib.Resource{URI: "file:///readme", Name: "Readme", Handler: func(ctx, uri) (string, string, error) {...}})
srv.RegisterPrompt(mcplib.Prompt{Name: "greet", Arguments: []mcplib.PromptArgument{{Name: "name", Required: true}}, Handler: ...})

http.Handle("/", srv)   // GET descriptor, POST JSON-RPC
// or: srv.RunStdio(ctx, os.Stdin, os.Stdout)

resp := srv.HandleRequest(ctx, mcplib.Request{JSONRPC: "2.0", ID: 1, Method: "tools/list"})
```

Methods: `initialize`, `ping`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `prompts/list`, `prompts/get`.

---

## Data / indexing / reflection

### reflectStruct — flatten/nested map ↔ struct

```go
import "github.com/loopsrage/gotools/lib/reflectStruct"

flat, err := reflectStruct.MarshallMap(nestedMap)   // dot-separated keys, arr[i] indices
err := reflectStruct.FindFields(appConfigPtr, flat) // populate struct by field name (case-insensitive)
```

### reflectTags — struct tag reflection

Used by `sqllib` to discover columns/tags and build `sql.Null[T]` scan args.

### jsonContainers — JSON container tree builder + searcher

`container/`, `containerTreeBuilder/`, `searcher/` — build and flatten nested JSON containers (used by `settings.Env`).

### jsonIndexer, index, informationalindex, dataCollection, dataWrapper

Indexing and data container primitives. `index` is the sync.Map sub-index (see core helpers).

---

## Indexing / queues

### indexQueue — multi-stage concurrent work queue

Used by `docpipeline`. `indexQueue.NewIndexQueue(ctx, actions)`, `NamedStage`, `IndexQueueActions`, per-stage worker counts, `EnqueueContext`, `Flush`, `InFlight`, `Completed`, `Total`.

```go
import "github.com/loopsrage/gotools/lib/indexQueue"

actions := indexQueue.NewIndexQueueActions(ctx, &wg, 4, stages...)
iq := indexQueue.NewIndexQueue(ctx, actions)
iq.Start(); iq.EnqueueContext(ctx, stage, id, task); iq.Flush(stage)
```

---

## Small utilities

- `measureTime` — timing measurement helpers.
- `timelib` — iterate time windows start→end or end→start (see core helpers).
- `quickTimeFormats` — quick time format helpers.
- `idlib` — ID helpers.
- `fileslib` — file helpers (`CalculateLocalMD5` used by docpipeline).
- `fuzzy` — fuzzy matching.
- `gclib` — GC helpers.
- `benchmarkledger` — benchmark recording ledger.
- `auditlib`, `etllib`, `oauthsession`, `webhooklib` — domain-specific helpers.
- `gen` — code generation nodes / `QueueActionWrapper`.
- `appcontroller` — app-level controller composition.

---

## Full package index (lib/, 55 packages)

| Category | Packages |
|---|---|
| Core helpers | concurr, index, onceler, toggle, timelib, controller, catchPanic, clsr, gziplib |
| Cloud | awslib, azurelib, secretslib |
| AI/LLM/OCR | ollamalib, gmaillib, ocrlib, visionlib, gocvlib, gosseractlib |
| Pipelines | docpipeline, pdfpipeline, etllib |
| Web/HTTP | weblib, httplib, webhooklib, scraperlib, emaillib |
| Databases | sqllib, mysqllib (+ awslib.sql, azurelib.sql) |
| Settings | settings, configBase, featureSettings, featureflags |
| Lifecycle | serviceController, controller, appcontroller, queueController, onceler |
| Resilience | resilience |
| MCP | mcplib |
| Data/index/reflection | indexQueue, jsonContainers, jsonIndexer, informationalindex, dataCollection, dataWrapper, reflectStruct, reflectTags |
| Utilities | measureTime, timelib, quickTimeFormats, idlib, fileslib, fuzzy, gclib, benchmarkledger, auditlib, gen |

Tests mirror `lib/` under `tests/` (34 package test dirs).

---

## Verification commands

Run from the repo root:

```sh
go build ./...        # all packages compile
go vet ./...          # no issues
go test ./...         # all tests pass
go test -race ./...   # race-clean
go test ./lib/<pkg>/...   # single package
```

Note: some packages (`gocvlib`, `gosseractlib`, `ocrlib` cgo variant, `pdfpipeline/pdfium`) require CGO/OpenCV/Tesseract system libraries; pure-Go packages build without them. `docpipeline` imports `go_mltools` which must be resolvable in the module graph.

---

## Quick start example (end-to-end)

```go
package main

import (
    "context"
    "fmt"
    "github.com/loopsrage/gotools/lib/secretslib"
    "github.com/loopsrage/gotools/lib/ocrlib"
)

func main() {
    ctx := context.Background()
    store := secretslib.NewStore(".env")
    apiKey := store.Get("OCR_API_KEY", "")

    reg, _ := ocrlib.NewDefaultRegistry(ctx, ocrlib.DefaultConfig())
    defer reg.CloseAll()

    res, err := reg.Get("").Recognize(ctx, ocrlib.RecognizeRequest{
        DocID: "doc1", PageIndex: 0, ImagePNG: pngBytes,
    })
    fmt.Println(res.Provider, res.Text, res.Confidence)
}
```
