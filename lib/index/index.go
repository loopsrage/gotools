package index

import (
	"errors"
	"fmt"
	"sync"
)

type Index struct {
	Map *sync.Map
}

func (i *Index) New(indexName string) *Index {
	i.Map.Store(indexName, &sync.Map{})
	return i
}

func (i *Index) LoadIndex(indexName string) *sync.Map {
	x, _ := i.Map.Load(indexName)
	switch x := x.(type) {
	case *sync.Map:
		return x
	default:
		return nil
	}
}

func (i *Index) StoreInIndex(indexName string, key, value interface{}) *Index {
	idx := i.LoadIndex(indexName)
	if idx == nil {
		i.New(indexName)
	}
	idx = i.LoadIndex(indexName)

	idx.Store(key, value)
	return i
}

func (i *Index) LoadOrStoreInIndex(indexName string, key, value interface{}) (actual interface{}, loaded bool) {
	// Atomically get-or-create the sub-map to avoid a check-then-act race.
	m, _ := i.Map.LoadOrStore(indexName, &sync.Map{})
	idx := m.(*sync.Map)
	return idx.LoadOrStore(key, value)
}

func (i *Index) LoadFromIndex(indexName string, key interface{}) (value interface{}, err error) {
	idx := i.LoadIndex(indexName)
	if idx == nil {
		return nil, errors.New(fmt.Sprintf("index %s does not exist", indexName))
	}
	ix, _ := idx.Load(key)
	return ix, nil
}

func (i *Index) RangeIndex(indexName string, rangeFunc func(key, value interface{})) (err error) {
	idx := i.LoadIndex(indexName)
	if idx == nil {
		return errors.New(fmt.Sprintf("index %s does not exist", indexName))
	}
	idx.Range(func(key, value any) bool {
		rangeFunc(key, value)
		return true
	})
	return nil
}

func (i *Index) DeleteIndex(indexName string) {
	i.Map.Delete(indexName)
}

func (i *Index) DeleteFromIndex(indexName string, key interface{}) (err error) {
	idx := i.LoadIndex(indexName)
	if idx == nil {
		return errors.New(fmt.Sprintf("index %s does not exist", indexName))
	}
	idx.Delete(key)
	return nil
}

func (i *Index) ListIndexes() (indexes []string) {
	i.Map.Range(func(key, _ any) bool {
		switch key := key.(type) {
		case string:
			indexes = append(indexes, key)
		}
		return true
	})
	return indexes
}

func NewIndex() *Index {
	return &Index{Map: &sync.Map{}}
}
