# HANDOFF.md — Session Continuity

> **IMPORTANT:** Claude Code MUST read this file at the start of every session. This tells you exactly where to pick up. Before ending a session, update this file with what to do next.

## What To Do Next

**All Phase 1–8 features + FEAT-27–32 are complete.** Potential future work:

1. **GitHub Actions release pipeline** — goreleaser, homebrew tap
2. **Ingress/NetworkPolicy scanner** — detect misconfigured ingress rules
3. **Custom classifier plugins** — user-defined patterns in config

### How the Full Pipeline Fits Together (implemented)

```
klarity
  → load ~/.klarityconfig.yaml
  → parallel goroutines per env×cluster (semaphore = parallel_clusters)
    → BuildClientset(context) — errors collected non-fatally
    → ResolveNamespaces(filter, cfg.Settings.DefaultNsExclude) → []string
    → for each namespace: ListUnhealthyPods/Deployments/HPAs/Services/Events/
                          Quotas/PVCs/DaemonSets/StatefulSets/Jobs/CronJobs
    → FetchLogs for CrashLoop pods → Summarize → PodIssue.LogSummary
    → ListPVCNames per namespace → AllPVCNames map
    → ScanResults{EnvName, ClusterCtx, all scanner outputs, AllPVCNames}
    → RunAll(results, classifiers) → []Finding
  → pkg/output.RenderReport (table) or RenderJSON (--output json)
```

### Context You'll Need

- `cmd/root.go` — full scan orchestration, `--output`/`--env` flags, `scanCluster()`, `filterEnv()`
- `pkg/output/` — `RenderReport`, `RenderJSON`, `SummaryCounts`; do NOT call lipgloss from JSON path
- `pkg/diagnosis/classifier.go` — Finding, Classifier interface, RunAll
- CLAUDE.md: classifiers return data, output layer is only formatter; never mutate K8s resources

## Previous Session Summary

**2026-03-22 — Session 28: ListWarningEvents — include Normal/BackOff events for image pull classification**

| File | What it does |
|---|---|
| `pkg/kube/events.go` | Second `List()` call: `type=Normal,reason=BackOff`. `addEvent()` helper deduplicates by (namespace, objectName, reason, message) and filters Normal events to BackOff-only. Fallback: if "field label not supported" error, fetches all Normal events and filters in Go. |
| `pkg/kube/events_test.go` | `makeTypedEvent()` helper (accepts event type param). `TestListWarningEvents_IncludesNormalBackOff` — verifies Warning/Failed included, Normal/BackOff included, Normal/Pulling excluded. `TestListWarningEvents_DeduplicatesBackOff` — verifies single finding after dedup when fake client returns events from both calls. |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

**2026-03-22 — Session 27: Audit fixes — DefaultNsExclude, NoEndpoints PodName, wrapText, Evicted dedup, CLAUDE.md**

5 targeted fixes from codebase audit:

| Fix | File(s) | Summary |
|---|---|---|
| L3: DefaultNsExclude never applied | `pkg/kube/namespaces.go`, `cmd/root.go`, `pkg/kube/namespaces_test.go` | Added `defaultExclude []string` param to `ResolveNamespaces()`; applied for mode=all with empty cluster exclude; 4 new tests |
| M4: NoEndpoints Service column "-" | `pkg/diagnosis/noendpoints.go`, `noendpoints_test.go` | Set `finding.PodName = s.ServiceName`; test updated with `wantPodName` field |
| H2: wrapText leading space | `pkg/output/table.go`, `pkg/output/output_test.go` | Skip space at break point; second line no longer has leading space |
| M3: Evicted duplicate finding | `pkg/diagnosis/container.go`, `container_test.go` | Removed Evicted case from ContainerErrorClassifier; EventClassifier is sole handler |
| L1+L2: CLAUDE.md stale refs | `CLAUDE.md` | Removed broken brainstorm.md reference; corrected errgroup→WaitGroup+semaphore; added DefaultNsExclude behavior note |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

