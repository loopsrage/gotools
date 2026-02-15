package totals

import (
	"sync"
	"sync/atomic"
)

type Totals struct {
	totals *sync.Map
}

func (o *Totals) total(prefix string) *int64 {
	x, _ := o.totals.Load(prefix)
	switch x := x.(type) {
	case *int64:
		return x
	default:
		y := new(int64)
		o.totals.Store(prefix, y)
		return y
	}
}

func (o *Totals) AddTotal(prefix string, add int64) {
	atomic.AddInt64(o.total(prefix), add)
}

func (o *Totals) Reset(prefix string) {
	o.totals.Delete(prefix)
}

func (o *Totals) Total(prefix string) int64 {
	x, _ := o.totals.Load(prefix)
	switch x := x.(type) {
	case *int64:
		return *x
	default:
		return 0
	}
}

func (o *Totals) Range(rangeFunc func(prefix string, total int64)) {
	o.totals.Range(func(key, value any) bool {
		switch key := key.(type) {
		case string:
			switch value := value.(type) {
			case *int64:
				rangeFunc(key, atomic.LoadInt64(value))
			}
		}
		return true
	})
}

func NewTotals() *Totals {
	return &Totals{
		totals: &sync.Map{},
	}
}
