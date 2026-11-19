package mcplib

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
)

// ProtocolVersion is the MCP JSON-RPC protocol revision advertised during initialize.
const ProtocolVersion = "2024-11-05"

// Tool describes a callable MCP tool and its JSON Schema input specification.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     func(ctx context.Context, args map[string]any) (any, error) `json:"-"`
}

// Resource describes a readable MCP resource URI.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Handler     func(ctx context.Context, uri string) (string, string, error) `json:"-"` // returns (content, mimeType, err)
}

// PromptArgument describes a prompt template parameter.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt describes an MCP prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Handler     func(ctx context.Context, args map[string]string) ([]PromptMessage, error) `json:"-"`
}

// PromptMessage represents a single role/content prompt message.
type PromptMessage struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// NewTextPromptMessage constructs a PromptMessage with text content.
func NewTextPromptMessage(role, text string) PromptMessage {
	var m PromptMessage
	m.Role = role
	m.Content.Type = "text"
	m.Content.Text = text
	return m
}

// Request models an inbound JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError models a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Response models an outbound JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// Server is a reusable JSON-RPC 2.0 MCP server supporting HTTP (single + batch) and stdio transports.
type Server struct {
	mu        sync.RWMutex
	Name      string
	Version   string
	tools     map[string]Tool
	resources map[string]Resource
	prompts   map[string]Prompt
}

// NewServer creates a named MCP Server.
func NewServer(name, version string) *Server {
	if version == "" {
		version = "1.0.0"
	}
	return &Server{
		Name:      name,
		Version:   version,
		tools:     make(map[string]Tool),
		resources: make(map[string]Resource),
		prompts:   make(map[string]Prompt),
	}
}

// RegisterTool adds or replaces a tool on the server.
func (s *Server) RegisterTool(t Tool) {
	if t.InputSchema == nil {
		t.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	s.mu.Lock()
	s.tools[t.Name] = t
	s.mu.Unlock()
}

// RegisterResource adds or replaces a resource URI on the server.
func (s *Server) RegisterResource(r Resource) {
	if r.MimeType == "" {
		r.MimeType = "application/json"
	}
	s.mu.Lock()
	s.resources[r.URI] = r
	s.mu.Unlock()
}

// RegisterPrompt adds or replaces a prompt template on the server.
func (s *Server) RegisterPrompt(p Prompt) {
	s.mu.Lock()
	s.prompts[p.Name] = p
	s.mu.Unlock()
}

// ToolNames returns sorted registered tool names.
func (s *Server) ToolNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.tools))
	for k := range s.tools {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ResourceURIs returns sorted registered resource URIs.
func (s *Server) ResourceURIs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.resources))
	for k := range s.resources {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PromptNames returns sorted registered prompt names.
func (s *Server) PromptNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.prompts))
	for k := range s.prompts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// HandleRequest processes a single JSON-RPC 2.0 request and returns nil for notifications.