**2026-03-22 — Session 26: Complete rewrite of image pull event handling**

Replaced single-event dedup with multi-event scan for image pull classification:

| File | What it does |
|---|---|
| `pkg/diagnosis/events.go` | New `isImagePullGroup()` detects if an event group is image-pull related. New `classifyImagePullGroup(events, objectName)` scans ALL events: (1) extract image from any event with `pulling image "..."`, (2) find diagnostic signal from any event, (3) fall back to `guessImagePullCause` with image, (4) actionable fallback with object name. `Classify()` routes image-pull groups through this path before reason-based dispatch. `bestEventForObject()` simplified to signal-first (used only for non-image-pull). All fallback messages changed from "check ErrImagePull reason above" to "check pod events for details" or `kubectl describe pod <name>`. |
| `pkg/diagnosis/events_test.go` | 1 new regression test: `ManyFailedFewBackOff_ImageExtracted` — 3 Failed + 2 BackOff events for same pod, verifies image extracted from BackOff and `guessImagePullCause` called. Updated 2 fallback message expectations. |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (538 test runs, all pass)

**2026-03-22 — Session 25: Image pull event dedup fix + CrashLoop probe-killed detection**

Two fixes:

| File | What it does |
|---|---|
| `pkg/diagnosis/events.go` | Fix 1: `bestEventForObject()` now uses 4-tier priority: (1) image+signal (e.g. ErrImagePull with "manifest unknown"), (2) signal only, (3) image only (e.g. BackOff with "pulling image"), (4) first event. Ensures events with diagnostic detail win over generic ones, while BackOff events with image names still beat Failed events with no image. |
| `pkg/diagnosis/events_test.go` | 1 new test: `TestEventClassifier_FailedThenBackOff_ImageWins` — "Failed" + "Error: ImagePullBackOff" vs "BackOff" + `pulling image "nginx:1.25"` → BackOff wins, image extracted. |
| `pkg/diagnosis/crashloop.go` | Fix 2: `looksLikeCleanExit()` detects clean shutdown patterns ([notice]+exit, signal 15, SIGTERM, graceful shutdown/stop, server shutting down). When `HasLivenessProbe` is true and log looks like clean exit, replaces OneLiner with "Process exited cleanly — likely killed by liveness probe". |
| `pkg/diagnosis/crashloop_test.go` | 9 new probe-killed detection subtests (nginx exit with/without probe, SIGTERM, graceful shutdown/stop, server shutting down, actual error not probe-killed, empty log) + 9 `looksLikeCleanExit` table tests. |
| `pkg/kube/pods.go` | Added `HasLivenessProbe bool` to `PodIssue`, `containerHasLivenessProbe()` helper, populated in `inspectContainerStatuses()`. |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (537 test runs, all pass)

**2026-03-22 — Session 24: FEAT-27 through FEAT-32**

Implemented 6 features in sequence:

| Feature | Files | Summary |
|---|---|---|
| FEAT-27: Node scanner | `pkg/kube/nodes.go`, `pkg/kube/nodes_test.go`, `pkg/diagnosis/nodes.go`, `pkg/diagnosis/nodes_test.go`, `pkg/diagnosis/classifier.go`, `pkg/output/table.go`, `cmd/root.go` | `ListUnhealthyNodes()` checks all 5 node conditions (NotReady, MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable). `NodeClassifier` with `classifyNodeCondition()`. Renders FIRST in table (before OOM). 3 kube tests + 11 classifier tests. |
| FEAT-28: config edit | `cmd/config.go` | `klarity config edit`: finds $EDITOR or nano/vim/vi, opens config interactively, validates after save. |
| FEAT-29: --version | `cmd/root.go` | `rootCmd.Version = "1.0.3"`, `./klarity --version` prints `klarity version 1.0.3`. |
| FEAT-30: config validate | `cmd/config.go` | `klarity config validate`: loads config, validates, checks each cluster context against kubeconfig with ✓/⚠️ output. |
| FEAT-31: watch mode | `cmd/root.go` | Fixed clear sequence (`\033[H\033[2J`), added watch header with interval, clean Ctrl+C exit. |
| FEAT-32: JSON restructure | `pkg/output/json.go`, `pkg/output/output_test.go` | Restructured from flat array to `{scan_time, environments[{name, tier, clusters[{context, findings{category: [...]}, total_issues}]}], summary{total_issues, by_environment}}`. All categories mapped. 5 JSON tests updated + 1 new. |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (517 test runs, all pass) | ✅ `./klarity --version` → 1.0.3

