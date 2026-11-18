package auditlib_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/loopsrage/gotools/lib/auditlib"
)

func TestAppendAndVerifyChain(t *testing.T) {
	log := auditlib.NewAuditLog("test_chain")
	log.Append("alice", "create", "order/1", map[string]interface{}{"amount": 100})
	log.Append("bob", "update", "order/1", map[string]interface{}{"amount": 200})

	if got := log.Count(); got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}
	if !log.Verify() {
		t.Fatal("Verify() = false, want true on unmodified chain")
	}
}

func TestTamperDetection(t *testing.T) {
	log := auditlib.NewAuditLog("test_tamper")
	log.Append("alice", "create", "order/1", map[string]interface{}{"amount": 100})

	entries := log.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(Entries()) = %d, want 1", len(entries))
	}
	tampered := entries[0]
	tampered["details"] = map[string]interface{}{"amount": 9999}
	log.Index().StoreInIndex(log.Name(), entries[0]["entry_id"], tampered)

	if log.Verify() {
		t.Fatal("Verify() = true, want false after payload tampering")
	}
}

func TestBrokenChainDetection(t *testing.T) {
	log := auditlib.NewAuditLog("test_chain_break")
	log.Append("alice", "create", "r/1", map[string]interface{}{})
	log.Append("bob", "update", "r/2", map[string]interface{}{})

	entries := log.Entries()
	e2 := entries[1]
	e2["previous_hash"] = "deadbeef"
	log.Index().StoreInIndex(log.Name(), e2["entry_id"], e2)

	if log.Verify() {
		t.Fatal("Verify() = true, want false after breaking previous_hash link")
	}
}

func TestDeletionAndInjectionDetection(t *testing.T) {
	log := auditlib.NewAuditLog("test_delete_inject")
	e1 := log.Append("alice", "create", "r/1", nil)
	log.Append("bob", "update", "r/1", nil)

	// Inject an out-of-band rogue entry
	rogue := auditlib.NewAuditEntry("mallory", "delete", "r/1", nil)
	log.Index().StoreInIndex(log.Name(), rogue.EntryID, rogue.ToMap())
	if log.Verify() {
		t.Fatal("Verify() = true, want false when rogue entry is injected")
	}
	_ = log.Index().DeleteFromIndex(log.Name(), rogue.EntryID)
	if !log.Verify() {
		t.Fatal("Verify() should recover once rogue entry is removed")
	}

	// Delete legitimate entry
	_ = log.Index().DeleteFromIndex(log.Name(), e1.EntryID)
	if log.Verify() {
		t.Fatal("Verify() = true, want false when entry is deleted")
	}
}

func TestGetEntryAndConcurrentAppend(t *testing.T) {
	log := auditlib.NewAuditLog("test_get")
	entry := log.Append("alice", "create", "r/1", map[string]interface{}{"k": "v"})

	fetched, ok := log.Get(entry.EntryID)
	if !ok || fetched == nil {
		t.Fatal("Get() failed to retrieve appended entry")
	}
	if fetched["actor"] != "alice" || fetched["action"] != "create" {
		t.Fatalf("Get() = %v, want actor=alice action=create", fetched)
	}
	if _, found := log.Get("nonexistent"); found {
		t.Fatal("Get(nonexistent) should return false")
	}

	// Concurrent stress test
	cLog := auditlib.NewAuditLog("concurrent_chain")
	var wg sync.WaitGroup
	const workers = 8
	const perWorker = 25
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				cLog.Append(
					fmt.Sprintf("worker-%d", workerID),
					"write",
					fmt.Sprintf("item/%d/%d", workerID, i),
					map[string]interface{}{"seq": i},
				)
			}
		}(w)
	}
	wg.Wait()

	if cLog.Count() != workers*perWorker {
		t.Fatalf("concurrent Count() = %d, want %d", cLog.Count(), workers*perWorker)
	}
	if !cLog.Verify() {
		t.Fatal("concurrent AuditLog failed hash-chain Verify()")
	}
}
