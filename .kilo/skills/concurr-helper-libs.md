# Kilo Skill: concurr & helper libraries (go_mltools/gotools)

Applies to: `go_mltools/lib/` and `gotools/lib/`

This skill documents how to use the shared Go helper libraries that underpin `go_mltools` and `gotools`: `concurr`, `index`, `onceler`, `toggle`, `timelib`, `controller`, `catchPanic`, `clsr`, and `gziplib`. These packages are pure-Go (stdlib only) and are designed to be composed into neural network components, game engine systems, and data pipelines.

---

## concurr — semaphore-limited concurrency & thread-safe collections

Package path: `lib/concurr` (module `github.com/loopsrage/gotools`)

### Purpose
Run work concurrently with a bounded number of in-flight goroutines (semaphore), plus thread-safe `Scanner`, `TsList`, and `ActionQueue` data structures. All hot loops in `go_mltools` neural components use `concurr` to parallelize per-element, per-row, per-head, per-timestep, and per-batch work.

### Config
```go
import "github.com/loopsrage/gotools/lib/concurr"

// NewConfig(maxInFlight) creates a semaphore-limited config.
cfg := concurr.NewConfig(8) // at most 8 goroutines in flight

// Acquire / Release around work you want to limit.
cfg.Acquire(ctx)
defer cfg.Release()
```

### ActionOn — parallel map over values
```go
type Action interface{ Action(on interface{}) }

// ActionOn(ctx, config, action, on...) runs action.Action(on[i]) for each
// value, semaphore-limited by config.
concurr.ActionOn(ctx, cfg, myAction, a, b, c)
```

### ActionQueue — channel-driven processing
```go
// ActionQueue(ctx, wg, config, action, queueChan) consumes from a channel,
// running each item via runAction (semaphore-limited).
concurr.ActionQueue(ctx, wg, config, action, queueChan)
```

### Scanner — thread-safe index-based data scanner (sync.Map)
```go
s := concurr.NewScanner()
s.Add(data)              // stores at next index, increments count
pos, val := s.Data()     // returns current position + value, advances
s.Count()                // number of items added
s.Next()                 // true while index > position
s.At(position)           // value at position
s.Reset()                // clear all state
```

### TsList — thread-safe list
```go
tl := concurr.NewTsList()
tl.Add(1, 2, 3)          // appends, returns count before add
tl.Set(0, 100)           // set element at position
tl.At(0)                 // get element
tl.All()                 // copy of all elements
tl.Count()               // length
```

### Example: parallel per-row matrix work
```go
func ParallelRows(rows int, fn func(int)) {
    cfg := concurr.NewConfig(int64(rows))
    var wg sync.WaitGroup
    for i := 0; i < rows; i++ {
        wg.Add(1)
        go func(ii int) {
            defer wg.Done()
            defer cfg.Release()
            cfg.Acquire(context.Background())
            fn(ii)
        }(i)
    }
    wg.Wait()
}
```

---

## index — sync.Map-backed named key-value index

Package path: `lib/index`

### Purpose
Named sub-indexes backed by `sync.Map` for reduced lock contention. Used in `go_mltools` to replace hand-rolled mutex maps in `shared_index`, `metrics_index`, and `corpus_sweep`.

```go
import "github.com/loopsrage/gotools/lib/index"

idx := index.NewIndex()

// Create/load a named sub-index and store a key-value pair.
idx.StoreInIndex("metrics", "loss", 0.42)

// Atomically get-or-create sub-map then LoadOrStore.
actual, loaded := idx.LoadOrStoreInIndex("metrics", "loss", 0.42)

// Load a value.
val, err := idx.LoadFromIndex("metrics", "loss")

// Range over a sub-index.
idx.RangeIndex("metrics", func(k, v interface{}) {
    fmt.Println(k, v)
})

// List all index names.
names := idx.ListIndexes()

// Delete a key or whole index.
idx.DeleteFromIndex("metrics", "loss")
idx.DeleteIndex("metrics")
```

---

## onceler — sync.Once-per-key execution

Package path: `lib/onceler`

### Purpose
Ensure a function runs only once per `(index, key)` pair. Built on top of `index.Index`.