**2026-03-22 — Session 23: klarity init fallback path fix — complete wizard flow with confirmation**

Fixed the init fallback path (triggered when context names lack env keywords like docker-desktop):

| File | What it does |
|---|---|
| `cmd/init.go` | Rewrote `runFallbackPath()`: env count prompt (1-10 validated), per-env name input + cluster multi-select with inline validation (warns + skips envs with 0 clusters), early exit if no envs have clusters, confirmation summary ("Ready to save: prod (3 clusters): ..."), save confirmation via huh.Confirm. Extracted `buildEnvironmentsFromInput(names, assignments)` as testable core — validates empty names, missing clusters, trims whitespace, sets tier via `InferTier`. |
| `cmd/init_test.go` | New file: 8 tests for `buildEnvironmentsFromInput` — single env, multiple envs, empty name (error), no clusters (error), no names (error), namespace mode defaults to all, whitespace name trimmed, missing assignment (error) |
| `pkg/config/detect.go` | Added exported `InferTier(name)` wrapper for `tierForLabel()` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (500 test runs, all pass)

**2026-03-22 — Session 22: Two event classifier regressions — BackOff dispatch + fallback truncation**

Fixed two regressions in the Warning Events classifier:

| File | What it does |
|---|---|
| `pkg/diagnosis/events.go` | Fix 1: `BackOff` reason now gets dedicated `classifyBackOff(message, image)` dispatch in the Classify switch — distinguishes CrashLoopBackOff ("restarting failed container" → extracts pod name, suggests `kubectl logs --previous`) from ImagePullBackOff ("pulling image" → delegates to `guessImagePullCause`). Removed `BackOff` from the generic `guessImagePullCause` path in default case. Fix 2: `classifyEventMessage` fallback no longer truncates with `...` — returns message as-is. |
| `pkg/diagnosis/events_test.go` | 7 new `classifyBackOff` tests (restart with pod, restart without pod, pulling with image, pulling typo, pulling no image, generic, empty) + 2 new reason-dispatch tests (BackOff restart, BackOff pulling) + updated fallback test to expect no truncation |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (492 test runs, all pass)

**2026-03-22 — Session 21: Namespace filtering — comma-separated --namespace, new --exclude-ns**

Extended namespace filtering with two complementary flags:

| File | What it does |
|---|---|
| `cmd/root.go` | `--namespace/-n` now accepts comma-separated list (e.g. `payments,analytics`), parsed by `parseCommaSeparated()`. New `--exclude-ns` flag excludes namespaces pre-scan via `applyNamespaceFilters()`. If both set, `--namespace` wins with stderr warning. `scanCluster()` applies include via `NamespaceFilter.Include` and exclude via `applyNamespaceFilters()` before any API calls. Removed old post-scan `filterByNamespace()`. |
| `cmd/root_test.go` | New file: 6 `parseCommaSeparated` tests (empty, single, multiple, spaces, trailing comma, only commas) + 7 `applyNamespaceFilters` tests (no filter, include passthrough, exclude, include wins, exclude non-existent, exclude all, empty list) |
| `cmd/init.go` | Added tip after config save: "Tip: use --namespace or --exclude-ns to filter scans at runtime" |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (482 test runs, all pass)

**2026-03-22 — Session 20: PVC pending display fix, preemption stripping, namespace column min-width**

Three fixes to Pending Pods display:

