package diagnosis

import (
	"strings"
	"testing"
	"time"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestEventClassifier(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		results ScanResults
		wantLen int
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "single warning event",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				Events: []kube.EventIssue{
					{
						Namespace:    "default",
						ObjectName:   "web-abc",
						ObjectKind:   "Pod",
						Reason:       "FailedScheduling",
						Message:      "0/3 nodes are available: insufficient memory",
						Count:        5,
						LastTimestamp: now,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "deduplication groups events per object to one finding",
			results: ScanResults{
				Events: []kube.EventIssue{
					{Namespace: "ns", ObjectName: "pod-a", Reason: "BackOff", Message: `Back-off pulling image "nginx:1.25"`, Count: 3, LastTimestamp: now},
					{Namespace: "ns", ObjectName: "pod-a", Reason: "ErrImagePull", Message: `Failed to pull image "nginx:1.25": manifest unknown`, Count: 1, LastTimestamp: now},
				},
			},
			wantLen: 1,
		},
		{
			name: "different objects are not deduplicated",
			results: ScanResults{
				Events: []kube.EventIssue{
					{Namespace: "ns", ObjectName: "pod-a", Reason: "BackOff", Message: "restart", Count: 1, LastTimestamp: now},
					{Namespace: "ns", ObjectName: "pod-b", Reason: "Unhealthy", Message: "readiness probe failed", Count: 3, LastTimestamp: now},
				},
			},
			wantLen: 2,
		},
		{
			name: "same name different namespaces kept separate",
			results: ScanResults{
				Events: []kube.EventIssue{
					{Namespace: "ns1", ObjectName: "pod-a", Reason: "BackOff", Message: "restart", Count: 1, LastTimestamp: now},
					{Namespace: "ns2", ObjectName: "pod-a", Reason: "Failed", Message: "pull error", Count: 1, LastTimestamp: now},
				},
			},
			wantLen: 2,
		},
	}

	c := EventClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.results)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			f := got[0]
			if f.Category != CategoryWarningEvent {
				t.Errorf("category = %q, want %q", f.Category, CategoryWarningEvent)
			}
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityInfo)
			}
			if f.DetailFields["reason"] == "" {
				t.Error("DetailFields missing reason")
			}
			if f.DetailFields["object_name"] == "" {
				t.Error("DetailFields missing object_name")
			}
		})
	}
}

