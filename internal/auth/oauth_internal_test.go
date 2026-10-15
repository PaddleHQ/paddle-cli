package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("code verifier length %d out of range [43, 128]", len(v))
	}
}

func TestCodeChallenge(t *testing.T) {
	verifier := "test-verifier-1234567890abcdefghijklmnop"
	challenge := CodeChallenge(verifier)
	if challenge == "" {
		t.Error("expected non-empty code challenge")
	}
	if challenge == verifier {
		t.Error("challenge should not equal verifier")
	}
}

func TestExchangeCode_Success(t *testing.T) {
	var gotForm url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = r.Form
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header for public client, got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"access_token":"at_123","refresh_token":"rt_123","token_type":"bearer","expires_in":3600,"scope":"offline_access"}`); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := OAuthConfig{
		TokenURL: srv.URL,
		ClientID: "paddle-cli-test",
	}

	before := time.Now()
	tok, err := ExchangeCode(context.Background(), cfg, "code_abc", "verifier_abc", "http://127.0.0.1:1234/callback")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}

	if tok.AccessToken != "at_123" {
		t.Errorf("AccessToken = %q, want at_123", tok.AccessToken)
	}
	if tok.RefreshToken != "rt_123" {
		t.Errorf("RefreshToken = %q, want rt_123", tok.RefreshToken)
	}
	if tok.ExpiresAt.Before(before.Add(3599*time.Second)) || tok.ExpiresAt.After(before.Add(3601*time.Second)) {
		t.Errorf("ExpiresAt = %v, want ~1h from now", tok.ExpiresAt)
	}

	wantForm := map[string]string{
		"grant_type":    "authorization_code",
		"code":          "code_abc",
		"redirect_uri":  "http://127.0.0.1:1234/callback",
		"client_id":     "paddle-cli-test",
		"code_verifier": "verifier_abc",
	}
	for k, v := range wantForm {
		if got := gotForm.Get(k); got != v {
			t.Errorf("form[%q] = %q, want %q", k, got, v)
		}
	}
	if _, ok := gotForm["client_secret"]; ok {
		t.Error("form contains client_secret, which a public client should not send for a public client")
	}
}

func TestExchangeCode_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := fmt.Fprint(w, `{"error":"invalid_grant","error_description":"code expired"}`); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := ExchangeCode(context.Background(), OAuthConfig{TokenURL: srv.URL, ClientID: "c"}, "code", "verifier", "http://127.0.0.1/callback")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") || !strings.Contains(err.Error(), "code expired") {
		t.Errorf("error = %v, want it to contain invalid_grant and description", err)
	}
}

func TestExchangeCode_NoExpiresIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"access_token":"at","token_type":"bearer"}`); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	tok, err := ExchangeCode(context.Background(), OAuthConfig{TokenURL: srv.URL, ClientID: "c"}, "code", "verifier", "http://127.0.0.1/callback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tok.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero when expires_in absent", tok.ExpiresAt)
	}
}

func TestRefreshAccessToken_Success(t *testing.T) {
	var gotForm url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"access_token":"at_new","refresh_token":"rt_new","token_type":"bearer","expires_in":3600}`); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	tok, err := RefreshAccessToken(context.Background(), OAuthConfig{TokenURL: srv.URL, ClientID: "paddle-cli-test"}, "rt_old")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if tok.AccessToken != "at_new" {
		t.Errorf("AccessToken = %q, want at_new", tok.AccessToken)
	}
	if tok.RefreshToken != "rt_new" {
		t.Errorf("RefreshToken = %q, want rt_new", tok.RefreshToken)
	}
	if gotForm.Get("grant_type") != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotForm.Get("grant_type"))
	}
	if gotForm.Get("refresh_token") != "rt_old" {
		t.Errorf("refresh_token = %q, want rt_old", gotForm.Get("refresh_token"))
	}
	if gotForm.Get("client_id") != "paddle-cli-test" {
		t.Errorf("client_id = %q, want paddle-cli-test", gotForm.Get("client_id"))
	}
}

func TestRefreshAccessToken_EmptyToken(t *testing.T) {
	_, err := RefreshAccessToken(context.Background(), OAuthConfig{TokenURL: "http://example"}, "")
	if err == nil {
		t.Fatal("expected error for empty refresh token")
	}
}

func TestRefreshAccessToken_PreservesOldRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"access_token":"at_new","token_type":"bearer","expires_in":3600}`); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	tok, err := RefreshAccessToken(context.Background(), OAuthConfig{TokenURL: srv.URL, ClientID: "c"}, "rt_preserved")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.RefreshToken != "rt_preserved" {
		t.Errorf("RefreshToken = %q, want rt_preserved (should fall back to input when response omits it)", tok.RefreshToken)
	}
}

func TestRefreshAccessToken_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := fmt.Fprint(w, `{"error":"invalid_grant","error_description":"token revoked"}`); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := RefreshAccessToken(context.Background(), OAuthConfig{TokenURL: srv.URL, ClientID: "c"}, "rt_bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error = %v, want invalid_grant", err)
	}
}

func TestPostToken_NonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := io.WriteString(w, "gateway down"); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := ExchangeCode(context.Background(), OAuthConfig{TokenURL: srv.URL, ClientID: "c"}, "code", "v", "http://127.0.0.1/callback")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "gateway down") {
		t.Errorf("error = %v, want it to contain status + body", err)
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	cfg := OAuthConfig{
		AuthURL:  "https://auth.example.test/oauth2/authorize",
		ClientID: "client_test",
		Scopes:   []string{"openid", "offline_access"},
	}
	got := buildAuthorizeURL(cfg, "http://127.0.0.1:1234/callback", "state_abc", "challenge_abc")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parsing authorize URL: %v", err)
	}
	if base := u.Scheme + "://" + u.Host + u.Path; base != cfg.AuthURL {
		t.Errorf("base URL = %q, want %q", base, cfg.AuthURL)
	}

	q := u.Query()
	wantParams := map[string]string{
		"response_type":         "code",
		"client_id":             "client_test",
		"redirect_uri":          "http://127.0.0.1:1234/callback",
		"scope":                 "openid offline_access",
		"state":                 "state_abc",
		"code_challenge":        "challenge_abc",
		"code_challenge_method": "S256",
	}
	for k, v := range wantParams {
		if got := q.Get(k); got != v {
			t.Errorf("param[%q] = %q, want %q", k, got, v)
		}
	}

	if len(q) != len(wantParams) {
		t.Errorf("authorize URL has params %v, want exactly %v", q, wantParams)
	}
}

func TestAuthorizeError(t *testing.T) {
	cases := []struct {
		name, code, desc, want string
	}{
		{"with description", "access_denied", "user canceled", "authorization failed: access_denied: user canceled"},
		{"without description", "server_error", "", "authorization failed: server_error"},
		{"invalid scope hints at the app config", "invalid_scope", "", "authorization failed: invalid_scope (the OAuth app may not be allowed to request one of the scopes)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := authorizeError(tc.code, tc.desc).Error(); got != tc.want {
				t.Errorf("authorizeError(%q, %q) = %q, want %q", tc.code, tc.desc, got, tc.want)
			}
		})
	}
}