| File | What it does |
|---|---|
| `pkg/diagnosis/pending.go` | Fix 1: PVC suggestion now replaces message entirely instead of appending — `pvcSuggestion` check runs first, if non-empty sets `detail["message"]` and skips scheduling message parsing. Fix 2: Preemption suffix stripped at top of `Classify()` via `stripPreemption(p.Message)` before `classifyPendingMessage()` or `parseSchedulingMessage()`. |
| `pkg/output/table.go` | Fix 3: Namespace column gets fixed `Width(30)` via column-aware `StyleFunc(row, col)` — prevents wrapping regardless of adjacent column content. Added `nsColWidth = 30` constant. |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (467 test runs, all pass)

**2026-03-22 — Session 19: 10 new error pattern classifiers**

Added classifiers for 10 real-world K8s error patterns:

| File | What it does |
|---|---|
| `pkg/diagnosis/container.go` | New file: `classifyMountError()` (ConfigMap/Secret not found, NFS timeout, CSI error), `classifyConfigError()` (missing key in ConfigMap/Secret, invalid env var), `classifyRunError()` (executable not found, missing binary, OCI runtime), `classifyImageNameError()` (unrendered Helm vars, invalid tag format), `classifyEviction()` (ephemeral storage, memory/disk pressure), `classifyFailedCreate()` (quota exceeded, admission webhook denied), `classifyProbeFailure()` (liveness/readiness/startup: 5xx, connection refused, status codes, timeout), `classifySandboxError()` (cgroup, CNI IP exhaustion, CNI plugin, failed sandbox). `ContainerErrorClassifier` catches pod-level CreateContainerConfigError/RunContainerError/InvalidImageName/Evicted. |
| `pkg/diagnosis/container_test.go` | 67 tests: 8 mount, 5 config, 7 run, 5 image name, 5 eviction, 7 failed create, 10 probe, 6 sandbox, 5 integration, all with empty-input cases |
| `pkg/diagnosis/events.go` | Reason-based dispatch in `Classify()`: FailedMount/Unhealthy/FailedCreate/FailedCreatePodSandBox/Evicted → specialized classifiers before generic path |
| `pkg/diagnosis/events_test.go` | 5 new reason-dispatch tests |
| `pkg/diagnosis/pending.go` | `topologySpreadRe` + classification: "N nodes: topology spread violated" |
| `pkg/diagnosis/pending_test.go` | 1 new topology spread test |
| `pkg/diagnosis/job.go` | `classifyJobFailure()`: BackoffLimitExceeded → "hit retry limit", DeadlineExceeded → "timed out", includes failed count |
| `pkg/diagnosis/job_test.go` | 6 new `classifyJobFailure` tests |
| `pkg/kube/pods.go` | Evicted pod detection (`Phase==Failed && Reason=="Evicted"`), `RunContainerError` added to `unhealthyWaitingReasons` |
| `cmd/root.go` | `ContainerErrorClassifier{}` registered in classifier list |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (467 test runs, all pass)

**2026-03-22 — Session 18: Compound scheduling message parser for Pending pods**

Added `parseSchedulingMessage()` to split compound K8s scheduling messages into structured, classified reasons:

| File | What it does |
|---|---|
| `pkg/diagnosis/pending.go` | `SchedulingReason` struct (Count, Kind, Summary). `parseSchedulingMessage()`: strips preemption suffix, splits by comma, classifies each reason (taint/affinity/resource/autoscaler/other). Taint classification: CriticalAddonsOnly, GPU role, CPU role, not-ready, unreachable, generic. Resource: cpu, memory, nvidia.com/gpu, nvidia.com/mig-*. Autoscaler: max node group, no scale-up. KeyVault CSI errors. `formatSchedulingReasons()`: single=plain text, multiple=bulleted list sorted by Count desc. `Classify()` uses parsed reasons for the `message` detail field. |
| `pkg/diagnosis/pending_test.go` | 20 new tests: single taint, compound 5 reasons, GPU taint, CriticalAddonsOnly, affinity only, mixed resources+taints, KeyVault CSI, Docker Desktop, empty/garbage, strip preemption, GPU resource, MIG slice, autoscaler max, no scale-up, not-ready taint, format single/multiple/empty, nodeWord, integration compound message |
| `pkg/output/table.go` | Removed `wrapText()` from Pending Reason column — shows structured multi-line output |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (386 test runs, all pass)

