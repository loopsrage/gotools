package index_test

import (
	"testing"

	"golangApp/lib/index"
)

func TestNewIndex(t *testing.T) {
	idx := index.NewIndex()
	if idx == nil {
		t.Fatal("NewIndex() returned nil")
	}
	if idx.Map == nil {
		t.Fatal("Map should be initialized")
	}
}

func TestStoreAndLoad(t *testing.T) {
	idx := index.NewIndex()
	idx.StoreInIndex("users", "alice", 30)
	idx.StoreInIndex("users", "bob", 25)

	val, err := idx.LoadFromIndex("users", "alice")
	if err != nil {
		t.Fatalf("LoadFromIndex() error: %v", err)
	}
	if val != 30 {
		t.Fatalf("LoadFromIndex('users', 'alice') = %v, want 30", val)
	}

	val, err = idx.LoadFromIndex("users", "bob")
	if err != nil {
		t.Fatalf("LoadFromIndex() error: %v", err)
	}
	if val != 25 {
		t.Fatalf("LoadFromIndex('users', 'bob') = %v, want 25", val)
	}
}

func TestLoadFromMissingIndex(t *testing.T) {
	idx := index.NewIndex()
	_, err := idx.LoadFromIndex("nonexistent", "key")
	if err == nil {
		t.Fatal("expected error for missing index")
	}
}

func TestLoadFromMissingKey(t *testing.T) {
	idx := index.NewIndex()
	idx.StoreInIndex("users", "alice", 30)
	val, err := idx.LoadFromIndex("users", "missing")
	if err != nil {
		t.Fatalf("LoadFromIndex() error: %v", err)
	}
	if val != nil {
		t.Fatalf("LoadFromIndex('users', 'missing') = %v, want nil", val)
	}
}

func TestLoadOrStoreInIndex(t *testing.T) {
	idx := index.NewIndex()
	val, loaded := idx.LoadOrStoreInIndex("cfg", "timeout", 30)
	if loaded {
		t.Fatal("first LoadOrStore should report loaded=false")
	}
	if val != 30 {
		t.Fatalf("LoadOrStore returned %v, want 30", val)
	}

	val, loaded = idx.LoadOrStoreInIndex("cfg", "timeout", 60)
	if !loaded {
		t.Fatal("second LoadOrStore should report loaded=true")
	}
	if val != 30 {
		t.Fatalf("LoadOrStore returned %v, want original 30", val)
	}
}

func TestDeleteFromIndex(t *testing.T) {
	idx := index.NewIndex()
	idx.StoreInIndex("users", "alice", 30)
	err := idx.DeleteFromIndex("users", "alice")
	if err != nil {
		t.Fatalf("DeleteFromIndex() error: %v", err)
	}
	val, err := idx.LoadFromIndex("users", "alice")
	if err != nil {
		t.Fatalf("LoadFromIndex() error after delete: %v", err)
	}
	if val != nil {
		t.Fatalf("LoadFromIndex('users', 'alice') after delete = %v, want nil", val)
	}
}

func TestDeleteFromMissingIndex(t *testing.T) {
	idx := index.NewIndex()
	err := idx.DeleteFromIndex("nonexistent", "key")
	if err == nil {
		t.Fatal("expected error for missing index on delete")
	}
}

func TestDeleteIndex(t *testing.T) {
	idx := index.NewIndex()
	idx.StoreInIndex("users", "alice", 30)
	idx.StoreInIndex("cfg", "timeout", 60)

	indexes := idx.ListIndexes()
	if len(indexes) != 2 {
		t.Fatalf("ListIndexes() = %d, want 2", len(indexes))
	}

	idx.DeleteIndex("users")
	indexes = idx.ListIndexes()
	if len(indexes) != 1 {
		t.Fatalf("after DeleteIndex('users'), ListIndexes() = %d, want 1", len(indexes))
	}
}

func TestRangeIndex(t *testing.T) {
	idx := index.NewIndex()
	idx.StoreInIndex("nums", "a", 1)
	idx.StoreInIndex("nums", "b", 2)
	idx.StoreInIndex("nums", "c", 3)

	count := 0
	err := idx.RangeIndex("nums", func(key, value interface{}) {
		count++
	})
	if err != nil {
		t.Fatalf("RangeIndex() error: %v", err)
	}
	if count != 3 {
		t.Fatalf("RangeIndex() called %d times, want 3", count)
	}
}

func TestRangeIndexMissing(t *testing.T) {
	idx := index.NewIndex()
	err := idx.RangeIndex("missing", func(key, value interface{}) {})
	if err == nil {
		t.Fatal("expected error for missing index on RangeIndex")
	}
}

func TestListIndexes(t *testing.T) {
	idx := index.NewIndex()
	if got := idx.ListIndexes(); len(got) != 0 {
		t.Fatalf("ListIndexes() on empty = %v, want empty", got)
	}

	idx.StoreInIndex("a", "k", "v")
	idx.StoreInIndex("b", "k", "v")
	idx.StoreInIndex("c", "k", "v")

	indexes := idx.ListIndexes()
	if len(indexes) != 3 {
		t.Fatalf("ListIndexes() = %d, want 3", len(indexes))
	}
}

func TestStoreInIndexCreatesIndex(t *testing.T) {
	idx := index.NewIndex()
	// Storing into a non-existent index should create it
	idx.StoreInIndex("new-index", "key", "value")
	val, err := idx.LoadFromIndex("new-index", "key")
	if err != nil {
		t.Fatalf("LoadFromIndex() error: %v", err)
	}
	if val != "value" {
		t.Fatalf("LoadFromIndex('new-index', 'key') = %v, want %v", val, "value")
	}
}

func TestLoadOrStoreInIndexMissingIndexCreates(t *testing.T) {
	idx := index.NewIndex()
	val, loaded := idx.LoadOrStoreInIndex("auto-created", "k", "v")
	if loaded {
		t.Fatal("first call should not be loaded")
	}
	if val != "v" {
		t.Fatalf("LoadOrStore returned %v, want %v", val, "v")
	}
}
