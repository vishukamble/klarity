package diagnosis

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

// ── classifyMountError tests ─────────────────────────────────────────────────

func TestClassifyMountError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "configmap not found with name",
			message: `MountVolume.SetUp failed for volume "config-vol" : configmap "app-config" not found`,
			want:    "ConfigMap 'app-config' not found — verify it exists in this namespace",
		},
		{
			name:    "secret not found with name",
			message: `MountVolume.SetUp failed for volume "tls-cert" : secret "tls-secret" not found`,
			want:    "Secret 'tls-secret' not found — verify it exists in this namespace",
		},
		{
			name:    "nfs mount failed with timeout",
			message: `mount failed: exit status 32, mounting NFS share, Connection timed out`,
			want:    "NFS mount timeout — check firewall allows port 2049 to NFS server",
		},
		{
			name:    "csi driver error",
			message: `rpc error: code = Internal desc = input/output error during volume attach`,
			want:    "CSI driver error — check CSI driver pod logs in kube-system",
		},
		{
			name:    "csi keyword alone",
			message: `AttachVolume.Attach failed: CSI driver timeout`,
			want:    "CSI driver error — check CSI driver pod logs in kube-system",
		},
		{
			name:    "mount.nfs error",
			message: `mount.nfs: access denied by server while mounting 10.0.0.1:/share`,
			want:    "NFS mount failed — check NFS server is reachable from node",
		},
		{
			name:    "fallback unknown mount error",
			message: `some unknown mount error happened`,
			want:    "Volume mount failed — check volume definition and storage availability",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Volume mount failed — check volume definition and storage availability",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyMountError(tt.message)
			if got != tt.want {
				t.Errorf("classifyMountError(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifyConfigError tests ────────────────────────────────────────────────

func TestClassifyConfigError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "missing key in ConfigMap",
			message: `couldn't find key "DATABASE_URL" in ConfigMap "app-config"`,
			want:    "Missing key 'DATABASE_URL' in ConfigMap 'app-config' — check configMapKeyRef spelling",
		},
		{
			name:    "missing key in Secret",
			message: `couldn't find key "password" in Secret "db-credentials"`,
			want:    "Missing key 'password' in Secret 'db-credentials' — check secretKeyRef spelling",
		},
		{
			name:    "invalid env var name",
			message: `couldn't create env var from key "invalid-key": not a valid environment variable name`,
			want:    "Invalid env var name in ConfigMap/Secret key — keys must match [A-Za-z_][A-Za-z0-9_]*",
		},
		{
			name:    "fallback config error",
			message: `some config error`,
			want:    "Container config error — verify all ConfigMap/Secret key references exist",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Container config error — verify all ConfigMap/Secret key references exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyConfigError(tt.message)
			if got != tt.want {
				t.Errorf("classifyConfigError(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifyRunError tests ───────────────────────────────────────────────────

func TestClassifyRunError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "command not found with name",
			message: `failed to start container: OCI runtime create failed: exec: "myapp": executable file not found in $PATH`,
			want:    "Command 'myapp' not found in container PATH — check image has this binary",
		},
		{
			name:    "no such file /bin/bash",
			message: `failed to start container: exec /bin/bash: no such file or directory`,
			want:    "Binary '/bin/bash' missing — minimal images lack bash, use /bin/sh or busybox",
		},
		{
			name:    "no such file /bin/sh",
			message: `exec /bin/sh: no such file or directory`,
			want:    "Binary '/bin/sh' missing — minimal images lack bash, use /bin/sh or busybox",
		},
		{
			name:    "no such file generic path",
			message: `exec: "/app/start.sh": no such file or directory`,
			want:    "Binary '/app/start.sh' missing — minimal images lack bash, use /bin/sh or busybox",
		},
		{
			name:    "OCI runtime error",
			message: `OCI runtime create failed: container_linux.go:370: runc did not start`,
			want:    "Container runtime failed to start — check command/entrypoint in pod spec",
		},
		{
			name:    "fallback run error",
			message: `failed to start container for unknown reason`,
			want:    "Container failed to start — check command and entrypoint in pod spec",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Container failed to start — check command and entrypoint in pod spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRunError(tt.message)
			if got != tt.want {
				t.Errorf("classifyRunError(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifyImageNameError tests ─────────────────────────────────────────────

func TestClassifyImageNameError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "invalid reference format",
			message: `Failed to pull image "{{ .Values.image }}": invalid reference format`,
			want:    "Malformed image name — check for unrendered Helm variables ({{ }}) or illegal characters",
		},
		{
			name:    "couldnt parse image reference",
			message: `couldn't parse image reference "my image:latest": invalid format`,
			want:    "Malformed image name — check for unrendered Helm variables ({{ }}) or illegal characters",
		},
		{
			name:    "failed to apply default image tag",
			message: `Failed to apply default image tag "repo:tag:extra": invalid format`,
			want:    "Invalid image tag format — image field contains illegal characters",
		},
		{
			name:    "fallback",
			message: `image name error`,
			want:    "Invalid image name — check image field syntax",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Invalid image name — check image field syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyImageNameError(tt.message)
			if got != tt.want {
				t.Errorf("classifyImageNameError(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifyEviction tests ───────────────────────────────────────────────────

func TestClassifyEviction(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "ephemeral storage exceeded",
			message: `The node was low on resource: ephemeral-storage. Container app was using 2Gi.`,
			want:    "Evicted: ephemeral storage exceeded — add ephemeral-storage limit or reduce log/tmp usage",
		},
		{
			name:    "memory pressure",
			message: `The node was low on resource: memory.`,
			want:    "Evicted: node memory pressure — add memory limits to prevent node-level OOM eviction",
		},
		{
			name:    "disk pressure",
			message: `The node had condition: [DiskPressure].`,
			want:    "Evicted: node disk pressure — check node disk usage, clean up or expand storage",
		},
		{
			name:    "fallback eviction",
			message: `pod evicted`,
			want:    "Pod evicted by kubelet — check node conditions: kubectl describe node <node>",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Pod evicted by kubelet — check node conditions: kubectl describe node <node>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyEviction(tt.message)
			if got != tt.want {
				t.Errorf("classifyEviction(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifyFailedCreate tests ───────────────────────────────────────────────

func TestClassifyFailedCreate(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "exceeded quota with resource",
			message: `exceeded quota: "compute-quota", requested: cpu=2, used: cpu=14, limited: cpu=16`,
			want:    "Quota 'compute-quota' exceeded for cpu — check namespace quota usage",
		},
		{
			name:    "exceeded quota no resource match",
			message: `exceeded quota: "obj-quota", requested: configmaps=1`,
			want:    "Quota 'obj-quota' exceeded — check namespace quota usage",
		},
		{
			name:    "admission webhook denied with reason",
			message: `admission webhook "validate.gatekeeper.sh" denied the request: [opa-required-labels] you must provide labels: app`,
			want:    "Admission webhook 'validate.gatekeeper.sh' rejected pod — [opa-required-labels] you must provide labels: app",
		},
		{
			name:    "admission webhook denied no detail",
			message: `admission webhook "policy.example.com" denied the request`,
			want:    "Admission webhook 'policy.example.com' rejected pod — check webhook logs for policy violation",
		},
		{
			name:    "admission webhook no denied keyword",
			message: `admission webhook "security.example.com" error`,
			want:    "Admission webhook rejected pod — check webhook logs for policy violation",
		},
		{
			name:    "fallback",
			message: `some other create error`,
			want:    "Failed to create pod — check events for details",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Failed to create pod — check events for details",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyFailedCreate(tt.message)
			if got != tt.want {
				t.Errorf("classifyFailedCreate(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifyProbeFailure tests ───────────────────────────────────────────────

func TestClassifyProbeFailure(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "liveness probe 5xx",
			message: `Liveness probe failed: HTTP probe failed with statuscode: 500`,
			want:    "Liveness probe: HTTP 5xx — app is running but unhealthy, check app logs",
		},
		{
			name:    "liveness probe connection refused",
			message: `Liveness probe failed: dial tcp 10.0.0.5:8080: connection refused`,
			want:    "Liveness probe: connection refused — app may have crashed or wrong port",
		},
		{
			name:    "liveness probe generic failure",
			message: `Liveness probe failed: something went wrong`,
			want:    "Liveness probe failed — app is unhealthy, check logs for crash or hang",
		},
		{
			name:    "readiness probe connection refused",
			message: `Readiness probe failed: dial tcp 10.0.0.5:8080: connection refused`,
			want:    "Readiness probe: connection refused — app not ready yet, check initialDelaySeconds",
		},
		{
			name:    "readiness probe status code",
			message: `Readiness probe failed: HTTP probe failed with statuscode: 503`,
			want:    "Readiness probe: HTTP 503 — app returning error on health endpoint",
		},
		{
			name:    "readiness probe generic",
			message: `Readiness probe failed: some check`,
			want:    "Readiness probe failed — pod removed from service endpoints until resolved",
		},
		{
			name:    "startup probe timeout",
			message: `Startup probe failed: context deadline exceeded`,
			want:    "Startup probe timeout — app taking too long to start, increase failureThreshold",
		},
		{
			name:    "startup probe generic",
			message: `Startup probe failed: other reason`,
			want:    "Startup probe failed — app not starting properly, check logs",
		},
		{
			name:    "fallback unknown probe",
			message: `something probe issue`,
			want:    "Probe failed — check probe configuration and app health endpoint",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Probe failed — check probe configuration and app health endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProbeFailure(tt.message)
			if got != tt.want {
				t.Errorf("classifyProbeFailure(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── classifySandboxError tests ───────────────────────────────────────────────

func TestClassifySandboxError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "cgroup error",
			message: `failed to create cgroup mount for container`,
			want:    "Node cgroup error — kubelet/runtime out of sync, node may need draining",
		},
		{
			name:    "cni ip exhaustion",
			message: `networkPlugin cni failed to set up pod network: failed to assign an IP address`,
			want:    "CNI IP exhaustion — subnet may be out of IPs, check IPAM and VPC subnet size",
		},
		{
			name:    "cni generic error",
			message: `networkPlugin cni failed to set up pod network: some cni error`,
			want:    "CNI plugin error — check CNI pod logs in kube-system",
		},
		{
			name:    "failed to start sandbox",
			message: `failed to start sandbox container: runtime error`,
			want:    "Pod sandbox failed — container runtime error, check kubelet logs on node",
		},
		{
			name:    "fallback sandbox",
			message: `sandbox creation problem`,
			want:    "Pod sandbox creation failed — check kubelet and container runtime logs",
		},
		{
			name:    "empty message",
			message: "",
			want:    "Pod sandbox creation failed — check kubelet and container runtime logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySandboxError(tt.message)
			if got != tt.want {
				t.Errorf("classifySandboxError(%q)\n  got:  %q\n  want: %q", tt.message, got, tt.want)
			}
		})
	}
}

// ── ContainerErrorClassifier integration tests ───────────────────────────────

func TestContainerErrorClassifier(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		message string
		wantLen int
		check   func(Finding) bool
	}{
		{
			name:    "CreateContainerConfigError",
			reason:  "CreateContainerConfigError",
			message: `couldn't find key "DB_HOST" in ConfigMap "app-config"`,
			wantLen: 1,
			check: func(f Finding) bool {
				return strings.Contains(f.OneLiner, "Missing key 'DB_HOST'")
			},
		},
		{
			name:    "RunContainerError",
			reason:  "RunContainerError",
			message: `exec: "myapp": executable file not found in $PATH`,
			wantLen: 1,
			check: func(f Finding) bool {
				return strings.Contains(f.OneLiner, "Command 'myapp'")
			},
		},
		{
			name:    "InvalidImageName",
			reason:  "InvalidImageName",
			message: `couldn't parse image reference "{{ .Values.image }}"`,
			wantLen: 1,
			check: func(f Finding) bool {
				return strings.Contains(f.OneLiner, "Malformed image name")
			},
		},
		{
			name:    "non-matching reason ignored",
			reason:  "CrashLoopBackOff",
			message: "restart",
			wantLen: 0,
		},
	}

	c := ContainerErrorClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				Pods: []kube.PodIssue{
					{
						Namespace:     "default",
						PodName:       "test-pod",
						ContainerName: "main",
						Reason:        tt.reason,
						Message:       tt.message,
					},
				},
			}
			got := c.Classify(results)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && tt.check != nil && !tt.check(got[0]) {
				t.Errorf("check failed for finding: %+v", got[0])
			}
		})
	}
}