**2026-03-21 — Session 17: Three fixes — PVC dedup, Why column no-truncate, client-go warning suppression**

Three small fixes:

| File | What it does |
|---|---|
| `pkg/diagnosis/pending.go` | Fix 1: Replaced `checkMissingPVCs()` (created separate `pvc_not_found` findings) with `pvcSuggestion()` that folds PVC hint into the main pending finding's `message` detail field. No more duplicate rows for the same pod. |
| `pkg/diagnosis/pending_test.go` | Updated `MissingPVC_WithSuggestion` and `MissingPVC_NoSuggestion` tests: now expect 1 finding (not 2), check PVC info in `message` field |
| `pkg/output/table.go` | Fix 2: Removed `wrapText()` from Warning Events Why column — renders OneLiner at full length, terminal handles natural wrapping |
| `cmd/root.go` | Fix 3: Added `rest.SetDefaultWarningHandler(rest.NoWarnings{})` in `init()` to suppress client-go deprecation warnings (e.g. "v1 Endpoints is deprecated") |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (366 test runs, all pass)

**2026-03-21 — Session 16: guessImagePullCause — tag heuristics for no-detail image pull events**

Added `guessImagePullCause(image, hasDetailEvent)` for when events lack detailed error messages:

| File | What it does |
|---|---|
| `pkg/diagnosis/events.go` | `guessImagePullCause()` with 4-branch tag heuristics: (1) nonsense tags (doesnotexist/fake/test123/etc.) → "Likely bad tag", (2) Levenshtein ≤ 2 from "latest" → "Likely typo: ... did you mean 'latest'?", (3) semver/well-known (latest/alpine/slim/stable/lts) → "Registry unreachable ... original error expired (>1h)", (4) fallback → "Pull failed ... original error expired". `extractTag()` handles registry:port/image:tag. Classifier Classify() calls `guessImagePullCause` when no signal found + image available. `classifyEventMessage` ImagePullBackOff/ErrImagePull branch now delegates to `guessImagePullCause`. |
| `pkg/diagnosis/events_test.go` | 10 classifier tests (added nonsense-tag and typo-tag integration tests), 20 classifyEventMessage tests, 4 extractImageFromMessage tests, 18 guessImagePullCause tests (all 4 branches + edge cases), 6 extractTag tests |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (366 test runs, all pass)

**2026-03-21 — Session 15: Warning Events fixes — signal-based dedup + image name in Why column**

Two fixes to the Warning Events classifier:

| File | What it does |
|---|---|
| `pkg/diagnosis/events.go` | Fix 1: Replaced reason-priority dedup with signal-based selection — `groupEventsByObject()` keeps all events, `bestEventForObject()` picks the one whose message has diagnostic signal (manifest unknown, 401, etc.); `collectImageFromGroup()` extracts image from any event in the group. Fix 2: `extractImageFromMessage()` parses `pull image "img:tag"` patterns; `classifyEventMessage()` now takes image param and includes it in image-pull-related outputs (e.g. "Tag not found: alpine:lates — verify tag exists"). |
| `pkg/diagnosis/events_test.go` | 8 classifier tests + 20 `classifyEventMessage` tests + 4 `extractImageFromMessage` tests |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (339 test runs, all pass)

**2026-03-21 — Session 14: Warning Events refactor — deduplication, message classification, table columns**

Refactored Warning Events to reduce noise and show actionable information:

| File | What it does |
|---|---|
| `pkg/output/table.go` | Warning Events columns changed from `Namespace | Object | Reason | Message` to `Namespace | Object | Category | Why`; rowFn simplified to use OneLiner directly |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (328 test runs, all pass)

