package diagnosis

import (
	"fmt"
)

// CrashLoopClassifier finds containers in CrashLoopBackOff.
// It reads LogSummary (populated by the scan loop after FEAT-18/19) for the OneLiner;
// falls back to a generic message when logs are not yet available.
type CrashLoopClassifier struct{}

func (CrashLoopClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, p := range results.Pods {
		if p.Reason != "CrashLoopBackOff" {
			continue
		}
		oneLiner := p.LogSummary
		if oneLiner == "" {
			oneLiner = fmt.Sprintf("Container %s crash-looping (restarts: %d)", p.ContainerName, p.RestartCount)
		}

		detail := map[string]string{
			"image":         p.Image,
			"restart_count": fmt.Sprintf("%d", p.RestartCount),
		}
		if p.Message != "" {
			detail["message"] = p.Message
		}
		if p.LogSummary != "" {
			detail["log_summary"] = p.LogSummary
		}

		findings = append(findings, Finding{
			Category:      CategoryCrashLoop,
			Severity:      SeverityCritical,
			EnvName:       results.EnvName,
			ClusterCtx:    results.ClusterCtx,
			Namespace:     p.Namespace,
			PodName:       p.PodName,
			ContainerName: p.ContainerName,
			OneLiner:      oneLiner,
			DetailFields:  detail,
		})
	}
	return findings
}
