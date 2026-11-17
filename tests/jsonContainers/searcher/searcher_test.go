package searcher_test

import (
	"strings"
	"testing"

	"golangApp/lib/jsonContainers/container"
	"golangApp/lib/jsonContainers/containerTreeBuilder"
	"golangApp/lib/jsonContainers/searcher"
)

func buildTestContainer() *container.Container {
	data := map[string]interface{}{
		"users": map[string]interface{}{
			"alice": map[string]interface{}{"role": "admin", "active": true},
			"bob":   map[string]interface{}{"role": "user", "active": false},
		},
		"config": map[string]interface{}{"debug": true},
	}
	return containerTreeBuilder.BuildContainerTree(nil, []string{"root"}, '.', data)
}

func TestDfsNil(t *testing.T) {
	output := searcher.Dfs(nil, nil)
	if output != nil {
		t.Fatalf("Dfs(nil) = %v, want nil", output)
	}
}

func TestDfsWithSearcher(t *testing.T) {
	root := buildTestContainer()
	ks := &keySearcherMock{key: "role", found: 0}
	searcher.Dfs(root, ks)
	if ks.found != 2 {
		t.Fatalf("Dfs found %d containers with 'role', want 2", ks.found)
	}
}

type keySearcherMock struct {
	key   string
	found int
}

func (k *keySearcherMock) Current(cnt *container.Container) {
	switch v := cnt.Value().(type) {
	case map[string]interface{}:
		if _, ok := v[k.key]; ok {
			k.found++
		}
	}
}

func TestFlatten(t *testing.T) {
	root := buildTestContainer()
	results := searcher.Flatten(root)
	if len(results) == 0 {
		t.Fatal("Flatten() returned empty results")
	}
	// Should contain leaf scalar values with flattened keys
	hasDebug := false
	hasRole := false
	for key, val := range results {
		keyStr, _ := key.(string)
		if strings.HasSuffix(keyStr, "debug") {
			hasDebug = true
			if val != true {
				t.Fatalf("debug value = %v, want true", val)
			}
		}
		if strings.HasSuffix(keyStr, "role") {
			hasRole = true
		}
	}
	if !hasDebug {
		t.Fatal("Flatten() results missing 'debug' key")
	}
	if !hasRole {
		t.Fatal("Flatten() results missing 'role' key")
	}
}

func TestFindObjectsWithKey(t *testing.T) {
	root := buildTestContainer()
	results := searcher.FindObjectsWithKey(root, "role")
	if len(results) != 2 {
		t.Fatalf("FindObjectsWithKey('role') = %d, want 2", len(results))
	}
	// Both alice and bob should have 'role' key
	foundAlice := false
	foundBob := false
	for path, cnt := range results {
		if path == "root.users.alice" {
			foundAlice = true
			if cnt.Value() == nil {
				t.Fatal("alice container has nil value")
			}
		}
		if path == "root.users.bob" {
			foundBob = true
		}
	}
	if !foundAlice || !foundBob {
		t.Fatalf("expected both alice and bob, found alice=%v bob=%v", foundAlice, foundBob)
	}
}

func TestFindObjectsWithKeyMissing(t *testing.T) {
	root := buildTestContainer()
	results := searcher.FindObjectsWithKey(root, "nonexistent")
	if len(results) != 0 {
		t.Fatalf("FindObjectsWithKey('nonexistent') = %d, want 0", len(results))
	}
}

func TestFindObjectsWithKeys(t *testing.T) {
	root := buildTestContainer()
	results := searcher.FindObjectsWithKeys(root, "role", "debug", "missing")
	if len(results) != 3 {
		t.Fatalf("FindObjectsWithKeys() = %d keys, want 3", len(results))
	}
	if len(results["role"]) != 2 {
		t.Fatalf("results['role'] = %d, want 2", len(results["role"]))
	}
	if len(results["debug"]) != 1 {
		t.Fatalf("results['debug'] = %d, want 1", len(results["debug"]))
	}
	if len(results["missing"]) != 0 {
		t.Fatalf("results['missing'] = %d, want 0", len(results["missing"]))
	}
}

func TestKeySearcherCurrent(t *testing.T) {
	root := container.NewContainer(nil, "root", map[string]interface{}{"name": "test"})
	child := container.NewContainer(root, "root.child", map[string]interface{}{"other": 1})

	s := &searcher.KeySearcher{Key: "name"}
	s.Current(root)
	s.Current(child)

	if len(s.Results) != 1 {
		t.Fatalf("KeySearcher found %d results, want 1", len(s.Results))
	}
	if s.Results[0] != root {
		t.Fatal("KeySearcher should return the root container")
	}
}
