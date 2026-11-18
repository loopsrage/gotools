// Package ocrlib provides a thread-safe hot-swappable OCR Provider registry and concrete
// adapters for Tesseract CLI/TSV, CGo gosseract, remote HTTP/OpenAI vision servers, Ollama,
// and go_mltools neural sequence recognizers.
package ocrlib

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sort"
	"strings"
	"sync"

	"github.com/loopsrage/gotools/lib/index"
)

const providersBucket = "ocr_providers"

// TesseractProviderConfig configures the Tesseract OCR provider.
type TesseractProviderConfig struct {
	Enabled     bool   `json:"Enabled" yaml:"Enabled"`
	BinaryPath  string `json:"BinaryPath" yaml:"BinaryPath"`
	TessdataDir string `json:"TessdataDir" yaml:"TessdataDir"`
	Language    string `json:"Language" yaml:"Language"`
	PSM         int    `json:"PSM" yaml:"PSM"`
}

// HTTPProviderConfig configures a generic HTTP OCR model endpoint.
type HTTPProviderConfig struct {
	Enabled   bool   `json:"Enabled" yaml:"Enabled"`
	Name      string `json:"Name" yaml:"Name"`
	Endpoint  string `json:"Endpoint" yaml:"Endpoint"`
	ModelID   string `json:"ModelID" yaml:"ModelID"`
	APIKey    string `json:"APIKey" yaml:"APIKey"`
	TimeoutMs int    `json:"TimeoutMs" yaml:"TimeoutMs"`
}

// OllamaProviderConfig configures an Ollama multimodal vision OCR model.
type OllamaProviderConfig struct {
	Enabled        bool   `json:"Enabled" yaml:"Enabled"`
	Model          string `json:"Model" yaml:"Model"`
	EmbeddingModel string `json:"EmbeddingModel" yaml:"EmbeddingModel"`
	Prompt         string `json:"Prompt" yaml:"Prompt"`
}

// MLToolsProviderConfig configures the go_mltools native neural SDF-OCR + BeamSearch provider.
type MLToolsProviderConfig struct {
	Enabled    bool  `json:"Enabled" yaml:"Enabled"`
	NumClasses int   `json:"NumClasses" yaml:"NumClasses"`
	BeamSize   int   `json:"BeamSize" yaml:"BeamSize"`
	Seed       int64 `json:"Seed" yaml:"Seed"`
}

// Config holds all swappable OCR provider configurations.
type Config struct {
	DefaultProvider string                  `json:"DefaultProvider" yaml:"DefaultProvider"`
	Tesseract       TesseractProviderConfig `json:"Tesseract" yaml:"Tesseract"`
	HTTP            HTTPProviderConfig      `json:"HTTP" yaml:"HTTP"`
	Ollama          OllamaProviderConfig    `json:"Ollama" yaml:"Ollama"`
	MLTools         MLToolsProviderConfig   `json:"MLTools" yaml:"MLTools"`
}

// DefaultConfig returns standard multi-provider OCR settings with Tesseract as default.
func DefaultConfig() Config {
	return Config{
		DefaultProvider: "tesseract",
		Tesseract: TesseractProviderConfig{
			Enabled:  true,
			Language: "eng",
			PSM:      6,
		},
		HTTP: HTTPProviderConfig{
			Name:      "http",
			ModelID:   "remote-ocr",
			TimeoutMs: 15000,
		},
		Ollama: OllamaProviderConfig{
			Model:  "llava",
			Prompt: "Extract all visible text verbatim without commentary.",
		},
		MLTools: MLToolsProviderConfig{
			Enabled:    true,
			NumClasses: 95,
			BeamSize:   4,
			Seed:       42,
		},
	}
}

// RecognizeRequest encapsulates a single denoised image region submitted to an OCR Provider.
type RecognizeRequest struct {
	DocID       string          `json:"doc_id"`
	PageIndex   int             `json:"page_index"`
	RegionIndex int             `json:"region_index"`
	ImagePNG    []byte          `json:"-"`
	Bounds      image.Rectangle `json:"bounds"`
	Language    string          `json:"language,omitempty"`
	Params      map[string]any  `json:"params,omitempty"`
}

// TokenBox records word/character-level bounding boxes and confidence when emitted by a model.
type TokenBox struct {
	Text       string          `json:"text"`
	Confidence float64         `json:"confidence,omitempty"`
	Box        image.Rectangle `json:"box"`
}

