package containerTreeBuilder_test

import (
	"testing"

	"golangApp/lib/jsonContainers/container"
	"golangApp/lib/jsonContainers/containerTreeBuilder"
)

func TestBuildContainerTreeNilNoStart(t *testing.T) {
	root := containerTreeBuilder.BuildContainerTree(nil, nil, '.')
	if root == nil {
		t.Fatal("BuildContainerTree(nil) returned nil")
	}
	if root.Path() != "root" {
		t.Fatalf("root Path() = %q, want %q", root.Path(), "root")
	}
	if root.Value() == nil {
		t.Fatal("root Value() should be an empty map")
	}
}

func TestBuildContainerTreeNilWithStart(t *testing.T) {
	start := map[string]interface{}{"name": "test", "count": 42}
	root := containerTreeBuilder.BuildContainerTree(nil, nil, '.', start)
	if root == nil {
		t.Fatal("BuildContainerTree(nil, start) returned nil")
	}
	if root.Value() == nil {
		t.Fatal("root Value() should be the start map")
	}
}

func TestBuildContainerTreeNestedMap(t *testing.T) {
	data := map[string]interface{}{
		"person": map[string]interface{}{
			"name": "Alice",
			"age":  30,
			"addr": map[string]interface{}{"city": "NYC"},
		},
	}
	root := containerTreeBuilder.BuildContainerTree(nil, []string{"root"}, '.', data)

	// Root has one child: "person" (a map)
	children, err := root.Children()
	if err != nil {
		t.Fatalf("Children() error: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child (person), got %d: %v", len(children), children)
	}
	person := children[0]
	if person.Path() != "root.person" {
		t.Fatalf("person Path() = %q, want %q", person.Path(), "root.person")
	}

	// person has one child: "addr" (a map); name and age are scalars
	personChildren, _ := person.Children()
	if len(personChildren) != 1 {
		t.Fatalf("expected 1 child (addr) under person, got %d", len(personChildren))
	}
	if personChildren[0].Path() != "root.person.addr" {
		t.Fatalf("addr Path() = %q, want %q", personChildren[0].Path(), "root.person.addr")
	}
}

func TestBuildContainerTreeNestedArray(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": 1},
			map[string]interface{}{"id": 2},
		},
	}
	root := containerTreeBuilder.BuildContainerTree(nil, []string{"root"}, '.', data)

	children, _ := root.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 child (items), got %d", len(children))
	}
	items := children[0]
	if items.Path() != "root.items" {
		t.Fatalf("items Path() = %q, want %q", items.Path(), "root.items")
	}

	itemsChildren, _ := items.Children()
	if len(itemsChildren) != 2 {
		t.Fatalf("expected 2 children under items (index 0 and 1), got %d", len(itemsChildren))
	}
	if itemsChildren[0].Path() != "root.items.0" {
		t.Fatalf("child 0 Path() = %q, want %q", itemsChildren[0].Path(), "root.items.0")
	}
	if itemsChildren[1].Path() != "root.items.1" {
		t.Fatalf("child 1 Path() = %q, want %q", itemsChildren[1].Path(), "root.items.1")
	}
}

func TestBuildContainerTreeExistingContainer(t *testing.T) {
	existing := container.NewContainer(nil, "existing", "existing-value")
	data := map[string]interface{}{"newkey": "newval"}
	root := containerTreeBuilder.BuildContainerTree(existing, []string{"existing"}, '.', data)

	if root != existing {
		t.Fatal("BuildContainerTree with existing container should reuse it")
	}
	if root.Value() != "existing-value" {
		t.Fatalf("root Value() = %v, want %v", root.Value(), "existing-value")
	}
	// "newkey" value is a string (scalar), so no child is created for it.
	// BuildContainerTree only recurses into maps and slices.
	children, _ := root.Children()
	if len(children) != 0 {
		t.Fatalf("expected 0 children (newval is a scalar), got %d", len(children))
	}
}
