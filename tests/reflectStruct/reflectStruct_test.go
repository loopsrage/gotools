package reflectStruct_test

import (
	"testing"

	"golangApp/lib/reflectStruct"
)

func TestMarshallMapFlat(t *testing.T) {
	input := map[string]interface{}{
		"name": "test",
		"port": 8080,
	}
	out, err := reflectStruct.MarshallMap(input)
	if err != nil {
		t.Fatalf("MarshallMap() error: %v", err)
	}
	if out["name"] != "test" {
		t.Fatalf("out['name'] = %v, want %v", out["name"], "test")
	}
	if out["port"] != 8080 {
		t.Fatalf("out['port'] = %v, want %v", out["port"], 8080)
	}
}

func TestMarshallMapNested(t *testing.T) {
	input := map[string]interface{}{
		"db": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}
	out, err := reflectStruct.MarshallMap(input)
	if err != nil {
		t.Fatalf("MarshallMap() error: %v", err)
	}
	if out["db.host"] != "localhost" {
		t.Fatalf("out['db.host'] = %v, want %v", out["db.host"], "localhost")
	}
	if out["db.port"] != 5432 {
		t.Fatalf("out['db.port'] = %v, want %v", out["db.port"], 5432)
	}
}

func TestMarshallMapEmpty(t *testing.T) {
	out, err := reflectStruct.MarshallMap(map[string]interface{}{})
	if err != nil {
		t.Fatalf("MarshallMap() error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("out len = %d, want 0", len(out))
	}
}

func TestFindFields(t *testing.T) {
	type Config struct {
		Name  string
		Port  int
		Debug bool
	}
	cfg := &Config{}
	xf := map[string]interface{}{
		"Name":  "myapp",
		"Port":  3000,
		"Debug": true,
	}
	err := reflectStruct.FindFields(cfg, xf)
	if err != nil {
		t.Fatalf("FindFields() error: %v", err)
	}
	if cfg.Name != "myapp" {
		t.Fatalf("cfg.Name = %q, want %q", cfg.Name, "myapp")
	}
	if cfg.Port != 3000 {
		t.Fatalf("cfg.Port = %d, want %d", cfg.Port, 3000)
	}
	if cfg.Debug != true {
		t.Fatalf("cfg.Debug = %v, want %v", cfg.Debug, true)
	}
}

func TestFindFieldsNil(t *testing.T) {
	err := reflectStruct.FindFields(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil appConfig")
	}
}

func TestFindFieldsNonStruct(t *testing.T) {
	x := 42
	err := reflectStruct.FindFields(&x, nil)
	if err == nil {
		t.Fatal("expected error for non-struct appConfig")
	}
}
