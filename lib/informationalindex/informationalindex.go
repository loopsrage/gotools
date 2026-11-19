package informationalindex

import (
	"context"
	"fmt"
	"strings"

	"github.com/loopsrage/go_mltools/ppsclient"
	"github.com/loopsrage/go_mltools/proceduralphasespace"
	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/idlib"
	"github.com/loopsrage/gotools/lib/mcplib"
)

// FSBaseCrudService exposes unified file CRUD operations over any fileslib.FS backend.
type FSBaseCrudService struct {
	FS fileslib.FS
}

// NewFSBaseCrudService wraps a fileslib.FS implementation.
func NewFSBaseCrudService(fs fileslib.FS) *FSBaseCrudService {
	return &FSBaseCrudService{FS: fs}
}

func (s *FSBaseCrudService) Read(ctx context.Context, path string) ([]byte, error) {
	return s.FS.Read(ctx, path)
}

func (s *FSBaseCrudService) Write(ctx context.Context, path string, data []byte) error {
	return s.FS.Write(ctx, path, data)
}

func (s *FSBaseCrudService) Delete(ctx context.Context, path string) error {
	return fileslib.Delete(ctx, s.FS, path)
}

func (s *FSBaseCrudService) List(ctx context.Context, pattern string) ([]string, error) {
	return s.FS.List(ctx, pattern)
}

func (s *FSBaseCrudService) Stat(ctx context.Context, path string) (fileslib.FileInfo, error) {
	return fileslib.Stat(ctx, s.FS, path)
}

// AgentInformationalGraphIndex manages agent knowledge/staging graph nodes strictly via
// go_mltools/ppsclient (zero PostgreSQL pgvector dependency).
type AgentInformationalGraphIndex struct {
	PPS        *ppsclient.Client
	Collection string
	Dim        int
}

// NewAgentInformationalGraphIndex constructs a PPS-backed informational graph index.
func NewAgentInformationalGraphIndex(client *ppsclient.Client, collection string, dim int) *AgentInformationalGraphIndex {
	if collection == "" {
		collection = "agent_informational_graph"
	}
	if dim <= 0 {
		dim = 128
	}
	return &AgentInformationalGraphIndex{
		PPS:        client,
		Collection: collection,
		Dim:        dim,
	}
}

