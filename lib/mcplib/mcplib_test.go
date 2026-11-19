package mcplib

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPServerHTTPBatchAndStdio(t *testing.T) {
	srv := NewServer("test-mcp", "1.0.0")
	srv.RegisterTool(Tool{
		Name:        "echo",
		Description: "echo input",
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			return map[string]any{"echo": args["msg"]}, nil
		},
	})
	srv.RegisterResource(Resource{
		URI:  "test://status",
		Name: "status",
		Handler: func(_ context.Context, _ string) (string, string, error) {
			return `{"ok":true}`, "application/json", nil
		},
	})
	srv.RegisterPrompt(Prompt{
		Name: "greet",
		Handler: func(_ context.Context, args map[string]string) ([]PromptMessage, error) {
			return []PromptMessage{NewTextPromptMessage("user", "hi "+args["who"])}, nil
		},
	})

	// Batch HTTP request
	batch := `[{"jsonrpc":"2.0","id":1,"method":"initialize"},{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"msg":"hello"}}},{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"test://status"}},{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"greet","arguments":{"who":"go"}}}]`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(batch))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var resps []Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resps); err != nil || len(resps) != 4 {
		t.Fatalf("batch decode: len=%d err=%v body=%s", len(resps), err, rec.Body.String())
	}

	// Stdio transport
	var out bytes.Buffer
	if err := srv.RunStdio(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"ping"}`+"\n"), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}
	if !strings.Contains(out.String(), `"pong":true`) {
		t.Fatalf("stdio output unexpected: %s", out.String())
	}
}
