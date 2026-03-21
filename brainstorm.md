# klarity — Kubernetes Environment Health Inspector

## Vision

One command. Every cluster. Everything that's wrong. Zero mutations.

`klarity` is a read-only Kubernetes diagnostic CLI that scans across multiple clusters and namespaces, categorizes unhealthy workloads by root cause, and presents a clear, actionable summary in your terminal. It's an inspector, not a surgeon — it tells you what's broken and why, but never touches your resources.

---

## The Problem

SREs managing multi-cluster environments face a fragmented diagnostic experience:

- **Lens** requires mouse-driven navigation through long lists of contexts and namespaces — fine for one cluster, painful for ten.
- **kubectl** demands `--context` and `--namespace` on every command, and you're left stitching together output mentally.
- **During upgrades**, you have to hop between clusters one at a time and manually check each namespace for fallout.
- **When something is broken**, figuring out *why* means digging through pod events, logs, and resource specs separately. A CrashLoopBackOff tells you nothing — you need to know if it's an OOM, a 401, a bad image tag, or a code error.

There's no single tool that answers: **"What's wrong across my entire environment right now, and why?"**

---

## Core Principles

1. **Read-only** — Only `get`, `list`, `logs` (tail). Never create, update, patch, or delete. Safe to run against production by design.
2. **Multi-cluster native** — Reads kubeconfig, scans across contexts in parallel via `client-go`. Not a wrapper around kubectl.
3. **Categorized diagnostics** — Don't just list unhealthy pods. Classify *why* they're unhealthy and surface the relevant details for that category.
4. **One-line error summaries** — A Java stack trace is 30 lines. The SRE needs one line: `NullPointerException at PaymentService.java:142` or `OOMKilled (requests: 256Mi, limits: 512Mi, ns quota: 2Gi)`.
5. **Configure once, run forever** — Onboarding wizard saves to `~/.klarityconfig.yaml`. After that, just run `klarity`.
6. **Environment-aware** — Clusters are grouped by environment (prod, dev, staging). Prod environments are visually prioritized.

---

## Language Choice: Go

| Factor | Go | Bash |
|---|---|---|
| K8s API access | `client-go` (native, same lib as kubectl) | Spawns kubectl processes, parses text |
| Multi-cluster parallelism | Goroutines, trivial | Background jobs, painful error handling |
| Log parsing / classification | Proper string handling, regex, structured logic | awk/grep chains, brittle |
| Distribution | Single static binary, `go install` | Requires kubectl, bash version compat |
| Terminal UI | lipgloss, tablewriter, bubbletea, color | tput, escape codes, limited |
| Error handling | Structured, typed errors | Exit codes, string matching |

**Verdict: Go.** The multi-context parallelism, log parsing, and distribution story make it the clear choice for an SRE-targeted CLI.

---

## Onboarding Flow

First run (`klarity init`):

### Phase 1: Auto-detect environments from kubeconfig

klarity reads the kubeconfig, parses context names, and attempts to derive environments from common naming patterns (e.g., `prod-us-east-1` → `prod`, `dev-cluster-3` → `dev`, `staging-eu` → `staging`). Keywords scanned: `prod`, `production`, `dev`, `development`, `staging`, `stg`, `test`, `testing`, `sandbox`, `qa`, `uat`.

**Happy path (environments detected in context names):**

```
$ klarity init

Reading ~/.kube/config...

Detected 10 clusters across 3 environments:

  prod (4 clusters)
    ✓ prod-us-east-1
    ✓ prod-us-west-2
    ✓ prod-eu-west-1
    ✓ prod-ap-south-1

  staging (2 clusters)
    ✓ staging-us-east-1
    ✓ staging-eu-west-1

  dev (4 clusters)
    ✓ dev-us-east-1
    ✓ dev-us-west-2
    ✓ dev-team-alpha
    ✓ dev-team-beta

Scan all 4 prod clusters? [Y/n] Y
Scan all 2 staging clusters? [Y/n] n

Select staging clusters (space to toggle, enter to confirm):
  [x] staging-us-east-1
  [ ] staging-eu-west-1

Scan all 4 dev clusters? [Y/n] n

Select dev clusters (space to toggle, enter to confirm):
  [x] dev-us-east-1
  [ ] dev-us-west-2
  [ ] dev-team-alpha
  [ ] dev-team-beta

Namespaces: scanning all namespaces (excluding kube-system, kube-public, kube-node-lease, default)
To customize, edit ~/.klarityconfig.yaml after setup.

✅ Config saved to ~/.klarityconfig.yaml
Run `klarity` to scan your environment.
```

