<h1 align="center">
  <br>
  ⬢ klarity
  <br>
</h1>

<h4 align="center">One command. Every cluster. Everything that's wrong.</h4>

<p align="center">
  <a href="https://github.com/vishukamble/klarity/releases">
    <img src="https://img.shields.io/github/v/release/vishukamble/klarity?color=blue" alt="Release">
  </a>
  <a href="https://github.com/vishukamble/klarity/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License">
  </a>
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8?logo=go" alt="Go">
</p>

<p align="center">
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#what-klarity-scans">What It Scans</a> •
  <a href="#all-flags">All Flags</a> •
  <a href="#configuration">Configuration</a> •
  <a href="CHANGELOG.md">Changelog</a>
  <a href="#aks--azure-cli-authentication">AKS Auth</a>
</p>

<p align="center">
  A read-only Kubernetes diagnostic CLI that scans multiple clusters in parallel,
  classifies unhealthy workloads by root cause, and renders categorized terminal
  tables with one-line error summaries. It tells you what's broken and why —
  never touches your resources.
</p>

---

## The Problem

SREs managing multi-cluster environments piece together health status from dozens
of `kubectl` commands across contexts and namespaces. Lens requires clicking 
through long lists. Neither tool answers the simple question:

> **"What's wrong across my entire environment right now, and why?"**

klarity answers that in one command.

---

## Installation
```bash
curl -sSL https://getklarity.dev/install.sh | sh
```

Or install with Go:
```bash
go install github.com/vishukamble/klarity@latest
```

Supports **macOS** and **Linux** (amd64 + arm64).

---

## Quick Start
```bash
# First-time setup — detects environments from kubeconfig, saves config
klarity init

# Scan everything
klarity

# Scan specific environment
klarity --env prod

# Scan specific cluster
klarity --context prod-us-east-1

# Filter to specific namespaces
klarity --namespace payments,analytics

# Exclude namespaces under construction
klarity --exclude-ns build-ns-1,build-ns-2

# Watch mode — continuous scanning
klarity --watch --interval 60

# JSON output for CI pipelines
klarity --output json | jq '.summary'
```

---

## What klarity Scans

| Resource | Checks |
|---|---|
| **ResourceQuotas** | Approaching or exceeding quota |
| **StatefulSets** | Stuck rollouts, replica mismatch |
| **DaemonSets** | Desired vs ready, misscheduled pods |
| **Services** | No matching endpoints (selector mismatch) |
| **HPAs** | At max replicas, metric overshoot, missing targets |
| **Deployments** | Unavailable replicas, ready/desired mismatch |
| **Jobs / CronJobs** | Failed jobs, suspended CronJobs, deadline exceeded |
| **PVCs** | Stuck in Pending, missing PVC references with typo suggestions |
| **Nodes** | NotReady, MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable |
| **Pods** | CrashLoopBackOff with log-extracted root cause, ImagePullBackOff, OOMKilled, Pending |
| **Warning Events** | Classified by root cause — mount failures, config errors, probe failures, evictions, admission webhooks, taint mismatches |

### Error Classification

klarity doesn't just report symptoms — it extracts root causes:

| What Kubernetes says | What klarity tells you |
|---|---|
| `CrashLoopBackOff` | `FATAL: password auth failed for "cartdb"` |
| `ImagePullBackOff` | `Likely typo: alpine:lates — did you mean 'latest'?` |
| `ImagePullBackOff` | `Registry auth failed: acr.io/app:v1.2 — check imagePullSecret` |
| `OOMKilled` | Container requests/limits + namespace quota usage |
| `Pending` | `PVC 'data-pvcc' not found (did you mean 'data-pvc'?)` |
| `Pending` | `4 nodes: GPU training pool — add toleration role=training-gpu` |
| `Unhealthy` | `Liveness probe: connection refused — check port and initialDelaySeconds` |
| `FailedMount` | `ConfigMap 'spark-conf' not found — verify it exists in this namespace` |

---

## All Flags

