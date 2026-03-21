package diagnosis

import (
	"fmt"
)

// QuotaClassifier finds resource quotas that are near or at exhaustion.
type QuotaClassifier struct{}

func (QuotaClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, q := range results.Quotas {
		severity := SeverityWarning
		if q.UsedPercent >= 95.0 {
			severity = SeverityCritical
		}

		oneLiner := fmt.Sprintf("%s quota at %.0f%% (%s/%s)", q.Resource, q.UsedPercent, q.Used, q.Hard)

		findings = append(findings, Finding{
			Category:   CategoryQuotaExhausted,
			Severity:   severity,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  q.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"quota_name":   q.QuotaName,
				"resource":     q.Resource,
				"used":         q.Used,
				"hard":         q.Hard,
				"used_percent": fmt.Sprintf("%.0f", q.UsedPercent),
			},
		})
	}
	return findings
}