func TestEventClassifier_PicksSignalOverGeneric(t *testing.T) {
	now := time.Now()

	// The ErrImagePull event has "manifest unknown" (signal).
	// The BackOff event has only "ImagePullBackOff" (no signal).
	// Classifier should pick the ErrImagePull message for its signal.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "ns", ObjectName: "pod-x", Reason: "BackOff", Message: `Back-off pulling image "alpine:lates": ImagePullBackOff`, Count: 5, LastTimestamp: now},
			{Namespace: "ns", ObjectName: "pod-x", Reason: "ErrImagePull", Message: `Failed to pull image "alpine:lates": manifest unknown`, Count: 1, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	want := "Tag not found: alpine:lates — verify tag exists"
	if got[0].OneLiner != want {
		t.Errorf("OneLiner = %q, want %q", got[0].OneLiner, want)
	}
}

func TestEventClassifier_NoSignalFallbackWithImage(t *testing.T) {
	now := time.Now()

	// Neither event has diagnostic signal, but image is extractable.
	// nginx:1.25 looks like a real semver tag → guessImagePullCause branch 3.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "ns", ObjectName: "pod-y", Reason: "Failed", Message: `Error: ErrImagePull`, Count: 1, LastTimestamp: now},
			{Namespace: "ns", ObjectName: "pod-y", Reason: "BackOff", Message: `Back-off pulling image "nginx:1.25"`, Count: 3, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	want := "Registry unreachable: nginx:1.25 — original error expired (>1h). Run: docker pull nginx:1.25 to diagnose"
	if got[0].OneLiner != want {
		t.Errorf("OneLiner = %q, want %q", got[0].OneLiner, want)
	}
}

func TestEventClassifier_FailedThenBackOff_ImageWins(t *testing.T) {
	now := time.Now()

	// Failed event comes first with generic message (no image).
	// BackOff event comes second with extractable image.
	// Image should be extracted from BackOff via multi-event scan.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "ns", ObjectName: "pod-img", Reason: "Failed", Message: `Error: ImagePullBackOff`, Count: 1, LastTimestamp: now},
			{Namespace: "ns", ObjectName: "pod-img", Reason: "BackOff", Message: `Back-off pulling image "nginx:1.25"`, Count: 5, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if !strings.Contains(got[0].OneLiner, "nginx:1.25") {
		t.Errorf("OneLiner should contain image name, got %q", got[0].OneLiner)
	}
	if strings.Contains(got[0].OneLiner, "Pull failing") {
		t.Errorf("should NOT show generic message, got %q", got[0].OneLiner)
	}
}

func TestEventClassifier_ManyFailedFewBackOff_ImageExtracted(t *testing.T) {
	now := time.Now()

	// Regression test: 3 Failed events (generic) + 2 BackOff events (with image).
	// The multi-event scan must find the image from the BackOff events.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "payments", ObjectName: "pay-api-xxx", Reason: "Failed", Message: `Error: ImagePullBackOff`, Count: 10, LastTimestamp: now},
			{Namespace: "payments", ObjectName: "pay-api-xxx", Reason: "Failed", Message: `Error: ImagePullBackOff`, Count: 8, LastTimestamp: now},
			{Namespace: "payments", ObjectName: "pay-api-xxx", Reason: "Failed", Message: `Error: ImagePullBackOff`, Count: 5, LastTimestamp: now},
			{Namespace: "payments", ObjectName: "pay-api-xxx", Reason: "BackOff", Message: `Back-off pulling image "nginx:1.25"`, Count: 12, LastTimestamp: now},
			{Namespace: "payments", ObjectName: "pay-api-xxx", Reason: "BackOff", Message: `Back-off pulling image "nginx:1.25"`, Count: 9, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	want := "Registry unreachable: nginx:1.25 — original error expired (>1h). Run: docker pull nginx:1.25 to diagnose"
	if got[0].OneLiner != want {
		t.Errorf("OneLiner = %q, want %q", got[0].OneLiner, want)
	}
	if got[0].DetailFields["object_name"] != "pay-api-xxx" {
		t.Errorf("object_name = %q, want pay-api-xxx", got[0].DetailFields["object_name"])
	}
}

func TestEventClassifier_NoSignalNonsenseTag(t *testing.T) {
	now := time.Now()

	// No signal, image has a nonsense tag → guessImagePullCause branch 1.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "ns", ObjectName: "pod-q", Reason: "Failed", Message: `Error: ErrImagePull`, Count: 1, LastTimestamp: now},
			{Namespace: "ns", ObjectName: "pod-q", Reason: "BackOff", Message: `Back-off pulling image "nginx:doesnotexist"`, Count: 3, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	want := "Likely bad tag: nginx:doesnotexist — tag looks invalid"
	if got[0].OneLiner != want {
		t.Errorf("OneLiner = %q, want %q", got[0].OneLiner, want)
	}
}

func TestEventClassifier_NoSignalTypoTag(t *testing.T) {
	now := time.Now()

	// No signal, image tag is a typo of "latest" → guessImagePullCause branch 2.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "ns", ObjectName: "pod-r", Reason: "BackOff", Message: `Back-off pulling image "nginx:lates": ImagePullBackOff`, Count: 5, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	want := "Likely typo: nginx:lates — did you mean 'latest'?"
	if got[0].OneLiner != want {
		t.Errorf("OneLiner = %q, want %q", got[0].OneLiner, want)
	}
}

func TestEventClassifier_ImageInWhy(t *testing.T) {
	now := time.Now()

	// Single event with signal + image in message.
	results := ScanResults{
		Events: []kube.EventIssue{
			{Namespace: "ns", ObjectName: "pod-z", Reason: "Failed", Message: `Failed to pull image "acr.io/myapp:v1.2": 401 Unauthorized`, Count: 1, LastTimestamp: now},
		},
	}
	c := EventClassifier{}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	want := "Auth failed: acr.io/myapp:v1.2 — check imagePullSecret"
	if got[0].OneLiner != want {
		t.Errorf("OneLiner = %q, want %q", got[0].OneLiner, want)
	}
}

func TestExtractImageFromMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "pull image with tag",
			message: `Failed to pull image "nginx:1.25": manifest unknown`,
			want:    "nginx:1.25",
		},
		{
			name:    "pulling image variant",
			message: `Back-off pulling image "acr.io/myapp:v1.2"`,
			want:    "acr.io/myapp:v1.2",
		},
		{
			name:    "no image in message",
			message: "0/3 nodes are available: insufficient memory",
			want:    "",
		},
		{
			name:    "malformed no closing quote",
			message: `Failed to pull image "nginx:latest`,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractImageFromMessage(tt.message)
			if got != tt.want {
				t.Errorf("extractImageFromMessage(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestClassifyEventMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		image   string
		want    string
	}{
		{
			name:    "manifest unknown with image",
			message: `Failed to pull image "alpine:lates": manifest unknown`,
			image:   "alpine:lates",
			want:    "Tag not found: alpine:lates — verify tag exists",
		},
		{
			name:    "manifest unknown without image",
			message: "manifest unknown for some reason",
			image:   "",
			want:    "Tag not found — verify image tag exists in registry",
		},
		{
			name:    "not found with image",
			message: `rpc error: code = NotFound desc = image not found`,
			image:   "myimg:v2",
			want:    "Tag not found: myimg:v2 — verify tag exists",
		},
		{
			name:    "401 unauthorized with image",
			message: `Failed to pull image "acr.io/myapp:v1.2": 401 Unauthorized`,
			image:   "acr.io/myapp:v1.2",
			want:    "Auth failed: acr.io/myapp:v1.2 — check imagePullSecret",
		},
		{
			name:    "no basic auth credentials no image",
			message: "no basic auth credentials",
			image:   "",
			want:    "Registry auth failed — check imagePullSecret is attached",
		},
		{
			name:    "403 forbidden with image",
			message: `Failed to pull image "gcr.io/proj/app:v3": 403 Forbidden`,
			image:   "gcr.io/proj/app:v3",
			want:    "Access denied: gcr.io/proj/app:v3 — check repository permissions",
		},
		{
			name:    "403 forbidden no image",
			message: "403 Forbidden accessing registry",
			image:   "",
			want:    "Registry access denied — check repository permissions",
		},
		{
			name:    "rate limit with image",
			message: "toomanyrequests: rate limit exceeded",
			image:   "nginx:latest",
			want:    "Rate limited: nginx:latest — add authenticated pull secret",
		},
		{
			name:    "rate limit no image",
			message: "toomanyrequests: You have reached your pull rate limit",
			image:   "",
			want:    "Docker Hub rate limit — add authenticated pull secret",
		},
		{
			name:    "timeout with image",
			message: "i/o timeout connecting to registry.example.com",
			image:   "nginx:1.25",
			want:    "Registry unreachable: nginx:1.25 — run: docker pull nginx:1.25",
		},
		{
			name:    "connection refused no image",
			message: "dial tcp: connection refused",
			image:   "",
			want:    "Registry unreachable — check network/firewall",
		},
		{
			name:    "PVC not found with name",
			message: `persistentvolumeclaim "data-vol" not found`,
			image:   "",
			want:    `PVC "data-vol" not found — check volumeClaimName for typos`,
		},
		{
			name:    "PVC not found without quotes",
			message: "persistentvolumeclaim not found in namespace",
			image:   "",
			want:    "PVC not found — check volumeClaimName for typos",
		},
		{
			name:    "insufficient memory",
			message: "0/3 nodes are available: Insufficient memory.",
			image:   "",
			want:    "No nodes with enough memory — lower requests or scale nodes",
		},
		{
			name:    "insufficient cpu",
			message: "0/3 nodes are available: Insufficient cpu.",
			image:   "",
			want:    "No nodes with enough CPU — lower requests or scale nodes",
		},
		{
			name:    "untolerated taint",
			message: "0/3 nodes are available: 3 node(s) had untolerated taint {node-role.kubernetes.io/master: }",
			image:   "",
			want:    "Node taint mismatch — pod needs a matching toleration",
		},
		{
			name:    "ImagePullBackOff with semver image",
			message: `Back-off pulling image "nginx:1.25": ImagePullBackOff`,
			image:   "nginx:1.25",
			want:    "Registry unreachable: nginx:1.25 — original error expired (>1h). Run: docker pull nginx:1.25 to diagnose",
		},
		{
			name:    "ImagePullBackOff no image",
			message: "ImagePullBackOff",
			image:   "",
			want:    "Pull failing — check pod events for details",
		},
		{
			name:    "fallback short message",
			message: "some unknown reason",
			image:   "",
			want:    "some unknown reason",
		},
		{
			name:    "fallback long message not truncated",
			message: "This is a very long event message that exceeds eighty characters in length and should not be truncated by the fallback case",
			image:   "",
			want:    "This is a very long event message that exceeds eighty characters in length and should not be truncated by the fallback case",
		},
		{
			name:    "UpdateFailed key with err reason",
			message: `error processing spec.data[0] (key: ask-imanage/dev/secret), err: rpc error: code = PermissionDenied`,
			image:   "",
			want:    "key ask-imanage/dev/secret — rpc error: code = PermissionDenied",
		},
		{
			name:    "UpdateFailed key without err",
			message: `error processing spec.data[0] (key: vault/prod/db-password)`,
			image:   "",
			want:    "key vault/prod/db-password — check value or access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyEventMessage(tt.message, tt.image)
			if got != tt.want {
				t.Errorf("classifyEventMessage(%q, %q)\n  got:  %q\n  want: %q", tt.message, tt.image, got, tt.want)
			}
		})
	}
}

func TestGuessImagePullCause(t *testing.T) {
	tests := []struct {
		name           string
		image          string
		hasDetailEvent bool
		want           string
	}{
		// hasDetailEvent=true returns empty (caller should use classifyEventMessage)
		{
			name:           "has detail event returns empty",
			image:          "nginx:1.25",
			hasDetailEvent: true,
			want:           "",
		},
		// Branch 1: nonsense tags
		{
			name:           "nonsense tag doesnotexist",
			image:          "myapp:doesnotexist",
			hasDetailEvent: false,
			want:           "Likely bad tag: myapp:doesnotexist — tag looks invalid",
		},
		{
			name:           "nonsense tag fake",
			image:          "registry.io/app:fake",
			hasDetailEvent: false,
			want:           "Likely bad tag: registry.io/app:fake — tag looks invalid",
		},
		{
			name:           "nonsense tag notaversion",
			image:          "nginx:notaversion",
			hasDetailEvent: false,
			want:           "Likely bad tag: nginx:notaversion — tag looks invalid",
		},
		{
			name:           "nonsense tag test123",
			image:          "nginx:test123",
			hasDetailEvent: false,
			want:           "Likely bad tag: nginx:test123 — tag looks invalid",
		},
		// Branch 2: typo of "latest"
		{
			name:           "typo lates",
			image:          "nginx:lates",
			hasDetailEvent: false,
			want:           "Likely typo: nginx:lates — did you mean 'latest'?",
		},
		{
			name:           "typo lastest",
			image:          "alpine:lastest",
			hasDetailEvent: false,
			want:           "Likely typo: alpine:lastest — did you mean 'latest'?",
		},
		{
			name:           "typo latets",
			image:          "busybox:latets",
			hasDetailEvent: false,
			want:           "Likely typo: busybox:latets — did you mean 'latest'?",
		},
		{
			name:           "typo ltest",
			image:          "myapp:ltest",
			hasDetailEvent: false,
			want:           "Likely typo: myapp:ltest — did you mean 'latest'?",
		},
		// Branch 3: semver / well-known tags
		{
			name:           "semver tag",
			image:          "nginx:1.25",
			hasDetailEvent: false,
			want:           "Registry unreachable: nginx:1.25 — original error expired (>1h). Run: docker pull nginx:1.25 to diagnose",
		},
		{
			name:           "semver with v prefix",
			image:          "myapp:v2.3.1",
			hasDetailEvent: false,
			want:           "Registry unreachable: myapp:v2.3.1 — original error expired (>1h). Run: docker pull myapp:v2.3.1 to diagnose",
		},
		{
			name:           "well-known tag latest",
			image:          "nginx:latest",
			hasDetailEvent: false,
			want:           "Registry unreachable: nginx:latest — original error expired (>1h). Run: docker pull nginx:latest to diagnose",
		},
		{
			name:           "well-known tag alpine",
			image:          "node:alpine",
			hasDetailEvent: false,
			want:           "Registry unreachable: node:alpine — original error expired (>1h). Run: docker pull node:alpine to diagnose",
		},
		{
			name:           "well-known tag slim",
			image:          "python:slim",
			hasDetailEvent: false,
			want:           "Registry unreachable: python:slim — original error expired (>1h). Run: docker pull python:slim to diagnose",
		},
		// Branch 4: fallback (unknown tag shape)
		{
			name:           "fallback unknown tag",
			image:          "myapp:custom-build-abc",
			hasDetailEvent: false,
			want:           "Pull failed: myapp:custom-build-abc — original error expired. Run: docker pull myapp:custom-build-abc to diagnose",
		},
		{
			name:           "fallback no tag",
			image:          "nginx",
			hasDetailEvent: false,
			want:           "Pull failed: nginx — original error expired. Run: docker pull nginx to diagnose",
		},
		// Registry with port — tag should still parse correctly
		{
			name:           "registry with port semver",
			image:          "registry:5000/myapp:v1.0",
			hasDetailEvent: false,
			want:           "Registry unreachable: registry:5000/myapp:v1.0 — original error expired (>1h). Run: docker pull registry:5000/myapp:v1.0 to diagnose",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessImagePullCause(tt.image, tt.hasDetailEvent)
			if got != tt.want {
				t.Errorf("guessImagePullCause(%q, %v)\n  got:  %q\n  want: %q", tt.image, tt.hasDetailEvent, got, tt.want)
			}
		})
	}
}

// ── Reason-based dispatch tests ──────────────────────────────────────────────

func TestEventClassifier_ReasonDispatch(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		reason   string
		message  string
		wantPart string
	}{
		{
			name:     "FailedMount dispatches to classifyMountError",
			reason:   "FailedMount",
			message:  `MountVolume.SetUp failed for volume "cfg" : configmap "missing-cm" not found`,
			wantPart: "ConfigMap 'missing-cm' not found",
		},
		{
			name:     "Unhealthy dispatches to classifyProbeFailure",
			reason:   "Unhealthy",
			message:  `Liveness probe failed: HTTP probe failed with statuscode: 500`,
			wantPart: "Liveness probe: HTTP 5xx",
		},
		{
			name:     "FailedCreate dispatches to classifyFailedCreate",
			reason:   "FailedCreate",
			message:  `exceeded quota: "compute-quota", requested: cpu=2`,
			wantPart: "Quota 'compute-quota' exceeded",
		},
		{
			name:     "FailedCreatePodSandBox dispatches to classifySandboxError",
			reason:   "FailedCreatePodSandBox",
			message:  `networkPlugin cni failed to set up pod network: failed to assign an IP address`,
			wantPart: "CNI IP exhaustion",
		},
		{
			name:     "Evicted dispatches to classifyEviction",
			reason:   "Evicted",
			message:  `The node was low on resource: ephemeral-storage.`,
			wantPart: "ephemeral storage exceeded",
		},
		{
			name:     "BackOff restarting dispatches to classifyBackOff",
			reason:   "BackOff",
			message:  `Back-off restarting failed container app in pod probe-fail-test-67774765c8-9mxzk_default(abc123)`,
			wantPart: "CrashLoopBackOff",
		},
		{
			name:     "BackOff pulling image dispatches to classifyBackOff",
			reason:   "BackOff",
			message:  `Back-off pulling image "nginx:lates"`,
			wantPart: "Likely typo: nginx:lates",
		},
	}

	c := EventClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := ScanResults{
				Events: []kube.EventIssue{
					{Namespace: "ns", ObjectName: "pod-test", ObjectKind: "Pod", Reason: tt.reason, Message: tt.message, Count: 1, LastTimestamp: now},
				},
			}
			got := c.Classify(results)
			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1", len(got))
			}
			if !strings.Contains(got[0].OneLiner, tt.wantPart) {
				t.Errorf("OneLiner = %q, want substring %q", got[0].OneLiner, tt.wantPart)
			}
		})
	}
}

