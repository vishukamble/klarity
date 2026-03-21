# REMEMBER.md — klarity Project State

> **IMPORTANT:** Claude Code MUST read this file at the start of every session. After completing any feature or making significant changes, update this file to reflect the current state. This is how we maintain continuity across sessions.

## Current State

**Last updated:** 2026-03-21
**Last session focus:** FEAT-SLACK — Slack integration for scan summaries
**Build status:** `go build` ✅ | `go vet` ✅ | `go test` ✅ (293 test runs, all pass)

## Project Initialization

- [x] `go mod init github.com/vishukamble/klarity` — module path used (NOT `github.com/vishu/klarity`)
- [x] Directory structure created (`cmd/`, `pkg/config/`, `pkg/kube/`, `pkg/diagnosis/`, `pkg/logs/`, `pkg/output/`)
- [x] Cobra CLI scaffolding (`main.go`, `cmd/root.go`)
- [x] `go build` passes with empty scaffolding

## Feature Tracker

### Phase 1: Foundation
- [x] **FEAT-01: Config schema** — `pkg/config/config.go` + `config_test.go`. Structs, Load(), Save(), Validate(). 20 table tests all pass. (2026-03-21)
- [x] **FEAT-02: Onboarding wizard (`klarity init`)** — `cmd/init.go` + `pkg/config/detect.go`. Happy path (auto-detected envs) and fallback path (manual naming). `charmbracelet/huh` forms. 24 new table tests for detect logic. (2026-03-21)
- [x] **FEAT-03: Multi-context client factory** — `pkg/kube/client.go`. `ClientsetBuilder` type, `BuildClientset()`, `ScanAll()` with errgroup + semaphore. 14 tests. (2026-03-21)

### Phase 2: Scanners
- [x] **FEAT-04: Pod scanner** — `pkg/kube/pods.go`. Detects CrashLoopBackOff, ImagePullBackOff, ErrImagePull, OOMKilled (from lastState), Pending, init container issues. 8 tests. (2026-03-21)
- [x] **FEAT-05: Deployment scanner** — `pkg/kube/deployments.go`. Flags unavailableReplicas > 0 or readyReplicas < desired. 5 tests. (2026-03-21)
- [x] **FEAT-06: HPA scanner** — `pkg/kube/hpa.go`. Flags at-ceiling (current == max), ScalingLimited condition, desired > max. Extracts CPU utilization. 5 tests. (2026-03-21)
- [x] **FEAT-07: Service scanner** — `pkg/kube/services.go`. Flags services with selector but no ready endpoint addresses. 5 tests. (2026-03-21)
- [x] **FEAT-08: Event collector** — `pkg/kube/events.go`. Returns Warning events within configurable lookback window (default 15 min). 5 tests. (2026-03-21)
- [x] **FEAT-09: Resource quota scanner** — `pkg/kube/resources.go`. `ListQuotaIssues()` (≥80% threshold) + `ListPendingPVCs()`. 7 tests. (2026-03-21)
- [x] **FEAT-10: DaemonSet/StatefulSet scanner** — `pkg/kube/daemonsets.go` + `statefulsets.go`. Flags ready < desired and misscheduled pods. 8 tests. (2026-03-21)
- [x] **FEAT-11: Job/CronJob scanner** — `pkg/kube/jobs.go`. `ListFailedJobs()` (failed > 0) + `ListSuspendedCronJobs()`. 8 tests. (2026-03-21)

Also added:
- [x] **Namespace resolver** — `pkg/kube/namespaces.go`. `ResolveNamespaces()` handles all/include/exclude modes. 5 tests. (2026-03-21)
- [x] **PodIssue.LogSummary** — added `LogSummary string` field to `PodIssue`; populated by scan loop after FEAT-18/19; crashloop classifier reads it. (2026-03-21)
- [x] **Log stubs → real impl** — `pkg/logs/parser.go` and `pkg/logs/summarizer.go` now fully implemented. (2026-03-21)

