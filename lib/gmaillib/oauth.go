package gmaillib

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	defaultGmailBaseURL = "https://gmail.googleapis.com/gmail/v1"
	defaultTokenURL     = "https://oauth2.googleapis.com/token"
	defaultAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultModifyScope  = "https://www.googleapis.com/auth/gmail.modify"
)

// GmailConfig configures OAuth2 credentials, rate limiting, and API endpoints for GmailClient.
type GmailConfig struct {
	CredentialsPath  string
	TokenPath        string
	UserID           string
	Scopes           []string
	RateLimitDelay   time.Duration
	DisableRateLimit bool
	InteractiveAuth  bool
	BaseURL          string
	TokenURL         string
	AuthURL          string
	AccessToken      string
	HTTPClient       *http.Client
}

// DefaultGmailConfig returns GmailConfig populated with standard toolset defaults:
// UserID="me", Scopes=["https://www.googleapis.com/auth/gmail.modify"], RateLimitDelay=300ms.
func DefaultGmailConfig() GmailConfig {
	return GmailConfig{
		UserID:         "me",
		Scopes:         []string{defaultModifyScope},
		RateLimitDelay: 300 * time.Millisecond,
		BaseURL:        defaultGmailBaseURL,
		TokenURL:       defaultTokenURL,
		AuthURL:        defaultAuthURL,
	}
}

func normalizeGmailConfig(cfg *GmailConfig) GmailConfig {
	out := DefaultGmailConfig()
	if cfg == nil {
		return out
	}
	out.CredentialsPath = cfg.CredentialsPath
	out.TokenPath = cfg.TokenPath
	if cfg.UserID != "" {
		out.UserID = cfg.UserID
	}
	if len(cfg.Scopes) > 0 {
		out.Scopes = append([]string(nil), cfg.Scopes...)
	}
	out.DisableRateLimit = cfg.DisableRateLimit
	if cfg.DisableRateLimit {
		out.RateLimitDelay = 0
	} else if cfg.RateLimitDelay > 0 {
		out.RateLimitDelay = cfg.RateLimitDelay
	}
	out.InteractiveAuth = cfg.InteractiveAuth
	if cfg.BaseURL != "" {
		out.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	if cfg.TokenURL != "" {
		out.TokenURL = cfg.TokenURL
	}
	if cfg.AuthURL != "" {
		out.AuthURL = cfg.AuthURL
	}
	out.AccessToken = cfg.AccessToken
	out.HTTPClient = cfg.HTTPClient
	return out
}

func defaultToolsetConfigPath(filename string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "toolset", filename)
	}
	return filepath.Join(home, ".config", "toolset", filename)
}