```go
import "github.com/loopsrage/gotools/lib/onceler"

o := onceler.NewOnceler()

// StoreOnce runs do() once for the given (index, key).
err := o.StoreOnce("myindex", "mykey", func() error {
    // expensive one-time init
    return nil
})
// Subsequent calls with same (index, key) are no-ops.
```

---

## toggle — atomic boolean switch

Package path: `lib/toggle`

### Purpose
Atomic boolean switch with `Set`, `Toggle`, and `Load`.

```go
import "github.com/loopsrage/gotools/lib/toggle"

t := toggle.NewToggle(false)
t.Set(true)
t.Toggle()   // flips and returns new state
t.Load()     // current bool
```

---

## timelib — time-window iteration

Package path: `lib/timelib`

### Purpose
Iterate time windows from start→end or end→start.

```go
import "github.com/loopsrage/gotools/lib/timelib"

type myTarget struct{}
func (m *myTarget) Window(t time.Time) { fmt.Println(t) }

// From start to end in windowSize increments.
timelib.FromStartToEnd(&myTarget{}, start, end, windowSize)

// From end to start.
timelib.FromEndToStart(&myTarget{}, start, end, windowSize)
```

---

## controller — ticker-based controller with reset

Package path: `lib/controller`

### Purpose
Wraps `time.Ticker` with interval-based reset and signal-driven cleanup (via `clsr`).

```go
import (
    "github.com/loopsrage/gotools/lib/clsr"
    "github.com/loopsrage/gotools/lib/controller"
    "time"
)

cc := clsr.NewCleanupComplete(cancelFunc, waitGroup)
ctrl := controller.NewController(cc, 100*time.Millisecond, false)

// ctrl.Ticker()  -> *time.Ticker
// ctrl.Reset(proceed)
// ctrl.Close()
```

---

## catchPanic — panic recovery with callback

Package path: `lib/catchPanic`

### Purpose
Recover from a panic in `f` and pass the recovered value to `cb`.

```go
import "github.com/loopsrage/gotools/lib/catchPanic"

catchPanic.CatchPanic(func() {
    // might panic
}, func(recovered interface{}) {
    // handle recovery
})
```

---

## clsr — signal-triggered cleanup

Package path: `lib/clsr`

### Purpose
Listen for OS signals (SIGTERM, SIGINT) and run cleanup closers once.

```go
import "github.com/loopsrage/gotools/lib/clsr"

cc := clsr.NewCleanupComplete(cancelFunc, waitGroup)
clsr.SigClosure(nil, cc, closer1, closer2)
// On signal: runs all closers, then closes cc.complete.
cc.Complete() // blocks until cleanup done
```

---

## gziplib — gzip compression/decompression

Package path: `lib/gziplib`

### Purpose
Gzip compress/decompress buffers with auto-detection.

```go
import "github.com/loopsrage/gotools/lib/gziplib"

gziplib.IsGzip(buffer)                      // true if gzip magic bytes
buf, err := gziplib.EnsureGunzip(buffer)    // decompress if gzipped
buf, err := gziplib.EnsureGunzipRW(rw)      // decompress read/write buffer
data, err := gziplib.Gzip(input)            // compress bytes
```

---

## Usage patterns in go_mltools

All neural_networks components that do per-element, per-row, per-head, per-timestep, or per-batch computation now use `concurr.ActionOn`-style semaphore-limited goroutines. Examples:

- `components/router/router.go` — SpectralEntropyRouter batch elements (8 in flight)
- `components/attention/attention.go` — per-sequence-position attention scores
- `components/kan/kan.go` — per-feature B-spline basis + per-output-feature accumulation
- `components/ssm/ssm.go` — S4Block/Mamba2Block per-timestep & per-state ops
- `components/modern/modern.go` — attention scores/softmax/FF residuals
- `components/neural_ode/neural_ode.go` — Forward/RK4Step/addScalarMul per element
- `common/hyperbolicmath/hyperbolicmath.go` — SafeHyperbolicProject, MobiusAddition, etc.

**Verification commands** (run in `go_mltools`):
```sh
go build ./...      # all packages compile
go vet ./...        # no issues
go test ./...       # all tests pass
go test -race ./... # race-clean
```

**Module note:** `go_mltools` imports these as `go_mltools/lib/<pkg>`; `gotools` imports them as `github.com/loopsrage/gotools/lib/<pkg>`.