### Phase 3: Diagnosis
- [x] **FEAT-12: Classifier interface** — `pkg/diagnosis/classifier.go`. Category (13 consts), Severity (Critical/Warning/Info), Finding struct, ScanResults struct, Classifier interface, RunAll(). (2026-03-21)
- [x] **FEAT-13: OOM classifier** — `pkg/diagnosis/oom.go` + `oom_test.go`. Finds OOMKilled PodIssues; Critical severity; detail: image, restart_count. 5 tests. (2026-03-21)
- [x] **FEAT-14: Image pull classifier** — `pkg/diagnosis/imagepull.go` + `imagepull_test.go`. Subtypes: auth_error, tag_not_found, registry_unreachable, unknown. 7 tests + 9 message-classification tests. (2026-03-21)
- [x] **FEAT-15: CrashLoop classifier** — `pkg/diagnosis/crashloop.go` + `crashloop_test.go`. Uses LogSummary for OneLiner (falls back to generic). Detail includes log_summary when present. 6 tests. (2026-03-21)
- [x] **FEAT-16: Pending classifier** — `pkg/diagnosis/pending.go` + `pending_test.go`. Subtypes: insufficient_cpu, insufficient_memory, unschedulable, pvc_not_bound, unknown. Injectable Now() for duration tests. 8 tests + 9 message-classification tests. (2026-03-21)
- [x] **FEAT-17: HPA classifier** — `pkg/diagnosis/hpa.go` + `hpa_test.go`. AtCeiling → Critical, ScalingLimited-only → Warning. CPU overshoot in one-liner. 6 tests. (2026-03-21)

### Phase 4: Log Analysis
- [x] **FEAT-18: Log tailer** — `pkg/logs/parser.go`. Real `FetchLogs` via `cs.CoreV1().Pods(ns).GetLogs()` with `TailLines` + `Previous`. 3 tests. (2026-03-21)
- [x] **FEAT-19: One-line summarizer** — `pkg/logs/summarizer.go`. Language-aware extraction: Java (last `Caused by:`), Python (exception after last traceback header), Go (`panic:`/`fatal error:`), generic (`FATAL`/`PANIC`/`Exception`/`Error`), connection errors, auth errors, fallback (last non-empty line). 23 tests + 3 priority-order tests. (2026-03-21)

### Phase 5: Output
- [x] **FEAT-20: Table renderer** — `pkg/output/table.go`. lipgloss/table per category; catSpec map (icon, label, headers, rowFn); critical envs first; empty categories hidden; "✅ No issues found" per cluster. 26 tests. (2026-03-21)
- [x] **FEAT-21: Color/tier theming** — `pkg/output/color.go`. critical=red, dev-named=green, standard=yellow; EnvColor/EnvEmoji/EnvHeaderStyle/SeverityStyle exported. (2026-03-21)
- [x] **FEAT-22: JSON output** — `pkg/output/json.go`. RenderJSON writes `[]jsonFinding` with no ANSI codes. --output json flag wired in cmd/root.go. (2026-03-21)
- [x] **FEAT-23: Summary footer** — `pkg/output/summary.go`. Per-env issue counts + "Next scan in Xm Ys" from scan interval. (2026-03-21)
- [x] **Full scan wiring** — `cmd/root.go`. Custom parallel scan loop (WaitGroup + semaphore); BuildClientset errors collected non-fatally; all 11 scanners called per namespace; logs fetched for CrashLoop pods; --output json|table; --env filter; graceful "no config" message. (2026-03-21)

### Phase 6: CLI Polish
- [x] **FEAT-24: Watch mode** — `--watch` flag + `--interval N` override; `signal.NotifyContext` for Ctrl-C; `clearScreen()` via ANSI; loop in `cmd/root.go`. (2026-03-21)
- [x] **FEAT-25: Filters** — `--namespace/-n`, `--context`, `--category` (comma-separated aliases like "oom,crashloop"); applied pre-scan + post-scan. (2026-03-21)
- [x] **FEAT-26: Config show command** — `cmd/config.go`; `klarity config show` (pretty-print) + `klarity config path`. (2026-03-21)

### Phase 7: Platform-Specific
- [x] **FEAT-30: kubelogin version detection** — `pkg/kube/client.go`: `DetectKubeloginVersion()`, `CheckKubeloginVersion()`, `parseKubeloginVersion()`, `KubeloginVersion.AtLeast()`. Advisory warning at scan start + during `klarity init` if kubelogin >= 0.1.19 detected with AKS exec credential. 17 new tests. README section added. (2026-03-21)

