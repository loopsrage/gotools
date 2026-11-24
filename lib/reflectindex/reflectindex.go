// Package reflectindex provides reflection-based indexing over Go structs
// using dot-notation paths (e.g. "Material.Color.R"). It walks exported struct
// fields with reflect, caching resolved field index chains so repeated lookups
// are cheap. Used by the lowering pipeline to read shader-parameter structs
// and emit them as GLSL/Vulkan uniform declarations.
package reflectindex

import (
	"errors"
	"reflect"
	"strings"
)

// ErrNotFound is returned when a dot-notation path does not resolve to a field.
var ErrNotFound = errors.New("reflectindex: path not found")

// Indexer reads values from a struct via dot-notation paths.
type Indexer struct {
	typ reflect.Type
	// cache maps a dot path to the reflect.StructField index chain and final type.
	cache map[string][]int
}

// NewIndexer builds an Indexer for the given struct type. Only exported fields
// are indexed; unexported fields are skipped.
func NewIndexer(v any) (*Indexer, error) {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, errors.New("reflectindex: NewIndexer requires a struct or pointer to struct")
	}
	ix := &Indexer{typ: t, cache: make(map[string][]int)}
	if err := ix.indexFields(t, nil, ""); err != nil {
		return nil, err
	}
	return ix, nil
}

func (ix *Indexer) indexFields(t reflect.Type, prefix []int, path string) error {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		idx := make([]int, len(prefix)+1)
		copy(idx, prefix)
		idx[len(prefix)] = i
		fp := path
		if fp == "" {
			fp = f.Name
		} else {
			fp = fp + "." + f.Name
		}
		ix.cache[fp] = idx
		if f.Type.Kind() == reflect.Struct {
			if err := ix.indexFields(f.Type, idx, fp); err != nil {
				return err
			}
		}
	}
	return nil
}

// Fields returns all registered dot-notation paths.
func (ix *Indexer) Fields() []string {
	out := make([]string, 0, len(ix.cache))
	for k := range ix.cache {
		out = append(out, k)
	}
	return out
}

// Get resolves a dot-notation path against v and returns the reflect.Value and
// its type. Returns ErrNotFound if the path does not exist.
func (ix *Indexer) Get(v any, path string) (reflect.Value, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return reflect.Value{}, errors.New("reflectindex: Get nil pointer")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct || rv.Type() != ix.typ {
		return reflect.Value{}, errors.New("reflectindex: Get value type mismatch")
	}
	idx, ok := ix.cache[path]
	if !ok {
		return reflect.Value{}, ErrNotFound
	}
	return rv.FieldByIndex(idx), nil
}

// GetInterface is like Get but returns an interface{} value.
func (ix *Indexer) GetInterface(v any, path string) (interface{}, bool) {
	val, err := ix.Get(v, path)
	if err != nil {
		return nil, false
	}
	if !val.CanInterface() {
		return nil, false
	}
	return val.Interface(), true
}

// Has returns true if path resolves to a field.
func (ix *Indexer) Has(path string) bool {
	_, ok := ix.cache[path]
	return ok
}

// SplitPath splits a dot-notation path into its field components.
func SplitPath(path string) []string {
	return strings.Split(path, ".")
}

// JoinsPath joins field components into a dot-notation path.
func JoinsPath(parts ...string) string {
	return strings.Join(parts, ".")
}
