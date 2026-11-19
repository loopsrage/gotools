package informationalindex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loopsrage/go_mltools/ppsclient"
	"github.com/loopsrage/gotools/lib/emaillib"
	"github.com/loopsrage/gotools/lib/featureflags"
	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/idlib"
	"github.com/loopsrage/gotools/lib/mcplib"
	"github.com/loopsrage/gotools/lib/resilience"
	"github.com/loopsrage/gotools/lib/secretslib"
	"github.com/loopsrage/gotools/lib/settings"
	"github.com/loopsrage/gotools/lib/webhooklib"
)

func TestInformationalIndexAndGotoolsSuite(t *testing.T) {
	// Test PPS-backed AgentInformationalGraphIndex + GeodesicLifecycleMCPServer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/collections":
			_, _ = w.Write([]byte(`{"name":"agent_informational_graph"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/points"):
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search"):
			_ = json.NewEncoder(w).Encode(map[string]any{"points": []map[string]any{{"id": "n1", "score": 0.95, "payload": map[string]any{"content": "hello"}}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/topics":
			_ = json.NewEncoder(w).Encode(map[string]any{"topics": []map[string]any{{"path": "demo", "record_count": 3}}})
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	cfg := ppsclient.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.OAuthToken = "tok"
	cfg.ShipLogs = false
	client := ppsclient.NewClient(cfg)

	idx := NewAgentInformationalGraphIndex(client, "agent_informational_graph", 64)
	nid, err := idx.UpsertNode(context.Background(), "", "demo", "Go vector index", nil)
	if err != nil || nid == "" {
		t.Fatalf("UpsertNode: id=%q err=%v", nid, err)
	}
	hits, err := idx.Search(context.Background(), "vector", 5, nil)
	if err != nil || len(hits) != 1 {
		t.Fatalf("Search: hits=%v err=%v", hits, err)
	}

	mcpSrv := NewGeodesicLifecycleMCPServer(client)
	if len(mcpSrv.ToolNames()) != 6 || len(mcpSrv.PromptNames()) != 2 {
		t.Fatalf("unexpected geodesic MCP catalog: tools=%v prompts=%v", mcpSrv.ToolNames(), mcpSrv.PromptNames())
	}
	resp := mcpSrv.HandleRequest(context.Background(), mcplib.Request{
		JSONRPC: "2.0", ID: 1, Method: "tools/call",
		Params: json.RawMessage(`{"name":"topic_list","arguments":{}}`),
	})
	if resp == nil || resp.Error != nil {
		t.Fatalf("topic_list call failed: %+v", resp)
	}

	// Quick verification of FSBaseCrudService, settings, idlib, resilience, featureflags, secrets, webhook, email
	crud := NewFSBaseCrudService(fileslib.NewMemoryFS())
	if err := crud.Write(context.Background(), "/a/b.txt", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if st, err := crud.Stat(context.Background(), "/a/b.txt"); err != nil || st.Size != 2 {
		t.Fatalf("Stat: %+v err=%v", st, err)
	}
	if err := crud.Delete(context.Background(), "/a/b.txt"); err != nil {
		t.Fatal(err)
	}
	ss, err := settings.LoadSettingsStoreFromBytes([]byte("server:\n  port: 8001\n"))
	if err != nil || ss.GetInt("server.port", 0) != 8001 {
		t.Fatalf("SettingsStore: %v", err)
	}
	if idlib.UUID5FromStrings("a", "b") != idlib.UUID5FromStrings("a", "b") {
		t.Fatal("UUID5 non-deterministic")
	}
	cb := resilience.NewCircuitBreaker("test", 2, 0)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.Allow() {
		t.Fatal("circuit should be open")
	}
	ff := featureflags.NewStore()
	ff.Set(featureflags.FeatureFlag{Name: "beta", Enabled: true, RolloutPct: 100})
	if !ff.Evaluate("beta", "u1", nil) {
		t.Fatal("flag should evaluate true")
	}
	sec := secretslib.NewStore()
	sec.Set("API_KEY", "secret123")
	if sec.Get("API_KEY", "") != "secret123" {
		t.Fatal("secret mismatch")
	}
	if sms, err := emaillib.FormatGoogleFiSMSAddress("+1 (555) 123-4567"); err != nil || sms != "5551234567@msg.fi.google.com" {
		t.Fatalf("SMS format: %q %v", sms, err)
	}
	if got := emaillib.RenderTemplate("Hello {name}!", map[string]any{"name": "Go"}); got != "Hello Go!" {
		t.Fatalf("RenderTemplate: %q", got)
	}
	wq := webhooklib.NewWebhookQueue(0, 0)
	wq.Enqueue(webhooklib.Event{ID: "e1", URL: srv.URL, Payload: map[string]any{"ok": true}})
	if d, f := wq.Flush(context.Background()); d != 1 || f != 0 {
		t.Fatalf("webhook flush: d=%d f=%d", d, f)
	}
}
