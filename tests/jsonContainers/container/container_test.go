package container_test

import (
	"testing"

	"golangApp/lib/jsonContainers/container"
)

func TestNewContainerRoot(t *testing.T) {
	c := container.NewContainer(nil, "", "root-value")
	if c == nil {
		t.Fatal("NewContainer(nil) returned nil")
	}
	if c.Path() != "root" {
		t.Fatalf("Path() = %q, want %q", c.Path(), "root")
	}
	if c.Parent() != nil {
		t.Fatal("Parent() should be nil for root")
	}
	if c.Root != c {
		t.Fatal("Root should be self for root container")
	}
	if got := c.Value(); got != "root-value" {
		t.Fatalf("Value() = %v, want %v", got, "root-value")
	}
	if c.GoString() != "root" {
		t.Fatalf("GoString() = %q, want %q", c.GoString(), "root")
	}
	if got := c.GetValue(); got != "root-value" {
		t.Fatalf("GetValue() = %v, want %v", got, "root-value")
	}
}

func TestNewContainerWithParent(t *testing.T) {
	parent := container.NewContainer(nil, "parent", "parent-value")
	child := container.NewContainer(parent, "parent.child", "child-value")

	if child.Path() != "parent.child" {
		t.Fatalf("Path() = %q, want %q", child.Path(), "parent.child")
	}
	if child.Parent() != parent {
		t.Fatal("Parent() should return the parent container")
	}
	if child.Root != parent.Root {
		t.Fatal("Root should be shared with parent")
	}
	if got := child.Value(); got != "child-value" {
		t.Fatalf("Value() = %v, want %v", got, "child-value")
	}
}

func TestAppendChildrenAndChildren(t *testing.T) {
	parent := container.NewContainer(nil, "parent", "parent-value")
	child1 := container.NewContainer(parent, "parent.c1", "v1")
	child2 := container.NewContainer(parent, "parent.c2", "v2")

	parent.AppendChildren(child1, child2)

	children, err := parent.Children()
	if err != nil {
		t.Fatalf("Children() error: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("Children() len = %d, want 2", len(children))
	}
	childPaths := make(map[string]bool)
	for _, c := range children {
		childPaths[c.Path()] = true
	}
	if !childPaths["parent.c1"] || !childPaths["parent.c2"] {
		t.Fatalf("unexpected children: %v", childPaths)
	}
}

func TestNewObject(t *testing.T) {
	parent := container.NewContainer(nil, "parent", "parent-value")
	child := parent.NewObject(parent, "parent.newchild", "newchild-value")

	if child.Path() != "parent.newchild" {
		t.Fatalf("Path() = %q, want %q", child.Path(), "parent.newchild")
	}
	if child.Parent() != parent {
		t.Fatal("new child Parent() should be parent")
	}
	if got := child.Value(); got != "newchild-value" {
		t.Fatalf("Value() = %v, want %v", got, "newchild-value")
	}
	// Child should be in parent's children
	children, _ := parent.Children()
	found := false
	for _, c := range children {
		if c == child {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("NewObject child not found in parent's children")
	}
}

func TestReadFromValueAndContainers(t *testing.T) {
	// NewContainer internally stores value and container in the index.
	root := container.NewContainer(nil, "root", "root-value")
	child := container.NewContainer(root, "root.child", "child-value")

	val, err := root.ReadFromValue("root")
	if err != nil {
		t.Fatalf("ReadFromValue('root') error: %v", err)
	}
	if val != "root-value" {
		t.Fatalf("ReadFromValue('root') = %v, want %v", val, "root-value")
	}

	val, err = root.ReadFromValue("root.child")
	if err != nil {
		t.Fatalf("ReadFromValue('root.child') error: %v", err)
	}
	if val != "child-value" {
		t.Fatalf("ReadFromValue('root.child') = %v, want %v", val, "child-value")
	}

	cont, err := root.ReadFromContainers("root.child")
	if err != nil {
		t.Fatalf("ReadFromContainers('root.child') error: %v", err)
	}
	if cont != child {
		t.Fatal("ReadFromContainers('root.child') should return the child container")
	}

	// ReadFromValue on missing path returns nil value, no error (index exists)
	val, err = root.ReadFromValue("missing.path")
	if err != nil {
		t.Fatalf("ReadFromValue('missing.path') unexpected error: %v", err)
	}
	if val != nil {
		t.Fatalf("ReadFromValue('missing.path') = %v, want nil", val)
	}

	// ReadFromContainers with invalid type: we can't easily inject a bad type
	// through public API, so just test missing path
	_, err = root.ReadFromContainers("missing.path")
	if err == nil {
		t.Fatal("expected error for missing container path")
	}
}

func TestRangeValues(t *testing.T) {
	root := container.NewContainer(nil, "root", "root-value")
	container.NewContainer(root, "root.a", 1)
	container.NewContainer(root, "root.b", 2)
	container.NewContainer(root, "root.c", 3)

	seen := make(map[string]interface{})
	err := root.RangeValues(func(key string, value interface{}) {
		seen[key] = value
	})
	if err != nil {
		t.Fatalf("RangeValues() error: %v", err)
	}
	// 4 entries: root's own value + 3 children
	if len(seen) != 4 {
		t.Fatalf("RangeValues() saw %d entries, want 4: %v", len(seen), seen)
	}
	if seen["root.a"] != 1 || seen["root.b"] != 2 || seen["root.c"] != 3 {
		t.Fatalf("unexpected range values: %v", seen)
	}
}

func TestRangeContainers(t *testing.T) {
	root := container.NewContainer(nil, "root", "root-value")
	container.NewContainer(root, "root.c1", "v1")
	container.NewContainer(root, "root.c2", "v2")

	seen := make(map[string]bool)
	err := root.RangeContainers(func(c *container.Container) {
		seen[c.Path()] = true
	})
	if err != nil {
		t.Fatalf("RangeContainers() error: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("RangeContainers() saw %d entries (root + 2 children), want 3: %v", len(seen), seen)
	}
	if !seen["root"] || !seen["root.c1"] || !seen["root.c2"] {
		t.Fatalf("unexpected range containers: %v", seen)
	}
}

func TestContainerNilParentPathRoot(t *testing.T) {
	c := container.NewContainer(nil, "anything", "anything-value")
	if c.Path() != "root" {
		t.Fatalf("Path() with nil parent = %q, want %q", c.Path(), "root")
	}
}