// StoredToken represents an OAuth2 token compatible with both Python google-auth's
// authorized_user JSON ("token", "refresh_token", "token_uri", "client_id", "client_secret", "expiry")
// and standard Go oauth2.Token JSON ("access_token", "refresh_token", "expiry").
type StoredToken struct {
	Token        string   `json:"token,omitempty"`
	AccessToken  string   `json:"access_token,omitempty"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	TokenURI     string   `json:"token_uri,omitempty"`
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	Expiry       string   `json:"expiry,omitempty"`
}

func (t *StoredToken) effectiveAccessToken() string {
	if t.AccessToken != "" {
		return t.AccessToken
	}
	return t.Token
}

func (t *StoredToken) IsValid(now time.Time) bool {
	if t == nil || t.effectiveAccessToken() == "" {
		return false
	}
	if t.Expiry == "" {
		return true
	}
	exp, err := parseExpiryTime(t.Expiry)
	if err != nil {
		return false
	}
	return now.Add(10 * time.Second).Before(exp)
}

func parseExpiryTime(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("gmaillib: unsupported token expiry format %q", raw)
}

type clientSecretsEnvelope struct {
	Installed *clientSecretsBlock `json:"installed,omitempty"`
	Web       *clientSecretsBlock `json:"web,omitempty"`
}

type clientSecretsBlock struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

func loadClientSecrets(path string) (*clientSecretsBlock, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env clientSecretsEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Installed != nil && env.Installed.ClientID != "" {
		return env.Installed, nil
	}
	if env.Web != nil && env.Web.ClientID != "" {
		return env.Web, nil
	}
	return nil, fmt.Errorf("gmaillib: invalid client secrets file %q", path)
}

type tokenManager struct {
	mu         sync.Mutex
	cfg        GmailConfig
	httpClient *http.Client
	cached     *StoredToken
}

func newTokenManager(cfg GmailConfig, httpClient *http.Client) *tokenManager {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &tokenManager{cfg: cfg, httpClient: httpClient}
}

func (tm *tokenManager) Token() (*oauth2.Token, error) {
	tok, err := tm.AccessToken(context.Background(), tm.httpClient)
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: tok,
		TokenType:   "Bearer",
	}, nil
}

func (tm *tokenManager) AccessToken(ctx context.Context, httpClient *http.Client) (string, error) {
	if tm.cfg.AccessToken != "" {
		return tm.cfg.AccessToken, nil
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now().UTC()
	if tm.cached != nil && tm.cached.IsValid(now) {
		return tm.cached.effectiveAccessToken(), nil
	}

	tokenPath := tm.cfg.TokenPath
	if tokenPath == "" {
		tokenPath = defaultToolsetConfigPath("gmail_token.json")
	}
	credsPath := tm.cfg.CredentialsPath
	if credsPath == "" {
		credsPath = defaultToolsetConfigPath("gmail_credentials.json")
	}

	var tok *StoredToken
	if raw, err := os.ReadFile(tokenPath); err == nil {
		var parsed StoredToken
		if jsonErr := json.Unmarshal(raw, &parsed); jsonErr == nil {
			tok = &parsed
		}
	}

	if tok != nil && tok.IsValid(now) {
		tm.cached = tok
		return tok.effectiveAccessToken(), nil
	}

	if tok != nil && tok.RefreshToken != "" {
		refreshed, err := tm.refreshToken(ctx, httpClient, tok, credsPath)
		if err != nil {
			return "", err
		}
		if err := saveStoredToken(tokenPath, refreshed); err != nil {
			return "", err
		}
		tm.cached = refreshed
		return refreshed.effectiveAccessToken(), nil
	}

	if !tm.cfg.InteractiveAuth {
		return "", fmt.Errorf("gmaillib: valid OAuth2 token not available at %q (refresh unavailable and InteractiveAuth is false)", tokenPath)
	}

	interactiveTok, err := tm.runLoopbackAuth(ctx, httpClient, credsPath)
	if err != nil {
		return "", err
	}
	if err := saveStoredToken(tokenPath, interactiveTok); err != nil {
		return "", err
	}
	tm.cached = interactiveTok
	return interactiveTok.effectiveAccessToken(), nil
}

func (tm *tokenManager) refreshToken(ctx context.Context, httpClient *http.Client, tok *StoredToken, credsPath string) (*StoredToken, error) {
	clientID := tok.ClientID
	clientSecret := tok.ClientSecret
	tokenURI := tok.TokenURI
	if (clientID == "" || clientSecret == "") && credsPath != "" {
		if cs, err := loadClientSecrets(credsPath); err == nil {
			if clientID == "" {
				clientID = cs.ClientID
			}
			if clientSecret == "" {
				clientSecret = cs.ClientSecret
			}
			if tokenURI == "" {
				tokenURI = cs.TokenURI
			}
		}
	}
	if tokenURI == "" || tm.cfg.TokenURL != defaultTokenURL {
		tokenURI = tm.cfg.TokenURL
	}
	if tokenURI == "" {
		tokenURI = defaultTokenURL
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", tok.RefreshToken)
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gmaillib: OAuth2 token refresh failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("gmaillib: OAuth2 refresh response returned empty access_token")
	}

	updated := *tok
	updated.Token = tokenResp.AccessToken
	updated.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		updated.RefreshToken = tokenResp.RefreshToken
	}
	if clientID != "" {
		updated.ClientID = clientID
	}
	if clientSecret != "" {
		updated.ClientSecret = clientSecret
	}
	updated.TokenURI = tokenURI
	if tokenResp.ExpiresIn > 0 {
		updated.Expiry = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	} else {
		updated.Expiry = time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	}
	if len(updated.Scopes) == 0 {
		updated.Scopes = append([]string(nil), tm.cfg.Scopes...)
	}
	return &updated, nil
}

func (tm *tokenManager) runLoopbackAuth(ctx context.Context, httpClient *http.Client, credsPath string) (*StoredToken, error) {
	cs, err := loadClientSecrets(credsPath)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challengeHash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])

	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- errors.New("gmaillib: OAuth2 state mismatch")
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("gmaillib: authorization error: %s", r.URL.Query().Get("error"))
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("Authentication complete. You may close this window."))
		codeCh <- code
	})
	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		_ = srv.Close()
	}()

	authBase := cs.AuthURI
	if authBase == "" || tm.cfg.AuthURL != defaultAuthURL {
		authBase = tm.cfg.AuthURL
	}
	q := url.Values{}
	q.Set("client_id", cs.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(tm.cfg.Scopes, " "))
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	fmt.Fprintf(os.Stderr, "Authorize Gmail access by visiting:\n%s?%s\n", authBase, q.Encode())

	var authCode string
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case e := <-errCh:
		return nil, e
	case authCode = <-codeCh:
	}

	tokenURI := cs.TokenURI
	if tokenURI == "" || tm.cfg.TokenURL != defaultTokenURL {
		tokenURI = tm.cfg.TokenURL
	}
	form := url.Values{}
	form.Set("code", authCode)
	form.Set("client_id", cs.ClientID)
	form.Set("client_secret", cs.ClientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gmaillib: OAuth2 code exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	exp := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn <= 0 {
		exp = time.Now().UTC().Add(time.Hour)
	}
	return &StoredToken{
		Token:        tokenResp.AccessToken,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenURI:     tokenURI,
		ClientID:     cs.ClientID,
		ClientSecret: cs.ClientSecret,
		Scopes:       append([]string(nil), tm.cfg.Scopes...),
		Expiry:       exp.Format(time.RFC3339),
	}, nil
}

func saveStoredToken(tokenPath string, tok *StoredToken) error {
	dir := filepath.Dir(tokenPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	encoded, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenPath, encoded, 0o600)
}
