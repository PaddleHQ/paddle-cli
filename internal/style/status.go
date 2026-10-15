package style

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// statusFamily classifies a state value for color selection.
type statusFamily int

const (
	statusPositive statusFamily = iota
	statusPending
	statusNegative
)

// statusColours maps the state values rendered today to their family.
// Derived by sweeping enum: blocks under status / state / *_status / *_state
// properties in the Paddle API OpenAPI spec.
// Lookups are case-insensitive; keys are lowercase. Both spellings of
// "cancel(l)ed" appear in different Paddle resources, hence both entries.
var statusColours = map[string]statusFamily{
	// Positive / live
	"active":     statusPositive,
	"approved":   statusPositive,
	"authorized": statusPositive,
	"available":  statusPositive,
	"captured":   statusPositive,
	"completed":  statusPositive,
	"delivered":  statusPositive,
	"funds_sent": statusPositive,
	"granted":    statusPositive,
	"paid":       statusPositive,
	"ready":      statusPositive,
	"reviewed":   statusPositive,
	"success":    statusPositive,
	"succeeded":  statusPositive,
	"verified":   statusPositive,
	"won":        statusPositive,
	// Pending / transitional
	"billed":                     statusPending,
	"created":                    statusPending,
	"draft":                      statusPending,
	"dropped":                    statusPending,
	"in_progress":                statusPending,
	"in_review":                  statusPending,
	"migrated":                   statusPending,
	"needs_retry":                statusPending,
	"not_attempted":              statusPending,
	"not_started":                statusPending,
	"past_due":                   statusPending,
	"pending":                    statusPending,
	"pending_acceptance":         statusPending,
	"pending_approval":           statusPending,
	"pending_classification":     statusPending,
	"pending_no_action_required": statusPending,
	"pending_review":             statusPending,
	"running":                    statusPending,
	"selected":                   statusPending,
	"submitted":                  statusPending,
	"submitted_for_review":       statusPending,
	"trialing":                   statusPending,
	// Negative / terminal
	"aborted":          statusNegative,
	"archived":         statusNegative,
	"canceled":         statusNegative,
	"cancelled":        statusNegative, //nolint:misspell // an internal API enum value
	"deactivated":      statusNegative,
	"disputed":         statusNegative,
	"error":            statusNegative,
	"expired":          statusNegative,
	"failed":           statusNegative,
	"failed_payment":   statusNegative,
	"full_refund":      statusNegative,
	"funds_rejected":   statusNegative,
	"inactive":         statusNegative,
	"invalid":          statusNegative,
	"lost":             statusNegative,
	"not_ready":        statusNegative,
	"overdue":          statusNegative,
	"paused":           statusNegative,
	"refund_requested": statusNegative,
	"rejected":         statusNegative,
	"revoked":          statusNegative,
	"unpaid":           statusNegative,
	"unpaid_invoice":   statusNegative,
	"voided":           statusNegative,
	"withdrawn":        statusNegative,
}

// Status renders a status/state column value in the color appropriate
// for its family. Unknown values pass through unchanged so we never
// guess wrong.
func (p Profile) Status(value string) string {
	family, ok := statusColours[strings.ToLower(value)]
	if !ok || !p.enabled {
		return value
	}
	var color string
	switch family {
	case statusPositive:
		color = ansiGreen
	case statusPending:
		color = ansiYellow
	case statusNegative:
		color = ansiRed
	}
	return p.style().Foreground(lipgloss.Color(color)).Render(value)
}
