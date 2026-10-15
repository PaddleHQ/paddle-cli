// Package client is the HTTP client for the Paddle API: request building,
// attribution headers, and API error parsing.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/build"
)

// Config holds per-invocation settings applied to every request.
type Config struct {
	BaseURL     string
	AccessToken string
	APIVersion  string
	Headers     map[string]string
	// ClientApp names the agent or automation driving the CLI, if any; sent
	// as X-Paddle-Client-App.
	ClientApp string
}

// Client sends requests to the Paddle API.
type Client struct {
	config     Config
	httpClient *http.Client
}

// New returns a Client configured with cfg.
func New(cfg Config) *Client {
	return &Client{
		config:     cfg,
		httpClient: &http.Client{},
	}
}

// Configure updates the client's configuration in place.
// This allows pre-allocated clients to be reconfigured after creation.
func (c *Client) Configure(cfg Config) {
	c.config = cfg
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
}

// Config returns the client's current configuration, so a caller can build
// a variant client (e.g. against a different BaseURL/AccessToken) without
// dropping headers and api-version already resolved for this invocation.
func (c *Client) Config() Config {
	return c.config
}

// Do sends a request to path relative to the configured BaseURL and returns
// the buffered response. Non-2xx statuses are returned, not treated as errors.
func (c *Client) Do(method, path string, queryParams map[string]string, body []byte) (*Response, error) {
	u, err := c.buildURL(path, queryParams)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setHeaders(req.Header)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

// buildURL joins path onto the configured BaseURL and applies queryParams.
func (c *Client) buildURL(path string, queryParams map[string]string) (string, error) {
	u, err := url.JoinPath(c.config.BaseURL, path)
	if err != nil {
		return "", fmt.Errorf("building URL: %w", err)
	}
	if len(queryParams) == 0 {
		return u, nil
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("parsing URL: %w", err)
	}
	q := parsed.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

// setHeaders applies the standard, auth, versioning and user-supplied headers.
func (c *Client) setHeaders(h http.Header) {
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", userAgent())
	setAttributionHeaders(h, c.config.ClientApp)

	if c.config.AccessToken != "" {
		h.Set("Authorization", "Bearer "+c.config.AccessToken)
	}

	if c.config.APIVersion != "" {
		h.Set("Paddle-Version", c.config.APIVersion)
	}

	for k, v := range c.config.Headers {
		h.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
}

// Response is a fully read HTTP response.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// setAttributionHeaders sets the analytics headers to attribute usage.
func setAttributionHeaders(h http.Header, clientApp string) {
	version := analyticsVersion()
	h.Set("X-SDK-Language", "cli")
	h.Set("X-SDK-Version", version)
	h.Set("X-Paddle-Client", "paddle-cli")
	h.Set("X-Paddle-Client-Version", version)
	if clientApp != "" {
		h.Set("X-Paddle-Client-App", clientApp)
	}
}

var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// analyticsVersion reports release tags as-is and every other build as "dev",
// so per-commit versions don't inflate analytics metric's cardinality.
func analyticsVersion() string {
	if releaseTag.MatchString(build.Version) && !strings.HasSuffix(build.Version, "-dirty") {
		return build.Version
	}
	return "dev"
}

func userAgent() string {
	return "paddle-cli/" + build.Version
}