func TestClassifyBackOff(t *testing.T) {
	tests := []struct {
		name    string
		message string
		image   string
		want    string
	}{
		{
			name:    "restarting failed container with pod name",
			message: `Back-off restarting failed container app in pod probe-fail-test-67774765c8-9mxzk_default(abc123)`,
			image:   "",
			want:    "Container restarting — pod is in CrashLoopBackOff, check logs: kubectl logs probe-fail-test-67774765c8-9mxzk --previous",
		},
		{
			name:    "restarting failed container without parseable pod",
			message: `Back-off restarting failed container`,
			image:   "",
			want:    "Container restarting — pod is in CrashLoopBackOff, check logs with --previous",
		},
		{
			name:    "pulling image with extractable image",
			message: `Back-off pulling image "nginx:1.25"`,
			image:   "nginx:1.25",
			want:    "Registry unreachable: nginx:1.25 — original error expired (>1h). Run: docker pull nginx:1.25 to diagnose",
		},
		{
			name:    "pulling image typo tag",
			message: `Back-off pulling image "alpine:lates"`,
			image:   "alpine:lates",
			want:    "Likely typo: alpine:lates — did you mean 'latest'?",
		},
		{
			name:    "pulling image no image extracted",
			message: `Back-off pulling image somewhere`,
			image:   "",
			want:    "Pull failing — check pod events for details",
		},
		{
			name:    "generic backoff message",
			message: `Back-off some other thing happening`,
			image:   "",
			want:    "Back-off: Back-off some other thing happening",
		},
		{
			name:    "empty message",
			message: "",
			image:   "",
			want:    "Back-off: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyBackOff(tt.message, tt.image)
			if got != tt.want {
				t.Errorf("classifyBackOff(%q, %q)\n  got:  %q\n  want: %q", tt.message, tt.image, got, tt.want)
			}
		})
	}
}

