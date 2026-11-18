package onceler

import (
	"errors"
	"github.com/loopsrage/gotools/lib/index"
	"sync"
)

type Onceler struct {
	*index.Index
}

func (o *Onceler) StoreOnce(index string, key interface{}, do func() error) error {
	newOnce := &sync.Once{}
	actual, _ := o.Index.LoadOrStoreInIndex(index, key, newOnce)
	once, ok := actual.(*sync.Once)
	if !ok {
		return errors.New("unexpected value type in index")
	}

	var errp = new(error)
	once.Do(func() {
		err := do()
		if err != nil {
			*errp = err
			return
		}
	})
	if *errp != nil {
		return *errp
	}
	return nil
}

func NewOnceler() *Onceler {
	o := &Onceler{Index: index.NewIndex()}
	return o
}
