# REMEMBER.md — klarity Project State

> **IMPORTANT:** Claude Code MUST read this file at the start of every session. After completing any feature or making significant changes, update this file to reflect the current state. This is how we maintain continuity across sessions.

## Current State

**Last updated:** 2026-03-23
**Last session focus:** Bug fixes, classifier corpus, kube-system hint, instant cache + history features
**Build status:** `go build` ✅ | `go vet` ✅ | `go test` ✅ (all pass, 260 test cases)

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
- [x] **FEAT-09: Resource quota scanner** — `pkg/kube/resources.go`. `ListQuotaIssues()` (≥80% threshold) + `ListPendingPVCs()` + `ListPVCNames()`. 9 tests. (2026-03-21)
- [x] **FEAT-10: DaemonSet/StatefulSet scanner** — `pkg/kube/daemonsets.go` + `statefulsets.go`. Flags ready < desired and misscheduled pods. 8 tests. (2026-03-21)
- [x] **FEAT-11: Job/CronJob scanner** — `pkg/kube/jobs.go`. `ListFailedJobs()` (failed > 0) + `ListSuspendedCronJobs()`. 8 tests. (2026-03-21)

Also added:
- [x] **Namespace resolver** — `pkg/kube/namespaces.go`. `ResolveNamespaces()` handles all/include/exclude modes. 5 tests. (2026-03-21)
- [x] **PodIssue.LogSummary** — added `LogSummary string` field to `PodIssue`; populated by scan loop after FEAT-18/19; crashloop classifier reads it. (2026-03-21)
- [x] **PodIssue.VolumeClaimNames** — added `VolumeClaimNames []string` field; populated for Pending pods from pod spec volumes; used by PendingClassifier for missing-PVC detection. (2026-03-21)
- [x] **ScanResults.AllPVCNames** — added `AllPVCNames map[string][]string` (namespace → PVC names); populated by scan loop via `ListPVCNames()`; enables classifier-level PVC cross-referencing without API calls. (2026-03-21)
- [x] **Log stubs → real impl** — `pkg/logs/parser.go` and `pkg/logs/summarizer.go` now fully implemented. (2026-03-21)

### Phase 3: Diagnosis
- [x] **FEAT-12: Classifier interface** — `pkg/diagnosis/classifier.go`. Category (13 consts), Severity (Critical/Warning/Info), Finding struct, ScanResults struct, Classifier interface, RunAll(). (2026-03-21)
- [x] **FEAT-13: OOM classifier** — `pkg/diagnosis/oom.go` + `oom_test.go`. Finds OOMKilled PodIssues; Critical severity; detail: image, restart_count. 5 tests. (2026-03-21)
- [x] **FEAT-14: Image pull classifier** — `pkg/diagnosis/imagepull.go` + `imagepull_test.go`. Subtypes: auth_error, tag_not_found, registry_unreachable, unknown. 7 tests + 9 message-classification tests. (2026-03-21)
- [x] **FEAT-15: CrashLoop classifier** — `pkg/diagnosis/crashloop.go` + `crashloop_test.go`. Uses LogSummary for OneLiner (falls back to generic). Detail includes log_summary when present. 6 tests. (2026-03-21)
- [x] **FEAT-16: Pending classifier** — `pkg/diagnosis/pending.go` + `pending_test.go`. Subtypes: insufficient_cpu, insufficient_memory, unschedulable, pvc_not_bound, unknown. Injectable Now() for duration tests. Missing-PVC detection with Levenshtein typo suggestions (distance ≤ 2). 8 tests + 9 message-classification tests + 5 PVC/Levenshtein tests. (2026-03-21)
- [x] **FEAT-17: HPA classifier** — `pkg/diagnosis/hpa.go` + `hpa_test.go`. AtCeiling → Critical, ScalingLimited-only → Warning. CPU overshoot in one-liner. 6 tests. (2026-03-21)

