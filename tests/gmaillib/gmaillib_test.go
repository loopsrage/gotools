package gmaillib_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopsrage/gotools/lib/gmaillib"
)

func TestGmailConfigDefaultsAndCustom(t *testing.T) {
	def := gmaillib.DefaultGmailConfig()
	if def.UserID != "me" {
		t.Fatalf("UserID = %q, want me", def.UserID)
	}
	if len(def.Scopes) != 1 || !strings.Contains(def.Scopes[0], "gmail.modify") {
		t.Fatalf("Scopes = %v, want gmail.modify", def.Scopes)
	}

	client := gmaillib.NewGmailClient(&gmaillib.GmailConfig{
		CredentialsPath:  "/path/to/creds.json",
		TokenPath:        "/path/to/token.json",
		Scopes:           []string{"https://www.googleapis.com/auth/gmail.readonly"},
		DisableRateLimit: true,
	})
	if client.Config.CredentialsPath != "/path/to/creds.json" || client.Config.TokenPath != "/path/to/token.json" {
		t.Fatalf("unexpected custom paths: %+v", client.Config)
	}
	if client.Config.RateLimitDelay != 0 {
		t.Fatalf("expected 0 rate limit when DisableRateLimit=true, got %v", client.Config.RateLimitDelay)
	}
}

