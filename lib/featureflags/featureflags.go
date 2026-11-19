package featureflags

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"sync"
	"time"
)

// FeatureFlag describes a toggleable flag with percentage rollout and user/attribute targeting.
type FeatureFlag struct {
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	RolloutPct  float64           `json:"rollout_pct"` // 0..100
	TargetUsers []string          `json:"target_users,omitempty"`
	TargetAttrs map[string]string `json:"target_attrs,omitempty"`
	Description string            `json:"description,omitempty"`
	UpdatedAt   string            `json:"updated_at"`
}

// Store provides thread-safe CRUD and deterministic evaluation for FeatureFlags.
type Store struct {
	mu    sync.RWMutex
	flags map[string]FeatureFlag
}

// NewStore creates an empty feature flag Store.
func NewStore() *Store {
	return &Store{flags: make(map[string]FeatureFlag)}
}

// Set creates or updates a feature flag.
func (s *Store) Set(flag FeatureFlag) FeatureFlag {
	if flag.RolloutPct < 0 {
		flag.RolloutPct = 0
	}
	if flag.RolloutPct > 100 {
		flag.RolloutPct = 100
	}
	flag.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	s.flags[flag.Name] = flag
	s.mu.Unlock()
	return flag
}

// Get retrieves a feature flag by name.
func (s *Store) Get(name string) (FeatureFlag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.flags[name]
	return f, ok
}

// Delete removes a feature flag by name.
func (s *Store) Delete(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.flags[name]; !ok {
		return false
	}
	delete(s.flags, name)
	return true
}

// List returns all flags sorted by name.
func (s *Store) List() []FeatureFlag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FeatureFlag, 0, len(s.flags))
	for _, f := range s.flags {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Evaluate computes whether flagName is active for userID and optional attributes.
func (s *Store) Evaluate(flagName, userID string, attrs map[string]string) bool {
	s.mu.RLock()
	f, ok := s.flags[flagName]
	s.mu.RUnlock()
	if !ok || !f.Enabled {
		return false
	}
	if len(f.TargetUsers) > 0 {
		for _, u := range f.TargetUsers {
			if u == userID {
				return true
			}
		}
	}
	if len(f.TargetAttrs) > 0 {
		allMatch := true
		for k, want := range f.TargetAttrs {
			if attrs[k] != want {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	if f.RolloutPct >= 100 {
		return true
	}
	if f.RolloutPct <= 0 {
		return false
	}
	// Deterministic SHA-256 percentage bucket in [0, 100)
	h := sha256.Sum256([]byte(flagName + ":" + userID))
	bucket := float64(binary.BigEndian.Uint32(h[:4])%10000) / 100.0
	return bucket < f.RolloutPct
}
