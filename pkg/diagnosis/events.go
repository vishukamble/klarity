package diagnosis

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vishukamble/klarity/pkg/kube"
)

// objKey groups events by namespace + object name.
type objKey struct{ ns, name string }

// groupEventsByObject collects all events per (namespace, objectName),
// preserving first-seen order of each object.
func groupEventsByObject(events []kube.EventIssue) ([]objKey, map[objKey][]kube.EventIssue) {
	grouped := make(map[objKey][]kube.EventIssue)
	var order []objKey
	seen := make(map[objKey]bool)
	for _, ev := range events {
		k := objKey{ev.Namespace, ev.ObjectName}
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], ev)
	}
	return order, grouped
}

// diagnosticSignals are substrings that indicate a message carries real
// diagnostic detail (as opposed to generic "ImagePullBackOff" or "ErrImagePull").
var diagnosticSignals = []string{
	"manifest unknown", "not found", "notfound",
	"401", "unauthorized", "no basic auth credentials",
	"403", "forbidden",
	"toomanyrequests", "rate limit",
	"timeout", "i/o timeout", "connection refused",
}

// messageHasSignal returns true if the message contains actionable diagnostic content.
func messageHasSignal(message string) bool {
	lower := strings.ToLower(message)
	for _, sig := range diagnosticSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// eventImagePullReasons are event reasons that may indicate an image pull problem.
var eventImagePullReasons = map[string]bool{
	"ErrImagePull": true,
	"Failed":       true,
	"BackOff":      true,
}

// isImagePullGroup returns true if any event in the group is image-pull related.
func isImagePullGroup(events []kube.EventIssue) bool {
	for _, ev := range events {
		if !eventImagePullReasons[ev.Reason] {
			continue
		}
		lower := strings.ToLower(ev.Message)
		if strings.Contains(lower, "pulling image") ||
			strings.Contains(lower, "pull image") ||
			strings.Contains(lower, "imagepullbackoff") ||
			strings.Contains(lower, "errimagepull") {
			return true
		}
	}
	return false
}

// classifyImagePullGroup scans ALL events for an object to produce the
// best image-pull classification. It searches every event for an image
// name and diagnostic signals rather than picking a single "best" event.
func classifyImagePullGroup(events []kube.EventIssue, objectName string) string {
	// Step 1: Extract image name from ANY event (pulling image "..." pattern).
	var image string
	for _, ev := range events {
		if img := extractImageFromMessage(ev.Message); img != "" {
			image = img
			break
		}
	}

	// Step 2: Look for diagnostic signal in ANY event message.
	for _, ev := range events {
		if messageHasSignal(ev.Message) {
			// Found detail — classify that message with the image we found.
			return classifyEventMessage(ev.Message, image)
		}
	}

	// Step 3: No signal found. Use tag heuristics if we have an image.
	if image != "" {
		return guessImagePullCause(image, false)
	}

	// Step 4: No image, no signal — actionable fallback.
	return fmt.Sprintf("Pull failing — run: kubectl describe pod %s for details", objectName)
}

// bestEventForObject picks the most informative non-image-pull event from
// a group. Used for events that aren't handled by classifyImagePullGroup.
func bestEventForObject(events []kube.EventIssue) kube.EventIssue {
	for _, ev := range events {
		if messageHasSignal(ev.Message) {
			return ev
		}
	}
	return events[0]
}

// imageRe extracts image name from messages like:
// Failed to pull image "nginx:1.25": ...
// Back-off pulling image "acr.io/myapp:v1.2"
var imageRe = regexp.MustCompile(`pull(?:ing)?\s+image\s+"([^"]+)"`)

// extractImageFromMessage pulls the image reference from a Kubernetes
// event message. Returns empty string if not found.
func extractImageFromMessage(message string) string {
	m := imageRe.FindStringSubmatch(message)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// pvcNameRe extracts a PVC name from event messages like:
// 'persistentvolumeclaim "my-pvc" not found'
var pvcNameRe = regexp.MustCompile(`persistentvolumeclaim\s+"([^"]+)"`)

// extractTag returns the tag portion of an image reference (after the last colon).
// If no colon is present, returns empty string.
func extractTag(image string) string {
	// Handle images with port like registry:5000/app:v1
	// The tag is after the last colon that comes after the last slash.
	lastSlash := strings.LastIndex(image, "/")
	tagPart := image
	if lastSlash >= 0 {
		tagPart = image[lastSlash:]
	}
	idx := strings.LastIndex(tagPart, ":")
	if idx < 0 {
		return ""
	}
	return tagPart[idx+1:]
}

// nonsenseTags are substrings that indicate a clearly invalid/test tag.
var nonsenseTags = []string{
	"doesnotexist", "notaversion", "thisdoes", "fake", "test123",
}

// wellKnownTags are tags that look intentional and valid.
var wellKnownTags = map[string]bool{
	"latest": true, "alpine": true, "slim": true, "stable": true, "lts": true,
}

// semverish returns true if the tag looks like a version number (digits, dots, optional v prefix).
var semverishRe = regexp.MustCompile(`^v?\d+(\.\d+)*(-[\w.]+)?$`)

// guessImagePullCause uses heuristics on the image tag string to produce
// a human-readable cause when no detailed ErrImagePull event is available.
// hasDetailEvent should be true when the event group contains a message
// with real diagnostic signal (manifest unknown, 401, etc.).
func guessImagePullCause(image string, hasDetailEvent bool) string {
	if hasDetailEvent {
		// Caller should use classifyEventMessage with the actual detail instead.
		return ""
	}

	tag := extractTag(image)
	lower := strings.ToLower(tag)

	// 1. Nonsense tag patterns
	for _, pat := range nonsenseTags {
		if strings.Contains(lower, pat) {
			return fmt.Sprintf("Likely bad tag: %s — tag looks invalid", image)
		}
	}

	// 2. Typo of "latest" (Levenshtein distance ≤ 2)
	if tag != "" && tag != "latest" && levenshtein(strings.ToLower(tag), "latest") <= 2 {
		return fmt.Sprintf("Likely typo: %s — did you mean 'latest'?", image)
	}

	// 3. Real version or well-known tag
	if wellKnownTags[lower] || semverishRe.MatchString(tag) {
		return fmt.Sprintf("Registry unreachable: %s — original error expired (>1h). Run: docker pull %s to diagnose", image, image)
	}

	// 4. Fallback
	return fmt.Sprintf("Pull failed: %s — original error expired. Run: docker pull %s to diagnose", image, image)
}

// classifyEventMessage maps a raw Kubernetes event message to a
// human-readable cause string. When image is non-empty it is included
// in image-pull-related messages.
func classifyEventMessage(message, image string) string {
	lower := strings.ToLower(message)

	// PVC not found (check before generic "not found")
	if strings.Contains(lower, "persistentvolumeclaim") && strings.Contains(lower, "not found") {
		if m := pvcNameRe.FindStringSubmatch(lower); len(m) > 1 {
			return fmt.Sprintf("PVC %q not found — check volumeClaimName for typos", m[1])
		}
		return "PVC not found — check volumeClaimName for typos"
	}

	// Image tag / manifest not found
	if strings.Contains(lower, "manifest unknown") || strings.Contains(lower, "not found") || strings.Contains(lower, "notfound") {
		if image != "" {
			return fmt.Sprintf("Tag not found: %s — verify tag exists", image)
		}
		return "Tag not found — verify image tag exists in registry"
	}

	// Registry auth
	if strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "no basic auth credentials") {
		if image != "" {
			return fmt.Sprintf("Auth failed: %s — check imagePullSecret", image)
		}
		return "Registry auth failed — check imagePullSecret is attached"
	}

	// Registry forbidden
	if strings.Contains(lower, "403") || strings.Contains(lower, "forbidden") {
		if image != "" {
			return fmt.Sprintf("Access denied: %s — check repository permissions", image)
		}
		return "Registry access denied — check repository permissions"
	}

	// Rate limit
	if strings.Contains(lower, "toomanyrequests") || strings.Contains(lower, "rate limit") {
		if image != "" {
			return fmt.Sprintf("Rate limited: %s — add authenticated pull secret", image)
		}
		return "Docker Hub rate limit — add authenticated pull secret"
	}

	// Network / timeout
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "connection refused") {
		if image != "" {
			return fmt.Sprintf("Registry unreachable: %s — run: docker pull %s", image, image)
		}
		return "Registry unreachable — check network/firewall"
	}

	// Insufficient memory
	if strings.Contains(lower, "insufficient memory") {
		return "No nodes with enough memory — lower requests or scale nodes"
	}

	// Insufficient cpu
	if strings.Contains(lower, "insufficient cpu") {
		return "No nodes with enough CPU — lower requests or scale nodes"
	}

	// Taint mismatch
	if strings.Contains(lower, "untolerated taint") {
		return "Node taint mismatch — pod needs a matching toleration"
	}

	// ImagePullBackOff alone (no diagnostic signal) — use tag heuristics
	if strings.Contains(lower, "imagepullbackoff") || strings.Contains(lower, "errimagepull") {
		if image != "" {
			return guessImagePullCause(image, false)
		}
		return "Pull failing — check pod events for details"
	}

	// Fallback: return message as-is (no truncation — let the terminal wrap).
	return message
}

