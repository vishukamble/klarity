package diagnosis

import (
	"fmt"
)

// OOMClassifier finds containers that were OOMKilled.
type OOMClassifier struct{}

func (OOMClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, p := range results.Pods {
		if p.Reason != "OOMKilled" {
			continue
		}
		oneLiner := fmt.Sprintf("Container %s OOMKilled (restarts: %d)", p.ContainerName, p.RestartCount)

		detail := map[string]string{
			"image":         p.Image,
			"restart_count": fmt.Sprintf("%d", p.RestartCount),
		}

		findings = append(findings, Finding{
			Category:      CategoryOOMKilled,
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