// RecognizeResult captures normalized OCR output from any registered Provider.
type RecognizeResult struct {
	Provider   string         `json:"provider"`
	ModelID    string         `json:"model_id"`
	Text       string         `json:"text"`
	Confidence float64        `json:"confidence"`
	Tokens     []TokenBox     `json:"tokens,omitempty"`
	Vector     []float64      `json:"vector,omitempty"`
	LatencyMs  float64        `json:"latency_ms"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Provider is the swappable OCR model contract.
type Provider interface {
	Name() string
	ModelID() string
	Recognize(ctx context.Context, req RecognizeRequest) (*RecognizeResult, error)
	Close() error
}

// ProviderInfo describes a registered OCR provider for discovery endpoints.
type ProviderInfo struct {
	Name      string `json:"name"`
	ModelID   string `json:"model_id"`
	IsDefault bool   `json:"is_default"`
}

// Registry manages thread-safe lookup and hot-swapping of OCR Provider implementations.
type Registry struct {
	idx         *index.Index
	mu          sync.RWMutex
	defaultName string
}

// NewRegistry creates an empty OCR Provider Registry.
func NewRegistry() *Registry {
	idx := index.NewIndex()
	idx.New(providersBucket)
	return &Registry{idx: idx}
}

// Register adds or replaces a Provider in the Registry. The first registered provider becomes default if unset.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return errors.New("ocrlib: nil provider")
	}
	name := strings.ToLower(strings.TrimSpace(p.Name()))
	if name == "" {
		return errors.New("ocrlib: provider Name() cannot be empty")
	}
	r.idx.StoreInIndex(providersBucket, name, p)

	r.mu.Lock()
	if r.defaultName == "" {
		r.defaultName = name
	}
	r.mu.Unlock()
	return nil
}

// Get retrieves a registered Provider by case-insensitive name.
func (r *Registry) Get(name string) (Provider, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		r.mu.RLock()
		key = r.defaultName
		r.mu.RUnlock()
	}
	val, err := r.idx.LoadFromIndex(providersBucket, key)
	if err != nil || val == nil {
		return nil, false
	}
	p, ok := val.(Provider)
	return p, ok
}

// DefaultName returns the current default provider name.
func (r *Registry) DefaultName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultName
}

// SetDefault switches the active default OCR provider to name (must already be registered).
func (r *Registry) SetDefault(name string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	if _, ok := r.Get(key); !ok {
		return fmt.Errorf("ocrlib: provider %q is not registered", name)
	}
	r.mu.Lock()
	r.defaultName = key
	r.mu.Unlock()
	return nil
}

// ResolveProviders resolves a list of requested provider names (or the active default when empty).
func (r *Registry) ResolveProviders(requested []string) ([]Provider, error) {
	if len(requested) == 0 {
		defName := r.DefaultName()
		p, ok := r.Get(defName)
		if !ok {
			return nil, errors.New("ocrlib: no default provider registered")
		}
		return []Provider{p}, nil
	}

	seen := make(map[string]bool, len(requested))
	out := make([]Provider, 0, len(requested))
	for _, raw := range requested {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" || seen[key] {
			continue
		}
		p, ok := r.Get(key)
		if !ok {
			return nil, fmt.Errorf("ocrlib: unknown provider %q (available: %s)", raw, strings.Join(r.Names(), ", "))
		}
		seen[key] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return r.ResolveProviders(nil)
	}
	return out, nil
}

// Names returns sorted names of all registered providers.
func (r *Registry) Names() []string {
	var names []string
	_ = r.idx.RangeIndex(providersBucket, func(key, _ interface{}) {
		if k, ok := key.(string); ok {
			names = append(names, k)
		}
	})
	sort.Strings(names)
	return names
}

// List returns metadata for all registered OCR providers.
func (r *Registry) List() []ProviderInfo {
	def := r.DefaultName()
	var infos []ProviderInfo
	_ = r.idx.RangeIndex(providersBucket, func(key, value interface{}) {
		k, ok1 := key.(string)
		p, ok2 := value.(Provider)
		if ok1 && ok2 {
			infos = append(infos, ProviderInfo{
				Name:      k,
				ModelID:   p.ModelID(),
				IsDefault: k == def,
			})
		}
	})
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// CloseAll closes all registered providers.
func (r *Registry) CloseAll() error {
	var firstErr error
	_ = r.idx.RangeIndex(providersBucket, func(_, value interface{}) {
		if p, ok := value.(Provider); ok {
			if err := p.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	})
	return firstErr
}

// NewDefaultRegistry builds and populates a Registry from Config.
func NewDefaultRegistry(ctx context.Context, cfg Config) (*Registry, error) {
	reg := NewRegistry()

	if cfg.Tesseract.Enabled {
		if tp, err := NewTesseractCLIProvider(cfg.Tesseract); err == nil {
			_ = reg.Register(tp)
		}
	}
	if cfg.MLTools.Enabled {
		mp, err := NewMLToolsProvider(cfg.MLTools)
		if err != nil {
			return nil, err
		}
		_ = reg.Register(mp)
	}
	if cfg.HTTP.Enabled && strings.TrimSpace(cfg.HTTP.Endpoint) != "" {
		hp, err := NewHTTPProvider(cfg.HTTP)
		if err != nil {
			return nil, err
		}
		_ = reg.Register(hp)
	}
	if cfg.Ollama.Enabled && strings.TrimSpace(cfg.Ollama.Model) != "" {
		op, err := NewOllamaProvider(ctx, cfg.Ollama)
		if err != nil {
			return nil, err
		}
		_ = reg.Register(op)
	}

	if cfg.DefaultProvider != "" {
		if _, ok := reg.Get(cfg.DefaultProvider); ok {
			_ = reg.SetDefault(cfg.DefaultProvider)
		}
	}
	return reg, nil
}
