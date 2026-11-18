package auditlib

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/index"
)

// AuditEntry represents a single immutable, SHA-256 hash-chained audit record.
type AuditEntry struct {
	EntryID      string                 `json:"entry_id"`
	Timestamp    string                 `json:"timestamp"`
	Actor        string                 `json:"actor"`
	Action       string                 `json:"action"`
	Resource     string                 `json:"resource"`
	Details      map[string]interface{} `json:"details"`
	PreviousHash *string                `json:"previous_hash"`
	CurrentHash  string                 `json:"current_hash"`
}

// AuditEntryOption customizes optional fields when constructing an AuditEntry.
type AuditEntryOption func(*AuditEntry)

// WithEntryID sets an explicit entry ID on an AuditEntry.
func WithEntryID(entryID string) AuditEntryOption {
	return func(e *AuditEntry) {
		e.EntryID = entryID
	}
}

// WithPreviousHash sets the previous chain hash on an AuditEntry.
func WithPreviousHash(prevHash *string) AuditEntryOption {
	return func(e *AuditEntry) {
		if prevHash == nil {
			e.PreviousHash = nil
			return
		}
		cp := *prevHash
		e.PreviousHash = &cp
	}
}

// WithTimestamp sets an explicit ISO-8601 UTC timestamp on an AuditEntry.
func WithTimestamp(ts string) AuditEntryOption {
	return func(e *AuditEntry) {
		e.Timestamp = ts
	}
}

