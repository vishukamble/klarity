package output

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/vishukamble/klarity/pkg/config"
)

// RenderEnvTable renders a lipgloss table showing environments and their
// clusters. noColor should be true when stdout is not a TTY; the caller is
// responsible for detecting this (e.g. via golang.org/x/term).
//
// Columns: Environment | Tier | Clusters | Context Names
// Critical-tier rows have their Environment and Context Names cells coloured
// red (lipgloss.Color("9")) unless noColor is true.
func RenderEnvTable(envs []config.Environment, noColor bool) string {
	if len(envs) == 0 {
		return "No environments configured."
	}

	criticalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	headerStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	baseStyle := lipgloss.NewStyle().Padding(0, 1)

	// Pre-compute row data and which rows are critical.
	type envRow struct {
		cells    []string
		critical bool
	}
	rows := make([]envRow, 0, len(envs))
	for _, env := range envs {
		ctxNames := make([]string, len(env.Clusters))
		for i, cl := range env.Clusters {
			ctxNames[i] = cl.Context
		}
		tierStr := "standard"
		isCritical := env.Tier == config.TierCritical
		if isCritical {
			tierStr = "critical"
		}
		rows = append(rows, envRow{
			cells:    []string{env.Name, tierStr, fmt.Sprintf("%d", len(env.Clusters)), strings.Join(ctxNames, "\n")},
			critical: isCritical,
		})
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			if !noColor && rows[row].critical && (col == 0 || col == 3) {
				return criticalStyle.Padding(0, 1)
			}
			return baseStyle
		}).
		Headers("Environment", "Tier", "Clusters", "Context Names")

	for _, r := range rows {
		t.Row(r.cells...)
	}

	return t.Render()
}
