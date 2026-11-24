package reflectindex

import (
	"reflect"
	"testing"
)

type Inner struct {
	R float64
	G float64
	B float64
}

type Material struct {
	Color Inner
	Name  string
}

type TestStruct struct {
	ID       int
	Active   bool
	Material Material
	Pos      [3]float64
}

func TestNewIndexerAndFields(t *testing.T) {
	ts := TestStruct{ID: 1, Active: true, Material: Material{Color: Inner{R: 1, G: 2, B: 3}, Name: "mat"}}
	ix, err := NewIndexer(&ts)
	if err != nil {
		t.Fatalf("NewIndexer error: %v", err)
	}
	fields := ix.Fields()
	found := map[string]bool{}
	for _, f := range fields {
		found[f] = true
	}
	expected := []string{"ID", "Active", "Material", "Material.Color", "Material.Color.R", "Material.Color.G", "Material.Color.B", "Material.Name", "Pos"}
	for _, e := range expected {
		if !found[e] {
			t.Fatalf("expected field %q not found, got %v", e, fields)
		}
	}
}

func TestGetAndGetInterface(t *testing.T) {
	ts := TestStruct{ID: 42, Active: true, Material: Material{Color: Inner{R: 0.5, G: 0.25, B: 0.125}, Name: "gold"}, Pos: [3]float64{1, 2, 3}}
	ix, err := NewIndexer(&ts)
	if err != nil {
		t.Fatalf("NewIndexer error: %v", err)
	}

	val, err := ix.Get(&ts, "ID")
	if err != nil {
		t.Fatalf("Get ID error: %v", err)
	}
	if val.Int() != 42 {
		t.Fatalf("expected ID 42, got %d", val.Int())
	}

	val, err = ix.Get(&ts, "Material.Color.R")
	if err != nil {
		t.Fatalf("Get Material.Color.R error: %v", err)
	}
	if val.Float() != 0.5 {
		t.Fatalf("expected R 0.5, got %f", val.Float())
	}

	iface, ok := ix.GetInterface(&ts, "Material.Name")
	if !ok {
		t.Fatal("GetInterface Material.Name failed")
	}
	if iface.(string) != "gold" {
		t.Fatalf("expected name gold, got %v", iface)
	}

	// Active
	val, err = ix.Get(&ts, "Active")
	if err != nil {
		t.Fatalf("Get Active error: %v", err)
	}
	if val.Bool() != true {
		t.Fatalf("expected Active true, got %v", val.Bool())
	}
}

func TestHas(t *testing.T) {
	ts := TestStruct{}
	ix, err := NewIndexer(&ts)
	if err != nil {
		t.Fatalf("NewIndexer error: %v", err)
	}
	if !ix.Has("ID") {
		t.Fatal("expected ID to exist")
	}
	if !ix.Has("Material.Color.R") {
		t.Fatal("expected Material.Color.R to exist")
	}
	if ix.Has("Material.Color.Alpha") {
		t.Fatal("expected Material.Color.Alpha to not exist")
	}
	if ix.Has("Nonexistent") {
		t.Fatal("expected Nonexistent to not exist")
	}
}

func TestGetNotFound(t *testing.T) {
	ts := TestStruct{}
	ix, err := NewIndexer(&ts)
	if err != nil {
		t.Fatalf("NewIndexer error: %v", err)
	}
	_, err = ix.Get(&ts, "Nope")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTypeMismatch(t *testing.T) {
	ts := TestStruct{}
	ix, err := NewIndexer(&ts)
	if err != nil {
		t.Fatalf("NewIndexer error: %v", err)
	}
	other := struct{ X int }{X: 1}
	_, err = ix.Get(&other, "ID")
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
}

func TestReflectTypeOfIndexer(t *testing.T) {
	ts := TestStruct{}
	ix, err := NewIndexer(&ts)
	if err != nil {
		t.Fatalf("NewIndexer error: %v", err)
	}
	_ = reflect.TypeOf(ix)
}
