package jsonIndexer_test

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/loopsrage/gotools/lib/jsonIndexer"
)

func TestNewJsonIndexer(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	if j == nil {
		t.Fatal("NewJsonIndexer() returned nil")
	}
}

func TestReadJson(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	jsonInput := `{"app": {"name": "testapp", "version": 2, "debug": true, "tags": ["a", "b"]}}`
	err := j.ReadJson(bytes.NewReader([]byte(jsonInput)))
	if err != nil {
		t.Fatalf("ReadJson() error: %v", err)
	}

	// Data is stored in the "values" index with flattened keys.
	// Test via FindValues which works correctly.
	re := regexp.MustCompile("version")
	results, err := j.FindValues(re)
	if err != nil {
		t.Fatalf("FindValues() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindValues('version') returned no results")
	}
}

func TestReadJsonInvalid(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	err := j.ReadJson(bytes.NewReader([]byte("not valid json")))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSetting(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	j.ReadJson(bytes.NewReader([]byte(`{"app": {"setting": "value"}}`)))
	// Data is stored in the "values" index with flattened key "app.setting"
	val, err := j.Setting("values", "app.setting")
	if err != nil {
		t.Fatalf("Setting() error: %v", err)
	}
	if val != "value" {
		t.Fatalf("Setting() = %v, want %v", val, "value")
	}
}

func TestSettingMissing(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	val, err := j.Setting("missing", "path")
	if err == nil {
		t.Fatal("expected error for missing app")
	}
	if val != nil {
		t.Fatalf("Setting() = %v, want nil", val)
	}
}

func TestInt64Defaults(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	if got := j.Int64("app", "version"); got != -1 {
		t.Fatalf("Int64() on empty = %d, want -1", got)
	}
}

func TestStringDefaults(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	if got := j.String("app", "name"); got != "" {
		t.Fatalf("String() on empty = %q, want %q", got, "")
	}
}

func TestBoolDefaults(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	if got := j.Bool("app", "debug"); got != false {
		t.Fatalf("Bool() on empty = %v, want false", got)
	}
}

func TestFindValues(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	j.ReadJson(bytes.NewReader([]byte(`{"app": {"foo": "bar", "baz": 42, "foobar": "test"}}`)))
	re := regexp.MustCompile("foo")
	results, err := j.FindValues(re)
	if err != nil {
		t.Fatalf("FindValues() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("FindValues(foo) = %d results, want 2", len(results))
	}
}

func TestFindValuesNoMatch(t *testing.T) {
	j := jsonIndexer.NewJsonIndexer()
	j.ReadJson(bytes.NewReader([]byte(`{"app": {"name": "test"}}`)))
	re := regexp.MustCompile("zzz")
	results, err := j.FindValues(re)
	if err != nil {
		t.Fatalf("FindValues() error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("FindValues(zzz) = %d results, want 0", len(results))
	}
}