**Fallback path (environment can't be derived from names):**

```
$ klarity init

Reading ~/.kube/config...

Found 6 clusters:
  • us-east-aks-01
  • us-west-aks-02
  • eu-central-aks-01
  • ap-south-aks-01
  • tools-central
  • sandbox-team-a

Could not detect environments from cluster names.

How many environments do you want to configure? > 3

Environment 1 name: > prod
Select clusters for prod (space to toggle, enter to confirm):
  [x] us-east-aks-01
  [x] us-west-aks-02
  [x] eu-central-aks-01
  [x] ap-south-aks-01
  [ ] tools-central
  [ ] sandbox-team-a

Environment 2 name: > tooling
Select clusters for tooling (space to toggle, enter to confirm):
  [ ] us-east-aks-01
  [ ] us-west-aks-02
  [ ] eu-central-aks-01
  [ ] ap-south-aks-01
  [x] tools-central
  [ ] sandbox-team-a

Environment 3 name: > dev
Select clusters for dev (space to toggle, enter to confirm):
  [ ] us-east-aks-01
  [ ] us-west-aks-02
  [ ] eu-central-aks-01
  [ ] ap-south-aks-01
  [ ] tools-central
  [x] sandbox-team-a

Namespaces: scanning all namespaces (excluding kube-system, kube-public, kube-node-lease, default)
To customize, edit ~/.klarityconfig.yaml after setup.

✅ Config saved to ~/.klarityconfig.yaml
```

### Phase 2: Namespace defaults

By default, klarity scans **all namespaces** in selected clusters, excluding system namespaces (`kube-system`, `kube-public`, `kube-node-lease`, `default`). Users can customize per-cluster namespace filtering by editing `~/.klarityconfig.yaml` directly — the onboarding wizard keeps it simple and the config file is human-editable for fine-tuning.

---

## Config File (`~/.klarityconfig.yaml`)

The config is structured so adding a new cluster is as simple as adding a few lines under the right environment block.

```yaml
version: 1

environments:
  - name: prod
    tier: critical          # critical | standard — controls display priority and coloring
    clusters:
      - context: prod-us-east-1
        namespaces:
          mode: all         # all (minus system) | include | exclude
          exclude:          # only used when mode: all or mode: exclude
            - kube-system
            - kube-public
            - kube-node-lease
            - default
      - context: prod-us-west-2
        namespaces:
          mode: all
          exclude:
            - kube-system
            - kube-public
            - kube-node-lease
            - default
      - context: prod-eu-west-1
        namespaces:
          mode: all
          exclude:
            - kube-system
            - kube-public
            - kube-node-lease
            - default

  - name: staging
    tier: standard
    clusters:
      - context: staging-us-east-1
        namespaces:
          mode: all
          exclude:
            - kube-system
            - kube-public
            - kube-node-lease
            - default

  - name: dev
    tier: standard
    clusters:
      - context: dev-us-east-1
        namespaces:
          mode: include     # only scan these specific namespaces
          include:
            - app-services
            - data-pipeline

# Adding a new cluster is this simple:
# Just add under the right environment:
#
#   - context: prod-ap-southeast-1
#     namespaces:
#       mode: all
#       exclude:
#         - kube-system
#         - kube-public
#         - kube-node-lease
#         - default

settings:
  log_tail_lines: 50             # how many log lines to pull for analysis
  parallel_clusters: 4           # max concurrent cluster scans
  scan_interval_seconds: 300     # default scan interval (5 min), overridable via --interval
  exclude_completed_jobs: true   # hide Completed pods by default
  default_ns_exclude:            # system namespaces excluded by default when mode: all
    - kube-system
    - kube-public
    - kube-node-lease
    - default
```

---

## What klarity Scans

For every selected namespace in every configured cluster, klarity performs read-only API calls to check:

| Resource | What We Check |
|---|---|
| **Pods** | Phase != Running/Succeeded, CrashLoopBackOff, ImagePullBackOff, ErrImagePull, OOMKilled, restartCount > threshold, pending pods |
| **Deployments** | unavailableReplicas > 0, replicas != readyReplicas, image tag mismatches across containers |
| **ReplicaSets** | Orphaned RS with 0 desired but still present (stale rollouts) |
| **DaemonSets** | desiredNumberScheduled != numberReady, misscheduled pods |
| **StatefulSets** | replicas != readyReplicas, stuck rollouts |
| **Services** | Services with no matching endpoints (selector mismatch) |
| **HPAs** | At max replicas (scaling ceiling), currentMetric >> targetMetric, targeting missing deployments |
| **Jobs/CronJobs** | Failed jobs, suspended CronJobs, long-running jobs past deadline |
| **Events** | Warning events in last 15 min (supplements pod-level checks) |
| **ResourceQuotas** | Namespace approaching or exceeding quota |
| **PVCs** | Pending PVCs (storage not provisioned) |

---

## Error Categorization & Output

### Design Decision: Separate Tables Per Error Category

Each error type gets its own section with a colored header and count. Empty categories are hidden. This lets SREs triage by root cause — all image issues are one fix, all OOMs are a different conversation.

### Output Structure

Output is ordered: **critical-tier environments first, then standard.** Within each environment, clusters are listed. Within each cluster, error categories are shown as separate tables. Empty categories are hidden.

```
╔══════════════════════════════════════════════════════════════════════╗
║  klarity scan — 2026-03-21 14:32:07 CST                            ║
║  Environments: 3 | Clusters: 7 scanned | Issues: 17 found          ║
╚══════════════════════════════════════════════════════════════════════╝

━━━ 🔴 PROD ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

── prod-us-east-1 ──────────────────────────────────────────────────

🏷️  Image Tag Errors (3 pods)
┌──────────────┬──────────────────┬─────────────────────────┬──────────────────────────────┐
│ Namespace    │ Pod              │ Container               │ Image                        │
├──────────────┼──────────────────┼─────────────────────────┼──────────────────────────────┤
│ payments     │ pay-api-7f8d-x   │ api                     │ acr.io/pay-api:v2.14.0-tyop  │
│ payments     │ pay-api-7f8d-z   │ api                     │ acr.io/pay-api:v2.14.0-tyop  │
│ orders       │ order-svc-9a2-q  │ worker                  │ acr.io/order-svc:lates       │
└──────────────┴──────────────────┴─────────────────────────┴──────────────────────────────┘

🔐 Image Pull Auth Errors — 401/403 (2 pods)
┌──────────────┬──────────────────┬──────────────────────────────┬─────────────────────────┐
│ Namespace    │ Pod              │ Image                        │ Error                   │
├──────────────┼──────────────────┼──────────────────────────────┼─────────────────────────┤
│ analytics    │ ingest-5c3-a     │ acr.io/ingest:v1.8.0         │ 401 Unauthorized        │
│ analytics    │ ingest-5c3-b     │ acr.io/ingest:v1.8.0         │ 401 Unauthorized        │
└──────────────┴──────────────────┴──────────────────────────────┴─────────────────────────┘

💀 OOMKilled (2 pods)
┌──────────────┬──────────────────┬──────────┬──────────┬──────────┬────────────────────┐
│ Namespace    │ Pod              │ Requests │ Limits   │ NS Quota │ Restarts (24h)     │
├──────────────┼──────────────────┼──────────┼──────────┼──────────┼────────────────────┤
│ ml-serving   │ model-inf-8x2-a  │ 512Mi    │ 1Gi      │ 8Gi/10Gi │ 14                 │
│ ml-serving   │ model-inf-8x2-b  │ 512Mi    │ 1Gi      │ 8Gi/10Gi │ 9                  │
└──────────────┴──────────────────┴──────────┴──────────┴──────────┴────────────────────┘

🔥 CrashLoopBackOff — Application Errors (3 pods)
┌──────────────┬──────────────────┬────────────┬───────────────────────────────────────────┐
│ Namespace    │ Pod              │ Restarts   │ Root Cause (from logs)                     │
├──────────────┼──────────────────┼────────────┼───────────────────────────────────────────┤
│ checkout     │ cart-svc-3d1-r   │ 47         │ FATAL: password auth failed for "cartdb"   │
│ checkout     │ cart-svc-3d1-s   │ 43         │ FATAL: password auth failed for "cartdb"   │
│ notifications│ email-wrk-1a-m   │ 12         │ ConnectionRefused: rabbitmq:5672            │
└──────────────┴──────────────────┴────────────┴───────────────────────────────────────────┘

⏳ Pending Pods (1 pod)
┌──────────────┬──────────────────┬─────────────┬──────────────────────────────────────────┐
│ Namespace    │ Pod              │ Pending For │ Reason                                    │
├──────────────┼──────────────────┼─────────────┼──────────────────────────────────────────┤
│ batch        │ etl-job-28x-run  │ 23m         │ 0/12 nodes: Insufficient memory           │
└──────────────┴──────────────────┴─────────────┴──────────────────────────────────────────┘

📈 HPA Scaling Issues (1 HPA)
┌──────────────┬──────────────────┬─────────┬─────────┬───────────┬────────────────────────┐
│ Namespace    │ HPA              │ Current │ Max     │ CPU Now   │ CPU Target             │
├──────────────┼──────────────────┼─────────┼─────────┼───────────┼────────────────────────┤
│ api-gateway  │ gateway-hpa      │ 20/20   │ 20      │ 89%       │ 60%                    │
└──────────────┴──────────────────┴─────────┴─────────┴───────────┴────────────────────────┘

── prod-us-west-2 ──────────────────────────────────────────────────

✅ No issues found.

── prod-eu-west-1 ──────────────────────────────────────────────────

✅ No issues found.

━━━ 🟡 STAGING ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

── staging-us-east-1 ───────────────────────────────────────────────

🔥 CrashLoopBackOff — Application Errors (1 pod)
┌──────────────┬──────────────────┬────────────┬───────────────────────────────────────────┐
│ Namespace    │ Pod              │ Restarts   │ Root Cause (from logs)                     │
├──────────────┼──────────────────┼────────────┼───────────────────────────────────────────┤
│ app-services │ feature-x-abc-1  │ 5          │ KeyError: 'DATABASE_URL' (env var missing) │
└──────────────┴──────────────────┴────────────┴───────────────────────────────────────────┘

━━━ 🟢 DEV ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

── dev-us-east-1 ───────────────────────────────────────────────────

✅ No issues found.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary: 12 issues in prod | 1 in staging | 0 in dev
Next scan in 4m 38s (--interval 300)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## Log Analysis Strategy

The one-line root cause extraction is the hardest and most valuable part. Strategy per error type:

### OOMKilled
- **Source:** `pod.status.containerStatuses[].lastState.terminated.reason == "OOMKilled"`
- **Show:** container requests, limits, namespace ResourceQuota usage
- **No log parsing needed** — the signal is in the pod spec and status

### Image Pull Errors
- **Source:** Pod events (ErrImagePull, ImagePullBackOff)
- **Classification:**
  - `401 Unauthorized` / `403 Forbidden` → Auth error (registry creds expired or missing imagePullSecret)
  - `manifest unknown` / `not found` → Tag doesn't exist (typo, not pushed)
  - `timeout` / `connection refused` → Registry unreachable
- **Show:** The full image string (so you can spot the typo) + the specific error

### CrashLoopBackOff (Application Errors)
- **Source:** `kubectl logs --tail=50 --previous` (get the logs from the crashed container)
- **One-line extraction logic (priority order):**
  1. Look for `FATAL`, `PANIC`, `Exception`, `Error` lines — take the first one
  2. For Java: find the `Caused by:` line closest to the bottom of the trace
  3. For Python: find the last line of the traceback (the actual exception)
  4. For Go: find the `panic:` line or `fatal error:` line
  5. For connection errors: extract `ConnectionRefused`, `ECONNREFUSED`, `dial tcp ... connection refused`
  6. For auth errors in logs: extract lines with `401`, `403`, `authentication failed`, `permission denied`
  7. Fallback: last non-empty line of logs
- **Show:** The one-line summary + restart count

### Pending Pods
- **Source:** Pod conditions + events
- **Classification:**
  - `Insufficient cpu/memory` → Scheduling failure (show node capacity if available)
  - `no nodes available` → Cluster scaling issue
  - `Unschedulable` → Taint/toleration or affinity mismatch
  - PVC pending → Storage not provisioned
- **Show:** How long it's been pending + the reason

---

## CLI Commands

```bash
# First-time setup
klarity init

# Full scan (uses ~/.klarityconfig.yaml)
klarity

# Scan specific environment only
klarity --env prod

# Scan specific cluster only
klarity --context prod-us-east-1

# Scan specific namespace in a cluster
klarity --context prod-us-east-1 --namespace payments

# Only show specific error categories
klarity --category oom,crashloop,imagepull

# Continuous scan mode (default interval from config, or override)
klarity --watch
klarity --watch --interval 60

# Output as JSON (for piping / CI integration)
klarity --output json

# Show current config
klarity config show

# Re-run onboarding
klarity init

# Increase log tail depth for this scan
klarity --log-lines 100
```

---

## Architecture

```
klarity/
├── cmd/                        # CLI commands (cobra)
│   ├── root.go                 # Main scan command
│   ├── init.go                 # Onboarding wizard
│   └── config.go               # Config management
├── pkg/
│   ├── config/                 # Config loading/saving (~/.klarityconfig.yaml)
│   │   ├── config.go           # Struct definitions, load/save
│   │   └── detect.go           # Environment auto-detection from context names
│   ├── kube/                   # Kubernetes client-go wrappers
│   │   ├── client.go           # Multi-context client factory
│   │   ├── pods.go             # Pod scanning
│   │   ├── deployments.go      # Deployment scanning
│   │   ├── hpa.go              # HPA scanning
│   │   ├── events.go           # Event collection
│   │   └── resources.go        # ResourceQuota, PVC, Service scanning
│   ├── diagnosis/              # Error classification engine
│   │   ├── classifier.go       # Main classifier (routes to sub-classifiers)
│   │   ├── oom.go              # OOM analysis
│   │   ├── imagepull.go        # Image pull error analysis
│   │   ├── crashloop.go        # CrashLoopBackOff log analysis
│   │   ├── pending.go          # Pending pod analysis
│   │   └── hpa.go              # HPA health analysis
│   ├── logs/                   # Log parsing and summarization
│   │   ├── parser.go           # Language-aware log parser
│   │   └── summarizer.go       # One-line summary generator
│   └── output/                 # Rendering
│       ├── table.go            # Terminal table rendering (lipgloss)
│       ├── color.go            # Color/tier theming
│       └── json.go             # JSON output mode
├── go.mod
├── go.sum
└── main.go
```

---

## Key Dependencies (Go)

- `k8s.io/client-go` — Kubernetes API access
- `github.com/spf13/cobra` — CLI framework
- `github.com/charmbracelet/lipgloss` — Terminal styling
- `github.com/charmbracelet/bubbletea` — Interactive TUI (for onboarding wizard)
- `github.com/charmbracelet/huh` — Form/survey library (for onboarding prompts)
- `github.com/olekukonez/tablewriter` — Table rendering (or lipgloss/table)
- `gopkg.in/yaml.v3` — Config file parsing

---

## Future Ideas (Not in V1)

- **Helm release cross-reference** — Map unhealthy pods to Helm releases to show which chart deployment caused the issue.
- **`klarity --ci`** — Exit code 1 if critical issues found. Use in CI/CD as a post-deploy health gate.
- **Network policy audit** — Flag pods with no network policies.
- **Certificate expiry** — Check cert-manager certificates nearing expiration.
- **Istio sidecar status** — Flag pods missing sidecars or with injection errors.
- **Node health summary** — NotReady nodes, disk/memory/PID pressure.
- **Slack/webhook integration** — Post scan summary to a channel.
- **HTML report export** — `klarity --output html > report.html` for sharing.
- **Custom checks via plugins** — Let teams define their own diagnostic rules.
- **Diff between scans** — Highlight what's new vs. resolved since last scan within watch mode.

---

## Claude Project Description

> **klarity** is a read-only Kubernetes diagnostic CLI written in Go. It scans multiple clusters and namespaces in parallel, classifies unhealthy workloads by root cause (OOM, image pull errors, CrashLoopBackOff with language-aware log analysis, pending scheduling, HPA ceiling, etc.), and renders categorized terminal tables with one-line error summaries. It uses `client-go` for Kubernetes API access, reads kubeconfig contexts, auto-detects environments (prod/staging/dev) from cluster names, persists configuration to `~/.klarityconfig.yaml`, and supports environment tier tagging (critical/standard) for prioritized output. It never mutates resources — strictly read-only. Continuous scan mode runs on a configurable interval. Target user: SREs managing multi-cluster environments who need a single command to see everything wrong across their infrastructure.
