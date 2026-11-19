package secretslib

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"sync"
)

// Store resolves secrets in priority order: in-memory overrides -> env vars ->
// `<NAME>_FILE` Docker/K8s mounted secret files -> `.env` files.
type Store struct {
	mu      sync.RWMutex
	cache   map[string]string
	envFile map[string]string
}

// NewStore creates a Secrets Store, optionally preloading key/value pairs from dotEnvPaths.
func NewStore(dotEnvPaths ...string) *Store {
	s := &Store{
		cache:   make(map[string]string),
		envFile: make(map[string]string),
	}
	for _, p := range dotEnvPaths {
		_ = s.LoadDotEnv(p)
	}
	return s
}

// LoadDotEnv parses a `.env` file into the fallback map without mutating process env.
func (s *Store) LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		s.envFile[k] = v
	}
	return sc.Err()
}

// Get returns a secret value by name, or defaultVal if absent.
func (s *Store) Get(name, defaultVal string) string {
	s.mu.RLock()
	if v, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return v
	}
	s.mu.RUnlock()

	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	if fp, ok := os.LookupEnv(name + "_FILE"); ok && fp != "" {
		if raw, err := os.ReadFile(strings.TrimSpace(fp)); err == nil {
			val := strings.TrimSpace(string(raw))
			s.Set(name, val)
			return val
		}
	}
	s.mu.RLock()
	if v, ok := s.envFile[name]; ok && v != "" {
		s.mu.RUnlock()
		return v
	}
	s.mu.RUnlock()
	return defaultVal
}

// Has returns true if the secret exists in cache, env, NAME_FILE, or .env.
func (s *Store) Has(name string) bool {
	return s.Get(name, "") != ""
}

// Set stores a secret value in memory.
func (s *Store) Set(name, value string) {
	s.mu.Lock()
	s.cache[name] = value
	s.mu.Unlock()
}

// Delete removes an in-memory secret override.
func (s *Store) Delete(name string) {
	s.mu.Lock()
	delete(s.cache, name)
	s.mu.Unlock()
}

// ListKeys returns sorted names of secrets stored in memory or loaded from .env (values never exposed).
func (s *Store) ListKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]struct{}, len(s.cache)+len(s.envFile))
	for k := range s.cache {
		seen[k] = struct{}{}
	}
	for k := range s.envFile {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
