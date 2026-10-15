package commands

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/output"
)

// WriteAPIError renders an API error in a form matching the user's chosen
// output format. For FormatJSON it emits the same shape as success output,
// compact when opts.Compact is set (agent mode), indented otherwise. For
// other formats it writes a single plain-text line.
func WriteAPIError(w io.Writer, apiErr *client.APIError, opts output.Options) {
	if opts.Format == output.FormatJSON {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		if !opts.Compact {
			enc.SetIndent("", "  ")
		}
		_ = enc.Encode(apiErr)
		return
	}
	_, _ = fmt.Fprintln(w, opts.Profile.Error(apiErr.Error()))
	for _, fe := range apiErr.Errors {
		_, _ = fmt.Fprintf(w, "  - %s: %s\n", fe.Field, opts.Profile.Warn(fe.Message))
	}
	if apiErr.DocumentationURL != "" {
		_, _ = fmt.Fprintf(w, "See %s\n", opts.Profile.Info(apiErr.DocumentationURL))
	}
	if apiErr.RequestID != "" {
		_, _ = fmt.Fprintf(w, "Request ID: %s\n", apiErr.RequestID)
	}
	_, _ = fmt.Fprintf(w, "Retryable: %t\n", apiErr.Retryable)
}
