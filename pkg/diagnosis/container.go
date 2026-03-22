package diagnosis

import (
	"fmt"
	"regexp"
	"strings"
)

// ── ContainerErrorClassifier ─────────────────────────────────────────────────

// ContainerErrorClassifier catches pods with container-start errors
// (CreateContainerConfigError, RunContainerError, InvalidImageName, Evicted)
// that are not covered by other classifiers.
type ContainerErrorClassifier struct{}

func (ContainerErrorClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, p := range results.Pods {
		var oneLiner string
		var sev Severity
		switch p.Reason {
		case "CreateContainerConfigError":
			oneLiner = classifyConfigError(p.Message)
			sev = SeverityWarning
		case "RunContainerError":
			oneLiner = classifyRunError(p.Message)
			sev = SeverityWarning
		case "InvalidImageName":
			oneLiner = classifyImageNameError(p.Message)
			sev = SeverityWarning
		case "Evicted":
			oneLiner = classifyEviction(p.Message)
			sev = SeverityWarning
		default:
			continue
		}

		findings = append(findings, Finding{
			Category:      CategoryWarningEvent,
			Severity:      sev,
			EnvName:       results.EnvName,
			ClusterCtx:    results.ClusterCtx,
			Namespace:     p.Namespace,
			PodName:       p.PodName,
			ContainerName: p.ContainerName,
			OneLiner:      oneLiner,
			DetailFields: map[string]string{
				"reason":      p.Reason,
				"object_name": p.PodName,
			},
		})
	}
	return findings
}

// ── FailedMount (Pattern 1) ──────────────────────────────────────────────────

var (
	quotedNameRe   = regexp.MustCompile(`"([^"]+)"`)
	cmNotFoundRe   = regexp.MustCompile(`(?i)configmap\s+"([^"]+)"\s+not\s+found`)
	secNotFoundRe  = regexp.MustCompile(`(?i)secret\s+"([^"]+)"\s+not\s+found`)
)

func classifyMountError(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "configmap") && strings.Contains(lower, "not found") {
		if m := cmNotFoundRe.FindStringSubmatch(message); len(m) > 1 {
			return fmt.Sprintf("ConfigMap '%s' not found — verify it exists in this namespace", m[1])
		}
		return "ConfigMap not found — verify it exists in this namespace"
	}

	if strings.Contains(lower, "secret") && strings.Contains(lower, "not found") {
		if m := secNotFoundRe.FindStringSubmatch(message); len(m) > 1 {
			return fmt.Sprintf("Secret '%s' not found — verify it exists in this namespace", m[1])
		}
		return "Secret not found — verify it exists in this namespace"
	}

	if (strings.Contains(lower, "mount failed") && strings.Contains(lower, "nfs")) ||
		strings.Contains(lower, "connection timed out") {
		return "NFS mount timeout — check firewall allows port 2049 to NFS server"
	}

	if (strings.Contains(lower, "rpc error") && strings.Contains(lower, "input/output error")) ||
		strings.Contains(lower, "csi") {
		return "CSI driver error — check CSI driver pod logs in kube-system"
	}

	if strings.Contains(lower, "mount.nfs") {
		return "NFS mount failed — check NFS server is reachable from node"
	}

	return "Volume mount failed — check volume definition and storage availability"
}

// ── CreateContainerConfigError (Pattern 2) ────────────────────────────────────

var (
	configMapKeyRe = regexp.MustCompile(`(?i)couldn't find key\s+"?([^"]+)"?\s+in\s+ConfigMap\s+"?([^"]+)"?`)
	secretKeyRe    = regexp.MustCompile(`(?i)couldn't find key\s+"?([^"]+)"?\s+in\s+Secret\s+"?([^"]+)"?`)
)