### Phase 4: Log Analysis
- [x] **FEAT-18: Log tailer** — `pkg/logs/parser.go`. Real `FetchLogs` via `cs.CoreV1().Pods(ns).GetLogs()` with `TailLines` + `Previous`. 3 tests. (2026-03-21)
- [x] **FEAT-19: One-line summarizer** — `pkg/logs/summarizer.go`. Language-aware extraction: Java (last `Caused by:`), Python (exception after last traceback header), Go (`panic:`/`fatal error:`), Go structured log (`"command failed" err="[...]"` → first flag + count), generic (`FATAL`/`PANIC`/`Exception`/`Error`), connection errors, auth errors, fallback (last non-empty line). 26 tests + 3 priority-order tests. (2026-03-21; goCommandFailed pattern added 2026-03-23)

### Phase 5: Output
- [x] **FEAT-20: Table renderer** — `pkg/output/table.go`. lipgloss/table per category; catSpec map (icon, label, headers, rowFn); critical envs first; empty categories hidden; "✅ No issues found" per cluster with dim kube-system hint when excluded (`kubeSystemExcluded()` helper); `wrapText()` at 80 chars for Pending Reason, Warning Event Message, CrashLoop Root Cause. 31 tests. (2026-03-21; kube-system hint added 2026-03-23)
- [x] **FEAT-21: Color/tier theming** — `pkg/output/color.go`. critical=red, dev-named=green, standard=yellow; EnvColor/EnvEmoji/EnvHeaderStyle/SeverityStyle exported. (2026-03-21)
- [x] **FEAT-22: JSON output** — `pkg/output/json.go`. RenderJSON writes `[]jsonFinding` with no ANSI codes. --output json flag wired in cmd/root.go. (2026-03-21)
- [x] **FEAT-23: Summary footer** — `pkg/output/summary.go`. Per-env issue counts + "Next scan in Xm Ys" from scan interval. (2026-03-21; scan history display added separately via `pkg/cache/log.go` + `--history` flag, 2026-03-23)
- [x] **Full scan wiring** — `cmd/root.go`. Custom parallel scan loop (WaitGroup + semaphore); BuildClientset errors collected non-fatally; all 11 scanners called per namespace; logs fetched for CrashLoop pods; --output json|table; --env filter; graceful "no config" message. (2026-03-21)

### Phase 6: CLI Polish
- [x] **FEAT-24: Watch mode** — `--watch` flag + `--interval N` override; `signal.NotifyContext` for Ctrl-C; `clearScreen()` via ANSI; loop in `cmd/root.go`. (2026-03-21)
- [x] **FEAT-25: Filters** — `--namespace/-n` (comma-separated, applied pre-scan via NamespaceFilter), `--exclude-ns` (comma-separated, applied pre-scan), `--context`, `--category` (comma-separated aliases like "oom,crashloop"); `--namespace` wins over `--exclude-ns` with warning. (2026-03-22)
- [x] **FEAT-26: Config show command** — `cmd/config.go`; `klarity config show` (pretty-print) + `klarity config path`. (2026-03-21)

### Phase 7: Platform-Specific
- [x] **FEAT-30: kubelogin version detection** — `pkg/kube/client.go`: `DetectKubeloginVersion()`, `CheckKubeloginVersion()`, `parseKubeloginVersion()`, `KubeloginVersion.AtLeast()`. Advisory warning at scan start + during `klarity init` if kubelogin >= 0.1.19 detected with AKS exec credential. 17 new tests. README section added. (2026-03-21)

### Session Additions (not in original tracker)

