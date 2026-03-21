# klarity

**One command. Every cluster. Everything that's wrong.**

klarity is a read-only Kubernetes diagnostic CLI that scans multiple clusters and namespaces in parallel, classifies unhealthy workloads by root cause, and renders categorized terminal tables with one-line error summaries.

It's an inspector, not a surgeon — it tells you what's broken and why, but never touches your resources.

[Website](https://getklarity.dev) · [Installation](#installation) · [Quick Start](#quick-start) · [Documentation](#documentation)

---

## The Problem

SREs managing multi-cluster environments piece together health status from dozens of `kubectl` commands across contexts and namespaces. Lens requires clicking through long lists. Neither answers the simple question:

> **"What's wrong across my entire environment right now, and why?"**

klarity answers that in one command.

## What It Does

```
╔══════════════════════════════════════════════════════════════════════╗
║  klarity scan — 2026-03-21 14:32:07 CST                            ║
║  Environments: 3 | Clusters: 7 scanned | Issues: 17 found          ║
╚══════════════════════════════════════════════════════════════════════╝

━━━ 🔴 PROD ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

── prod-us-east-1 ──────────────────────────────────────────────────

🏷️  Image Tag Errors (3 pods)
┌──────────────┬──────────────────┬───────────┬──────────────────────────────┐
│ Namespace    │ Pod              │ Container │ Image                        │
├──────────────┼──────────────────┼───────────┼──────────────────────────────┤
│ payments     │ pay-api-7f8d-x   │ api       │ acr.io/pay-api:v2.14.0-tyop  │
│ orders       │ order-svc-9a2-q  │ worker    │ acr.io/order-svc:lates       │
└──────────────┴──────────────────┴───────────┴──────────────────────────────┘

💀 OOMKilled (2 pods)
┌──────────────┬──────────────────┬──────────┬──────────┬──────────┬────────────┐
│ Namespace    │ Pod              │ Requests │ Limits   │ NS Quota │ Restarts   │
├──────────────┼──────────────────┼──────────┼──────────┼──────────┼────────────┤
│ ml-serving   │ model-inf-8x2-a  │ 512Mi    │ 1Gi      │ 8Gi/10Gi │ 14        │
└──────────────┴──────────────────┴──────────┴──────────┴──────────┴────────────┘

🔥 CrashLoopBackOff — Application Errors (2 pods)
┌──────────────┬──────────────────┬──────────┬─────────────────────────────────────┐
│ Namespace    │ Pod              │ Restarts │ Root Cause (from logs)               │
├──────────────┼──────────────────┼──────────┼─────────────────────────────────────┤
│ checkout     │ cart-svc-3d1-r   │ 47       │ FATAL: password auth failed "cartdb" │
│ notifications│ email-wrk-1a-m   │ 12       │ ConnectionRefused: rabbitmq:5672     │
└──────────────┴──────────────────┴──────────┴─────────────────────────────────────┘

━━━ 🟢 DEV ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ No issues found.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary: 12 issues in prod | 1 in staging | 0 in dev
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Key Features

- **Multi-cluster scanning** — reads your kubeconfig, scans all configured contexts in parallel via `client-go`
- **Environment-aware** — auto-detects prod/staging/dev from context names, displays critical environments first
- **Categorized error tables** — separate tables per root cause (OOM, image pull, crash loop, pending, HPA ceiling), not one overwhelming list
- **One-line error summaries** — extracts root cause from logs: Java `Caused by`, Python tracebacks, Go panics, connection errors. No more reading 30-line stack traces
- **OOM context** — shows requests, limits, and namespace quota usage alongside OOMKilled pods
- **Image pull diagnostics** — distinguishes tag typos from 401 auth errors from registry timeouts
- **HPA health** — flags HPAs pegged at max replicas or unable to meet target metrics
- **Strictly read-only** — only `get`, `list`, and log tail operations. Safe to run against production
- **Configure once** — `klarity init` onboarding wizard saves to `~/.klarityconfig.yaml`, then just run `klarity`
- **Watch mode** — continuous scanning on a configurable interval
- **JSON output** — `klarity --output json` for CI pipelines and automation

## What klarity Scans

| Resource | Checks |
|---|---|
| Pods | CrashLoopBackOff, ImagePullBackOff, OOMKilled, Pending, high restart count |
| Deployments | unavailable replicas, ready/desired mismatch |
| DaemonSets | desired vs ready, misscheduled pods |
| StatefulSets | stuck rollouts, replica mismatch |
| Services | no matching endpoints (selector mismatch) |
| HPAs | at max replicas, metric overshoot, missing targets |
| Jobs/CronJobs | failed jobs, suspended CronJobs, past deadline |
| ResourceQuotas | approaching or exceeding quota |
| PVCs | stuck in Pending |
| Events | Warning events in last 15 min |

## Installation

```bash
# Go install
go install github.com/vishukamble/klarity@latest

# Or download binary from releases
curl -sSL https://getklarity.dev/install.sh | sh
```

## Quick Start

```bash
# First-time setup — reads kubeconfig, detects environments, saves config
klarity init

# Scan everything
klarity

# Scan specific environment
klarity --env prod

# Scan specific cluster
klarity --context prod-us-east-1

# Watch mode
klarity --watch --interval 60

# Filter by error category
klarity --category oom,crashloop

# JSON output for CI
klarity --output json
```

## Configuration

klarity stores config in `~/.klarityconfig.yaml`. The onboarding wizard generates this, but it's human-editable:

```yaml
version: 1
environments:
  - name: prod
    tier: critical
    clusters:
      - context: prod-us-east-1
        namespaces:
          mode: all
          exclude: [kube-system, kube-public, kube-node-lease, default]
      - context: prod-us-west-2
        namespaces:
          mode: all
          exclude: [kube-system, kube-public, kube-node-lease, default]
  - name: staging
    tier: standard
    clusters:
      - context: staging-us-east-1
        namespaces:
          mode: include
          include: [app-services, data-pipeline]
settings:
  log_tail_lines: 50
  parallel_clusters: 4
  scan_interval_seconds: 300
```

Adding a new cluster is as simple as appending a few lines under the right environment.

## AKS / Azure CLI Authentication

If you use Azure Kubernetes Service with `kubelogin` for authentication, be aware of a version-specific issue that affects multi-cluster scanning.

**Last known good version:** `v0.1.17`
**Regression introduced:** `v0.1.19`

kubelogin v0.1.19 introduced a regression where `azurecli` token cache mode re-prompts for authentication on every context switch. Because klarity scans multiple clusters in parallel, this causes interactive auth prompts to block goroutines mid-scan, hanging the process.

klarity automatically detects your kubelogin version at startup and prints a warning if >= 0.1.19 is found.

**Affected auth mode:** `azurecli` only. `spn` (service principal) and `workloadidentity` modes are **not affected** since they don't use interactive prompts.

### Pinning to v0.1.17

```bash
# Direct download (Linux amd64)
curl -Lo kubelogin.zip https://github.com/Azure/kubelogin/releases/download/v0.1.17/kubelogin-linux-amd64.zip
unzip kubelogin.zip -d kubelogin-bin
sudo mv kubelogin-bin/bin/linux_amd64/kubelogin /usr/local/bin/kubelogin

# Direct download (macOS arm64)
curl -Lo kubelogin.zip https://github.com/Azure/kubelogin/releases/download/v0.1.17/kubelogin-darwin-arm64.zip
unzip kubelogin.zip -d kubelogin-bin
sudo mv kubelogin-bin/bin/darwin_arm64/kubelogin /usr/local/bin/kubelogin

# If installed via brew, pin after downgrading
brew pin kubelogin
```

All releases: https://github.com/Azure/kubelogin/releases/tag/v0.1.17

## Documentation

See the [docs](./docs/) directory for:
- [Design Document](./docs/brainstorm.md) — full architecture, error categorization logic, output mockups
- [Project State](./docs/REMEMBER.md) — current implementation status
- [Contributing](./CONTRIBUTING.md)

## Built With

- [client-go](https://github.com/kubernetes/client-go) — Kubernetes API access
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Charmbracelet](https://charm.sh/) — lipgloss, bubbletea, huh for terminal UI
- [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) — parallel cluster scanning

## License

[Apache 2.0](./LICENSE) *(or your chosen license)*

---

<p align="center">
  <strong>klarity</strong> — because you shouldn't need 10 terminal tabs to know what's broken<br>
  <a href="https://getklarity.dev">getklarity.dev</a>
</p>