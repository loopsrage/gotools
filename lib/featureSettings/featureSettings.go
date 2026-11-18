package featureSettings

import (
	"reflect"
	"sync"
)

type Config struct {
	attributes *sync.Map `bson:"-"`
}

func NewConfig() *Config {
	cfg := &Config{
		attributes: &sync.Map{},
	}
	return cfg
}

func (c *Config) SetAttribute(key string, value interface{}) {
	c.attributes.Store(key, value)
}

func (c *Config) Attribute(key interface{}) interface{} {
	x, _ := c.attributes.Load(key)
	return x
}

func (c *Config) AllAttributes() (result map[string]interface{}) {
	result = make(map[string]interface{})
	c.attributes.Range(func(key, value interface{}) bool {
		result[key.(string)] = value
		return true
	})
	return result
}

func (c *Config) ReadAttributes(to interface{}) {
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

func (c *Config) AggregateAttributes(on interface{}) {
	wt := reflect.TypeOf(on)
	if wt.Kind() == reflect.Ptr {
		wt = wt.Elem()
	}

	wv := reflect.ValueOf(on)
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

			if field.CanInterface() {
				c.SetAttribute(wt.Field(i).Name, field.Interface())
			}
		}
	}
}
