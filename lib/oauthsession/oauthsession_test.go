package oauthsession

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionSignVerifyRevokeAndCookie(t *testing.T) {
	mgr := NewManager(Config{
		SessionSecret:  "unit-test-secret",
		SessionTTL:     time.Hour,
		AllowedDomains: []string{"example.com"},
		AdminEmails:    []string{"admin@example.com"},
	})
	if _, _, err := mgr.IssueSession("1", "bad@other.com", "Bad", "", nil); err == nil {
		t.Fatal("expected disallowed domain error")
	}
	tok, claims, err := mgr.IssueSession("sub-1", "admin@example.com", "Admin", "", nil)
	if err != nil || !HasRole(&claims, "admin") {
		t.Fatalf("IssueSession: claims=%+v err=%v", claims, err)
	}
	got, err := mgr.VerifySession(tok)
	if err != nil || got.Email != "admin@example.com" {
		t.Fatalf("VerifySession: got=%+v err=%v", got, err)
	}
	rec := httptest.NewRecorder()
	mgr.SetSessionCookie(rec, tok)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if extracted, err := mgr.ExtractRequestSession(req); err != nil || extracted.Sub != "sub-1" {
		t.Fatalf("ExtractRequestSession: %+v err=%v", extracted, err)
	}
	mgr.Revoke(claims.JTI, claims.ExpiresAt)
	if _, err := mgr.VerifySession(tok); err == nil {
		t.Fatal("expected revoked session error")
	}
}
