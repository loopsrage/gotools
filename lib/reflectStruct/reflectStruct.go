package reflectStruct

import (
	"encoding/json"
	"errors"
	"reflect"
)

// MarshallMap flattens nested maps into a single map with dot-separated keys.
func MarshallMap(m map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	flatten("", m, result)
	return result, nil
}

func flatten(prefix string, v interface{}, out map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, sub := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, sub, out)
		}
	case []interface{}:
		for i, sub := range val {
			key := prefix + "[" + itoa(i) + "]"
			flatten(key, sub, out)
		}
	default:
		out[prefix] = v
	}
}

func itoa(x int) string {
	if x == 0 {
		return "0"
	}
	var buf []byte
	for x > 0 {
		buf = append([]byte{byte('0' + x%10)}, buf...)
		x /= 10
	}
	return string(buf)
}

// FindFields populates fields of appConfig based on the flattened map xf.
func FindFields(appConfig interface{}, xf map[string]interface{}) error {
	if appConfig == nil {
		return errors.New("appConfig is nil")
	}
	v := reflect.ValueOf(appConfig)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("appConfig is not a struct or pointer to struct")
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		// Try to match by field name (case-insensitive)
		name := field.Name
		if val, ok := xf[name]; ok {
			setField(fv, val)
		}
		// Also try lowercase
		if val, ok := xf[lower(name)]; ok {
			setField(fv, val)
		}
	}
	return nil
}

func setField(f reflect.Value, val interface{}) {
	if !f.CanSet() {
		return
	}
	switch f.Kind() {
	case reflect.String:
		if s, ok := val.(string); ok {
			f.SetString(s)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := val.(type) {
		case int64:
			f.SetInt(v)
		case int:
			f.SetInt(int64(v))
		case float64:
			f.SetInt(int64(v))
		case string:
			f.SetString(v)
		}
	case reflect.Bool:
		if b, ok := val.(bool); ok {
			f.SetBool(b)
		}
	case reflect.Float64, reflect.Float32:
		if v, ok := val.(float64); ok {
			f.SetFloat(v)
		}
	}
}

func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		out[i] = c
	}
	return string(out)
}

var _ = json.Unmarshal