func classifyConfigError(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "couldn't find key") && strings.Contains(lower, "configmap") {
		if m := configMapKeyRe.FindStringSubmatch(message); len(m) == 3 {
			return fmt.Sprintf("Missing key '%s' in ConfigMap '%s' — check configMapKeyRef spelling", m[1], m[2])
		}
		return "Missing key in ConfigMap — check configMapKeyRef spelling"
	}

	if strings.Contains(lower, "couldn't find key") && strings.Contains(lower, "secret") {
		if m := secretKeyRe.FindStringSubmatch(message); len(m) == 3 {
			return fmt.Sprintf("Missing key '%s' in Secret '%s' — check secretKeyRef spelling", m[1], m[2])
		}
		return "Missing key in Secret — check secretKeyRef spelling"
	}

	if strings.Contains(lower, "valid environment variable") || strings.Contains(lower, "invalid-key") {
		return "Invalid env var name in ConfigMap/Secret key — keys must match [A-Za-z_][A-Za-z0-9_]*"
	}

	return "Container config error — verify all ConfigMap/Secret key references exist"
}

// ── RunContainerError (Pattern 3) ─────────────────────────────────────────────

var execCmdRe = regexp.MustCompile(`exec:\s+"([^"]+)"`)

func classifyRunError(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "executable file not found") {
		if m := execCmdRe.FindStringSubmatch(message); len(m) > 1 {
			return fmt.Sprintf("Command '%s' not found in container PATH — check image has this binary", m[1])
		}
		return "Command not found in container PATH — check image has this binary"
	}

	if strings.Contains(lower, "no such file or directory") {
		// Try to extract the path.
		for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
			if strings.Contains(lower, candidate) {
				return fmt.Sprintf("Binary '%s' missing — minimal images lack bash, use /bin/sh or busybox", candidate)
			}
		}
		if m := quotedNameRe.FindStringSubmatch(message); len(m) > 1 {
			return fmt.Sprintf("Binary '%s' missing — minimal images lack bash, use /bin/sh or busybox", m[1])
		}
		return "Binary missing — minimal images lack bash, use /bin/sh or busybox"
	}

	if strings.Contains(lower, "oci runtime") {
		return "Container runtime failed to start — check command/entrypoint in pod spec"
	}

	return "Container failed to start — check command and entrypoint in pod spec"
}

// ── InvalidImageName (Pattern 4) ──────────────────────────────────────────────

func classifyImageNameError(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "couldn't parse image reference") || strings.Contains(lower, "invalid reference format") {
		return "Malformed image name — check for unrendered Helm variables ({{ }}) or illegal characters"
	}

	if strings.Contains(lower, "failed to apply default image tag") {
		return "Invalid image tag format — image field contains illegal characters"
	}

	return "Invalid image name — check image field syntax"
}

// ── Evicted (Pattern 5) ───────────────────────────────────────────────────────

func classifyEviction(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "ephemeral-storage") {
		return "Evicted: ephemeral storage exceeded — add ephemeral-storage limit or reduce log/tmp usage"
	}

	if strings.Contains(lower, "memory") {
		return "Evicted: node memory pressure — add memory limits to prevent node-level OOM eviction"
	}

	if strings.Contains(lower, "diskpressure") || strings.Contains(lower, "disk pressure") {
		return "Evicted: node disk pressure — check node disk usage, clean up or expand storage"
	}

	return "Pod evicted by kubelet — check node conditions: kubectl describe node <node>"
}

// ── FailedCreate (Pattern 6) ──────────────────────────────────────────────────

var (
	quotaNameRe   = regexp.MustCompile(`(?i)exceeded quota[:\s]+"?([^",]+)"?`)
	webhookNameRe = regexp.MustCompile(`(?i)admission webhook\s+"([^"]+)"`)
	webhookMsgRe  = regexp.MustCompile(`denied the request:\s*(.+)$`)
)

