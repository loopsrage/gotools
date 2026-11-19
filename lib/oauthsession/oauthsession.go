package oauthsession

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config controls Google OAuth2 code/ID-token authentication and HMAC-SHA256 session cookies.
type Config struct {
	ClientID       string        `json:"client_id"`
	ClientSecret   string        `json:"-"`
	RedirectURI    string        `json:"redirect_uri"`
	SessionSecret  string        `json:"-"`
	CookieName     string        `json:"cookie_name"`
	CookieSecure   bool          `json:"cookie_secure"`
	SessionTTL     time.Duration `json:"session_ttl"`
	AllowedDomains []string      `json:"allowed_domains,omitempty"`
	AdminEmails    []string      `json:"admin_emails,omitempty"`
	AuthEnabled    bool          `json:"auth_enabled"`
}

// SessionClaims represents the verified user identity stored in a signed session cookie.
type SessionClaims struct {
	JTI       string   `json:"jti"`
	Sub       string   `json:"sub"`
	Email     string   `json:"email"`
	Name      string   `json:"name,omitempty"`
	Picture   string   `json:"picture,omitempty"`
	Roles     []string `json:"roles"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}

// Manager handles session signing, verification, revocation, Google OAuth2 code exchange,
// ID token validation, and RBAC checks.
type Manager struct {
	mu      sync.RWMutex
	cfg     Config
	revoked map[string]int64
	client  *http.Client
}

// NewManager initializes a Manager; generates a random ephemeral SessionSecret if empty.
func NewManager(cfg Config) *Manager {
	if cfg.CookieName == "" {
		cfg.CookieName = "agent_brain_session"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}
	if cfg.SessionSecret == "" {
		var rnd [32]byte
		_, _ = rand.Read(rnd[:])
		cfg.SessionSecret = hex.EncodeToString(rnd[:])
	}
	return &Manager{
		cfg:     cfg,
		revoked: make(map[string]int64),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// ConfigSnapshot returns a safe copy of Config (excluding secrets).
func (m *Manager) ConfigSnapshot() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := m.cfg
	cp.ClientSecret = ""
	cp.SessionSecret = ""
	return cp
}

// IssueSession creates and HMAC-SHA256 signs a SessionClaims token.
func (m *Manager) IssueSession(sub, email, name, picture string, roles []string) (string, SessionClaims, error) {
	if email == "" && sub == "" {
		return "", SessionClaims{}, errors.New("oauthsession: email or sub required")
	}
	if !m.isDomainAllowed(email) {
		return "", SessionClaims{}, fmt.Errorf("oauthsession: domain not allowed for %q", email)
	}
	if len(roles) == 0 {
		roles = m.defaultRolesFor(email)
	}
	var jtiBuf [16]byte
	_, _ = rand.Read(jtiBuf[:])
	now := time.Now().UTC()
	claims := SessionClaims{
		JTI:       hex.EncodeToString(jtiBuf[:]),
		Sub:       sub,
		Email:     strings.ToLower(strings.TrimSpace(email)),
		Name:      name,
		Picture:   picture,
		Roles:     roles,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(m.cfg.SessionTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", SessionClaims{}, err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	sig := m.sign(enc)
	return enc + "." + sig, claims, nil
}

// VerifySession validates signature, expiry, and revocation status of a signed token.
func (m *Manager) VerifySession(token string) (*SessionClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return nil, errors.New("oauthsession: malformed session token")
	}
	expected := m.sign(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return nil, errors.New("oauthsession: invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("oauthsession: decode claims: %w", err)
	}
	var claims SessionClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("oauthsession: parse claims: %w", err)
	}
	if time.Now().UTC().Unix() > claims.ExpiresAt {
		return nil, errors.New("oauthsession: session expired")
	}
	m.mu.RLock()
	_, isRevoked := m.revoked[claims.JTI]
	m.mu.RUnlock()
	if isRevoked {
		return nil, errors.New("oauthsession: session revoked")
	}
	return &claims, nil
}

// Revoke adds a JTI to the revocation set.
func (m *Manager) Revoke(jti string, expiresAt int64) {
	if jti == "" {
		return
	}
	m.mu.Lock()
	m.revoked[jti] = expiresAt
	m.mu.Unlock()
}

// SetSessionCookie writes the signed session token as an HttpOnly SameSite=Lax cookie.
func (m *Manager) SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(m.cfg.SessionTTL.Seconds()),
	})
}

// ClearSessionCookie expires the session cookie on the client.
func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ExtractRequestSession extracts and verifies a session from the Authorization Bearer header or session cookie.
func (m *Manager) ExtractRequestSession(r *http.Request) (*SessionClaims, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if claims, err := m.VerifySession(tok); err == nil {
			return claims, nil
		}
	}
	if c, err := r.Cookie(m.cfg.CookieName); err == nil && c.Value != "" {
		return m.VerifySession(c.Value)
	}
	return nil, errors.New("oauthsession: no valid session")
}

// ExchangeGoogleAuthCode exchanges an OAuth2 authorization code for tokens and returns user claims.
func (m *Manager) ExchangeGoogleAuthCode(ctx context.Context, code, redirectURI string) (map[string]any, error) {
	if m.cfg.ClientID == "" || m.cfg.ClientSecret == "" {
		return nil, errors.New("oauthsession: google client_id/client_secret not configured")
	}
	if redirectURI == "" {
		redirectURI = m.cfg.RedirectURI
	}
	form := url.Values{
		"code":          {code},
		"client_id":     {m.cfg.ClientID},
		"client_secret": {m.cfg.ClientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauthsession: google token exchange HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tokResp map[string]any
	if err := json.Unmarshal(body, &tokResp); err != nil {
		return nil, err
	}
	return tokResp, nil
}

// VerifyGoogleIDToken verifies a Google OIDC ID token via Google's tokeninfo endpoint and checks audience.
func (m *Manager) VerifyGoogleIDToken(ctx context.Context, idToken string) (sub, email, name, picture string, err error) {
	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", "", "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", "", "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", "", "", "", fmt.Errorf("oauthsession: invalid google id_token HTTP %d", resp.StatusCode)
	}
	var info struct {
		Aud     string `json:"aud"`
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", "", "", "", err
	}
	if m.cfg.ClientID != "" && info.Aud != m.cfg.ClientID {
		return "", "", "", "", fmt.Errorf("oauthsession: aud mismatch %q", info.Aud)
	}
	if info.Email == "" {
		return "", "", "", "", errors.New("oauthsession: missing email in id_token")
	}
	return info.Sub, info.Email, info.Name, info.Picture, nil
}

// HasRole checks whether claims include role (or "admin").
func HasRole(claims *SessionClaims, role string) bool {
	if claims == nil {
		return false
	}
	for _, r := range claims.Roles {
		if r == role || r == "admin" {
			return true
		}
	}
	return false
}

// SecurityHeadersMiddleware attaches Google One-Tap compatible COOP and Referrer-Policy headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) sign(data string) string {
	mac := hmac.New(sha256.New, []byte(m.cfg.SessionSecret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) isDomainAllowed(email string) bool {
	if len(m.cfg.AllowedDomains) == 0 {
		return true
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(email)), "@")
	if len(parts) != 2 {
		return false
	}
	for _, d := range m.cfg.AllowedDomains {
		if strings.EqualFold(strings.TrimSpace(d), parts[1]) {
			return true
		}
	}
	return false
}

func (m *Manager) defaultRolesFor(email string) []string {
	low := strings.ToLower(strings.TrimSpace(email))
	for _, adm := range m.cfg.AdminEmails {
		if strings.EqualFold(strings.TrimSpace(adm), low) {
			return []string{"admin", "user"}
		}
	}
	return []string{"user"}
}