- [x] **UX: single-cluster auto-select in init wizard** — `cmd/init.go`: `assignClusters(available, promptFn)` helper; if `len(available) == 1` returns immediately without calling promptFn; fallback path prints "✓ Only one cluster available — auto-selected: …". 3 new tests in `cmd/init_test.go`. (2026-03-23)
- [x] **UX: empty selection re-prompt instead of fatal exit** — `assignClusters()` loops until `len(chosen) > 0`, printing "✗ No clusters selected — please select at least one." on each empty response; replaces old skip-and-continue-then-fatal-at-end behavior. (2026-03-23)
- [x] **Corpus: Go structured log "command failed" err=[...] pattern** — `pkg/logs/summarizer.go`: `goCommandFailed()` finds `"command failed" err="[flag1, flag2, ...]"` lines, splits by `", "`, returns `"command failed: flag1 (and N more)"` or plain for single flag; priority slot 3b (after go panic/fatal, before generic FATAL). 3 new table tests. (2026-03-23)
- [x] **Cache layer (`pkg/cache/cache.go`)** — `Cache{ScannedAt time.Time, Findings []diagnosis.Finding}` stored at `~/.klarity_cache` as JSON. `Load` returns `(nil, nil)` for missing, `(nil, err)` for corrupt. `Save` writes mode 0600. `Age` / `IsStale(threshold)`. `Equal` for order-independent comparison (sorts by composite key before JSON marshal). `cmd/root.go`: `gatherFindings()` extracted from `doScan`; cache shown instantly with `(cached Xm ago, scanning...)` header → background goroutine → compare → "✓ Still current" or clearScreen+re-render. Watch mode still writes cache. `--output json` bypasses cache path. 10 tests. (2026-03-23)
- [x] **History log (`pkg/cache/log.go`)** — `LogEntry{ScannedAt, Environments map[string]int, Total}` appended as NDJSON lines to `~/.klarity.log` after every scan (manual + watch). `AppendLog` creates file if absent. `ReadLog(path, last)` returns last N entries; skips malformed lines. `FilterLog(entries, env)` keeps entries where env count > 0. `--history [N]` flag on root command (`NoOptDefVal = "10"`); `--history --env prod` filters. `showHistory()` renders formatted table. 7 tests. (2026-03-23)

### Phase 9: CLI Polish & Scanning
- [x] **FEAT-27: Node scanner** — `pkg/kube/nodes.go` `ListUnhealthyNodes()` checks NotReady/MemoryPressure/DiskPressure/PIDPressure/NetworkUnavailable. `pkg/diagnosis/nodes.go` `NodeClassifier` with `classifyNodeCondition()`. Renders FIRST in output. 14 tests. (2026-03-22)
- [x] **FEAT-28: `klarity config edit`** — `cmd/config.go`: opens config in $EDITOR (fallback: nano/vim/vi), validates after save. (2026-03-22)
- [x] **FEAT-29: `--version` flag** — `cmd/root.go`: `rootCmd.Version = "1.0.3"`, prints `klarity version 1.0.3`. (2026-03-22)
- [x] **FEAT-30b: `klarity config validate`** — `cmd/config.go`: loads config, runs Validate(), checks each cluster context against kubeconfig with ✓/⚠️ output. (2026-03-22)
- [x] **FEAT-31: Watch mode improvements** — `cmd/root.go`: fixed clear sequence, added watch header with interval, clean Ctrl+C exit. (2026-03-22)
- [x] **FEAT-32: JSON output completeness** — `pkg/output/json.go`: restructured from flat array to `{scan_time, environments[…], summary{…}}` with all categories mapped including NodeIssue. 6 tests. (2026-03-22)

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

## ListWarningEvents BackOff Fix (2026-03-22, Session 28)

- **Root cause** — `classifyImagePullGroup()` couldn't extract image names because Normal/BackOff events (which carry `Back-off pulling image "..."`) were never fetched; only `type=Warning` events were collected
- **Fix** — `pkg/kube/events.go`: second `List()` call with `FieldSelector: "type=Normal,reason=BackOff"`; results merged into single slice; deduplication by `(namespace, objectName, reason, message)` using `\x00`-separated key; Normal events with any reason other than BackOff filtered out in `addEvent()`
- **Fallback** — if cluster returns "field label not supported" for the compound selector, retries with `type=Normal` and filters `reason=="BackOff"` in Go
- **Tests added** — `makeTypedEvent()` helper; `TestListWarningEvents_IncludesNormalBackOff` (Warning/Failed included, Normal/BackOff included, Normal/Pulling excluded); `TestListWarningEvents_DeduplicatesBackOff` (fake client returns events from both calls, dedup ensures single finding)

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

## Audit Fixes (2026-03-22, Session 27)

- **L3: DefaultNsExclude never applied** — Added `defaultExclude []string` parameter to `ResolveNamespaces()`; call site in `cmd/root.go` passes `cfg.Settings.DefaultNsExclude`; 4 new tests covering all mode interactions; `CLAUDE.md` updated with DefaultNsExclude behavior note
- **M4: NoEndpoints shows '-' instead of service name** — Set `finding.PodName = s.ServiceName` in `pkg/diagnosis/noendpoints.go`; test updated with `wantPodName` field
- **H2: wrapText leading space** — Fixed `wrapText()` to skip space at break point so second line has no leading space; test expectation updated
- **M3: Evicted duplicate finding** — Removed `Evicted` case from `ContainerErrorClassifier`; integration test removed; `EventClassifier` is the sole handler for Evicted events
- **L1+L2: CLAUDE.md stale references** — Removed broken `@docs/brainstorm.md` reference; corrected "errgroup" to "WaitGroup + semaphore"

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