func classifyFailedCreate(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "exceeded quota") {
		name := ""
		resource := ""
		if m := quotaNameRe.FindStringSubmatch(message); len(m) > 1 {
			name = m[1]
		}
		// Try to extract resource type (e.g. "cpu", "memory", "pods").
		for _, r := range []string{"cpu", "memory", "pods", "requests.cpu", "requests.memory", "limits.cpu", "limits.memory"} {
			if strings.Contains(lower, r) {
				resource = r
				break
			}
		}
		if name != "" && resource != "" {
			return fmt.Sprintf("Quota '%s' exceeded for %s — check namespace quota usage", name, resource)
		}
		if name != "" {
			return fmt.Sprintf("Quota '%s' exceeded — check namespace quota usage", name)
		}
		return "Quota exceeded — check namespace quota usage"
	}

	if strings.Contains(lower, "admission webhook") && strings.Contains(lower, "denied") {
		webhookName := ""
		if m := webhookNameRe.FindStringSubmatch(message); len(m) > 1 {
			webhookName = m[1]
		}
		reason := ""
		if m := webhookMsgRe.FindStringSubmatch(message); len(m) > 1 {
			reason = strings.TrimSpace(m[1])
		}
		if webhookName != "" && reason != "" {
			return fmt.Sprintf("Admission webhook '%s' rejected pod — %s", webhookName, reason)
		}
		if webhookName != "" {
			return fmt.Sprintf("Admission webhook '%s' rejected pod — check webhook logs for policy violation", webhookName)
		}
		return "Admission webhook rejected pod — check webhook logs for policy violation"
	}

	if strings.Contains(lower, "admission webhook") {
		return "Admission webhook rejected pod — check webhook logs for policy violation"
	}

	return "Failed to create pod — check events for details"
}

// ── Probe failures (Pattern 7) ────────────────────────────────────────────────

var statusCodeRe = regexp.MustCompile(`(?i)statuscode:\s*(\d+)`)

func classifyProbeFailure(message string) string {
	lower := strings.ToLower(message)

	// Liveness probe
	if strings.Contains(lower, "liveness probe failed") {
		if strings.Contains(lower, "statuscode: 5") || strings.Contains(lower, "statuscode:5") {
			return "Liveness probe: HTTP 5xx — app is running but unhealthy, check app logs"
		}
		if strings.Contains(lower, "connection refused") {
			return "Liveness probe: connection refused — app may have crashed or wrong port"
		}
		return "Liveness probe failed — app is unhealthy, check logs for crash or hang"
	}

	// Readiness probe
	if strings.Contains(lower, "readiness probe failed") {
		if strings.Contains(lower, "connection refused") {
			return "Readiness probe: connection refused — app not ready yet, check initialDelaySeconds"
		}
		if m := statusCodeRe.FindStringSubmatch(message); len(m) > 1 {
			return fmt.Sprintf("Readiness probe: HTTP %s — app returning error on health endpoint", m[1])
		}
		return "Readiness probe failed — pod removed from service endpoints until resolved"
	}

	// Startup probe
	if strings.Contains(lower, "startup probe failed") {
		if strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") {
			return "Startup probe timeout — app taking too long to start, increase failureThreshold"
		}
		return "Startup probe failed — app not starting properly, check logs"
	}

	return "Probe failed — check probe configuration and app health endpoint"
}

// ── FailedCreatePodSandBox (Pattern 10) ───────────────────────────────────────

func classifySandboxError(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "cgroup") {
		return "Node cgroup error — kubelet/runtime out of sync, node may need draining"
	}

	if strings.Contains(lower, "networkplugin cni failed") || strings.Contains(lower, "cni plugin") {
		if strings.Contains(lower, "failed to assign") || strings.Contains(lower, "ip address") {
			return "CNI IP exhaustion — subnet may be out of IPs, check IPAM and VPC subnet size"
		}
		return "CNI plugin error — check CNI pod logs in kube-system"
	}

	if strings.Contains(lower, "failed to start sandbox") {
		return "Pod sandbox failed — container runtime error, check kubelet logs on node"
	}

	return "Pod sandbox creation failed — check kubelet and container runtime logs"
}