**2026-03-21 — Session 13: wrapText for long table columns + init fallback guard**

Fixed horizontal overflow in table columns containing long free-text from K8s events/logs, and added guard for init wizard fallback path:

| File | What it does |
|---|---|
| `pkg/output/table.go` | Added `wrapText(s, maxWidth)` helper: breaks at word boundary before maxWidth, hard-breaks if no spaces; `wrapWidth = 80` const; applied to Pending Reason, Warning Event Message, CrashLoop Root Cause columns |
| `pkg/output/output_test.go` | 5 new `TestWrapText` subtests: empty string, short (no wrap), exact boundary, word break, no-spaces hard break |
| `cmd/init.go` | Added guard before save: if `len(cfg.Environments) == 0` after wizard, returns clear error message |
| `pkg/config/detect_test.go` | 3 new tests: `TestDetectEnvironments_DockerDesktop`, `TestBuildManualConfig_DockerDesktop`, `TestBuildManualConfig_NoClustersSelected` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (309 test runs, all pass)

**2026-03-21 — Session 12: PendingClassifier missing-PVC detection with Levenshtein typo suggestions**

Enhanced PendingClassifier to detect pods stuck Pending due to missing PVC references, with typo correction suggestions:

| File | What it does |
|---|---|
| `pkg/kube/pods.go` | Added `VolumeClaimNames []string` to `PodIssue`; `extractVolumeClaimNames()` pulls PVC claim names from pod spec volumes for Pending pods |
| `pkg/kube/resources.go` | Added `ListPVCNames()` — returns all PVC names in a namespace (all phases) |
| `pkg/kube/resources_test.go` | 2 new tests: `TestListPVCNames_Empty`, `TestListPVCNames_ReturnAll` |
| `pkg/diagnosis/classifier.go` | Added `AllPVCNames map[string][]string` to `ScanResults` |
| `pkg/diagnosis/pending.go` | `checkMissingPVCs()` emits findings for PVCs referenced by pod but not existing in namespace; `closestPVCName()` suggests typo corrections within Levenshtein distance ≤ 2; `levenshtein()` single-row DP implementation |
| `pkg/diagnosis/pending_test.go` | 5 new tests: MissingPVC_WithSuggestion, MissingPVC_NoSuggestion, PVCExists_NoExtraFinding, TestLevenshtein (10 cases), TestClosestPVCName (6 cases) |
| `cmd/root.go` | Wired `ListPVCNames()` call in scan loop; initialized `AllPVCNames` map in `ScanResults` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (300 test runs, all pass)

**2026-03-21 — Session 11: FEAT-SLACK — Slack integration for scan summaries**

Full Slack notification system:

| File | What it does |
|---|---|
| `pkg/config/config.go` | `SlackConfig` + `NotificationsConfig` structs; mode constants (`webhook`/`bot_token`); severity constants (`all`/`high`/`critical`); validation for slack fields |
| `pkg/notifications/slack.go` | `FormatSummary()` builds Block Kit message (header, env counts, top 5 findings, footer); `TestConnection()` sends test message; `SendSummary()` posts after scan with severity filter + on_issues_only; `classifySlackError()` maps HTTP/API errors to user-friendly guidance (401/403, channel_not_found, not_in_channel, missing_scope, invalid_payload, fallback); injectable `HTTPClient` interface |
| `pkg/notifications/slack_test.go` | 27 tests: FormatSummary (5 cases), FormatTestMessage, classifySlackError (7 cases), mock HTTP integration (webhook success, bot token success, webhook failure, bot API error, disabled, on_issues_only, posts with findings), filterBySeverity (4 cases), min_severity filter integration |
| `cmd/slack.go` | `klarity slack setup` — interactive wizard: mode select → credential input → test message → severity filter → on_issues_only → save config |
| `cmd/root.go` | Wired `SendSummary()` after scan render; errors non-fatal (stderr); `countConfigClusters()` helper |
| `cmd/config.go` | `config show` displays slack settings when enabled |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (293 test runs, all pass)