## Audit Fixes (2026-03-21, Session 9)

- **C1: Missing classifiers** — Created 8 new classifiers (NoEndpoints, Quota, PVC, DaemonSet, StatefulSet, Job, CronJob, Event) with tests; registered all 13 in `cmd/root.go`
- **H1/M4: Pending pod Message empty** — Added `pendingMessage()` in `pkg/kube/pods.go` to extract scheduling reason from Pod conditions (PodScheduled=False)
- **H2: filterByNamespace/filterByCategory mutate backing array** — Changed from `findings[:0]` to `var out []diagnosis.Finding`
- **H3: config show swallows errors** — Now only treats `os.ErrNotExist` as "no config found"; other errors propagated
- **M1: WarningEvent column mismatch** — Custom rowFn for WarningEvent (4 columns: Namespace, Object, Reason, Message) instead of genericRow (3 columns)
- **M5: exclude_completed_jobs unused** — Wired into `ListFailedJobs()` as `excludeCompleted` param; skips jobs with CompletionTime set
- **M6: Duplicate favicon tags** — Removed duplicate `<link rel="icon">` block in `website/index.html`

## 10 New Error Pattern Classifiers (2026-03-22, Session 19)

New file `pkg/diagnosis/container.go` with 8 classifier functions + `ContainerErrorClassifier`:

| Pattern | Function | Key signals |
|---|---|---|
| FailedMount | `classifyMountError()` | ConfigMap/Secret not found (name extraction), NFS timeout, CSI error, mount.nfs |
| CreateContainerConfigError | `classifyConfigError()` | Missing key in ConfigMap/Secret (key+name extraction), invalid env var name |
| RunContainerError | `classifyRunError()` | Executable not found (cmd extraction), no such file (path extraction), OCI runtime |
| InvalidImageName | `classifyImageNameError()` | Invalid reference format (Helm {{ }}), failed to apply default image tag |
| Evicted | `classifyEviction()` | ephemeral-storage, memory pressure, DiskPressure |
| FailedCreate | `classifyFailedCreate()` | Exceeded quota (name+resource), admission webhook denied (name+reason) |
| Unhealthy (probes) | `classifyProbeFailure()` | Liveness/Readiness/Startup probe: 5xx, connection refused, status code, timeout |
| FailedCreatePodSandBox | `classifySandboxError()` | cgroup error, CNI IP exhaustion, CNI plugin error, failed sandbox |

Other changes:
- **TopologySpreadConstraint** in `parseSchedulingMessage()` — "N nodes: topology spread violated"
- **Job failure classification** — `classifyJobFailure()`: BackoffLimitExceeded → "hit retry limit", DeadlineExceeded → "timed out", includes failed pod count
- **Evicted pod detection** — `pkg/kube/pods.go`: catches `pod.Status.Phase == Failed && pod.Status.Reason == "Evicted"`
- **RunContainerError** added to `unhealthyWaitingReasons` in pods.go
- **ContainerErrorClassifier** registered in `cmd/root.go` — catches CreateContainerConfigError, RunContainerError, InvalidImageName, Evicted pod-level issues
- **Event reason-based dispatch** in `EventClassifier.Classify()` — FailedMount, Unhealthy, FailedCreate, FailedCreatePodSandBox, Evicted routed to specialized classifiers before generic message classification
- 81 new tests; total 467 test runs, all pass

## Compound Scheduling Message Parser (2026-03-22, Session 18)

