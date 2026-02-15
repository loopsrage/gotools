package concurr

import (
	"sync"
)

// TsList represents a thread-safe list structure.
type TsList struct {
	mu   sync.RWMutex
	data []interface{}
}

// Count returns the number of elements in the list.
func (t *TsList) Count() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return int64(len(t.data))
}

func (t *TsList) Add(x ...interface{}) int64 {
	if len(x) == 0 {
		return -1
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	c := int64(len(t.data))
	t.data = append(t.data, x...)
	return c
}

func (t *TsList) Set(position int64, value interface{}) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if position >= 0 && position < int64(len(t.data)) {
		t.data[position] = value
	}
}

func (t *TsList) At(position int64) interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if position >= 0 && position < int64(len(t.data)) {
		return t.data[position]
	}
	return nil
}

func (t *TsList) All() (values []interface{}) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	values = make([]interface{}, len(t.data))
	copy(values, t.data)
	return values
}

func NewTsList() *TsList {
	return &TsList{
		data: make([]interface{}, 0),
	}
}
