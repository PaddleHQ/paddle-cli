package client_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/client"
)

func TestParseAPIError_FromBody(t *testing.T) {
	body := []byte(`{
		"error": {
			"type": "request_error",
			"code": "not_found",
			"detail": "Entity pro_01gsz not found",
			"documentation_url": "https://developer.paddle.com/errors/shared/not_found"
		},
		"meta": {"request_id": "req_body_01"}
	}`)

	resp := &client.Response{
		StatusCode: 404,
		Body:       body,
		Headers:    http.Header{"X-Request-Id": []string{"req_header_01"}},
	}
	apiErr := client.ParseAPIError(resp)
	if apiErr == nil {
		t.Fatal("expected non-nil error")
	}
	if apiErr.Code != "not_found" {
		t.Errorf("Code = %q, want not_found", apiErr.Code)
	}
	if apiErr.Status != 404 {
		t.Errorf("Status = %d, want 404", apiErr.Status)
	}
	if apiErr.Detail != "Entity pro_01gsz not found" {
		t.Errorf("Detail = %q", apiErr.Detail)
	}
	if apiErr.RequestID != "req_header_01" {
		t.Errorf("RequestID = %q, want req_header_01 (header should win over body)", apiErr.RequestID)
	}
	if apiErr.Retryable {
		t.Errorf("Retryable = true, want false (404 is not retryable)")
	}
}

func TestParseAPIError_RequestIDFromBodyWhenHeaderMissing(t *testing.T) {
	body := []byte(`{
		"error": {"code": "not_found", "detail": "x"},
		"meta": {"request_id": "req_body_02"}
	}`)
	resp := &client.Response{StatusCode: 404, Body: body, Headers: http.Header{}}
	apiErr := client.ParseAPIError(resp)
	if apiErr.RequestID != "req_body_02" {
		t.Errorf("RequestID = %q, want req_body_02", apiErr.RequestID)
	}
}

func TestParseAPIError_RequestIDEmptyWhenAbsent(t *testing.T) {
	body := []byte(`{"error": {"code": "not_found", "detail": "x"}}`)
	resp := &client.Response{StatusCode: 404, Body: body, Headers: http.Header{}}
	apiErr := client.ParseAPIError(resp)
	if apiErr.RequestID != "" {
		t.Errorf("RequestID = %q, want empty", apiErr.RequestID)
	}
}

func TestParseAPIError_RetryableMatrix(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{400, false},
		{401, false},
		{404, false},
		{422, false},
		{429, true},
		{500, true},
		{503, true},
	}
	for _, tc := range cases {
		body := []byte(`{"error": {"code": "x", "detail": "y"}}`)
		resp := &client.Response{StatusCode: tc.status, Body: body, Headers: http.Header{}}
		apiErr := client.ParseAPIError(resp)
		if apiErr.Retryable != tc.want {
			t.Errorf("status %d: Retryable = %v, want %v", tc.status, apiErr.Retryable, tc.want)
		}
	}
}

func TestParseAPIError_InvalidJSON(t *testing.T) {
	resp := &client.Response{
		StatusCode: 500,
		Body:       []byte("internal server error"),
		Headers:    http.Header{"X-Request-Id": []string{"req_header_03"}},
	}
	apiErr := client.ParseAPIError(resp)
	if apiErr == nil {
		t.Fatal("expected non-nil error")
	}
	if apiErr.Status != 500 {
		t.Errorf("Status = %d, want 500", apiErr.Status)
	}
	if apiErr.RequestID != "req_header_03" {
		t.Errorf("RequestID = %q, want req_header_03 (header should still be captured on unparseable body)", apiErr.RequestID)
	}
	if !apiErr.Retryable {
		t.Errorf("Retryable = false, want true (500 is retryable)")
	}
}