| Flag | Example | What it does |
|---|---|---|
| `--env` | `--env prod` | Scan only this environment |
| `--context` | `--context prod-us-east-1` | Scan only this cluster |
| `-n, --namespace` | `--namespace payments,analytics` | Scan only these namespaces (comma-separated) |
| `--exclude-ns` | `--exclude-ns build-ns-1,build-ns-2` | Skip these namespaces (ignored if --namespace set) |
| `--category` | `--category oom,crashloop,imagepull` | Show only these error categories |
| `--watch` | `--watch` | Continuously scan and refresh |
| `--interval` | `--interval 60` | Override scan interval in seconds |
| `-o, --output` | `--output json` | Output format: `table` (default) or `json` |
| `--log-lines` | `--log-lines 100` | Log lines to pull per pod for crash analysis |
| `--history` | `--history` or `--history 20` | Show scan history. Optional number = how many entries (default 10). `--history --env prod` to filter by environment |

## Config Commands

| Command | What it does |
|---|---|
| `klarity init` | Interactive setup wizard — detects environments, saves config |
| `klarity config show` | Print current config |
| `klarity config edit` | Open config in `$EDITOR` |
| `klarity slack setup` | Configure Slack notifications |
| `klarity config validate` | Validate config and verify cluster contexts exist in kubeconfig |
| `klarity --history` | Show history of past scans from `~/.klarity.log`. Use `--history 20` for more, `--history --env prod` to filter |

---

## Configuration

klarity stores config in `~/.klarityconfig.yaml`. The onboarding wizard
generates this automatically — it's human-editable for fine-tuning:
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
  - name: dev
    tier: standard
    clusters:
      - context: dev-us-east-1
        namespaces:
          mode: include
          include: [app-services, data-pipeline]
settings:
  log_tail_lines: 50
  parallel_clusters: 4
  scan_interval_seconds: 300
```

Adding a new cluster is appending a few lines under the right environment.
Namespace filtering is per-cluster — mix `all`, `include`, and `exclude` modes
across clusters in the same environment.

---

## Cache & History

klarity writes two files after every scan:

| File | Purpose |
|---|---|
| `~/.klarity_cache` | Last scan result — shown instantly on next run while a fresh scan runs in the background |
| `~/.klarity.log` | Append-only log of every scan with timestamps and issue counts per environment |

Running `klarity` mid-interval shows cached results immediately with a `(cached Xm ago, scanning...)` label. If the background scan finds different results, the screen re-renders automatically. If the scan fails, the cached result is kept with a warning.


## AKS / Azure CLI Authentication

If you use Azure Kubernetes Service with `kubelogin`, be aware of a version
regression that affects multi-cluster scanning.

| | Version |
|---|---|
| ✅ Last known good | `v0.1.17` |
| ❌ Regression introduced | `v0.1.19` |

kubelogin v0.1.19 changed the `azurecli` token cache behavior, causing
re-authentication prompts on every context switch. Because klarity scans
clusters in parallel via goroutines, this blocks mid-scan.

klarity detects your kubelogin version at startup and warns if >= v0.1.19
is found on AKS clusters.

**Affected:** `azurecli` auth mode only.  
**Not affected:** `spn` and `workloadidentity` modes.

### Pin to v0.1.17
```bash
# macOS (arm64)
curl -Lo kubelogin.zip https://github.com/Azure/kubelogin/releases/download/v0.1.17/kubelogin-darwin-arm64.zip
unzip kubelogin.zip && sudo mv bin/darwin_arm64/kubelogin /usr/local/bin/kubelogin

# Linux (amd64)
curl -Lo kubelogin.zip https://github.com/Azure/kubelogin/releases/download/v0.1.17/kubelogin-linux-amd64.zip
unzip kubelogin.zip && sudo mv bin/linux_amd64/kubelogin /usr/local/bin/kubelogin
```

---

## Built With

- [client-go](https://github.com/kubernetes/client-go) — Kubernetes API access
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Charmbracelet](https://charm.sh/) — lipgloss, bubbletea, huh for terminal UI
- [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) — parallel cluster scanning

---

## License

[Apache 2.0](./LICENSE)

---

<p align="center">
  <strong>klarity</strong> — because you shouldn't need 10 terminal tabs to know what's broken<br>
  <a href="https://getklarity.dev">getklarity.dev</a> •
  <a href="https://github.com/vishukamble/klarity">GitHub</a>
</p>