**2026-03-21 — Session 10: FEAT-30 kubelogin version detection + AKS auth guidance**

Added advisory kubelogin version detection for AKS users:

| File | What it does |
|---|---|
| `pkg/kube/client.go` | `DetectKubeloginVersion()` runs `kubelogin --version`, `parseKubeloginVersion()` extracts semver, `KubeloginVersion.AtLeast()` for comparison, `CheckKubeloginVersion()` returns warning string if >= 0.1.19 |
| `pkg/kube/kubelogin_test.go` | 17 tests: version parsing (7 cases), AtLeast comparison (6 cases), threshold checks (2 cases), subtests |
| `cmd/root.go` | Calls `CheckKubeloginVersion()` before scanning, prints warning to stderr |
| `cmd/init.go` | `hasKubeloginExec()` checks kubeconfig auth infos for exec credential with `kubelogin` command; shows warning during onboarding if detected |
| `README.md` | New "AKS / Azure CLI Authentication" section with version guidance, pin commands, download links |

Design: entirely advisory — never fatal, never blocks scanning. Only `azurecli` mode affected; `spn` and `workloadidentity` are fine. Warning printed to stderr so it doesn't pollute `--output json`.

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (266 test runs, all pass)

**2026-03-21 — Session 9: Full codebase audit + bug fixes**

Comprehensive audit across 8 dimensions (correctness vs spec, critical rules, K8s logic, concurrency, error handling, test coverage, output, config). Found and fixed:

| Priority | Issue | Fix |
|---|---|---|
| C1 | 8 missing classifiers — scan data for NoEndpoints/Quota/PVC/DaemonSet/StatefulSet/Job/CronJob/Event was collected but never classified | Created 8 classifier files + 8 test files; registered all 13 classifiers in `cmd/root.go` |
| H1/M4 | Pending pods had empty Message — PendingClassifier always returned `unknown` subtype | Added `pendingMessage()` to extract scheduling reason from Pod conditions |
| H2 | `filterByNamespace`/`filterByCategory` used `findings[:0]` trick, mutating shared backing array | Changed to fresh `var out []diagnosis.Finding` |
| H3 | `config show` swallowed all config load errors as "no config found" | Only treat `os.ErrNotExist` as missing config; propagate other errors |
| M1 | WarningEvent had 4 headers but `genericRow` returned 3 columns | Custom rowFn extracting object_name, reason, message from DetailFields |
| M5 | `exclude_completed_jobs` config setting was never checked | Added `excludeCompleted` param to `ListFailedJobs`; skips jobs with `CompletionTime` |
| M6 | Duplicate favicon `<link>` tags in `website/index.html` | Removed duplicate block |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (249 test runs, all pass)

**2026-03-21 — Session 8: FEAT-24–26 + golangci-lint**

Implemented all remaining CLI polish features:

| Feature | File | What it does |
|---|---|---|
| FEAT-24: Watch mode | `cmd/root.go` | `--watch` loops scan→render→sleep; `--interval N` overrides config; `signal.NotifyContext` for Ctrl-C; `\033[2J\033[H` clear |
| FEAT-25: Filters | `cmd/root.go` | `--context`, `--namespace/-n`, `--category` (alias map: oom/crashloop/imagepull/…); applied pre-scan + post-scan |
| FEAT-26: Config show | `cmd/config.go` | `klarity config show` (pretty-printed human-readable), `klarity config path` |

Also fixed `golangci-lint` issue: ineffectual assignment in `pkg/diagnosis/pending.go` (initial `oneLiner` value was immediately overwritten). Changed to `var oneLiner string` + conditional assignment.

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (168 tests) | ✅ `golangci-lint`

**2026-03-21 — Session 7: FEAT-20–23 + full scan wiring**

Implemented the complete output layer and wired the end-to-end scan pipeline:

