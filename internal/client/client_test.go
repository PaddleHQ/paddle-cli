package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/build"
	"github.com/PaddleHQ/paddle-cli/internal/client"
)

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/customers" {
			t.Errorf("expected /customers, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected auth header, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": []any{},
			"meta": map[string]any{"request_id": "abc"},
		}); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	defer server.Close()

	c := client.New(client.Config{
		BaseURL:     server.URL,
		AccessToken: "test-token",
	})

	resp, err := c.Do("GET", "/customers", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClient_Config(t *testing.T) {
	cfg := client.Config{
		BaseURL:     "https://example.com",
		AccessToken: "test-token",
		APIVersion:  "1",
		Headers:     map[string]string{"X-Test": "1"},
	}
	c := client.New(cfg)

	got := c.Config()
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("Config() = %+v, want %+v", got, cfg)
	}
}

func TestClient_CustomHeaders(t *testing.T) {
	var gotHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	c := client.New(client.Config{
		BaseURL:     server.URL,
		AccessToken: "test-token",
		Headers:     map[string]string{"X-Custom": "hello", "X-Debug": "true"},
	})

	_, err := c.Do("GET", "/test", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotHeaders.Get("X-Custom") != "hello" {
		t.Errorf("expected X-Custom: hello, got %q", gotHeaders.Get("X-Custom"))
	}
	if gotHeaders.Get("X-Debug") != "true" {
		t.Errorf("expected X-Debug: true, got %q", gotHeaders.Get("X-Debug"))
	}
}

func TestClient_PaddleVersionHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Paddle-Version")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{},"meta":{}}`))
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIVersion: "2025-01-01"})
	_, err := c.Do("GET", "/test", nil, nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotHeader != "2025-01-01" {
		t.Errorf("got Paddle-Version=%q, want %q", gotHeader, "2025-01-01")
	}
}

func TestClient_NoPaddleVersionHeaderWhenUnset(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Paddle-Version")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{},"meta":{}}`))
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL})
	_, _ = c.Do("GET", "/test", nil, nil)
	if gotHeader != "" {
		t.Errorf("expected no Paddle-Version header, got %q", gotHeader)
	}
}

func TestClient_UserAgent(t *testing.T) {
	origVersion := build.Version
	t.Cleanup(func() {
		build.Version = origVersion
	})

	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	cases := []struct {
		name    string
		version string
		want    string
	}{
		{"release build", "1.2.3", "paddle-cli/1.2.3"},
		{"dev build", "dev", "paddle-cli/dev"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build.Version = tc.version

			c := client.New(client.Config{BaseURL: server.URL})
			if _, err := c.Do("GET", "/test", nil, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotUserAgent != tc.want {
				t.Errorf("expected User-Agent %q, got %q", tc.want, gotUserAgent)
			}
		})
	}
}

func TestClient_AttributionHeaders(t *testing.T) {
	origVersion := build.Version
	t.Cleanup(func() { build.Version = origVersion })

	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	cases := []struct {
		name        string
		version     string
		wantVersion string
	}{
		{"release tag", "v0.1.0", "v0.1.0"},
		{"pre-release tag", "v0.2.0-rc.1", "v0.2.0-rc.1"},
		{"untagged build", "dev-91202f4", "dev"},
		{"commits after a tag", "v0.1.0-3-ga8c5128", "dev"},
		{"dirty tagged build", "v0.1.0-dirty", "dev"},
		{"dirty build after a tag", "v0.1.0-3-ga8c5128-dirty", "dev"},
		{"no git", "dev", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build.Version = tc.version
			c := client.New(client.Config{BaseURL: server.URL})
			if _, err := c.Do("GET", "/test", nil, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := map[string]string{
				"X-SDK-Language":          "cli",
				"X-SDK-Version":           tc.wantVersion,
				"X-Paddle-Client":         "paddle-cli",
				"X-Paddle-Client-Version": tc.wantVersion,
			}
			for k, v := range want {
				if got.Get(k) != v {
					t.Errorf("%s = %q, want %q", k, got.Get(k), v)
				}
			}
			if _, ok := got["X-Paddle-Client-App"]; ok {
				t.Errorf("X-Paddle-Client-App sent without a detected agent: %q", got.Get("X-Paddle-Client-App"))
			}
		})
	}
}

func TestClient_CustomUserAgentKeepsAttributionHeaders(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	c := client.New(client.Config{
		BaseURL:   server.URL,
		ClientApp: "claude-code",
		Headers:   map[string]string{"User-Agent": "my-wrapper/1.0"},
	})
	if _, err := c.Do("GET", "/test", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := got.Get("User-Agent"); v != "my-wrapper/1.0" {
		t.Errorf("User-Agent = %q, want my-wrapper/1.0", v)
	}
	for _, k := range []string{"X-SDK-Language", "X-SDK-Version", "X-Paddle-Client", "X-Paddle-Client-Version", "X-Paddle-Client-App"} {
		if got.Get(k) == "" {
			t.Errorf("%s missing when User-Agent is overridden", k)
		}
	}
}

func TestClient_ClientAppHeader(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	c := client.New(client.Config{BaseURL: server.URL, ClientApp: "claude-code"})
	if _, err := c.Do("GET", "/test", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v := got.Get("X-Paddle-Client-App"); v != "claude-code" {
		t.Errorf("X-Paddle-Client-App = %q, want claude-code", v)
	}
}
