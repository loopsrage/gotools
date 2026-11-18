package container

import (
	"errors"
	"github.com/loopsrage/gotools/lib/concurr"
	"github.com/loopsrage/gotools/lib/index"
	"sync/atomic"
)

// Container implements json_helper.JsonObjectHandler to be used in json_helper.UnmarshalHelper
// Each json object is stored in Container.Value and subsequent children are appended to Container.Children

const valuesString = "Values"
const containersString = "Containers"

type Container struct {
	Root     *Container
	parent   *atomic.Value
	path     *atomic.Value
	value    *atomic.Value
	children *concurr.TsList

	containerIndex *index.Index
}

func (c *Container) GoString() string {
	return c.Path()
}

func (c *Container) Parent() *Container {
	switch x := c.parent.Load().(type) {
	case *Container:
		return x
	default:
		return nil
	}
}

func (c *Container) Children() (childs []*Container, err error) {
	for _, v := range c.children.All() {
		switch v := v.(type) {
		case *Container:
			childs = append(childs, v)
		default:
			continue
		}
	}
	return childs, nil
}

func (c *Container) Value() interface{} {
	return c.value.Load()
}

func (c *Container) Path() string {
	switch x := c.path.Load().(type) {
	case string:
		return x
	default:
		return ""
	}
}

func (c *Container) NewObject(parent *Container, path string, value interface{}) *Container {
	nc := NewContainer(parent, path, value)
	c.AppendChildren(nc)
	return nc
}

func (c *Container) GetValue() interface{} {
	return c.Value()
}

func (c *Container) AppendChildren(children ...*Container) {
	var x []interface{}
	for _, v := range children {
		x = append(x, v)
	}
	c.children.Add(x...)
}

func (c *Container) ReadFromValue(path string) (interface{}, error) {
	return c.containerIndex.LoadFromIndex(valuesString, path)
}

func (c *Container) ReadFromContainers(path string) (*Container, error) {
	x, _ := c.containerIndex.LoadFromIndex(containersString, path)
	switch x := x.(type) {
	case *Container:
		return x, nil
	default:
		return nil, errors.New("invalid container type")
	}
}

func (c *Container) RangeValues(rng func(key string, value interface{})) (err error) {
	return c.containerIndex.RangeIndex(valuesString, func(key, value interface{}) {
		switch key := key.(type) {
		case string:
			rng(key, value)
		}
	})
}

func (c *Container) RangeContainers(rng func(value *Container)) (err error) {
	return c.containerIndex.RangeIndex(containersString, func(key, value interface{}) {
		switch value := value.(type) {
		case *Container:
			rng(value)
		}
	})
}

func NewContainer(parent *Container, path string, value interface{}) *Container {
	if parent == nil {
		path = "root"
	}

	var existingIndex *index.Index

	p := &atomic.Value{}
	p.Store(path)

	v := &atomic.Value{}
	v.Store(value)

	pa := &atomic.Value{}
	pa.Store(parent)

	cn := &Container{
		parent:   pa,
		path:     p,
		value:    v,
		children: concurr.NewTsList(),
	}

	if parent != nil {
		existingIndex = parent.containerIndex
		cn.containerIndex = existingIndex
		cn.Root = parent.Root
	}

	if parent == nil {
		existingIndex = index.NewIndex()
		existingIndex.New(valuesString)
		existingIndex.New(containersString)
		cn.containerIndex = existingIndex
		cn.Root = cn
	}

	cn.containerIndex.StoreInIndex(valuesString, path, value)
	cn.containerIndex.StoreInIndex(containersString, path, cn)

	return cn
}