// NewAuditEntry constructs an AuditEntry and computes its canonical SHA-256 CurrentHash.
func NewAuditEntry(actor, action, resource string, details map[string]interface{}, opts ...AuditEntryOption) *AuditEntry {
	e := &AuditEntry{
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Details:  cloneMap(details),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	if e.EntryID == "" {
		e.EntryID = newEntryID()
	}
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.Details == nil {
		e.Details = map[string]interface{}{}
	}
	e.CurrentHash = e.ComputeHash()
	return e
}

// ComputeHash calculates the deterministic SHA-256 hex digest over the entry's canonical JSON payload.
func (e *AuditEntry) ComputeHash() string {
	details := e.Details
	if details == nil {
		details = map[string]interface{}{}
	}
	var prev interface{}
	if e.PreviousHash != nil {
		prev = *e.PreviousHash
	}
	payload := map[string]interface{}{
		"action":        e.Action,
		"actor":         e.Actor,
		"details":       details,
		"entry_id":      e.EntryID,
		"previous_hash": prev,
		"resource":      e.Resource,
		"timestamp":     e.Timestamp,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("auditlib: failed to marshal canonical audit payload: %v", err))
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// ToMap serializes the AuditEntry into a map suitable for storage in index.Index.
func (e *AuditEntry) ToMap() map[string]interface{} {
	var prev interface{}
	if e.PreviousHash != nil {
		prev = *e.PreviousHash
	}
	return map[string]interface{}{
		"entry_id":      e.EntryID,
		"timestamp":     e.Timestamp,
		"actor":         e.Actor,
		"action":        e.Action,
		"resource":      e.Resource,
		"details":       cloneMap(e.Details),
		"previous_hash": prev,
		"current_hash":  e.CurrentHash,
	}
}

// AuditEntryFromMap reconstructs an AuditEntry from a map and recomputes CurrentHash
// so Verify() detects any field or payload tampering.
func AuditEntryFromMap(data map[string]interface{}) (*AuditEntry, error) {
	if data == nil {
		return nil, errors.New("auditlib: entry map is nil")
	}
	actor, ok1 := data["actor"].(string)
	action, ok2 := data["action"].(string)
	resource, ok3 := data["resource"].(string)
	entryID, ok4 := data["entry_id"].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 || entryID == "" {
		return nil, errors.New("auditlib: missing required string fields (actor, action, resource, entry_id)")
	}

	var details map[string]interface{}
	if rawDetails, exists := data["details"]; exists && rawDetails != nil {
		m, ok := rawDetails.(map[string]interface{})
		if !ok {
			return nil, errors.New("auditlib: details must be map[string]interface{}")
		}
		details = m
	} else {
		details = map[string]interface{}{}
	}

	var opts []AuditEntryOption
	opts = append(opts, WithEntryID(entryID))

	if rawPrev, exists := data["previous_hash"]; exists && rawPrev != nil {
		switch p := rawPrev.(type) {
		case string:
			opts = append(opts, WithPreviousHash(&p))
		case *string:
			opts = append(opts, WithPreviousHash(p))
		default:
			return nil, errors.New("auditlib: invalid previous_hash type")
		}
	}

	if rawTS, exists := data["timestamp"]; exists && rawTS != nil {
		ts, ok := rawTS.(string)
		if !ok {
			return nil, errors.New("auditlib: invalid timestamp type")
		}
		opts = append(opts, WithTimestamp(ts))
	}

	return NewAuditEntry(actor, action, resource, details, opts...), nil
}

// AuditLog maintains an append-only, SHA-256 hash-chained audit trail backed by index.Index.
type AuditLog struct {
	mu      sync.RWMutex
	index   *index.Index
	order   *concurr.TsList
	name    string
	lockKey string
	headKey string
}

type headState struct {
	previousHash *string
	count        int
}

// NewAuditLog initializes a named hash-chained AuditLog (defaulting to "audit" when name is empty).
func NewAuditLog(name string) *AuditLog {
	if name == "" {
		name = "audit"
	}
	l := &AuditLog{
		index:   index.NewIndex(),
		order:   concurr.NewTsList(),
		name:    name,
		lockKey: name + ":lock",
		headKey: name + ":head",
	}
	l.index.New(name)
	l.ensureHeadLocked()
	return l
}

// Name returns the sub-index bucket name used by this AuditLog.
func (l *AuditLog) Name() string { return l.name }

// Index exposes the underlying index.Index storing head state and entry maps.
func (l *AuditLog) Index() *index.Index { return l.index }

func (l *AuditLog) ensureHeadLocked() {
	val, err := l.index.LoadFromIndex(l.name, l.headKey)
	if err != nil || val == nil {
		l.storeHeadLocked(headState{previousHash: nil, count: 0})
	}
}

func (l *AuditLog) headLocked() headState {
	val, err := l.index.LoadFromIndex(l.name, l.headKey)
	if err != nil || val == nil {
		return headState{previousHash: nil, count: 0}
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return headState{previousHash: nil, count: 0}
	}
	var prev *string
	if rawPrev, exists := m["previous_hash"]; exists && rawPrev != nil {
		switch p := rawPrev.(type) {
		case string:
			cp := p
			prev = &cp
		case *string:
			if p != nil {
				cp := *p
				prev = &cp
			}
		}
	}
	cnt := 0
	switch c := m["count"].(type) {
	case int:
		cnt = c
	case int64:
		cnt = int(c)
	case float64:
		cnt = int(c)
	}
	return headState{previousHash: prev, count: cnt}
}

func (l *AuditLog) storeHeadLocked(h headState) {
	var prev interface{}
	if h.previousHash != nil {
		prev = *h.previousHash
	}
	l.index.StoreInIndex(l.name, l.headKey, map[string]interface{}{
		"previous_hash": prev,
		"count":         h.count,
	})
}

// Append creates a new chained AuditEntry, persists it into Index, advances head, and returns it.
func (l *AuditLog) Append(actor, action, resource string, details map[string]interface{}) *AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	head := l.headLocked()
	entry := NewAuditEntry(actor, action, resource, details, WithPreviousHash(head.previousHash))
	l.index.StoreInIndex(l.name, entry.EntryID, entry.ToMap())
	l.order.Add(entry.EntryID)

	curHash := entry.CurrentHash
	l.storeHeadLocked(headState{
		previousHash: &curHash,
		count:        head.count + 1,
	})
	return entry
}

// Get retrieves the stored map for entryID, if present.
func (l *AuditLog) Get(entryID string) (map[string]interface{}, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	val, err := l.index.LoadFromIndex(l.name, entryID)
	if err != nil || val == nil {
		return nil, false
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return cloneMap(m), true
}

// Entries returns all stored audit entry maps in append order (including any unindexed entries).
func (l *AuditLog) Entries() []map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.entriesLocked()
}

func (l *AuditLog) entriesLocked() []map[string]interface{} {
	orderedIDs := l.order.All()
	seen := make(map[string]struct{}, len(orderedIDs))
	out := make([]map[string]interface{}, 0, len(orderedIDs))

	for _, rawID := range orderedIDs {
		id, ok := rawID.(string)
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		val, err := l.index.LoadFromIndex(l.name, id)
		if err != nil || val == nil {
			continue
		}
		if m, ok := val.(map[string]interface{}); ok && m["entry_id"] != nil {
			out = append(out, cloneMap(m))
		}
	}

	// Include any out-of-band entries injected directly into the underlying Index.
	_ = l.index.RangeIndex(l.name, func(key, value interface{}) {
		k, ok := key.(string)
		if !ok || k == l.headKey || k == l.lockKey {
			return
		}
		if _, already := seen[k]; already {
			return
		}
		if m, ok := value.(map[string]interface{}); ok && m["entry_id"] != nil {
			out = append(out, cloneMap(m))
		}
	})

	return out
}

// Verify recomputes every entry's SHA-256 hash and checks full hash-chain continuity and count integrity.
func (l *AuditLog) Verify() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	head := l.headLocked()
	entries := l.entriesLocked()
	if len(entries) != head.count || int(l.order.Count()) != head.count {
		return false
	}

	var prevHash *string
	for _, entryMap := range entries {
		if !hashPtrEqual(extractOptString(entryMap["previous_hash"]), prevHash) {
			return false
		}
		reconstructed, err := AuditEntryFromMap(entryMap)
		if err != nil {
			return false
		}
		storedHash, ok := entryMap["current_hash"].(string)
		if !ok || reconstructed.CurrentHash != storedHash {
			return false
		}
		nextPrev := storedHash
		prevHash = &nextPrev
	}

	return hashPtrEqual(prevHash, head.previousHash)
}

// Count returns the recorded entry count in the log head.
func (l *AuditLog) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.headLocked().count
}

func extractOptString(v interface{}) *string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case string:
		cp := s
		return &cp
	case *string:
		if s == nil {
			return nil
		}
		cp := *s
		return &cp
	default:
		return nil
	}
}

func hashPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]interface{}, len(in))
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}

func newEntryID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:])
}
