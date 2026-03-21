// Package output handles all terminal and JSON rendering for klarity.
// This is the ONLY package that may call lipgloss or produce ANSI codes.
// All other packages return structured data.
package output

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// ── Palette ───────────────────────────────────────────────────────────────────

var (
	red    = lipgloss.Color("9")  // bright red   — critical
	yellow = lipgloss.Color("11") // bright yellow — standard / warning
	green  = lipgloss.Color("10") // bright green  — dev / info
	dim    = lipgloss.Color("240")
	white  = lipgloss.Color("15")
)

// ── Env / tier styles ─────────────────────────────────────────────────────────

// EnvColor returns the lipgloss color for an environment based on tier and name.
// critical → red | dev-named → green | standard → yellow
func EnvColor(env config.Environment) lipgloss.Color {
	if env.Tier == config.TierCritical {
		return red
	}
	lower := strings.ToLower(env.Name)
	if strings.Contains(lower, "dev") || strings.Contains(lower, "development") {
		return green
	}
	return yellow
}

// EnvEmoji returns the bullet emoji for an environment header line.
func EnvEmoji(env config.Environment) string {
	switch EnvColor(env) {
	case red:
		return "🔴"
	case green:
		return "🟢"
	default:
		return "🟡"
	}
}

// EnvHeaderStyle returns a bold lipgloss style coloured for the environment.
func EnvHeaderStyle(env config.Environment) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(EnvColor(env))
}

// ── Severity styles ───────────────────────────────────────────────────────────

// SeverityStyle returns a lipgloss style for a diagnosis severity level.
func SeverityStyle(sev diagnosis.Severity) lipgloss.Style {
	switch sev {
	case diagnosis.SeverityCritical:
		return lipgloss.NewStyle().Bold(true).Foreground(red)
	case diagnosis.SeverityWarning:
		return lipgloss.NewStyle().Foreground(yellow)
	default:
		return lipgloss.NewStyle().Foreground(green)
	}
}

// ── Table styles ──────────────────────────────────────────────────────────────

// TableHeaderStyle returns the lipgloss style used for table header cells.
var TableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(white)

// TableBorderStyle returns the style for table border characters.
var TableBorderStyle = lipgloss.NewStyle().Foreground(dim)

// TableCellStyle is the base style for data cells.
var TableCellStyle = lipgloss.NewStyle().Padding(0, 1)

// ── Misc ──────────────────────────────────────────────────────────────────────

// DimStyle renders text in a muted colour (used for "no issues" messages).
var DimStyle = lipgloss.NewStyle().Foreground(dim)

// BoldStyle renders text in bold white.
var BoldStyle = lipgloss.NewStyle().Bold(true).Foreground(white)