func TestExtractTag(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"nginx:1.25", "1.25"},
		{"nginx:latest", "latest"},
		{"acr.io/myapp:v1.2", "v1.2"},
		{"registry:5000/myapp:v1.0", "v1.0"},
		{"nginx", ""},
		{"registry:5000/myapp", ""},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := extractTag(tt.image)
			if got != tt.want {
				t.Errorf("extractTag(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestUpdateFailed_KeyExtracted(t *testing.T) {
	results := ScanResults{
		EnvName:    "prod",
		ClusterCtx: "prod-cluster",
		Events: []kube.EventIssue{
			{
				Namespace:  "vault",
				ObjectName: "secret-store",
				ObjectKind: "SecretProviderClass",
				Reason:     "UpdateFailed",
				Message:    `error processing spec.data[0] (key: ask-imanage/dev/secret), err: rpc error: code = PermissionDenied`,
				Count:      3,
			},
		},
	}
	c := EventClassifier{}
	findings := c.Classify(results)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	oneLiner := findings[0].OneLiner
	if !strings.Contains(oneLiner, "ask-imanage/dev/secret") {
		t.Errorf("OneLiner %q does not contain key path %q", oneLiner, "ask-imanage/dev/secret")
	}
	if strings.Contains(oneLiner, "key:") {
		t.Errorf("OneLiner %q still contains truncated 'key:' text", oneLiner)
	}
}
