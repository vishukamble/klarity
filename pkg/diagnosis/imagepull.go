package diagnosis

import (
	"fmt"
	"strings"
)

// imagePullSubtype labels the root cause of an image pull failure.
type imagePullSubtype string

const (
	imagePullAuth     imagePullSubtype = "auth_error"      // 401/403 — credential issue
	imagePullTag      imagePullSubtype = "tag_not_found"   // manifest unknown — bad tag
	imagePullRegistry imagePullSubtype = "registry_unreachable" // timeout/connection refused
	imagePullUnknown  imagePullSubtype = "unknown"
)

var imagePullReasons = map[string]bool{
	"ImagePullBackOff": true,
	"ErrImagePull":     true,
	// Note: InvalidImageName is intentionally excluded — it is handled by
	// ContainerErrorClassifier which provides a more specific diagnosis via
	// classifyImageNameError(). Including it here would produce duplicate findings.
}

func classifyImagePullMessage(msg string) imagePullSubtype {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "401") ||
		strings.Contains(lower, "403") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "authentication"):
		return imagePullAuth
	case strings.Contains(lower, "manifest unknown") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "tag does not exist") ||
		strings.Contains(lower, "no such image"):
		return imagePullTag
	case strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no route to host") ||
		strings.Contains(lower, "dial") ||
		strings.Contains(lower, "i/o timeout"):
		return imagePullRegistry
	default:
		return imagePullUnknown
	}
}

// ImagePullClassifier finds containers stuck on image pull failures.
type ImagePullClassifier struct{}

func (ImagePullClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, p := range results.Pods {
		if !imagePullReasons[p.Reason] {
			continue
		}
		subtype := classifyImagePullMessage(p.Message)
		oneLiner := fmt.Sprintf("Image pull failed for %s (%s: %s)", p.Image, p.Reason, subtype)

		detail := map[string]string{
			"image":         p.Image,
			"reason":        p.Reason,
			"subtype":       string(subtype),
			"restart_count": fmt.Sprintf("%d", p.RestartCount),
		}
		if p.Message != "" {
			detail["message"] = p.Message
		}

		findings = append(findings, Finding{
			Category:      CategoryImagePull,
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