// ── BackOff reason classifier ────────────────────────────────────────────────

// podNameFromBackoffMsg extracts a pod name from messages like:
// "Back-off restarting failed container app in pod probe-fail-test-67774765c8-9mxzk_default(...)"
var backoffPodRe = regexp.MustCompile(`in pod\s+(\S+?)(?:_|\(|$)`)

// classifyBackOff classifies a BackOff event based on message content.
// BackOff events can represent either CrashLoopBackOff or ImagePullBackOff.
func classifyBackOff(message, image string) string {
	lower := strings.ToLower(message)

	// CrashLoopBackOff: "restarting failed container"
	if strings.Contains(lower, "restarting failed container") {
		if m := backoffPodRe.FindStringSubmatch(message); len(m) > 1 {
			return fmt.Sprintf("Container restarting — pod is in CrashLoopBackOff, check logs: kubectl logs %s --previous", m[1])
		}
		return "Container restarting — pod is in CrashLoopBackOff, check logs with --previous"
	}

	// ImagePullBackOff: "pulling image"
	if strings.Contains(lower, "pulling image") {
		if image != "" {
			return guessImagePullCause(image, false)
		}
		return "Pull failing — check pod events for details"
	}

	// Fallback: show the message without truncation.
	return "Back-off: " + message
}

