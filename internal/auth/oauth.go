// Package auth implements the OAuth 2.0 authorization-code flow with PKCE
// used by `paddle login`, plus token refresh.
package auth

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
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// paramClientID is the OAuth client_id parameter name.
const paramClientID = "client_id"

var (
	errStateMismatch       = errors.New("state mismatch: possible CSRF")
	errNoCode              = errors.New("no code in callback")
	errCallbackTimeout     = errors.New("timed out waiting for authorization callback")
	errAuthorizationFailed = errors.New("authorization failed")
	errNoRefreshToken      = errors.New("no refresh token available")
	errTokenEndpoint       = errors.New("token endpoint error")
	errTokenEndpointStatus = errors.New("token endpoint returned")
	errEmptyAccessToken    = errors.New("token endpoint returned empty access_token")
	errUnsupportedPlatform = errors.New("unsupported platform")
	errUnexpectedAddr      = errors.New("callback listener has a non-TCP address")
)

// OAuthConfig describes the authorization server and client used for login.
type OAuthConfig struct {
	AuthURL     string
	TokenURL    string
	ClientID    string
	RedirectURI string
	Scopes      []string
	// Agent, when true, prints the auth URL to stdout instead of opening a browser.
	Agent bool
	// Output is where the agent-mode auth URL is written. Defaults to os.Stdout.
	Output io.Writer
}

// Token is a token set returned by the authorization server.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	// Scope is the space-separated scopes the server granted; empty when the
	// server omitted it (RFC 6749: identical to the scopes requested).
	Scope string `json:"scope"`
}

// TokenResponse mirrors the wire format of the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

type tokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// GenerateCodeVerifier returns a random PKCE code verifier (RFC 7636).
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeChallenge returns the S256 PKCE code challenge for verifier.
func CodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// generateState returns a cryptographically random state parameter for CSRF protection.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// httpClient is the client used for token endpoint calls. It has a timeout to
// avoid hanging login flows if the auth server is unreachable.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// LoginInteractive runs the browser-based PKCE login, receiving the
// authorization code on a loopback callback server.
func LoginInteractive(cfg OAuthConfig) (*Token, error) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generating code verifier: %w", err)
	}
	challenge := CodeChallenge(verifier)

	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}

	listener, port, err := listenLoopback()
	if err != nil {
		return nil, err
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	authURL := buildAuthorizeURL(cfg, redirectURI, state, challenge)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callbackHandler(state, codeCh, errCh))

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(context.Background()) }()

	out := cfg.Output
	if out == nil {
		out = discardWriter{}
	}
	if cfg.Agent {
		_, _ = fmt.Fprintf(out, "Open this URL in your browser to authenticate:\n\n  %s\n\n", authURL)
	} else if err := openBrowser(authURL); err != nil {
		_, _ = fmt.Fprintf(out, "Could not open browser. Open this URL manually:\n\n  %s\n\n", authURL)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, errCallbackTimeout
	}

	return ExchangeCode(context.Background(), cfg, code, verifier, redirectURI)
}

// listenLoopback opens a listener on an ephemeral 127.0.0.1 port.
func listenLoopback() (net.Listener, int, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, fmt.Errorf("starting callback server: %w", err)
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, 0, fmt.Errorf("starting callback server: %w", errUnexpectedAddr)
	}
	return listener, addr.Port, nil
}

// callbackHandler receives the authorization redirect, sending the code or
// the failure to the login flow.
func callbackHandler(state string, codeCh chan<- string, errCh chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errParam := q.Get("error"); errParam != "" {
			errCh <- authorizeError(errParam, q.Get("error_description"))
			http.Error(w, "Authentication failed. You can close this window.", http.StatusBadRequest)
			return
		}
		if got := q.Get("state"); got != state {
			errCh <- fmt.Errorf("%w (got %q)", errStateMismatch, got)
			http.Error(w, "State mismatch. You can close this window.", http.StatusBadRequest)
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- errNoCode
			http.Error(w, "No authorization code received. You can close this window.", http.StatusBadRequest)
			return
		}
		codeCh <- code
		_, _ = fmt.Fprintln(w, "Authentication successful! You can close this window.")
	}
}

// authorizeError describes an authorization-server error redirect; invalid_scope
// gets a hint because servers rarely say which scope the app can't request.
func authorizeError(code, desc string) error {
	detail := code
	if desc != "" {
		detail += ": " + desc
	}
	if code == "invalid_scope" {
		detail += " (the OAuth app may not be allowed to request one of the scopes)"
	}
	return fmt.Errorf("%w: %s", errAuthorizationFailed, detail)
}

// buildAuthorizeURL constructs the OAuth authorize URL for the PKCE
// loopback-redirect flow.
func buildAuthorizeURL(cfg OAuthConfig, redirectURI, state, challenge string) string {
	params := url.Values{
		"response_type":         {"code"},
		paramClientID:           {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(cfg.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return cfg.AuthURL + "?" + params.Encode()
}

// ExchangeCode swaps an authorization code for a token set. The PKCE verifier
// and the exact redirect_uri used in the auth request must be supplied.
func ExchangeCode(ctx context.Context, cfg OAuthConfig, code, verifier, redirectURI string) (*Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		paramClientID:   {cfg.ClientID},
		"code_verifier": {verifier},
	}
	return postToken(ctx, cfg.TokenURL, form)
}

// RefreshAccessToken exchanges a refresh token for a new token set.
// If the response does not include a new refresh token, the existing one is preserved.
func RefreshAccessToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*Token, error) {
	if refreshToken == "" {
		return nil, errNoRefreshToken
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		paramClientID:   {cfg.ClientID},
	}
	tok, err := postToken(ctx, cfg.TokenURL, form)
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return tok, nil
}

// postToken sends a token-endpoint request and decodes the resulting token set.
func postToken(ctx context.Context, tokenURL string, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling token endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, tokenEndpointError(resp.StatusCode, body)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, errEmptyAccessToken
	}

	expiresAt := time.Time{}
	if tr.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        tr.Scope,
	}, nil
}

// tokenEndpointError builds the error for a non-200 token-endpoint response,
// preferring the RFC 6749 error fields when the body carries them.
func tokenEndpointError(status int, body []byte) error {
	var te tokenErrorResponse
	if json.Unmarshal(body, &te) == nil && te.Error != "" {
		if te.ErrorDescription != "" {
			return fmt.Errorf("%w: %s: %s", errTokenEndpoint, te.Error, te.ErrorDescription)
		}
		return fmt.Errorf("%w: %s", errTokenEndpoint, te.Error)
	}
	return fmt.Errorf("%w %d: %s", errTokenEndpointStatus, status, strings.TrimSpace(string(body)))
}

func openBrowser(target string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{target}
	case "linux":
		name, args = "xdg-open", []string{target}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return errUnsupportedPlatform
	}
	//nolint:gosec // G204: fixed per-OS opener; target is the authorize URL we built, passed as an argument, not via a shell.
	if err := exec.CommandContext(context.Background(), name, args...).Start(); err != nil {
		return fmt.Errorf("starting %s: %w", name, err)
	}
	return nil
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
