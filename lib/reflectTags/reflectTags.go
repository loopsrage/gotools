package reflectTags

import (
	"errors"
	"reflect"
)

// ReflectTags provides reflection over struct fields and their tags.
type ReflectTags struct {
	typ    reflect.Type
	val    reflect.Value
	fields []reflect.StructField
}

// FindTags scans the struct (or pointer to struct) for fields and records their tags.
func (rt *ReflectTags) FindTags(to interface{}, tag string) error {
	v := reflect.ValueOf(to)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return errors.New("to is nil pointer")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("to is not a struct or pointer to struct")
	}
	rt.typ = v.Type()
	rt.val = v
	rt.fields = make([]reflect.StructField, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		rt.fields = append(rt.fields, v.Type().Field(i))
	}
	return nil
}

// RangeFieldTypes calls fn for each field with its name, type, and reflect.Value.
func (rt *ReflectTags) RangeFieldTypes(fn func(name string, _type reflect.Type, value reflect.Value)) error {
	for i, f := range rt.fields {
		fv := rt.val.Field(i)
		fn(f.Name, f.Type, fv)
	}
	return nil
}

// Tag returns the value of the given tag for the named field, or the field name if not found.
func (rt *ReflectTags) Tag(name, tag string) string {
	for _, f := range rt.fields {
		if f.Name == name {
			tagVal := f.Tag.Get(tag)
			if tagVal != "" {
				return tagVal
			}
			return name
		}
	}
	return name
}

// Field returns the reflect.Value for the given column name.
func (rt *ReflectTags) Field(col string) (reflect.Value, error) {
	// First try by field name
	for i, f := range rt.fields {
		if f.Name == col {
			return rt.val.Field(i), nil
		}
	}
	// Then try by common tag values
	for i, f := range rt.fields {
		for _, tagName := range []string{"json", "db", "column"} {
			if tv := f.Tag.Get(tagName); tv != "" && tv == col {
				return rt.val.Field(i), nil
			}
		}
	}
	return reflect.Value{}, errors.New("field not found: " + col)
}
