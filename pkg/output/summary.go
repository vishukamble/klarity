package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)


// renderFooter writes the summary line.
// envTotals maps env name → finding count.
func renderFooter(w io.Writer, cfg *config.Config, envTotals map[string]int, startTime time.Time) {
	sep := strings.Repeat("━", sepWidth)
	fmt.Fprintln(w, DimStyle.Render(sep))

	// Build "X issues in prod | Y in staging | 0 in dev" line.
	parts := make([]string, 0, len(cfg.Environments))
	for _, env := range sortedEnvs(cfg) {
		n := envTotals[env.Name]
		parts = append(parts, fmt.Sprintf("%d in %s", n, env.Name))
	}
	summary := "Summary: " + strings.Join(parts, " | ")
	fmt.Fprintln(w, BoldStyle.Render(summary))

	fmt.Fprintln(w, DimStyle.Render(sep))
}

// SummaryCounts returns per-severity totals across all findings.
func SummaryCounts(findings []diagnosis.Finding) (critical, warning, info int) {
	for _, f := range findings {
		switch f.Severity {
		case diagnosis.SeverityCritical:
			critical++
		case diagnosis.SeverityWarning:
			warning++
		default:
			info++
		}
	}
	return
}
