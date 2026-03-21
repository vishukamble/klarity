package diagnosis

import (
	"fmt"
)

// EventClassifier converts warning events into findings.
type EventClassifier struct{}

func (EventClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, ev := range results.Events {
		msg := ev.Message
		prefix := ev.Reason + ": "
		maxLen := 120
		if len(prefix)+len(msg) > maxLen {
			msg = msg[:maxLen-len(prefix)-3] + "..."
		}
		oneLiner := prefix + msg

		findings = append(findings, Finding{
			Category:   CategoryWarningEvent,
			Severity:   SeverityInfo,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  ev.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"object_kind": ev.ObjectKind,
				"object_name": ev.ObjectName,
				"count":       fmt.Sprintf("%d", ev.Count),
				"reason":      ev.Reason,
			},
		})
	}
	return findings
}