| File | What it does |
|---|---|
| `pkg/output/color.go` | EnvColor (critical=red, dev=green, standard=yellow), EnvEmoji, EnvHeaderStyle, SeverityStyle, shared lipgloss vars |
| `pkg/output/table.go` | RenderReport: title banner, env headers (sorted critical-first), cluster sub-headers, category tables (catSpec map per category with icon/label/headers/rowFn), "✅ No issues found" when empty |
| `pkg/output/json.go` | RenderJSON: zero ANSI, snake_case JSON array |
| `pkg/output/summary.go` | renderFooter: per-env counts, "Next scan in Xm Ys", SummaryCounts() |
| `cmd/root.go` | Full scan: parallel WaitGroup+semaphore, BuildClientset errors collected non-fatally (both clusters shown in ⚠ list), all 11 kube scanners called per namespace, log fetch for CrashLoop, --output json\|table, --env filter, SilenceUsage on user-facing errors |

Also fixed `pkg/config/config.go`: ErrNotExist now wrapped with `%w` so `errors.Is` propagates correctly.
Added `hpa_name` to HPAClassifier DetailFields for use by table renderer.

26 new tests in `pkg/output/`; total 168 tests, all passing.
End-to-end verified: `./klarity` with fake clusters shows both cluster errors inline + correct report structure.

**2026-03-21 — Session 6: FEAT-18 + FEAT-19**

Replaced stubs in `pkg/logs/`: real FetchLogs via GetLogs stream; Summarize with 7-priority language-aware extraction (Java/Python/Go/Generic/ConnErr/Auth/Fallback). 26 new tests.

**2026-03-21 — Session 5: FEAT-12 through FEAT-17 + stubs**

Full diagnosis layer: OOMClassifier, ImagePullClassifier, CrashLoopClassifier, PendingClassifier, HPAClassifier. All with table tests. 14 new tests.

**2026-03-21 — Session 4: FEAT-04 through FEAT-11**

Full scanner layer in pkg/kube/: pods, deployments, hpa, services, events, resources, daemonsets, statefulsets, jobs, namespaces. 44 new tests.

## Decisions Made

- All scanners take a single namespace string; `ResolveNamespaces()` called by scan loop
- `QuotaThreshold = 80.0` — exported constant
- OOMKilled emitted as separate PodIssue alongside CrashLoopBackOff
- `autoscaling/v2` for HPAs (k8s 1.23+)
- `ScanResults` is flat (one per cluster); EnvName + ClusterCtx set by scan loop
- `PendingClassifier` has injectable `Now func() time.Time` for tests
- `FetchLogs` stays in `pkg/logs/` (not moved to `pkg/kube/`)
- lipgloss table used (not tablewriter); `charmbracelet/lipgloss/table`
- Scan loop uses WaitGroup+semaphore (not ScanAll/errgroup) so BuildClientset errors don't abort entire scan
- Scan errors from BuildClientset shown in report body as ⚠ warnings (not on stderr)

## Open Questions / Things To Resolve

_All resolved. Previous questions and their resolutions:_
- Watch mode: ANSI clear (`\033[2J\033[H`) — decided, implemented
- `--category` filter: takes alias names (e.g. "oom", "crashloop") — decided, implemented
- `config show`: pretty-formatted (not raw YAML) — decided, implemented

## Marketing Website

`website/index.html` + `website/style.css` — static site for S3 + CloudFront. Not part of Go module.
Favicon installed: `<link rel="icon">` tags added to `<head>` in index.html.

## Session Protocol

When starting a new session:
1. Read this file (HANDOFF.md)
2. Read REMEMBER.md for full project state
3. Run `go build -o klarity .` to verify current state compiles
4. Run `go test ./...` to verify tests pass
5. Pick up from "What To Do Next" above

When ending a session:
1. Run `go build` and `go test` — make sure everything passes
2. Update REMEMBER.md: mark completed features, update "Last updated" and "Last session focus"
3. Update this file: move completed items out of "Next Steps", add new next steps, write session summary
4. Commit all changes including doc updates