- **`parseSchedulingMessage()`** — Splits compound K8s scheduling messages (`0/49 nodes are available: ...`) into individual `SchedulingReason` structs with Count, Kind, Summary
- **`SchedulingReason`** — Count (N nodes), Kind (taint/affinity/resource/autoscaler/other), Summary (human-readable)
- **Classification rules** — Taints: CriticalAddonsOnly, GPU role, CPU role, not-ready, unreachable, generic. Affinity: nodeAffinity/nodeSelector. Resources: cpu, memory, nvidia.com/gpu, nvidia.com/mig-*. Autoscaler: max node group, no scale-up. KeyVault CSI errors.
- **`stripPreemption()`** — Removes "preemption: ..." suffix (no diagnostic value)
- **`formatSchedulingReasons()`** — Single reason: plain text. Multiple: bulleted list sorted by Count descending
- **Output integration** — Pending Reason column no longer wraps; shows structured multi-line summary instead of raw message
- 20 new tests; total 386 test runs, all pass

## Bugfixes (2026-03-21, Session 17)

- **Fix 1: PVC duplicate row** — `checkMissingPVCs()` replaced with `pvcSuggestion()` that folds PVC hint into the main pending finding's `message` detail field instead of creating a separate `pvc_not_found` finding row
- **Fix 2: Why column truncation** — Removed `wrapText()` from Warning Events Why column; the column now renders at full length, letting the terminal handle natural wrapping
- **Fix 3: client-go deprecation warning** — Added `rest.SetDefaultWarningHandler(rest.NoWarnings{})` in `cmd/root.go` `init()` to suppress "v1 Endpoints is deprecated" stderr noise

## Warning Events Refactor (2026-03-21, Sessions 14–15)