func TestIsRetryableStatus(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{404, false},
		{409, false},
		{422, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}
	for _, tc := range cases {
		if got := client.IsRetryableStatus(tc.status); got != tc.want {
			t.Errorf("IsRetryableStatus(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestAPIError_ExitCode_StatusMatrix(t *testing.T) {
	cases := []struct {
		status int
		want   int
	}{
		{200, client.ExitGeneralError}, // shouldn't happen, but defensively map to 1
		{400, client.ExitValidation},
		{401, client.ExitAuthFailure},
		{403, client.ExitForbidden},
		{404, client.ExitNotFound},
		{409, client.ExitConflict},
		{422, client.ExitValidation},
		{429, client.ExitRateLimited},
		{499, client.ExitGeneralError}, // unknown 4xx
		{500, client.ExitServerError},
		{502, client.ExitServerError},
		{503, client.ExitServerError},
		{504, client.ExitServerError},
		{599, client.ExitServerError},
	}
	for _, tc := range cases {
		err := &client.APIError{Status: tc.status}
		if got := err.ExitCode(); got != tc.want {
			t.Errorf("status %d: ExitCode() = %d, want %d", tc.status, got, tc.want)
		}
	}
}

// The Paddle API answers authentication failures with 403, so the error code,
// not the status, separates "not authenticated" from "not permitted".
func TestAPIError_ExitCode_403ByErrorCode(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"authentication_missing", client.ExitAuthFailure},
		{"authentication_malformed", client.ExitAuthFailure},
		{"invalid_token", client.ExitAuthFailure},
		{"forbidden", client.ExitForbidden},
		{"", client.ExitForbidden},
	}
	for _, tc := range cases {
		err := &client.APIError{Status: 403, Code: tc.code}
		if got := err.ExitCode(); got != tc.want {
			t.Errorf("403 %q: ExitCode() = %d, want %d", tc.code, got, tc.want)
		}
	}
}

// timeoutError satisfies net.Error with Timeout()=true.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// errUsage stands in for commands.ErrUsage so the client package can be
// tested without importing internal/commands. The real wiring uses
// commands.ErrUsage directly via errors.Is: ExitCodeFor accepts a target
// sentinel injected by the caller.
var errUsage = errors.New("usage error")

func TestExitCodeFor_APIError(t *testing.T) {
	err := &client.APIError{Status: 404}
	code, apiErr := client.ExitCodeFor(err, errUsage)
	if code != client.ExitNotFound {
		t.Errorf("code = %d, want %d", code, client.ExitNotFound)
	}
	if apiErr == nil {
		t.Error("apiErr = nil, want non-nil")
	} else if apiErr.Status != 404 {
		t.Errorf("apiErr.Status = %d, want 404", apiErr.Status)
	}
}

func TestExitCodeFor_WrappedAPIError(t *testing.T) {
	err := fmt.Errorf("doing thing: %w", &client.APIError{Status: 401})
	code, apiErr := client.ExitCodeFor(err, errUsage)
	if code != client.ExitAuthFailure {
		t.Errorf("code = %d, want %d", code, client.ExitAuthFailure)
	}
	if apiErr == nil {
		t.Error("apiErr = nil, want non-nil")
	} else if apiErr.Status != 401 {
		t.Errorf("apiErr.Status = %d, want 401", apiErr.Status)
	}
}

func TestExitCodeFor_UsageError(t *testing.T) {
	err := fmt.Errorf("%w: bad flag", errUsage)
	code, apiErr := client.ExitCodeFor(err, errUsage)
	if code != client.ExitUsageError {
		t.Errorf("code = %d, want %d", code, client.ExitUsageError)
	}
	if apiErr != nil {
		t.Errorf("apiErr = %+v, want nil", apiErr)
	}
}

func TestExitCodeFor_DeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("dial: %w", context.DeadlineExceeded)
	code, apiErr := client.ExitCodeFor(err, errUsage)
	if code != client.ExitTimeout {
		t.Errorf("code = %d, want %d (timeout)", code, client.ExitTimeout)
	}
	if apiErr != nil {
		t.Errorf("apiErr = %+v, want nil", apiErr)
	}
}

func TestExitCodeFor_NetTimeout(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "http://example", Err: timeoutError{}}
	code, apiErr := client.ExitCodeFor(err, errUsage)
	if code != client.ExitTimeout {
		t.Errorf("code = %d, want %d", code, client.ExitTimeout)
	}
	if apiErr != nil {
		t.Errorf("apiErr = %+v, want nil", apiErr)
	}
}

func TestExitCodeFor_Fallback(t *testing.T) {
	err := errors.New("something else")
	code, apiErr := client.ExitCodeFor(err, errUsage)
	if code != client.ExitGeneralError {
		t.Errorf("code = %d, want %d", code, client.ExitGeneralError)
	}
	if apiErr != nil {
		t.Errorf("apiErr = %+v, want nil", apiErr)
	}
}

// compile-time assertion that timeoutError satisfies net.Error.
var _ net.Error = timeoutError{}
