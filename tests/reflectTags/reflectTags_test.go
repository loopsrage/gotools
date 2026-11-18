package reflectTags_test

import (
	"reflect"
	"testing"

	"github.com/loopsrage/gotools/lib/reflectTags"
)

type testStruct struct {
	Name   string `json:"name" db:"db_name"`
	Count  int    `json:"count"`
	Active bool   `json:"active"`
}

func TestReflectTagsFindTags(t *testing.T) {
	rt := &reflectTags.ReflectTags{}
	s := &testStruct{Name: "test", Count: 42, Active: true}
	err := rt.FindTags(s, "json")
	if err != nil {
		t.Fatalf("FindTags() error: %v", err)
	}
}

func TestReflectTagsTag(t *testing.T) {
	rt := &reflectTags.ReflectTags{}
	s := &testStruct{}
	err := rt.FindTags(s, "json")
	if err != nil {
		t.Fatalf("FindTags() error: %v", err)
	}
	if got := rt.Tag("Name", "json"); got != "name" {
		t.Fatalf("Tag('Name', 'json') = %q, want %q", got, "name")
	}
	if got := rt.Tag("Count", "json"); got != "count" {
		t.Fatalf("Tag('Count', 'json') = %q, want %q", got, "count")
	}
	if got := rt.Tag("Missing", "json"); got != "Missing" {
		t.Fatalf("Tag('Missing', 'json') = %q, want %q", got, "Missing")
	}
}

func TestReflectTagsRangeFieldTypes(t *testing.T) {
	rt := &reflectTags.ReflectTags{}
	s := &testStruct{Name: "test", Count: 42, Active: true}
	if err := rt.FindTags(s, "json"); err != nil {
		t.Fatalf("FindTags() error: %v", err)
	}

	count := 0
	err := rt.RangeFieldTypes(func(name string, _type reflect.Type, value reflect.Value) {
		count++
	})
	if err != nil {
		t.Fatalf("RangeFieldTypes() error: %v", err)
	}
	if count != 3 {
		t.Fatalf("RangeFieldTypes() called %d times, want 3", count)
	}
}

func TestReflectTagsField(t *testing.T) {
	rt := &reflectTags.ReflectTags{}
	s := &testStruct{Name: "test"}
	if err := rt.FindTags(s, "json"); err != nil {
		t.Fatalf("FindTags() error: %v", err)
	}

	fv, err := rt.Field("Name")
	if err != nil {
		t.Fatalf("Field('Name') error: %v", err)
	}
	if !fv.IsValid() {
		t.Fatal("Field('Name') returned invalid reflect.Value")
	}
	if fv.String() != "test" {
		t.Fatalf("Field('Name').String() = %q, want %q", fv.String(), "test")
	}

	// Field by tag name
	fv, err = rt.Field("db_name")
	if err != nil {
		t.Fatalf("Field('db_name') error: %v", err)
	}
	if !fv.IsValid() {
		t.Fatal("Field('db_name') returned invalid reflect.Value")
	}
}

func TestReflectTagsFieldNotFound(t *testing.T) {
	rt := &reflectTags.ReflectTags{}
	s := &testStruct{}
	if err := rt.FindTags(s, "json"); err != nil {
		t.Fatalf("FindTags() error: %v", err)
	}
	_, err := rt.Field("NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent field")
	}
}

func TestReflectTagsFindTagsNil(t *testing.T) {
	rt := &reflectTags.ReflectTags{}
	err := rt.FindTags(nil, "json")
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}
