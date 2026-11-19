package benchmarkledger

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// CodeBlock represents a fenced code block extracted from a markdown artifact or transcript.
type CodeBlock struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Source   string `json:"source"`
}

// BrainConversation summarizes a single Antigravity brain directory.
type BrainConversation struct {
	ConversationID string      `json:"conversation_id"`
	Dir            string      `json:"dir"`
	Artifacts      []string    `json:"artifacts"`
	TranscriptPath string      `json:"transcript_path,omitempty"`
	StepCount      int         `json:"step_count"`
	CodeBlocks     []CodeBlock `json:"code_blocks,omitempty"`
	Summary        string      `json:"summary,omitempty"`
	UpdatedAt      string      `json:"updated_at"`
}

// AntigravityBrainLedger scans an Antigravity brain root directory, extracts transcripts,
// markdown artifacts, fenced code blocks, directory trees, and live SHA-256 signatures.
type AntigravityBrainLedger struct {
	RootDir string
}

// NewAntigravityBrainLedger creates a ledger scanner rooted at rootDir.
func NewAntigravityBrainLedger(rootDir string) *AntigravityBrainLedger {
	return &AntigravityBrainLedger{RootDir: rootDir}
}

var reFencedCode = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\n(.*?)```")

// ExtractCodeBlocks extracts fenced markdown code blocks from text.
func ExtractCodeBlocks(source, text string) []CodeBlock {
	matches := reFencedCode.FindAllStringSubmatch(text, -1)
	out := make([]CodeBlock, 0, len(matches))
	for _, m := range matches {
		lang := strings.TrimSpace(m[1])
		code := strings.TrimSpace(m[2])
		if code == "" {
			continue
		}
		out = append(out, CodeBlock{Language: lang, Code: code, Source: source})
	}
	return out
}

// ListConversations enumerates all conversation folders under RootDir.
func (l *AntigravityBrainLedger) ListConversations() ([]BrainConversation, error) {
	if l.RootDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(l.RootDir)
	if err != nil {
		return nil, err
	}
	var out []BrainConversation
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		convDir := filepath.Join(l.RootDir, ent.Name())
		info, _ := ent.Info()
		conv := BrainConversation{
			ConversationID: ent.Name(),
			Dir:            convDir,
		}
		if info != nil {
			conv.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		// Scan top-level markdown artifacts
		if files, err := os.ReadDir(convDir); err == nil {
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".md") {
					conv.Artifacts = append(conv.Artifacts, f.Name())
					if raw, err := os.ReadFile(filepath.Join(convDir, f.Name())); err == nil {
						conv.CodeBlocks = append(conv.CodeBlocks, ExtractCodeBlocks(f.Name(), string(raw))...)
						if conv.Summary == "" {
							lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
							if len(lines) > 0 {
								conv.Summary = strings.TrimLeft(strings.TrimSpace(lines[0]), "# ")
							}
						}
					}
				}
			}
		}
		sort.Strings(conv.Artifacts)
		// Check compact transcript
		tp := filepath.Join(convDir, ".system_generated", "logs", "transcript.jsonl")
		if _, err := os.Stat(tp); err == nil {
			conv.TranscriptPath = tp
			conv.StepCount = countJSONLLines(tp)
		}
		out = append(out, conv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// LiveSignature computes a deterministic SHA-256 signature over conversation IDs, artifacts, and mtimes.
func (l *AntigravityBrainLedger) LiveSignature() string {
	convs, _ := l.ListConversations()
	h := sha256.New()
	for _, c := range convs {
		h.Write([]byte(c.ConversationID + "|" + c.UpdatedAt + "|" + strings.Join(c.Artifacts, ",") + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func countJSONLLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	n := 0
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	return n
}

// BenchmarkEntry captures one benchmark run stored in `benchmark_history_ledger.json`.
type BenchmarkEntry struct {
	RunID         string         `json:"run_id"`
	ModelName     string         `json:"model_name"`
	Harness       string         `json:"harness"`
	PassRate      float64        `json:"pass_rate"`
	OverallScore  float64        `json:"overall_score"`
	AvgLatencyMs  float64        `json:"avg_latency_ms"`
	TimestampUTC  string         `json:"timestamp_utc"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// BenchmarkLedger manages persistent benchmark history and ranked leaderboards on disk.
type BenchmarkLedger struct {
	mu      sync.RWMutex
	path    string
	entries []BenchmarkEntry
}

// NewBenchmarkLedger opens or initializes a BenchmarkLedger backed by jsonPath.
func NewBenchmarkLedger(jsonPath string) *BenchmarkLedger {
	bl := &BenchmarkLedger{path: jsonPath}
	if jsonPath != "" {
		_ = bl.Load()
	}
	return bl
}

// Load reads benchmark entries from disk (supporting array or `{"entries":[...]}` formats).
func (bl *BenchmarkLedger) Load() error {
	raw, err := os.ReadFile(bl.path)
	if err != nil {
		return err
	}
	var list []BenchmarkEntry
	if err := json.Unmarshal(raw, &list); err == nil {
		bl.mu.Lock()
		bl.entries = list
		bl.mu.Unlock()
		return nil
	}
	var wrapped struct {
		Entries []BenchmarkEntry `json:"entries"`
		History []BenchmarkEntry `json:"history"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return err
	}
	bl.mu.Lock()
	if len(wrapped.Entries) > 0 {
		bl.entries = wrapped.Entries
	} else {
		bl.entries = wrapped.History
	}
	bl.mu.Unlock()
	return nil
}

// Append records a new entry and atomically persists to disk if path is set.
func (bl *BenchmarkLedger) Append(e BenchmarkEntry) error {
	if e.TimestampUTC == "" {
		e.TimestampUTC = time.Now().UTC().Format(time.RFC3339)
	}
	bl.mu.Lock()
	bl.entries = append(bl.entries, e)
	cp := make([]BenchmarkEntry, len(bl.entries))
	copy(cp, bl.entries)
	p := bl.path
	bl.mu.Unlock()

	if p == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o644)
}

// History returns all entries (newest first).
func (bl *BenchmarkLedger) History() []BenchmarkEntry {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	out := make([]BenchmarkEntry, len(bl.entries))
	copy(out, bl.entries)
	sort.Slice(out, func(i, j int) bool { return out[i].TimestampUTC > out[j].TimestampUTC })
	return out
}

// Leaderboard returns the best score per (model_name, harness) sorted by OverallScore descending.
func (bl *BenchmarkLedger) Leaderboard() []BenchmarkEntry {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	best := make(map[string]BenchmarkEntry)
	for _, e := range bl.entries {
		k := e.ModelName + "|" + e.Harness
		if cur, ok := best[k]; !ok || e.OverallScore > cur.OverallScore {
			best[k] = e
		}
	}
	out := make([]BenchmarkEntry, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OverallScore == out[j].OverallScore {
			return out[i].AvgLatencyMs < out[j].AvgLatencyMs
		}
		return out[i].OverallScore > out[j].OverallScore
	})
	return out
}
