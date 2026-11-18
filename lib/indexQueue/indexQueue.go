package indexQueue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/loopsrage/gotools/lib/catchPanic"
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/index"
	"github.com/loopsrage/gotools/lib/queueController"
)

const stageActionsBucket = "STAGE_ACTIONS"

// WorkerServiceConfig configures background stage flushing and concurrency for an ActionConfig.
type WorkerServiceConfig struct {
	Identity       string
	WorkerCount    int64
	WorkerInterval time.Duration
	StartNow       bool
}

// ActionConfig binds a named stage's QueueController, worker config, and flush callback.
type ActionConfig struct {
	Config      *WorkerServiceConfig
	Queue       *queueController.QueueController
	Wrapper     *queueController.QueueActionWrapper
	WorkerCount int64
	Flush       func() int
}

// StageFunc is the callback signature executed for each dequeued item in a stage.
type StageFunc func(iq *IndexQueue, qd *queueController.QueueData) error

// StageSpec associates a stage action key with its execution function.
type StageSpec struct {
	Name string
	Fn   StageFunc
}

// NamedStage creates a StageSpec for NewIndexQueueActions.
func NamedStage(name string, fn StageFunc) StageSpec {
	return StageSpec{Name: name, Fn: fn}
}

type stageRunner struct {
	name string
	fn   StageFunc
}

func (s *stageRunner) QueueAction(qd *queueController.QueueData) (retErr error) {
	var iq *IndexQueue
	if raw := qd.Attribute("index_queue"); raw != nil {
		iq, _ = raw.(*IndexQueue)
	}
	actKey := qd.String("action_key")
	if actKey == "" {
		actKey = s.name
	}
	if iq != nil {
		defer iq.TaskComplete(actKey)
	}
	catchPanic.CatchPanic(func() {
		retErr = s.fn(iq, qd)
	}, func(recovered interface{}) {
		retErr = fmt.Errorf("indexQueue: stage %q panic: %v", actKey, recovered)
	})
	return retErr
}

// NewIndexQueueActions builds a stage map of ActionConfigs backed by QueueActionWrappers,
// matching toolset's new_index_queue(worker_count, *actions).
func NewIndexQueueActions(ctx context.Context, wg *sync.WaitGroup, workerCount int64, stages ...StageSpec) map[string]*ActionConfig {
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	if workerCount <= 0 {
		workerCount = 2
	}
	actions := make(map[string]*ActionConfig, len(stages))
	for _, st := range stages {
		runner := &stageRunner{name: st.Name, fn: st.Fn}
		wrapper := queueController.NewQueueActionWrapper(ctx, wg, runner)
		wrapper.SetDebug(false)
		actions[st.Name] = &ActionConfig{
			Queue:       wrapper.QueueController,
			Wrapper:     wrapper,
			WorkerCount: workerCount,
		}
	}
	return actions
}

// IndexQueue manages backpressure-bounded, multi-stage task queues backed by index.Index
// and queueController.QueueController.
type IndexQueue struct {
	Index        *index.Index
	MaxIndexSize int64
	OnProgress   func(completed, total int64)

	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	cond       *sync.Cond
	startOnce  sync.Once
	closed     bool
	producerWG sync.WaitGroup

	inFlightTasks  map[string]int64
	maxSizes       map[string]int64
	completedTasks int64
	totalTasks     int64
}

// NewIndexQueue initializes an IndexQueue from a stage ActionConfig map.
func NewIndexQueue(parentCtx context.Context, actions map[string]*ActionConfig) *IndexQueue {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	iq := &IndexQueue{
		Index:         index.NewIndex(),
		MaxIndexSize:  256,
		ctx:           ctx,
		cancel:        cancel,
		inFlightTasks: make(map[string]int64, len(actions)),
		maxSizes:      make(map[string]int64, len(actions)),
	}
	iq.cond = sync.NewCond(&iq.mu)
	iq.Index.New(stageActionsBucket)

	for name, cfg := range actions {
		if cfg == nil {
			continue
		}
		iq.Index.New(name)
		iq.inFlightTasks[name] = 0
		iq.maxSizes[name] = iq.MaxIndexSize
		wc := cfg.WorkerCount
		if wc <= 0 {
			wc = 2
		}
		if cfg.Config == nil {
			cfg.Config = &WorkerServiceConfig{
				Identity:       name,
				WorkerCount:    wc,
				WorkerInterval: 2 * time.Second,
				StartNow:       true,
			}
		}
		stageName := name
		cfg.Flush = func() int {
			return iq.Flush(stageName)
		}
		iq.StoreStageAction(name, cfg)
	}
	return iq
}