- **Part 1: Deduplication** — `groupEventsByObject()` collects all events per (ns, objectName); `bestEventForObject()` picks the event with diagnostic signal content (manifest unknown, 401, etc.) over generic ones; one finding per object
- **Part 2: Message classification** — `classifyEventMessage(message, image)` maps raw K8s messages to human-readable causes; image name included when available (e.g. "Tag not found: alpine:lates — verify tag exists")
- **Part 3: Table columns** — Changed from `Namespace | Object | Reason | Message` to `Namespace | Object | Category | Why`; OneLiner now contains classified message with image name
- **Image extraction** — `extractImageFromMessage()` parses `pull image "img:tag"` / `pulling image "img:tag"` patterns; `collectImageFromGroup()` searches all events for an object to find the image
- **No-signal fallback** — `guessImagePullCause(image, hasDetailEvent)` uses 4-branch tag heuristics: (1) nonsense tags → "Likely bad tag", (2) Levenshtein ≤ 2 from "latest" → "Likely typo", (3) semver/well-known → "Registry unreachable ... original error expired", (4) fallback → "Pull failed ... original error expired"
- 57 new tests (classifier + classifyEventMessage + extractImageFromMessage + guessImagePullCause + extractTag); total 366 test runs, all pass

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
- 2026-03-21: Missing-PVC detection lives in `PendingClassifier.checkMissingPVCs()` — pure function of `ScanResults`, no API calls; `VolumeClaimNames` and `AllPVCNames` populated at scan time
- 2026-03-21: Levenshtein distance threshold for PVC typo suggestions is ≤ 2; `closestPVCName()` returns best match or "" if none within threshold
- 2026-03-21: `ListPVCNames()` returns ALL PVC names in a namespace (all phases), not just pending ones — needed for typo cross-referencing
- 2026-03-21: `wrapText()` applied only to free-text columns (Pending Reason, Warning Event Message, CrashLoop Root Cause) at 80 chars; fixed-width columns (Namespace, Pod, etc.) never wrapped
- 2026-03-21: `klarity init` fallback path guard: if wizard completes with zero environments, returns user-facing error instead of saving invalid config
- 2026-03-21: Warning Events deduplicated per (namespace, objectName) — one row per object, keeping highest-priority event (ErrImagePull > Failed > FailedScheduling > BackOff > others)
- 2026-03-21: `classifyEventMessage(message, image)` maps raw K8s event messages to human-readable causes; PVC check runs before generic "not found" to avoid false positive; fallback truncates at 80 chars; image name included in image-pull-related branches when available
- 2026-03-21: Event dedup uses message signal content (not reason priority) — `messageHasSignal()` checks for diagnostic substrings; `collectImageFromGroup()` extracts image from any event in the group
- 2026-03-21: `guessImagePullCause()` uses tag heuristics when no ErrImagePull detail exists; Levenshtein threshold ≤ 2 for "latest" typo detection (consistent with PVC typo threshold); `extractTag()` handles registry:port/image:tag correctly
- 2026-03-21: Nonsense tag patterns checked before typo/semver to avoid false positives (e.g. "fake" won't match typo branch)
- 2026-03-21: PVC missing-reference info folded into main Pending finding's `message` field via `pvcSuggestion()` — no separate `pvc_not_found` row; avoids duplicate rows for the same pod
- 2026-03-21: Warning Events Why column has no `wrapText()` — relies on terminal natural wrapping to avoid cutting off actionable commands like `docker pull <image>`
- 2026-03-21: `rest.SetDefaultWarningHandler(rest.NoWarnings{})` called in `cmd/root.go` `init()` — suppresses all client-go deprecation warnings on stderr
- 2026-03-22: Pending Reason column has no `wrapText()` — shows structured multi-line scheduling reasons instead of raw message
- 2026-03-22: `parseSchedulingMessage()` strips preemption suffix, splits by comma, classifies each reason independently, sorts by Count descending
- 2026-03-22: `insufficientRe` captures resource name with `[\w./\-]+` and trims trailing period — avoids matching sentence-ending dots as part of resource names
- 2026-03-22: Single scheduling reason renders without bullet; multiple reasons use `•` prefix per line
- 2026-03-22: KeyVault CSI errors detected before scheduling message split — returns single "other" kind reason
- 2026-03-22: Event reason-based dispatch in `EventClassifier.Classify()` uses switch on `best.Reason` before falling through to `classifyEventMessage()` — FailedMount, Unhealthy, FailedCreate, FailedCreatePodSandBox, Evicted each routed to specialized classifiers
- 2026-03-22: `ContainerErrorClassifier` handles pod-level container errors (CreateContainerConfigError, RunContainerError, InvalidImageName, Evicted) — emits findings under `CategoryWarningEvent`
- 2026-03-22: Evicted pods detected in `ListUnhealthyPods()` via `pod.Status.Phase == Failed && pod.Status.Reason == "Evicted"` — surfaced both as pod-level findings (ContainerErrorClassifier) and event-level (EventClassifier if matching events exist)
- 2026-03-22: Mount error classifier uses `cmNotFoundRe`/`secNotFoundRe` (specific regexes) instead of generic `quotedNameRe` — avoids matching volume name instead of configmap/secret name
- 2026-03-22: `classifyJobFailure()` checks conditions for BackoffLimitExceeded/DeadlineExceeded before falling back to generic message — includes failed pod count in all messages
- 2026-03-22: TopologySpreadConstraint in scheduling messages classified as kind "affinity" — consistent with other node-matching failures
- 2026-03-22: PVC suggestion replaces message entirely (not appended) — `pvcSuggestion` check runs first, skips scheduling message parsing if non-empty
- 2026-03-22: Preemption suffix stripped in `Classify()` before `classifyPendingMessage()` — uses existing `stripPreemption()` on `p.Message` before any processing
- 2026-03-22: Namespace column has fixed `Width(30)` via `StyleFunc(row, col)` in table builder — prevents wrapping when adjacent columns have long content
- 2026-03-22: `--namespace` accepts comma-separated list (parsed by `parseCommaSeparated`); applied pre-scan via `NamespaceFilter.Include`; no post-scan filtering
- 2026-03-22: `--exclude-ns` flag excludes namespaces before API calls via `applyNamespaceFilters()`; ignored with warning if `--namespace` also set
- 2026-03-22: `cmd/root_test.go` added — first test file in `cmd/` package; tests `parseCommaSeparated` and `applyNamespaceFilters`
- 2026-03-22: `BackOff` event reason gets dedicated `classifyBackOff()` dispatch — distinguishes CrashLoopBackOff ("restarting failed container") from ImagePullBackOff ("pulling image"); removed `BackOff` from generic `guessImagePullCause` path in default switch case
- 2026-03-22: `classifyEventMessage()` fallback no longer truncates with `...` — returns message as-is, terminal handles wrapping
- 2026-03-22: `classifyBackOff()` extracts pod name from "in pod <name>_namespace(...)" for actionable kubectl command in CrashLoopBackOff case
- 2026-03-22: Image pull events use `classifyImagePullGroup()` which scans ALL events for image names and signals, instead of picking one "best" event — guaranteed to find image from BackOff events regardless of how many Failed events exist
- 2026-03-22: `isImagePullGroup()` detects image-pull groups by checking if any event has pulling/pull image/imagepullbackoff/errimagepull in its message with a matching reason
- 2026-03-22: `bestEventForObject()` simplified back to signal-first, used only for non-image-pull events
- 2026-03-22: All fallback messages for image pull failures changed from "check ErrImagePull reason above" to actionable "check pod events for details" or "kubectl describe pod <name>"
- 2026-03-22: `HasLivenessProbe` field on `PodIssue` — populated from pod spec by `containerHasLivenessProbe()`; used by CrashLoopClassifier to detect probe-killed exits
- 2026-03-22: `looksLikeCleanExit()` detects clean shutdown patterns ([notice]+exit, signal 15, SIGTERM, graceful shutdown/stop, server shutting down); combined with `HasLivenessProbe` to suggest probe-killed diagnosis
- 2026-03-22: `buildEnvironmentsFromInput()` is the testable core of fallback path — takes names + assignments, returns `[]config.Environment`, validates empty names and missing clusters
- 2026-03-22: `config.InferTier()` exported wrapper for `tierForLabel()` — used by fallback path to set tier from user-entered env name
- 2026-03-22: Fallback path shows confirmation summary before saving ("Ready to save: ..."); user can decline with `Save to ~/.klarityconfig.yaml? [N]`
- 2026-03-22: Fallback path validates per-env cluster selection inline (warns + skips empty envs) and fails early if no environments have clusters
- 2026-03-22: `buildEnvironmentsFromInput()` looks up assignments by original key then trimmed key — handles whitespace in form input
- 2026-03-21: Warning Events table columns changed from `Reason | Message` to `Category | Why` — Category is the K8s reason, Why is the classified message
- 2026-03-22: `ResolveNamespaces()` accepts `defaultExclude []string` as 4th param; applied only for mode=all with empty cluster exclude; ignored for include/exclude modes and when cluster has explicit exclude list
- 2026-03-22: `NoEndpointsClassifier` sets `finding.PodName = s.ServiceName` so table Service column shows actual service name instead of "-"
- 2026-03-22: `wrapText()` skips the space at break point — second line has no leading space character
- 2026-03-22: `ContainerErrorClassifier` no longer handles Evicted reason — EventClassifier already handles Evicted events; removing from ContainerErrorClassifier eliminates duplicate findings
- 2026-03-22: `ListWarningEvents()` makes two API calls — first for `type=Warning`, second for `type=Normal,reason=BackOff`; results merged and deduplicated by (namespace, objectName, reason, message); BackOff events contain image names needed by `classifyImagePullGroup()`; fallback to all-Normal fetch if cluster rejects compound field selector
- 2026-03-22: Normal/BackOff events included in results even though type=Normal — `classifyImagePullGroup()` needs the image name from `Back-off pulling image "..."` messages; all other Normal event reasons are filtered out in Go
- 2026-03-23: `pkg/cache` imports `pkg/diagnosis` — cache stores `[]diagnosis.Finding` directly (no intermediate type); JSON serialization works because Category/Severity are string aliases and Finding has all plain fields
- 2026-03-23: Cache bypassed for `--output json` — ANSI-free JSON path needs no cache headers; cache still written after live scan for next table-mode run
- 2026-03-23: `Equal()` sorts findings by composite key (`Category+EnvName+ClusterCtx+Namespace+PodName+OneLiner`) before JSON marshal — guards against non-deterministic goroutine ordering
- 2026-03-23: `--history` uses cobra `NoOptDefVal = "10"` — `--history` alone → 10, `--history 20` → 20, absent → 0 (disabled); checked before config load so no kubeconfig needed for history display
- 2026-03-23: `assignClusters` is the testable core of the cluster-selection UX — huh form injected via `promptFn func([]string) ([]string, error)`; single-cluster fast path and re-prompt loop both tested without TTY

---

**When you finish a feature:** Change `[ ]` to `[x]`, add the date, update "Last updated" and "Last session focus" at the top, and update HANDOFF.md with what to do next.
