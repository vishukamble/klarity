package diagnosis

import (
	"fmt"
)

// PVCClassifier finds PersistentVolumeClaims stuck in Pending.
type PVCClassifier struct{}

func (PVCClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, p := range results.PVCs {
		sc := p.StorageClass
		if sc == "" {
			sc = "<none>"
		}
		cap := p.Capacity
		if cap == "" {
			cap = "<unknown>"
		}

		oneLiner := fmt.Sprintf("PVC %s pending (%s, class: %s)", p.PVCName, cap, sc)

		findings = append(findings, Finding{
			Category:   CategoryPVCPending,
			Severity:   SeverityWarning,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  p.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"storage_class": sc,
				"capacity":      cap,
			},
		})
	}
	return findings
}
