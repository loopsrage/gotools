package queueController

import (
	"golangApp/lib/concurr"
	"golangApp/lib/index"
)

type QueueData struct {
	*index.Index
	trace *concurr.TsList
}

const attributes = "attributes"

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
	nd := &QueueData{
		Index: index.NewIndex(),
		trace: c.trace,
	}
	return nd
}

func NewQueueData() *QueueData {
	return &QueueData{
		Index: index.NewIndex(),
		trace: concurr.NewTsList(),
	}
}