### Phase 8: Notifications
- [x] **FEAT-SLACK: Slack integration** — New `pkg/notifications/` package + `cmd/slack.go`. (2026-03-21)
  - `cmd/slack.go`: `klarity slack setup` interactive wizard (webhook or bot token, test message, severity filter, save)
  - `pkg/notifications/slack.go`: `SendSummary()`, `TestConnection()`, `FormatSummary()` (Block Kit), `classifySlackError()` (6 error cases + fallback), `filterBySeverity()`, injectable `HTTPClient` interface for testing
  - `pkg/config/config.go`: `SlackConfig`, `NotificationsConfig` structs; validation for mode/webhook_url/bot_token/channel/min_severity
  - `cmd/root.go`: wired `SendSummary()` after scan render; errors are non-fatal (printed to stderr)
  - `cmd/config.go`: `config show` displays slack settings when enabled
  - 27 new tests (FormatSummary, error classification, mock HTTP, severity filtering, on_issues_only)

## Marketing Website

- `website/index.html` + `website/style.css` — static marketing site, to be hosted on S3 + CloudFront. Added 2026-03-21.
- Not part of the Go build; keep it out of `go build` / `go test` scope.

## Known Issues / Blockers

_None._

## Audit Fixes (2026-03-21, Session 9)

- **C1: Missing classifiers** — Created 8 new classifiers (NoEndpoints, Quota, PVC, DaemonSet, StatefulSet, Job, CronJob, Event) with tests; registered all 13 in `cmd/root.go`
- **H1/M4: Pending pod Message empty** — Added `pendingMessage()` in `pkg/kube/pods.go` to extract scheduling reason from Pod conditions (PodScheduled=False)
- **H2: filterByNamespace/filterByCategory mutate backing array** — Changed from `findings[:0]` to `var out []diagnosis.Finding`
- **H3: config show swallows errors** — Now only treats `os.ErrNotExist` as "no config found"; other errors propagated
- **M1: WarningEvent column mismatch** — Custom rowFn for WarningEvent (4 columns: Namespace, Object, Reason, Message) instead of genericRow (3 columns)
- **M5: exclude_completed_jobs unused** — Wired into `ListFailedJobs()` as `excludeCompleted` param; skips jobs with CompletionTime set
- **M6: Duplicate favicon tags** — Removed duplicate `<link rel="icon">` block in `website/index.html`

## Architecture Decisions Made

- 2026-03-21: Module path is `github.com/vishukamble/klarity` (not `github.com/vishu/klarity`)
- 2026-03-21: `brainstorm.md` lives at repo root, not in `docs/`
- 2026-03-21: `NamespaceFilter.Mode=exclude` with empty Exclude list is valid
- 2026-03-21: `devops-tools` does NOT match `dev` — word-boundary check in detect.go
- 2026-03-21: `huh` forms require TTY; init.go not unit-tested — logic lives in detect.go
- 2026-03-21: `ScanAll` uses `kubernetes.Interface` for fake injection in tests
- 2026-03-21: `parallel_clusters: 0` coerced to 1 inside `ScanAll`
- 2026-03-21: OOMKilled emitted as separate PodIssue even when container is also in CrashLoopBackOff — diagnosis layer correlates them
- 2026-03-21: `QuotaThreshold = 80.0` — quota issues reported at ≥80% usage
- 2026-03-21: `ResolveNamespaces` is a shared helper called by the scan loop, not inside individual scanners
- 2026-03-21: All scanner functions take `(ctx, cs, namespace string)` — one namespace per call; caller iterates namespaces
- 2026-03-21: `pendingMessage()` extracts from PodScheduled=False condition first, then falls back to any condition with a message
- 2026-03-21: `ListFailedJobs` takes `excludeCompleted bool` — skips jobs with `CompletionTime != nil` when true
- 2026-03-21: `filterByNamespace`/`filterByCategory` use fresh slices (not `findings[:0]`) to avoid mutating shared backing arrays
- 2026-03-21: kubelogin version check is advisory only — never fatal, never blocks scanning. Warns on stderr for >= 0.1.19 (azurecli regression). `klarity init` only checks if exec credential with `kubelogin` command found in kubeconfig.
- 2026-03-21: Slack notifications are non-fatal — errors printed to stderr, never block scanning or output
- 2026-03-21: `pkg/notifications/` uses injectable `HTTPClient` interface (not `*http.Client`) for testability
- 2026-03-21: Slack config lives under `notifications.slack` in config YAML, not under `settings`
- 2026-03-21: `klarity slack setup` tests connection before saving — if test fails, config is not written
- 2026-03-21: `min_severity` filter applied before `on_issues_only` check — so "critical only + on_issues_only" won't post if only Warning/Info findings exist

---

**When you finish a feature:** Change `[ ]` to `[x]`, add the date, update "Last updated" and "Last session focus" at the top, and update HANDOFF.md with what to do next.
