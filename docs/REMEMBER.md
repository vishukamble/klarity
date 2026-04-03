# REMEMBER.md — klarity Project State

> **IMPORTANT:** Claude Code MUST read this file at the start of every session. After completing any feature or making significant changes, update this file to reflect the current state.

## Current State

**Last updated:** 2026-04-02
**Last session focus:** --rescan flag, 5-min cache TTL, honest footer — v1.1.4
**Build status:** `go build` ✅ | `go vet` ✅ | `go test` ✅ (all pass)

## Feature Tracker

- [x] **FEAT-01:** Config schema — `pkg/config/config.go`, Load/Save/Validate
- [x] **FEAT-02:** Onboarding wizard (`klarity init`) — `cmd/init.go` + `cmd/init_tui.go`, full-screen Bubbletea TUI, 5-phase wizard (Groupings/Assign/Tiers/Default/Save); `runFallbackPath` kept for future manual assignment path
- [x] **FEAT-36:** `klarity config` interactive menu — `cmd/config_tui.go`, full-screen Bubbletea; Show/Edit/$EDITOR (ExecProcess)/Validate/ChangeDefault/Reinit; inline default-env selector
- [x] **FEAT-03:** Multi-context client factory — `pkg/kube/client.go`, BuildClientset/BuildClientsetWithRateLimit
- [x] **FEAT-04:** Pod scanner — `pkg/kube/pods.go`, CrashLoopBackOff/ImagePullBackOff/OOMKilled/Pending/Evicted
- [x] **FEAT-05:** Deployment scanner — `pkg/kube/deployments.go`, unavailableReplicas
- [x] **FEAT-06:** HPA scanner — `pkg/kube/hpa.go`, at-ceiling/ScalingLimited/CPU utilization
- [x] **FEAT-07:** Service scanner — `pkg/kube/services.go`, selector with no ready endpoints
- [x] **FEAT-08:** Event collector — `pkg/kube/events.go`, Warning + Normal/BackOff events, 15min window, dedup
- [x] **FEAT-09:** Resource quota scanner — `pkg/kube/resources.go`, ListQuotaIssues/ListPendingPVCs/ListPVCNames
- [x] **FEAT-10:** DaemonSet/StatefulSet scanner — `pkg/kube/daemonsets.go` + `statefulsets.go`
- [x] **FEAT-11:** Job/CronJob scanner — `pkg/kube/jobs.go`, ListFailedJobs/ListSuspendedCronJobs
- [x] **Namespace resolver** — `pkg/kube/namespaces.go`, ResolveNamespaces + MatchNamespaces (filepath.Match wildcards)
- [x] **FEAT-12:** Classifier interface — `pkg/diagnosis/classifier.go`, Finding/ScanResults/Classifier/RunAll (13 categories)
- [x] **FEAT-13:** OOM classifier — `pkg/diagnosis/oom.go`, Critical severity
- [x] **FEAT-14:** Image pull classifier — `pkg/diagnosis/imagepull.go`, subtypes + guessImagePullCause tag heuristics
- [x] **FEAT-15:** CrashLoop classifier — `pkg/diagnosis/crashloop.go`, LogSummary + probe-killed detection (looksLikeCleanExit)
- [x] **FEAT-16:** Pending classifier — `pkg/diagnosis/pending.go`, compound scheduling parser + PVC Levenshtein suggestions
- [x] **FEAT-17:** HPA classifier — `pkg/diagnosis/hpa.go`, AtCeiling/ScalingLimited + CPU multiplier (≥2× overshoot)
- [x] **FEAT-18:** Log tailer — `pkg/logs/parser.go`, FetchLogs via client-go GetLogs
- [x] **FEAT-19:** One-line summarizer — `pkg/logs/summarizer.go`, Java/Python/Go/generic language-aware extraction
- [x] **FEAT-20:** Table renderer — `pkg/output/table.go`, lipgloss/table, catSpec map, wrapText (rune-safe), kube-system hint
- [x] **FEAT-21:** Color/tier theming — `pkg/output/color.go`, critical=red/dev=green/standard=yellow
- [x] **FEAT-22:** JSON output — `pkg/output/json.go`, structured `{scan_time, environments, summary}`, no ANSI
- [x] **FEAT-23:** Summary footer — `pkg/output/summary.go`, per-env counts + scan interval
- [x] **FEAT-24:** Watch mode — `--watch`/`--interval`, signal.NotifyContext, clearScreen
- [x] **FEAT-25:** Filters — `--namespace/-n`, `--exclude-ns`, `--context`, `--category` (comma-separated, pre-scan)
- [x] **FEAT-26:** Config show/path — `cmd/config.go`, pretty-print + path subcommand
- [x] **FEAT-27:** Node scanner — `pkg/kube/nodes.go` + `pkg/diagnosis/nodes.go`, 5 conditions, renders first
- [x] **FEAT-28:** `klarity config edit` — opens $EDITOR (fallback nano/vim/vi), validates after save
- [x] **FEAT-29:** `--version` flag — `cmd/root.go`, currently `1.1.3` ✓
- [x] **FEAT-30:** kubelogin version detection — advisory warning for >= 0.1.19 AKS azurecli mode
- [x] **FEAT-30b:** `klarity config validate` — context check against kubeconfig with ✓/⚠️
- [x] **FEAT-32:** JSON output completeness — all categories including NodeIssue
- [x] **FEAT-SLACK:** Slack — `pkg/notifications/slack.go` + `cmd/slack.go`; `klarity slack setup` wizard; `klarity slack send` (default=critical-tier, `--env`, `--all`); FormatSummary grouped env→cluster→category
- [x] **FEAT-35:** Default environment — `Settings.DefaultEnv`, `--no-default`, banner display, large-cluster warning
- [x] **Cache layer** — `pkg/cache/cache.go`, instant display + background compare, `~/.klarity_cache`
- [x] **History log** — `pkg/cache/log.go`, NDJSON at `~/.klarity.log`, `--history [N]` flag
- [x] **Container error classifiers** — `pkg/diagnosis/container.go`, 8 patterns (FailedMount, ConfigError, RunError, ImageName, Eviction, FailedCreate, Probes, Sandbox)
- [x] **Multi-strategy env detection** — `pkg/config/detect.go`, AKS/EKS/generic + matchLocalCluster (dev-local for minikube/kind/docker-desktop/k3d)
- [x] **Parallel namespace scanning** — WaitGroup+semaphore, `parallel_namespaces: 10` setting
- [x] **`klarity env` command** — `cmd/env.go` + `pkg/output/envtable.go`, RenderEnvTable, alias `ls`
- [x] **Multi-env `--env/-e`** — `filterByEnvs`, sorted multi-error for all missing names
- [x] **API rate limits** — `BuildClientsetWithRateLimit`, `api_qps`/`api_burst` settings, klog silence
- [x] **preprod keyword** — critical tier, before "prod" in envKeywords
- [x] **`klarity update` command** — `cmd/update.go`, GitHub API + tarball download + atomic binary replace
- [x] **`klarity init --reset`** — guard (exits if config exists), auto-backup to `.bak`
- [x] **Wildcard namespace matching** — `MatchNamespaces()` with filepath.Match semantics
- [x] **v1.1.2 audit fixes** — C1 glob exclude, H1 cache bypass, H2 HPA ÷0, H3 Slack UTF-8, M1 banner width, M2 object_kind, L1 empty slice guard, L2 negative parallel_namespaces validation
- [x] **v1.1.3 Bubbletea TUI** — `klarity init` now uses full-screen TUI (AltScreen); `klarity config` (no subcommand) opens interactive menu; 13 new TUI unit tests
- [x] **v1.1.4 cache UX** — `--rescan` flag forces fresh scan (bypasses cache); 5-min TTL auto-expires stale cache; footer "Next scan in X" removed; cached banner → "(from Xs ago · --rescan to force fresh)"; 3 new tests

