# HANDOFF.md — Session Continuity

> **IMPORTANT:** Claude Code MUST read this file at the start of every session. This tells you exactly where to pick up. Before ending a session, update this file with what to do next.

## What To Do Next

**All Phase 1–6 features are complete.** Potential future work:

1. **FEAT-27: Node scanner** — flag NotReady/MemoryPressure/DiskPressure nodes
2. **FEAT-28: Slack/webhook alerting** — post summary on findings above threshold
3. **FEAT-29: `klarity config edit`** — open config in $EDITOR
4. **Install script** — `https://getklarity.dev/install.sh` referenced in website hero
5. **GitHub Actions release pipeline** — goreleaser, homebrew tap

### How the Full Pipeline Fits Together (implemented)

```
klarity
  → load ~/.klarityconfig.yaml
  → parallel goroutines per env×cluster (semaphore = parallel_clusters)
    → BuildClientset(context) — errors collected non-fatally
    → ResolveNamespaces(filter) → []string
    → for each namespace: ListUnhealthyPods/Deployments/HPAs/Services/Events/
                          Quotas/PVCs/DaemonSets/StatefulSets/Jobs/CronJobs
    → FetchLogs for CrashLoop pods → Summarize → PodIssue.LogSummary
    → ScanResults{EnvName, ClusterCtx, all scanner outputs}
    → RunAll(results, classifiers) → []Finding
  → pkg/output.RenderReport (table) or RenderJSON (--output json)
```

### Context You'll Need

- `cmd/root.go` — full scan orchestration, `--output`/`--env` flags, `scanCluster()`, `filterEnv()`
- `pkg/output/` — `RenderReport`, `RenderJSON`, `SummaryCounts`; do NOT call lipgloss from JSON path
- `pkg/diagnosis/classifier.go` — Finding, Classifier interface, RunAll
- CLAUDE.md: classifiers return data, output layer is only formatter; never mutate K8s resources

## Previous Session Summary

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