func (s *Server) HandleRequest(ctx context.Context, req Request) *Response {
	switch req.Method {
	case "initialize":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": ProtocolVersion,
				"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
				"capabilities": map[string]any{
					"tools":     map[string]any{"listChanged": false},
					"resources": map[string]any{"subscribe": false, "listChanged": false},
					"prompts":   map[string]any{"listChanged": false},
				},
			},
		}
	case "notifications/initialized", "initialized":
		if req.ID == nil {
			return nil
		}
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "ping":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"pong": true}}
	case "tools/list":
		s.mu.RLock()
		names := make([]string, 0, len(s.tools))
		for k := range s.tools {
			names = append(names, k)
		}
		sort.Strings(names)
		list := make([]Tool, 0, len(names))
		for _, k := range names {
			list = append(list, s.tools[k])
		}
		s.mu.RUnlock()
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": list}}
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errResp(req.ID, -32602, "invalid params: "+err.Error())
		}
		if params.Arguments == nil {
			params.Arguments = make(map[string]any)
		}
		s.mu.RLock()
		tool, ok := s.tools[params.Name]
		s.mu.RUnlock()
		if !ok || tool.Handler == nil {
			return errResp(req.ID, -32601, fmt.Sprintf("unknown tool: %s", params.Name))
		}
		val, err := tool.Handler(ctx, params.Arguments)
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"isError": true,
					"content": []map[string]any{{"type": "text", "text": err.Error()}},
				},
			}
		}
		var text string
		switch tv := val.(type) {
		case string:
			text = tv
		default:
			b, _ := json.Marshal(tv)
			text = string(b)
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"isError": false,
				"content": []map[string]any{{"type": "text", "text": text}},
			},
		}
	case "resources/list":
		s.mu.RLock()
		uris := make([]string, 0, len(s.resources))
		for k := range s.resources {
			uris = append(uris, k)
		}
		sort.Strings(uris)
		list := make([]Resource, 0, len(uris))
		for _, u := range uris {
			list = append(list, s.resources[u])
		}
		s.mu.RUnlock()
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resources": list}}
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errResp(req.ID, -32602, "invalid params: "+err.Error())
		}
		s.mu.RLock()
		res, ok := s.resources[params.URI]
		s.mu.RUnlock()
		if !ok || res.Handler == nil {
			return errResp(req.ID, -32602, fmt.Sprintf("unknown resource: %s", params.URI))
		}
		text, mime, err := res.Handler(ctx, params.URI)
		if err != nil {
			return errResp(req.ID, -32000, err.Error())
		}
		if mime == "" {
			mime = res.MimeType
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"contents": []map[string]any{
					{"uri": params.URI, "mimeType": mime, "text": text},
				},
			},
		}
	case "prompts/list":
		s.mu.RLock()
		names := make([]string, 0, len(s.prompts))
		for k := range s.prompts {
			names = append(names, k)
		}
		sort.Strings(names)
		list := make([]Prompt, 0, len(names))
		for _, n := range names {
			list = append(list, s.prompts[n])
		}
		s.mu.RUnlock()
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"prompts": list}}
	case "prompts/get":
		var params struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errResp(req.ID, -32602, "invalid params: "+err.Error())
		}
		s.mu.RLock()
		pr, ok := s.prompts[params.Name]
		s.mu.RUnlock()
		if !ok || pr.Handler == nil {
			return errResp(req.ID, -32601, fmt.Sprintf("unknown prompt: %s", params.Name))
		}
		msgs, err := pr.Handler(ctx, params.Arguments)
		if err != nil {
			return errResp(req.ID, -32000, err.Error())
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"description": pr.Description,
				"messages":    msgs,
			},
		}
	default:
		return errResp(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func errResp(id any, code int, msg string) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
}

// ServeHTTP handles HTTP GET (server descriptor) and HTTP POST (single or batch JSON-RPC 2.0).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":            s.Name,
			"version":         s.Version,
			"protocolVersion": ProtocolVersion,
			"tools_count":     len(s.ToolNames()),
			"resources_count": len(s.ResourceURIs()),
			"prompts_count":   len(s.PromptNames()),
		})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(errResp(nil, -32600, "POST required"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errResp(nil, -32700, err.Error()))
		return
	}
	trimmed := trimLeftSpace(raw)
	if len(trimmed) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errResp(nil, -32700, "empty request body"))
		return
	}
	if trimmed[0] == '[' {
		var batch []Request
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errResp(nil, -32700, err.Error()))
			return
		}
		var out []*Response
		for _, req := range batch {
			if resp := s.HandleRequest(r.Context(), req); resp != nil {
				out = append(out, resp)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	var req Request
	if err := json.Unmarshal(trimmed, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errResp(nil, -32700, err.Error()))
		return
	}
	resp := s.HandleRequest(r.Context(), req)
	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// RunStdio runs newline-delimited JSON-RPC 2.0 over r and w until EOF or ctx cancellation.
func (s *Server) RunStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		line := trimLeftSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(errResp(nil, -32700, err.Error()))
			continue
		}
		if resp := s.HandleRequest(ctx, req); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func trimLeftSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
		i++
	}
	return b[i:]
}
