package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

type whoamiAPIResponse struct {
	Data struct {
		Caller struct {
			Account struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"account"`
			AuthType    string   `json:"auth_type"`
			Permissions []string `json:"permissions"`
		} `json:"caller"`
	} `json:"data"`
}

type whoamiTableRow struct {
	Environment string `json:"environment"`
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	AuthType    string `json:"auth_type"`
	Permissions string `json:"permissions,omitempty"`
}

var errNotAuthenticated = errors.New("not authenticated for environment")

// NewWhoamiCmd builds `paddle whoami`, which shows the identity behind the active credential.
func NewWhoamiCmd(root *Root) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:     "whoami",
		Short:   "Show who you're authenticated as in the active environment",
		GroupID: GroupIDSession,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !root.HasCredential {
				return fmt.Errorf("%w %q; run `paddle login` first", errNotAuthenticated, root.Environment)
			}

			resp, err := root.Client().Do("GET", "/", nil, nil)
			if err != nil {
				return err
			}
			if resp.StatusCode >= 400 {
				return client.ParseAPIError(resp)
			}

			opts := root.OutputOpts()
			opts.Writer = cmd.OutOrStdout()

			if opts.Format == output.FormatJSON {
				// Pass raw body through: injectMetaEnvironment in printJSON
				// handles meta.environment automatically via opts.Environment.
				return output.Print(resp.Body, nil, opts)
			}
			return printWhoami(resp.Body, full, opts)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Expand permission list in table output (JSON always includes full permissions)")
	return cmd
}

// printWhoami renders a curated table view of the `GET /` response.
// Default: one row with environment, account, auth type, and permission count.
// With full=true: summary row (no permissions) followed by a permission-per-row table.
func printWhoami(body []byte, full bool, opts output.Options) error {
	var apiResp whoamiAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("parsing whoami response: %w", err)
	}
	caller := apiResp.Data.Caller

	envName := opts.Profile.EnvName(opts.Environment)

	row := whoamiTableRow{
		Environment: envName,
		AccountID:   caller.Account.ID,
		AccountName: caller.Account.Name,
		AuthType:    caller.AuthType,
	}
	cols := []output.ColumnDef{
		{Header: "ENVIRONMENT", Field: fieldEnvironment},
		{Header: "ACCOUNT ID", Field: "account_id"},
		{Header: "ACCOUNT NAME", Field: "account_name"},
		{Header: "AUTH TYPE", Field: "auth_type"},
	}

	if !full {
		row.Permissions = fmt.Sprintf("%d permissions", len(caller.Permissions))
		cols = append(cols, output.ColumnDef{Header: "PERMISSIONS", Field: "permissions"})
	}

	payload, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("encoding whoami row: %w", err)
	}

	if err := output.Print(payload, cols, opts); err != nil {
		return err
	}

	if full {
		return printWhoamiPermissions(caller.Permissions, opts)
	}
	return nil
}

// printWhoamiPermissions renders one permission per row.
func printWhoamiPermissions(permissions []string, opts output.Options) error {
	type permissionRow struct {
		Permission string `json:"permission"`
	}

	rows := make([]permissionRow, len(permissions))
	for i, p := range permissions {
		rows[i] = permissionRow{Permission: p}
	}

	payload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("encoding whoami permissions: %w", err)
	}

	return output.Print(payload, []output.ColumnDef{{Header: "PERMISSION", Field: "permission"}}, opts)
}