// Start launches all registered QueueController worker pools and periodic stage flush loops.
func (iq *IndexQueue) Start() {
	iq.startOnce.Do(func() {
		var stages []struct {
			name string
			cfg  *ActionConfig
		}
		_ = iq.Index.RangeIndex(stageActionsBucket, func(key, value interface{}) {
			name, ok1 := key.(string)
			cfg, ok2 := value.(*ActionConfig)
			if ok1 && ok2 && cfg != nil {
				stages = append(stages, struct {
					name string
					cfg  *ActionConfig
				}{name: name, cfg: cfg})
			}
		})

		for _, st := range stages {
			wc := st.cfg.WorkerCount
			if wc <= 0 && st.cfg.Config != nil && st.cfg.Config.WorkerCount > 0 {
				wc = st.cfg.Config.WorkerCount
			}
			if wc <= 0 {
				wc = 2
			}
			if st.cfg.Queue != nil {
				st.cfg.Queue.Start(concurr.NewConfig(wc))
			}
			if st.cfg.Config != nil && st.cfg.Config.StartNow && st.cfg.Config.WorkerInterval > 0 {
				iq.producerWG.Add(1)
				go iq.runStageProducer(st.name, st.cfg.Config.WorkerInterval)
			}
		}
	})
}

func (iq *IndexQueue) runStageProducer(actionKey string, interval time.Duration) {
	defer iq.producerWG.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-iq.ctx.Done():
			return
		case <-ticker.C:
			iq.Flush(actionKey)
		}
	}
}

// StoreStageAction registers or replaces a stage ActionConfig in STAGE_ACTIONS.
func (iq *IndexQueue) StoreStageAction(name string, cfg *ActionConfig) {
	iq.Index.StoreInIndex(stageActionsBucket, name, cfg)
}

// LoadStageAction retrieves a registered stage ActionConfig by name.
func (iq *IndexQueue) LoadStageAction(name string) *ActionConfig {
	val, err := iq.Index.LoadFromIndex(stageActionsBucket, name)
	if err != nil || val == nil {
		return nil
	}
	cfg, _ := val.(*ActionConfig)
	return cfg
}

// SetMaxSize configures the maximum concurrent in-flight task capacity for actionKey.
func (iq *IndexQueue) SetMaxSize(actionKey string, maxSize int64) {
	if maxSize <= 0 {
		maxSize = 1
	}
	iq.mu.Lock()
	iq.maxSizes[actionKey] = maxSize
	iq.cond.Broadcast()
	iq.mu.Unlock()
}

// Enqueue adds an item to actionKey's index stage, blocking when in-flight capacity is full.
func (iq *IndexQueue) Enqueue(actionKey, key string, value interface{}) (string, error) {
	return iq.EnqueueContext(iq.ctx, actionKey, key, value)
}

// EnqueueContext adds an item to actionKey's index stage, respecting ctx cancellation while waiting on backpressure.
func (iq *IndexQueue) EnqueueContext(ctx context.Context, actionKey, key string, value interface{}) (string, error) {
	if actionKey == "" {
		return "", errors.New("indexQueue: actionKey cannot be empty")
	}
	if ctx == nil {
		ctx = iq.ctx
	}
	stopWake := context.AfterFunc(ctx, func() {
		iq.cond.Broadcast()
	})
	defer stopWake()

	var progressCb func(int64, int64)
	var completedSnap, totalSnap int64

	iq.mu.Lock()
	if _, exists := iq.inFlightTasks[actionKey]; !exists {
		iq.inFlightTasks[actionKey] = 0
		iq.maxSizes[actionKey] = 100
	}
	for !iq.closed && ctx.Err() == nil && iq.inFlightTasks[actionKey] >= iq.maxSizes[actionKey] {
		iq.cond.Wait()
	}
	if iq.closed {
		iq.mu.Unlock()
		return "", errors.New("indexQueue: queue is closed")
	}
	if err := ctx.Err(); err != nil {
		iq.mu.Unlock()
		return "", err
	}

	iq.inFlightTasks[actionKey]++
	totalCurrent := iq.sumInFlightLocked()
	if candidate := totalCurrent + iq.completedTasks; candidate > iq.totalTasks {
		iq.totalTasks = candidate
	}
	progressCb = iq.OnProgress
	completedSnap = iq.completedTasks
	totalSnap = iq.totalTasks
	iq.mu.Unlock()

	if key == "" {
		key = newHexUUID()
	}
	iq.Index.StoreInIndex(actionKey, key, value)

	if progressCb != nil {
		progressCb(completedSnap, totalSnap)
	}
	return key, nil
}