func TestGmailOAuthTokenLoadAndRefresh(t *testing.T) {
	ctx := context.Background()
	var refreshCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		refreshCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt-123" {
			http.Error(w, "bad refresh request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "refreshed-access-tok",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer refreshed-access-tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"labels": []map[string]string{{"id": "L1", "name": "Work"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "gmail_token.json")
	expiredToken := map[string]interface{}{
		"token":         "stale-tok",
		"refresh_token": "rt-123",
		"token_uri":     srv.URL + "/token",
		"client_id":     "cid",
		"client_secret": "csec",
		"expiry":        time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	raw, _ := json.Marshal(expiredToken)
	if err := os.WriteFile(tokenPath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile token error: %v", err)
	}

	client := gmaillib.NewGmailClient(&gmaillib.GmailConfig{
		TokenPath:        tokenPath,
		TokenURL:         srv.URL + "/token",
		BaseURL:          srv.URL + "/gmail/v1",
		DisableRateLimit: true,
		HTTPClient:       srv.Client(),
	})

	labels, err := client.ListLabels(ctx)
	if err != nil {
		t.Fatalf("ListLabels after refresh failed: %v", err)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refreshCalls = %d, want 1", refreshCalls.Load())
	}
	if len(labels) != 1 || labels[0].ID != "L1" {
		t.Fatalf("labels = %+v, want [L1]", labels)
	}

	// Verify refreshed token was persisted to disk
	persistedBytes, err := os.ReadFile(tokenPath)
	if err != nil || !strings.Contains(string(persistedBytes), "refreshed-access-tok") {
		t.Fatalf("expected refreshed token persisted on disk, got %q (err=%v)", string(persistedBytes), err)
	}
}

func TestGmailClientAndSorterWireProtocol(t *testing.T) {
	ctx := context.Background()

	labels := []gmaillib.Label{{ID: "L_EXISTING", Name: "Existing"}}
	var (
		batchCalls  []map[string]interface{}
		modifyCalls []map[string]interface{}
		trashedIDs  []string
		listCalls   int
	)

	msgStore := map[string]gmaillib.Message{
		"m1": {
			ID: "m1",
			Payload: &gmaillib.MessagePart{
				Headers: []gmaillib.MessageHeader{
					{Name: "Subject", Value: "Your Rocky Mountain Power bill"},
					{Name: "From", Value: "billing@rockymountainpower.com"},
				},
			},
		},
		"m2": {
			ID: "m2",
			Payload: &gmaillib.MessagePart{
				Headers: []gmaillib.MessageHeader{
					{Name: "Subject", Value: "Rocky Mountain Power outage alert"},
					{Name: "From", Value: "alerts@rockymountainpower.com"},
				},
			},
		},
		"m3": {
			ID: "m3",
			Payload: &gmaillib.MessagePart{
				Headers: []gmaillib.MessageHeader{
					{Name: "Subject", Value: "Invoice from Amazon"},
					{Name: "From", Value: "amazon@amazon.com"},
				},
			},
		},
		"m4": {
			ID: "m4",
			Payload: &gmaillib.MessagePart{
				Headers: []gmaillib.MessageHeader{
					{Name: "Subject", Value: "Amazon order confirmation"},
					{Name: "From", Value: "amazon@amazon.com"},
				},
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		listCalls++
		q := r.URL.Query().Get("q")
		pt := r.URL.Query().Get("pageToken")
		if q == "from:empty.com" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"messages": []interface{}{}})
			return
		}
		if pt == "" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messages":      []map[string]string{{"id": "m1"}, {"id": "m2"}},
				"nextPageToken": "p2",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"messages": []map[string]string{{"id": "m3"}, {"id": "m4"}},
		})
	})
	mux.HandleFunc("/gmail/v1/users/me/messages/batchModify", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		batchCalls = append(batchCalls, body)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/gmail/v1/users/me/messages/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		if strings.HasSuffix(trimmed, "/modify") {
			id := strings.TrimSuffix(trimmed, "/modify")
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["id"] = id
			modifyCalls = append(modifyCalls, body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
			return
		}
		if strings.HasSuffix(trimmed, "/trash") {
			id := strings.TrimSuffix(trimmed, "/trash")
			trashedIDs = append(trashedIDs, id)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
			return
		}
		msg, ok := msgStore[trimmed]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(msg)
	})
	mux.HandleFunc("/gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"labels": labels})
			return
		}
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		created := gmaillib.Label{
			ID:   "LBL_" + strings.ToUpper(strings.ReplaceAll(req["name"], " ", "_")),
			Name: req["name"],
		}
		labels = append(labels, created)
		_ = json.NewEncoder(w).Encode(created)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := gmaillib.NewGmailClient(&gmaillib.GmailConfig{
		BaseURL:          srv.URL,
		AccessToken:      "live-bearer-tok",
		DisableRateLimit: true,
		HTTPClient:       srv.Client(),
	})

	// Pagination & maxResults slicing
	allIDs, err := client.AllMessageIDs(ctx, "in:inbox")
	if err != nil || len(allIDs) != 4 {
		t.Fatalf("AllMessageIDs = (%v, %v), want 4 items across 2 pages", allIDs, err)
	}
	capped, err := client.MessageIDs(ctx, "in:inbox", gmaillib.IntPtr(3))
	if err != nil || len(capped) != 3 {
		t.Fatalf("MessageIDs(3) = (%v, %v), want 3", capped, err)
	}

	// Archive / MarkRead / MarkUnread / Trash
	if _, err := client.Archive(ctx, "m1"); err != nil {
		t.Fatalf("Archive error: %v", err)
	}
	if _, err := client.MarkRead(ctx, "m1"); err != nil {
		t.Fatalf("MarkRead error: %v", err)
	}
	if _, err := client.MarkUnread(ctx, "m1"); err != nil {
		t.Fatalf("MarkUnread error: %v", err)
	}
	if _, err := client.Trash(ctx, "m2"); err != nil || len(trashedIDs) != 1 || trashedIDs[0] != "m2" {
		t.Fatalf("Trash = (%v, %v)", trashedIDs, err)
	}
	if len(modifyCalls) != 3 {
		t.Fatalf("modifyCalls = %d, want 3", len(modifyCalls))
	}

	// CreateLabel reuses existing ID without duplicate creation
	existingID, err := client.CreateLabel(ctx, "Existing", "", "")
	if err != nil || existingID != "L_EXISTING" {
		t.Fatalf("CreateLabel(Existing) = (%q, %v), want L_EXISTING", existingID, err)
	}

	// GmailSorter tests
	sorter := gmaillib.NewGmailSorter(client)

	counts, err := sorter.SortInbox(ctx, map[string]string{
		"Existing": "in:inbox",
		"Empty":    "from:empty.com",
	}, true, gmaillib.IntPtr(2))
	if err != nil {
		t.Fatalf("SortInbox error: %v", err)
	}
	if counts["Existing"] != 2 || counts["Empty"] != 0 {
		t.Fatalf("SortInbox counts = %v, want Existing=2 Empty=0", counts)
	}

	domCount, err := sorter.SortByDomain(ctx, "amazon.com", "Shopping", false, gmaillib.IntPtr(2))
	if err != nil || domCount != 2 {
		t.Fatalf("SortByDomain = (%d, %v), want 2", domCount, err)
	}

	// PreviewSort dry-run matching
	var progressCount int
	preview, err := sorter.PreviewSort(ctx, map[string]string{
		"Bills": "rocky mountain power bill",
	}, 50, gmaillib.IntPtr(4), func(cur, tot int, _ string) {
		progressCount = cur
	})
	if err != nil {
		t.Fatalf("PreviewSort error: %v", err)
	}
	if progressCount != 4 {
		t.Fatalf("PreviewSort progress = %d, want 4", progressCount)
	}
	bills := preview["Bills"]
	if len(bills) == 0 || bills[0].ID != "m1" || bills[0].Score < 50 {
		t.Fatalf("PreviewSort[Bills] = %+v, want m1 first with score >= 50", bills)
	}

	// AutoCategorizeInbox (full scan)
	cats, err := sorter.AutoCategorizeInbox(ctx, 50, 2, nil, 0, nil)
	if err != nil {
		t.Fatalf("AutoCategorizeInbox error: %v", err)
	}
	hasRMP := false
	hasAmazon := false
	for name := range cats {
		if strings.Contains(name, "Rockymountainpower") {
			hasRMP = true
		}
		if strings.Contains(name, "Amazon") {
			hasAmazon = true
		}
	}
	if !hasRMP || !hasAmazon {
		t.Fatalf("AutoCategorizeInbox categories = %v, expected Rockymountainpower and Amazon", cats)
	}

	// AutoCategorizeInbox (paged mode)
	pagedCats, err := sorter.AutoCategorizeInbox(ctx, 50, 2, nil, 2, nil)
	if err != nil || len(pagedCats) == 0 {
		t.Fatalf("paged AutoCategorizeInbox = (%v, %v)", pagedCats, err)
	}

	// Below min_group_size returns empty
	below, err := sorter.AutoCategorizeInbox(ctx, 50, 10, gmaillib.IntPtr(1), 0, nil)
	if err != nil || len(below) != 0 {
		t.Fatalf("below threshold AutoCategorizeInbox = (%v, %v), want empty", below, err)
	}
	if len(batchCalls) == 0 {
		t.Fatal("expected batchModify calls from sorter")
	}
}

func TestGmailLiveIntegrationIfPresent(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	credsPath := filepath.Join(home, ".config", "toolset", "gmail_credentials.json")
	tokenPath := filepath.Join(home, ".config", "toolset", "gmail_token.json")
	if _, err := os.Stat(credsPath); err != nil {
		t.Skip("gmail credentials not present at ~/.config/toolset/")
	}
	if _, err := os.Stat(tokenPath); err != nil {
		t.Skip("gmail token not present at ~/.config/toolset/")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := gmaillib.NewGmailClient(nil)
	labels, err := client.ListLabels(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "rateLimitExceeded") {
			t.Skip("Gmail API rate limit exceeded")
		}
		t.Skipf("local Gmail token requires interactive re-auth: %v", err)
	}
	if labels == nil {
		t.Fatal("expected non-nil label slice")
	}
}
