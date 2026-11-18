package queueController

import (
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/index"
	"reflect"
	"sync/atomic"
)

type QueueD interface {
	Derivative() string
	CopyDerivative(derivative string) QueueD
}

type QueueData struct {
	*index.Index
	Data       *index.Index
	Derivitive *atomic.Value
	trace      *concurr.TsList
}

const attributes = "attributes"

func (c *QueueData) Derivative() string {
	if c.Derivitive == nil {
		return ""
	}

	switch d := c.Derivitive.Load().(type) {
	case string:
		return d
	default:
		return ""
	}
}

func (c *QueueData) CopyDerivative(derivative string) QueueD {
	der := &atomic.Value{}
	der.Store(derivative)

	return &QueueData{
		Index:      c.Index,
		Data:       c.Data,
		Derivitive: der,
		trace:      c.trace,
	}
}

func (c *QueueData) ReadAttributes(to interface{}) {
	if to == nil || c.Index == nil {
		return
	}

	wt := reflect.TypeOf(to)
	if wt.Kind() == reflect.Ptr {
		wt = wt.Elem()
	}

	wv := reflect.ValueOf(to)
	if wv.Kind() == reflect.Ptr {
		wv = wv.Elem()
	}

	if wt.Kind() == reflect.Struct {
		fieldCount := wt.NumField()
		for i := 0; i < fieldCount; i++ {
			field := wv.Field(i)
			if !wt.Field(i).IsExported() {
				continue
			}

			if field.CanSet() {
				val := c.Attribute(wt.Field(i).Name)
				if val == nil {
					continue
				}

				vx := reflect.ValueOf(val)
				if !vx.CanConvert(field.Type()) {
					continue
				}
				field.Set(vx.Convert(field.Type()))
			}
		}
	}
}

func (c *QueueData) Setting(appName, path string) (interface{}, error) {
	return c.LoadFromIndex(appName, path)
}

func (c *QueueData) Int64(path string) int64 {
	x, _ := c.Setting(attributes, path)
	switch x := x.(type) {
	case int64:
		return x
	default:
		return -1
	}
}

func (c *QueueData) String(path string) string {
	x, _ := c.Setting(attributes, path)
	switch x := x.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func (c *QueueData) Strings(path string) []string {
	x, _ := c.Setting(attributes, path)
	switch x := x.(type) {
	case []string:
		return x
	default:
		return nil
	}
}

func (c *QueueData) Bool(path string) bool {
	x, _ := c.Setting(attributes, path)
	switch x := x.(type) {
	case bool:
		return x
	default:
		return false
	}
}

func (c *QueueData) SetAttribute(key string, value interface{}) {
	c.StoreInIndex(attributes, key, value)
}

func (c *QueueData) Attribute(key string) interface{} {
	x, _ := c.LoadFromIndex(attributes, key)
	return x
}

func (c *QueueData) AppendTrace(source string) {
	c.trace.Add(source)
}

func (c *QueueData) Trace() *concurr.TsList {
	return c.trace
}

func (c *QueueData) SetAttributes(attributes map[string]interface{}) {
	for k, v := range attributes {
		c.SetAttribute(k, v)
	}
}

func (c *QueueData) Attributes() map[string]interface{} {
	out := make(map[string]interface{})
	c.RangeIndex(attributes, func(key, value interface{}) {
		switch k := key.(type) {
		case string:
			out[k] = value
		}
	})
	return out
}

func (c *QueueData) NewQueueData() *QueueData {
	idx := index.NewIndex()
	nd := &QueueData{
		Index: idx,
		Data:  idx,
		trace: c.trace,
	}
	return nd
}

func NewQueueData() *QueueData {
	idx := index.NewIndex()
	return &QueueData{
		Index: idx,
		Data:  idx,
		trace: concurr.NewTsList(),
	}
}
