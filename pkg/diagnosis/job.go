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
		oneLiner := classifyJobFailure(j.JobName, j.Failed, j.Conditions)

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

// classifyJobFailure produces a human-readable one-liner for a failed job.
func classifyJobFailure(jobName string, failed int32, conditions []string) string {
	allConds := strings.ToLower(strings.Join(conditions, " "))

	if strings.Contains(allConds, "backofflimitexceeded") {
		return fmt.Sprintf("Job %s failed: hit retry limit (%d failures) — check logs of failed pods for root cause", jobName, failed)
	}

	if strings.Contains(allConds, "deadlineexceeded") {
		return fmt.Sprintf("Job %s timed out: exceeded activeDeadlineSeconds (%d failures) — check for performance bottleneck", jobName, failed)
	}

	oneLiner := fmt.Sprintf("Job %s has %d failed pod(s)", jobName, failed)
	if len(conditions) > 0 {
		oneLiner += ": " + conditions[0]
	}
	return oneLiner
}
