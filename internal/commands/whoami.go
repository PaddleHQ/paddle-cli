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
		AuthSellerID string   `json:"auth_seller_id"`
		AuthUserID   string   `json:"auth_user_id"`
		Scopes       []string `json:"scopes"`
	} `json:"data"`
}

type whoamiTableRow struct {
	Environment string `json:"environment"`
	SellerID    string `json:"seller_id"`
	UserID      string `json:"user_id"`
	Scopes      string `json:"scopes,omitempty"`
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

			resp, err := root.Client().Do("GET", "/check/whoami", nil, nil)
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
				// --full has no effect on JSON (scopes always included in full data).
				return output.Print(resp.Body, nil, opts)
			}
			return printWhoamiTable(resp.Body, full, opts)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Expand scope list in table output (JSON always includes full scopes)")
	return cmd
}

// printWhoamiTable renders a curated table view of the /check/whoami response.
// Default: one row with environment, collapsed seller/user IDs, and scope count.
// With full=true: summary row (no scopes) followed by a scope-per-row table.
func printWhoamiTable(body []byte, full bool, opts output.Options) error {
	var apiResp whoamiAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("parsing whoami response: %w", err)
	}
	d := apiResp.Data

	envName := opts.Profile.EnvName(opts.Environment)
	sellerID := d.AuthSellerID
	userID := d.AuthUserID

	if !full {
		scopeStr := fmt.Sprintf("%d scopes", len(d.Scopes))
		payload, err := json.Marshal(whoamiTableRow{
			Environment: envName,
			SellerID:    sellerID,
			UserID:      userID,
			Scopes:      scopeStr,
		})
		if err != nil {
			return fmt.Errorf("encoding whoami row: %w", err)
		}
		cols := []output.ColumnDef{
			{Header: "ENVIRONMENT", Field: fieldEnvironment},
			{Header: "SELLER ID", Field: "seller_id"},
			{Header: "USER ID", Field: "user_id"},
			{Header: "SCOPES", Field: "scopes"},
		}
		return output.Print(payload, cols, opts)
	}

	// --full: summary row without scopes, then scope list as separate rows.
	summaryPayload, err := json.Marshal(whoamiTableRow{
		Environment: envName,
		SellerID:    sellerID,
		UserID:      userID,
	})
	if err != nil {
		return fmt.Errorf("encoding whoami summary: %w", err)
	}
	summaryCols := []output.ColumnDef{
		{Header: "ENVIRONMENT", Field: fieldEnvironment},
		{Header: "SELLER ID", Field: "seller_id"},
		{Header: "USER ID", Field: "user_id"},
	}
	if err := output.Print(summaryPayload, summaryCols, opts); err != nil {
		return err
	}

	type scopeRow struct {
		Scope string `json:"scope"`
	}
	rows := make([]scopeRow, len(d.Scopes))
	for i, s := range d.Scopes {
		rows[i] = scopeRow{Scope: s}
	}
	scopePayload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("encoding whoami scopes: %w", err)
	}
	return output.Print(scopePayload, []output.ColumnDef{{Header: "SCOPE", Field: "scope"}}, opts)
}
