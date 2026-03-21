package diagnosis

import (
	"fmt"
)

// DaemonSetClassifier finds DaemonSets that are degraded (ready < desired or misscheduled > 0).
type DaemonSetClassifier struct{}

func (DaemonSetClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, ds := range results.DaemonSets {
		oneLiner := fmt.Sprintf("DaemonSet %s %d/%d ready", ds.DaemonSetName, ds.Ready, ds.Desired)
		if ds.Misscheduled > 0 {
			oneLiner += fmt.Sprintf(" (%d misscheduled)", ds.Misscheduled)
		}

		findings = append(findings, Finding{
			Category:   CategoryDaemonSetDegraded,
			Severity:   SeverityWarning,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  ds.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"desired":      fmt.Sprintf("%d", ds.Desired),
				"ready":        fmt.Sprintf("%d", ds.Ready),
				"misscheduled": fmt.Sprintf("%d", ds.Misscheduled),
			},
		})
	}
	return findings
}