// UpsertNode indexes a graph node into proceduralphaseserver via ppsclient.
func (idx *AgentInformationalGraphIndex) UpsertNode(ctx context.Context, nodeID, topicPath, content string, payload map[string]any) (string, error) {
	if idx.PPS == nil {
		return "", fmt.Errorf("informationalindex: nil pps client")
	}
	if nodeID == "" {
		nodeID = idlib.UUID5FromStrings(idx.Collection, topicPath, content)
	}
	if topicPath == "" {
		topicPath = "collections." + ppsclient.SanitizeSegment(idx.Collection)
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["content"] = content
	payload["topic_path"] = topicPath
	vec := proceduralphasespace.EmbedPhaseSpaceText(content, idx.Dim, proceduralphasespace.DefaultCurvature)
	_, _ = idx.PPS.CreateCollection(ctx, idx.Collection, topicPath, "poincare")
	err := idx.PPS.UpsertCollectionPoints(ctx, idx.Collection, []ppsclient.UpsertItem{{
		ID:         nodeID,
		Collection: idx.Collection,
		TopicPath:  topicPath,
		Modality:   "text",
		Content:    content,
		Vector:     vec,
		Payload:    payload,
	}})
	return nodeID, err
}

// Search queries the informational graph via ppsclient Dual-Level Hyperbolic RRF.
func (idx *AgentInformationalGraphIndex) Search(ctx context.Context, query string, topK int, filter *ppsclient.Filter) ([]ppsclient.SearchHit, error) {
	if idx.PPS == nil {
		return nil, fmt.Errorf("informationalindex: nil pps client")
	}
	if topK <= 0 {
		topK = 10
	}
	vec := proceduralphasespace.EmbedPhaseSpaceText(query, idx.Dim, proceduralphasespace.DefaultCurvature)
	return idx.PPS.SearchCollection(ctx, idx.Collection, ppsclient.SearchRequest{
		QueryText:   query,
		QueryVector: vec,
		Filter:      filter,
		TopK:        topK,
	})
}

// DeleteNode deletes a node by ID from proceduralphaseserver.
func (idx *AgentInformationalGraphIndex) DeleteNode(ctx context.Context, topicPath, nodeID string) (bool, error) {
	if idx.PPS == nil {
		return false, fmt.Errorf("informationalindex: nil pps client")
	}
	if topicPath == "" {
		topicPath = "collections." + ppsclient.SanitizeSegment(idx.Collection)
	}
	return idx.PPS.DeleteRecord(ctx, topicPath, nodeID)
}

// NewGeodesicLifecycleMCPServer creates the canonical 6-tool / 2-prompt MCP server
// (`pgvector-geodesic-lifecycle`) backed 100% by go_mltools/ppsclient (zero pgvector).
func NewGeodesicLifecycleMCPServer(client *ppsclient.Client) *mcplib.Server {
	srv := mcplib.NewServer("pgvector-geodesic-lifecycle", "2.0.0-pps")

	srv.RegisterTool(mcplib.Tool{
		Name:        "topic_learn",
		Description: "Train a proceduralphaseserver topic sheaf with online RG-Flow steps.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic_path": map[string]any{"type": "string"},
				"texts":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"steps":      map[string]any{"type": "integer"},
				"lr":         map[string]any{"type": "number"},
			},
			"required": []string{"topic_path", "texts"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if client == nil {
				return nil, fmt.Errorf("pps client not configured")
			}
			tp, _ := args["topic_path"].(string)
			texts := anyToStrings(args["texts"])
			steps := anyToInt(args["steps"], 1)
			lr := anyToFloat(args["lr"], 0.01)
			return client.LearnTopic(ctx, tp, texts, steps, lr)
		},
	})

	srv.RegisterTool(mcplib.Tool{
		Name:        "topic_blend",
		Description: "Blend multiple topic sheafs using Einstein midpoint superposition.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"weights":     map[string]any{"type": "array", "items": map[string]any{"type": "number"}},
			},
			"required": []string{"topic_paths"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if client == nil {
				return nil, fmt.Errorf("pps client not configured")
			}
			paths := anyToStrings(args["topic_paths"])
			weights := anyToFloats(args["weights"])
			return client.BlendTopics(ctx, paths, weights)
		},
	})

	srv.RegisterTool(mcplib.Tool{
		Name:        "topic_lock",
		Description: "Set VFS read-only (292) or read-write (420) lock mode on a topic sheaf.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic_path": map[string]any{"type": "string"},
				"read_only":  map[string]any{"type": "boolean"},
			},
			"required": []string{"topic_path", "read_only"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if client == nil {
				return nil, fmt.Errorf("pps client not configured")
			}
			tp, _ := args["topic_path"].(string)
			ro, _ := args["read_only"].(bool)
			mode, err := client.LockTopic(ctx, tp, ro)
			if err != nil {
				return nil, err
			}
			return map[string]any{"topic_path": tp, "vfs_mode": mode, "read_only": ro}, nil
		},
	})

	srv.RegisterTool(mcplib.Tool{
		Name:        "topic_unlearn",
		Description: "Perform Shannon-entropy unlearning and optional record purge on a topic sheaf.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic_path":    map[string]any{"type": "string"},
				"target_text":   map[string]any{"type": "string"},
				"purge_records": map[string]any{"type": "boolean"},
				"steps":         map[string]any{"type": "integer"},
				"lr":            map[string]any{"type": "number"},
			},
			"required": []string{"topic_path", "target_text"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if client == nil {
				return nil, fmt.Errorf("pps client not configured")
			}
			tp, _ := args["topic_path"].(string)
			txt, _ := args["target_text"].(string)
			purge, _ := args["purge_records"].(bool)
			steps := anyToInt(args["steps"], 1)
			lr := anyToFloat(args["lr"], 0.01)
			return client.UnlearnTopic(ctx, tp, txt, purge, steps, lr)
		},
	})

	srv.RegisterTool(mcplib.Tool{
		Name:        "topic_search_hrrf",
		Description: "Search proceduralphaseserver using Dual-Level Hyperbolic Reciprocal Rank Fusion.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":      map[string]any{"type": "string"},
				"collection": map[string]any{"type": "string"},
				"topics":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"top_k":      map[string]any{"type": "integer"},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if client == nil {
				return nil, fmt.Errorf("pps client not configured")
			}
			q, _ := args["query"].(string)
			coll, _ := args["collection"].(string)
			topics := anyToStrings(args["topics"])
			topK := anyToInt(args["top_k"], 10)
			vec := proceduralphasespace.EmbedPhaseSpaceText(q, 128, proceduralphasespace.DefaultCurvature)
			req := ppsclient.SearchRequest{
				QueryText:   q,
				QueryVector: vec,
				Topics:      topics,
				Collection:  coll,
				TopK:        topK,
			}
			if strings.TrimSpace(coll) != "" {
				return client.SearchCollection(ctx, coll, req)
			}
			return client.SearchRecords(ctx, req)
		},
	})

	srv.RegisterTool(mcplib.Tool{
		Name:        "topic_list",
		Description: "List all superpositioned topic sheafs and relativistic proper time tau on proceduralphaseserver.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ map[string]any) (any, error) {
			if client == nil {
				return nil, fmt.Errorf("pps client not configured")
			}
			return client.ListTopics(ctx)
		},
	})

	srv.RegisterPrompt(mcplib.Prompt{
		Name:        "learn_topic_protocol",
		Description: "Structured protocol for training and winding up a PPS topic sheaf.",
		Arguments:   []mcplib.PromptArgument{{Name: "topic_path", Required: true}},
		Handler: func(_ context.Context, args map[string]string) ([]mcplib.PromptMessage, error) {
			tp := args["topic_path"]
			return []mcplib.PromptMessage{
				mcplib.NewTextPromptMessage("user", fmt.Sprintf("Execute /learn protocol on topic sheaf %q via topic_learn, verify recall via topic_search_hrrf, and lock if frozen.", tp)),
			}, nil
		},
	})

	srv.RegisterPrompt(mcplib.Prompt{
		Name:        "unlearn_topic_protocol",
		Description: "Structured protocol for Shannon-entropy unlearning on a PPS topic sheaf.",
		Arguments:   []mcplib.PromptArgument{{Name: "topic_path", Required: true}, {Name: "target_text", Required: true}},
		Handler: func(_ context.Context, args map[string]string) ([]mcplib.PromptMessage, error) {
			return []mcplib.PromptMessage{
				mcplib.NewTextPromptMessage("user", fmt.Sprintf("Execute Shannon-entropy unlearning on topic %q targeting %q via topic_unlearn and verify removal via topic_search_hrrf.", args["topic_path"], args["target_text"])),
			}, nil
		},
	})

	return srv
}

func anyToStrings(v any) []string {
	switch tv := v.(type) {
	case []string:
		return tv
	case []any:
		out := make([]string, 0, len(tv))
		for _, it := range tv {
			out = append(out, fmt.Sprint(it))
		}
		return out
	case string:
		if tv != "" {
			return []string{tv}
		}
	}
	return nil
}

func anyToFloats(v any) []float64 {
	switch tv := v.(type) {
	case []float64:
		return tv
	case []any:
		out := make([]float64, 0, len(tv))
		for _, it := range tv {
			if f, ok := it.(float64); ok {
				out = append(out, f)
			}
		}
		return out
	}
	return nil
}

func anyToInt(v any, def int) int {
	switch tv := v.(type) {
	case int:
		return tv
	case float64:
		return int(tv)
	default:
		return def
	}
}

func anyToFloat(v any, def float64) float64 {
	switch tv := v.(type) {
	case float64:
		return tv
	case int:
		return float64(tv)
	default:
		return def
	}
}
