package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// SettingsStore is a thread-safe hierarchical YAML/JSON settings store with dotted-path
// lookups (`Get("rag.chunk_size")`), atomic YAML persistence, and env overrides.
type SettingsStore struct {
	mu   sync.RWMutex
	path string
	data map[string]any
}

// NewSettingsStore initializes a SettingsStore from an optional initial map.
func NewSettingsStore(initial map[string]any) *SettingsStore {
	cp := make(map[string]any, len(initial))
	for k, v := range initial {
		cp[k] = v
	}
	return &SettingsStore{data: cp}
}

// LoadSettingsStoreFromBytes parses YAML or JSON bytes into a new SettingsStore.
func LoadSettingsStoreFromBytes(raw []byte) (*SettingsStore, error) {
	s := NewSettingsStore(nil)
	if err := s.ReloadFromBytes(raw); err != nil {
		return nil, err
	}
	return s, nil
}

// LoadSettingsStoreFromFile loads YAML or JSON settings from filePath.
func LoadSettingsStoreFromFile(filePath string) (*SettingsStore, error) {
	s := NewSettingsStore(nil)
	s.path = filePath
	if err := s.ReloadFromFile(filePath); err != nil {
		return nil, err
	}
	return s, nil
}

// ReloadFromBytes replaces the store contents from YAML/JSON bytes.
func (s *SettingsStore) ReloadFromBytes(raw []byte) error {
	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("settings.ReloadFromBytes: %w", err)
	}
	if parsed == nil {
		parsed = make(map[string]any)
	}
	s.mu.Lock()
	s.data = normalizeYAMLMap(parsed)
	s.mu.Unlock()
	return nil
}

// ReloadFromFile reads YAML/JSON settings from filePath (or the previously loaded path if empty).
func (s *SettingsStore) ReloadFromFile(filePath string) error {
	s.mu.RLock()
	if filePath == "" {
		filePath = s.path
	}
	s.mu.RUnlock()
	if filePath == "" {
		return fmt.Errorf("settings.ReloadFromFile: empty path")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if err := s.ReloadFromBytes(raw); err != nil {
		return err
	}
	s.mu.Lock()
	s.path = filePath
	s.mu.Unlock()
	return nil
}

// MergeFromFile overlays values from filePath on top of existing settings recursively.
func (s *SettingsStore) MergeFromFile(filePath string) error {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = deepMergeMaps(s.data, normalizeYAMLMap(parsed))
	s.path = filePath
	return nil
}

// Get retrieves a value by dotted path (e.g. "server.port" or "rag.chunk_size").
func (s *SettingsStore) Get(dottedKey string, def any) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if dottedKey == "" {
		return def
	}
	parts := strings.Split(dottedKey, ".")
	var cur any = s.data
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return def
		}
		nxt, exists := m[p]
		if !exists || nxt == nil {
			return def
		}
		cur = nxt
	}
	return cur
}

// Set assigns a value at a dotted path, creating intermediate maps as needed.
func (s *SettingsStore) Set(dottedKey string, val any) {
	if dottedKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string]any)
	}
	parts := strings.Split(dottedKey, ".")
	cur := s.data
	for i := 0; i < len(parts)-1; i++ {
		p := parts[i]
		nxt, ok := cur[p].(map[string]any)
		if !ok {
			nxt = make(map[string]any)
			cur[p] = nxt
		}
		cur = nxt
	}
	cur[parts[len(parts)-1]] = val
}

// GetString returns the string value at dottedKey or def.
func (s *SettingsStore) GetString(dottedKey, def string) string {
	v := s.Get(dottedKey, nil)
	if v == nil {
		return def
	}
	switch tv := v.(type) {
	case string:
		return tv
	default:
		return fmt.Sprint(tv)
	}
}

// GetInt returns the integer value at dottedKey or def.
func (s *SettingsStore) GetInt(dottedKey string, def int) int {
	v := s.Get(dottedKey, nil)
	if v == nil {
		return def
	}
	switch tv := v.(type) {
	case int:
		return tv
	case int64:
		return int(tv)
	case float64:
		return int(tv)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(tv)); err == nil {
			return n
		}
	}
	return def
}

// GetFloat64 returns the float64 value at dottedKey or def.
func (s *SettingsStore) GetFloat64(dottedKey string, def float64) float64 {
	v := s.Get(dottedKey, nil)
	if v == nil {
		return def
	}
	switch tv := v.(type) {
	case float64:
		return tv
	case int:
		return float64(tv)
	case int64:
		return float64(tv)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(tv), 64); err == nil {
			return f
		}
	}
	return def
}

// GetBool returns the boolean value at dottedKey or def.
func (s *SettingsStore) GetBool(dottedKey string, def bool) bool {
	v := s.Get(dottedKey, nil)
	if v == nil {
		return def
	}
	switch tv := v.(type) {
	case bool:
		return tv
	case string:
		low := strings.ToLower(strings.TrimSpace(tv))
		if low == "true" || low == "1" || low == "yes" {
			return true
		}
		if low == "false" || low == "0" || low == "no" {
			return false
		}
	case int:
		return tv != 0
	}
	return def
}

// GetStringSlice returns a string slice at dottedKey or def.
func (s *SettingsStore) GetStringSlice(dottedKey string, def []string) []string {
	v := s.Get(dottedKey, nil)
	if v == nil {
		return def
	}
	switch tv := v.(type) {
	case []string:
		out := make([]string, len(tv))
		copy(out, tv)
		return out
	case []any:
		out := make([]string, 0, len(tv))
		for _, item := range tv {
			out = append(out, fmt.Sprint(item))
		}
		return out
	}
	return def
}

// RawAll returns a deep-cloned map of all settings.
func (s *SettingsStore) RawAll() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := json.Marshal(s.data)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = make(map[string]any)
	}
	return out
}

// ToJSON serializes the entire store to pretty JSON.
func (s *SettingsStore) ToJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.MarshalIndent(s.data, "", "  ")
}

// WriteYAMLFile atomically persists the current settings map to filePath (or the store's loaded path).
func (s *SettingsStore) WriteYAMLFile(filePath string) error {
	s.mu.RLock()
	if filePath == "" {
		filePath = s.path
	}
	raw, err := yaml.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if filePath == "" {
		return fmt.Errorf("settings.WriteYAMLFile: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".settings-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, filePath)
}

func normalizeYAMLMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalizeYAMLVal(v)
	}
	return out
}

func normalizeYAMLVal(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return normalizeYAMLMap(tv)
	case map[any]any:
		out := make(map[string]any, len(tv))
		for k, val := range tv {
			out[fmt.Sprint(k)] = normalizeYAMLVal(val)
		}
		return out
	case []any:
		out := make([]any, len(tv))
		for i, item := range tv {
			out[i] = normalizeYAMLVal(item)
		}
		return out
	default:
		return v
	}
}

func deepMergeMaps(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if ovMap, ok := v.(map[string]any); ok {
			if baseMap, ok2 := out[k].(map[string]any); ok2 {
				out[k] = deepMergeMaps(baseMap, ovMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}
