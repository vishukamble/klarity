package diagnosis

import (
	"fmt"
	"time"
)

// CronJobClassifier finds CronJobs that are suspended.
type CronJobClassifier struct{}

func (CronJobClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, cj := range results.CronJobs {
		oneLiner := fmt.Sprintf("CronJob %s suspended (schedule: %s)", cj.CronJobName, cj.Schedule)

		lastSched := "never"
		if cj.LastSchedule != nil {
			lastSched = cj.LastSchedule.Format(time.RFC3339)
		}

		findings = append(findings, Finding{
			Category:   CategoryCronJobSuspended,
			Severity:   SeverityInfo,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  cj.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"schedule":      cj.Schedule,
				"last_schedule": lastSched,
			},
		})
	}
	return findings
}
