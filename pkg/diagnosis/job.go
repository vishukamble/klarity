package diagnosis

import (
	"fmt"
	"strings"
)

// JobClassifier finds Jobs with failed pods.
type JobClassifier struct{}

func (JobClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, j := range results.Jobs {
		oneLiner := fmt.Sprintf("Job %s has %d failed pod(s)", j.JobName, j.Failed)
		if len(j.Conditions) > 0 {
			oneLiner += ": " + j.Conditions[0]
		}

		condStr := strings.Join(j.Conditions, "; ")
		if condStr == "" {
			condStr = "<none>"
		}

		findings = append(findings, Finding{
			Category:   CategoryJobFailed,
			Severity:   SeverityWarning,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  j.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"failed":     fmt.Sprintf("%d", j.Failed),
				"conditions": condStr,
			},
		})
	}
	return findings
}
