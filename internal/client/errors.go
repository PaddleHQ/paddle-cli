package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

// APIError is a non-2xx response from the Paddle API.
type APIError struct {
	Status           int          `json:"status"`
	Type             string       `json:"type"`
	Code             string       `json:"code"`
	Detail           string       `json:"detail"`
	DocumentationURL string       `json:"documentation_url"`
	RequestID        string       `json:"request_id,omitempty"`
	Retryable        bool         `json:"retryable"`
	Errors           []FieldError `json:"errors,omitempty"`
}

// FieldError is a validation error for a single request field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (status %d)", e.Code, e.Detail, e.Status)
	}
	return fmt.Sprintf("API error (status %d): %s", e.Status, e.Detail)
}

// ParseAPIError builds an APIError from resp, falling back to the raw body as
// Detail when it is not a Paddle error envelope.
func ParseAPIError(resp *Response) *APIError {
	apiErr := &APIError{
		Status:    resp.StatusCode,
		Retryable: IsRetryableStatus(resp.StatusCode),
	}

	if resp.Headers != nil {
		apiErr.RequestID = resp.Headers.Get("X-Request-Id")
	}

	var envelope struct {
		Error struct {
			Type             string       `json:"type"`
			Code             string       `json:"code"`
			Detail           string       `json:"detail"`
			DocumentationURL string       `json:"documentation_url"`
			Errors           []FieldError `json:"errors"`
		} `json:"error"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(resp.Body, &envelope); err != nil || envelope.Error.Code == "" {
		apiErr.Detail = string(resp.Body)
		return apiErr
	}

	apiErr.Type = envelope.Error.Type
	apiErr.Code = envelope.Error.Code
	apiErr.Detail = envelope.Error.Detail
	apiErr.DocumentationURL = envelope.Error.DocumentationURL
	apiErr.Errors = envelope.Error.Errors

	if apiErr.RequestID == "" {
		apiErr.RequestID = envelope.Meta.RequestID
	}

	return apiErr
}

// Exit codes per the safety spec. Codes are HTTP-derived where possible so
// agents can branch on failure class without parsing error prose.
const (
	ExitSuccess      = 0
	ExitGeneralError = 1   // fallback for unclassified errors
	ExitUsageError   = 2   // wrong flags / unknown subcommand / bad args
	ExitNotFound     = 3   // 404
	ExitAuthFailure  = 4   // 401: not authenticated
	ExitForbidden    = 5   // 403: authenticated but not allowed
	ExitRateLimited  = 6   // 429
	ExitValidation   = 7   // 400 / 422: request body or query rejected
	ExitConflict     = 8   // 409
	ExitServerError  = 9   // 5xx
	ExitTimeout      = 124 // request deadline exceeded
)

// ExitCode maps the HTTP status carried by an APIError onto the semantic exit
// code spec. Anything outside the recognized set falls through to
// ExitGeneralError so unexpected statuses are still distinguishable from
// usage errors and timeouts.
func (e *APIError) ExitCode() int {
	switch e.Status {
	case 400, 422:
		return ExitValidation
	case 401:
		return ExitAuthFailure
	case 403:
		return ExitForbidden
	case 404:
		return ExitNotFound
	case 409:
		return ExitConflict
	case 429:
		return ExitRateLimited
	}
	if e.Status >= 500 && e.Status < 600 {
		return ExitServerError
	}
	return ExitGeneralError
}

// IsRetryableStatus reports whether an HTTP status code indicates a transient
// failure worth retrying. 429 (rate-limited) and any 5xx are retryable; all
// other 4xx are not.
func IsRetryableStatus(status int) bool {
	if status == 429 {
		return true
	}
	return status >= 500 && status < 600
}

// ExitCodeFor classifies err for process exit. The second return is non-nil
// when err wraps an *APIError; callers should pass that to WriteAPIError
// instead of printing err.Error(). The helper itself never writes anything.
//
// usageSentinel is the "usage error" sentinel from the commands package
// (passed in to avoid an import cycle).
func ExitCodeFor(err error, usageSentinel error) (int, *APIError) {
	if err == nil {
		return ExitSuccess, nil
	}

	if apiErr, ok := errors.AsType[*APIError](err); ok {
		code := apiErr.ExitCode()
		return code, apiErr
	}

	if isTimeoutError(err) {
		return ExitTimeout, nil
	}

	if usageSentinel != nil && errors.Is(err, usageSentinel) {
		return ExitUsageError, nil
	}

	return ExitGeneralError, nil
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
