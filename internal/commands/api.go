// Package commands holds the hand-crafted cobra commands and the root
// command wiring.
package commands

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

var (
	errPathIsURL       = errors.New("must be a path (not a full URL)")
	errPathMissingPath = errors.New("missing path component")
)

// NewAPICmd builds `paddle api`, which sends a raw request to any endpoint.
func NewAPICmd(root *Root) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make a raw API request",
		Long: "Make a raw API request to any Paddle endpoint. Uses current environment auth and base URL. " +
			"The path may include an embedded query string (e.g. /customers?status=active) which is merged with --query flags; " +
			"on conflict, --query wins.",
		GroupID:     GroupIDOther,
		Annotations: callsAPIAnnotations(),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPI(cmd, root, args[0])
		},
	}

	cmd.Flags().String("method", "GET", "HTTP method (GET, POST, PATCH, DELETE)")
	cmd.Flags().String("json", "", "JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().StringArray("query", nil,
		"Query parameter (repeatable, format: key=value). Merged with any embedded query in <path>; --query wins on conflict.")

	return cmd
}

func runAPI(cmd *cobra.Command, root *Root, rawPath string) error {
	method, _ := cmd.Flags().GetString("method")
	jsonInput, _ := cmd.Flags().GetString("json")
	queryFlags, _ := cmd.Flags().GetStringArray("query")

	parsed, err := url.Parse(rawPath)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", rawPath, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" {
		return fmt.Errorf("invalid path %q: %w", rawPath, errPathIsURL)
	}

	queryParams := mergeQueryParams(parsed.Query(), queryFlags)

	if parsed.Path == "" {
		return fmt.Errorf("invalid path %q: %w", rawPath, errPathMissingPath)
	}

	var body []byte
	if jsonInput != "" {
		body, err = readInput(jsonInput)
		if err != nil {
			return fmt.Errorf("reading --json: %w", err)
		}
	}

	resp, err := root.Client().Do(method, parsed.Path, queryParams, body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return client.ParseAPIError(resp)
	}

	opts := root.OutputOpts()
	opts.Writer = cmd.OutOrStdout()
	return output.Print(resp.Body, nil, opts)
}

// mergeQueryParams flattens the path's embedded query (first value wins) and
// overlays key=value --query flags on top.
func mergeQueryParams(embedded url.Values, queryFlags []string) map[string]string {
	queryParams := make(map[string]string)
	for k, vs := range embedded {
		if len(vs) > 0 {
			queryParams[k] = vs[0]
		}
	}
	for _, q := range queryFlags {
		k, v, ok := strings.Cut(q, "=")
		if ok {
			queryParams[k] = v
		}
	}
	return queryParams
}

func readInput(input string) ([]byte, error) {
	if input == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return b, nil
	}
	if after, ok := strings.CutPrefix(input, "@"); ok {
		//nolint:gosec,wrapcheck // reading a user-named file is the point of --json @file; the PathError already names it
		return os.ReadFile(after)
	}
	return []byte(input), nil
}