## Architecture Decisions

1. Module path is `github.com/vishukamble/klarity` (NOT `github.com/vishu/klarity`).
2. `huh` forms require TTY; init wizard form logic not unit-tested — testable helpers (assignClusters, buildEnvironmentsFromInput) are separate from huh calls.
3. OOMKilled emitted as a separate PodIssue even when container is also in CrashLoopBackOff — classifiers correlate them.
4. `QuotaThreshold = 80.0` — quota issues reported at ≥80% usage.
5. kubelogin check is advisory only — never fatal; warns stderr for >= 0.1.19 (azurecli mode only).
6. Slack errors are non-fatal — stderr only, never block scanning or output.
7. `ListWarningEvents()` makes two API calls: `type=Warning` + `type=Normal,reason=BackOff`; BackOff events carry image names needed by `classifyImagePullGroup()`; fallback to all-Normal if cluster rejects compound selector.
8. Cache bypassed for `--output json` — ANSI-free path needs no cache headers; cache still written after live scan.
9. `cache.Equal()` sorts findings by composite key (`Category+EnvName+ClusterCtx+Namespace+PodName+OneLiner`) before JSON marshal to guard against goroutine ordering.
10. `--history` uses cobra `NoOptDefVal="10"` so `--history` alone → 10, `--history 20` → 20, absent → disabled.
11. `tierForLabel` uses `containsWord` — "prod-intel" → critical, "reproduced-cluster" → standard (no substring match).
12. `parallel_namespaces: 0` in old configs coerced to 10 at runtime (not a validation error).
13. `Settings.DefaultEnv` validated at scan time, not on config load — error surfaced only when env is absent.
14. `default_env` banner and large-cluster warning only fires when both `--env` and `--context` are absent.
15. `matchLocalCluster` is Strategy 0 before AKS/EKS/generic — minikube/kind/docker-desktop/rancher-desktop/k3d → "dev-local" (standard tier).
16. "preprod" is critical tier (checked before "prod") — it's a production-class environment.
17. `filterByEnvs` returns a sorted multi-error for all missing names at once — user sees every typo in one shot.
18. `klarity slack setup` tests connection before saving — if test fails, config is not written.
19. PVC suggestion replaces `message` entirely in Pending findings via `pvcSuggestion()` — no separate `pvc_not_found` row.
20. `ContainerErrorClassifier` handles pod-level errors (CreateContainerConfigError, RunContainerError, InvalidImageName); `EventClassifier` is sole handler for Evicted — no duplicate findings.
21. `api_qps`/`api_burst` use `omitempty` — old configs without these fields produce clean YAML; 0-values coerced to 50/100 at runtime.
22. `ResolveNamespaces()` 4th param `defaultExclude` applied only for mode=all with empty cluster exclude list; ignored for include/exclude modes.
23. `MatchNamespaces()` passthrough on empty patterns; invalid patterns return error with pattern name; dedup preserves candidate order.
24. `classifyImagePullGroup()` scans ALL events for the object (not just the "best" one) to guarantee image name extraction from BackOff events.
25. `applyNamespaceFilters()` uses `filepath.Match` for `--exclude-ns` patterns — same semantics as `MatchNamespaces()`.
26. `filteredScan` includes `flagExcludeNs != ""` — cache is bypassed when `--exclude-ns` is the only filter flag.
27. `parallel_namespaces: 0` is valid (coerced to 10 at runtime); negative values are rejected at validation.
28. Slack block text truncated at rune 2900 (not byte 2900) — safe for multi-byte characters.
29. `ContainerErrorClassifier` findings include `object_kind: "Pod"` in DetailFields.

30. `initTUIModel` uses 0-indexed phases internally (0=Groupings…4=Save); `nextPhase`/`prevPhase` helpers skip phase 1 when `len(unmatched)==0`.
31. TUI colour palette defined once in `cmd/init_tui.go` as package-level `var` blocks; `cmd/config_tui.go` uses them directly (same package, no re-import of lipgloss needed in config_tui.go).
32. `runConfigMenu()` in `cmd/config.go` owns the `tea.NewProgram` call and post-TUI dispatch (show/validate/reinit); `cmd/config_tui.go` is pure model/view.
33. `runFallbackPath` kept in `cmd/init.go` (uses huh) for future "Edit groupings" path; currently never called since TUI shows "edit not yet available" message.
34. `runInit(nil, nil)` is safe — nil cmd guard added at kubelogin warning print site; `&cobra.Command{}` passed from `runConfigMenu` reinit action.

## Known Issues / Blockers

_None._

---

**When you finish a feature:** Change `[ ]` to `[x]`, update "Last updated" and "Last session focus", and update HANDOFF.md.