// Flush drains all pending items stored in actionKey's index into its stage QueueController.
func (iq *IndexQueue) Flush(actionKey string) int {
	if iq.ctx.Err() != nil {
		return 0
	}
	stageCfg := iq.LoadStageAction(actionKey)
	if stageCfg == nil || stageCfg.Queue == nil {
		return 0
	}
	if stageCfg.Queue.CheckContext() != nil {
		return 0
	}
	subMap := iq.Index.LoadIndex(actionKey)
	if subMap == nil {
		return 0
	}

	flushed := 0
	subMap.Range(func(k, _ any) bool {
		if iq.ctx.Err() != nil || stageCfg.Queue.CheckContext() != nil {
			return false
		}
		val, loaded := subMap.LoadAndDelete(k)
		if !loaded {
			return true
		}
		qd := queueController.NewQueueData()
		qd.SetAttributes(map[string]interface{}{
			"action_key":  actionKey,
			"key":         k,
			"obj":         val,
			"index_queue": iq,
		})
		stageCfg.Queue.SendSelf(qd)
		flushed++
		return true
	})
	return flushed
}

// FlushAll drains pending items across all registered stages.
func (iq *IndexQueue) FlushAll() int {
	var names []string
	_ = iq.Index.RangeIndex(stageActionsBucket, func(key, _ interface{}) {
		if name, ok := key.(string); ok {
			names = append(names, name)
		}
	})
	total := 0
	for _, n := range names {
		total += iq.Flush(n)
	}
	return total
}

// TaskComplete decrements in-flight count for actionKey and wakes blocked producers and completion waiters.
func (iq *IndexQueue) TaskComplete(actionKey string) {
	var progressCb func(int64, int64)
	var completedSnap, totalSnap int64

	iq.mu.Lock()
	if iq.inFlightTasks[actionKey] > 0 {
		iq.inFlightTasks[actionKey]--
		iq.completedTasks++
		progressCb = iq.OnProgress
		completedSnap = iq.completedTasks
		totalSnap = iq.totalTasks
		iq.cond.Broadcast()
	}
	iq.mu.Unlock()

	if progressCb != nil {
		progressCb(completedSnap, totalSnap)
	}
}

// WaitForCompletions blocks until all in-flight tasks across all stages reach zero or ctx is cancelled.
func (iq *IndexQueue) WaitForCompletions(ctx context.Context) error {
	if ctx == nil {
		ctx = iq.ctx
	}
	stopWake := context.AfterFunc(ctx, func() {
		iq.cond.Broadcast()
	})
	defer stopWake()

	iq.mu.Lock()
	defer iq.mu.Unlock()
	for iq.sumInFlightLocked() > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if iq.closed {
			return errors.New("indexQueue: closed while tasks still in flight")
		}
		iq.cond.Wait()
	}
	return nil
}

// InFlight returns the current in-flight count for a specific stage.
func (iq *IndexQueue) InFlight(actionKey string) int64 {
	iq.mu.Lock()
	defer iq.mu.Unlock()
	return iq.inFlightTasks[actionKey]
}

// TotalInFlight returns the sum of in-flight tasks across all stages.
func (iq *IndexQueue) TotalInFlight() int64 {
	iq.mu.Lock()
	defer iq.mu.Unlock()
	return iq.sumInFlightLocked()
}

// Completed returns the total number of completed tasks.
func (iq *IndexQueue) Completed() int64 {
	iq.mu.Lock()
	defer iq.mu.Unlock()
	return iq.completedTasks
}

// Total returns the high-water total task count tracked across the queue's lifetime.
func (iq *IndexQueue) Total() int64 {
	iq.mu.Lock()
	defer iq.mu.Unlock()
	return iq.totalTasks
}

func (iq *IndexQueue) sumInFlightLocked() int64 {
	var sum int64
	for _, c := range iq.inFlightTasks {
		sum += c
	}
	return sum
}

// Close cancels background producer loops and unblocks all waiting goroutines.
func (iq *IndexQueue) Close() {
	iq.mu.Lock()
	iq.closed = true
	iq.cond.Broadcast()
	iq.mu.Unlock()

	iq.cancel()
	iq.producerWG.Wait()
}

func newHexUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:])
}