// EventClassifier converts warning events into findings.
// Events are grouped per object; the most diagnostic event is selected
// and its message is classified into a human-readable cause.
type EventClassifier struct{}

func (EventClassifier) Classify(results ScanResults) []Finding {
	order, grouped := groupEventsByObject(results.Events)

	var findings []Finding
	for _, k := range order {
		events := grouped[k]

		var why string
		var best kube.EventIssue

		// Image-pull events get special multi-event handling.
		if isImagePullGroup(events) {
			why = classifyImagePullGroup(events, k.name)
			best = bestEventForObject(events)
		} else {
			best = bestEventForObject(events)

			// Reason-based dispatch for specialized classifiers.
			switch best.Reason {
			case "FailedMount":
				why = classifyMountError(best.Message)
			case "Unhealthy":
				why = classifyProbeFailure(best.Message)
			case "FailedCreate":
				why = classifyFailedCreate(best.Message)
			case "FailedCreatePodSandBox":
				why = classifySandboxError(best.Message)
			case "Evicted":
				why = classifyEviction(best.Message)
			case "BackOff":
				why = classifyBackOff(best.Message, extractImageFromMessage(best.Message))
			default:
				why = classifyEventMessage(best.Message, extractImageFromMessage(best.Message))
			}
		}

		findings = append(findings, Finding{
			Category:   CategoryWarningEvent,
			Severity:   SeverityInfo,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  best.Namespace,
			OneLiner:   why,
			DetailFields: map[string]string{
				"object_kind": best.ObjectKind,
				"object_name": best.ObjectName,
				"count":       fmt.Sprintf("%d", best.Count),
				"reason":      best.Reason,
			},
		})
	}
	return findings
}
