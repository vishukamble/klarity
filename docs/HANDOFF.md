# HANDOFF.md — Session Continuity

> **IMPORTANT:** Claude Code MUST read this file at the start of every session. Before ending a session, update this file with what to do next.

## What To Do Next

All core features implemented. All audit bugs fixed. Bubbletea TUI shipped. Cache UX polished.

### High priority

- **`klarity uninstall` confirmation prompt** — `cmd/uninstall.go` still lacks interactive confirmation before deleting files; add a `huh.Confirm` prompt before any removal.
- **TUI "Edit groupings" path** — `cmd/init_tui.go` shows "edit not yet available" when user presses `e` in phase 0. Wire up `runFallbackPath` (kept in `cmd/init.go`) as the edit path, or implement inline editing in the TUI.

### Done this session (v1.1.4)

- ✅ `cmd/root.go` — `flagRescan bool` added; `--rescan` flag bypasses cache (sets `filteredScan=true`); `cacheTTLMinutes=5` constant; stale-TTL check nulls `cachedData` before hit check; cached banner changed to `"(from %s ago · --rescan to force fresh)"`
- ✅ `pkg/output/summary.go` — removed "Next scan in Xm Ys (--interval N)" countdown from `renderFooter`; footer now shows only per-env summary counts
- ✅ `cmd/root_test.go` — 3 new tests: `TestCacheTTL_StaleSkipped`, `TestCacheTTL_FreshUsed`, `TestRescanFlag`
- ✅ Version bumped to `1.1.4`

### Done previous session (v1.1.3)

- ✅ `cmd/init_tui.go` — new file: `initTUIModel` (5-phase Bubbletea TUI); `runNewWizardTUI` replaces `runNewWizard`; `tea.WithAltScreen()`
- ✅ `cmd/config_tui.go` — new file: `configMenuModel` single-screen menu; `tea.ExecProcess` for inline editor; inline default-env selector
- ✅ `cmd/config.go` — `RunE: runConfigMenu` added to configCmd; `runConfigMenu()` launches TUI and dispatches post-action
- ✅ `cmd/init.go` — removed dead helpers (`runNewWizard`, `promptDefaultEnv`, `detectedToEnvs`, `selectedToEnvs`, `totalConfigClusters`); `runInit` now calls `runNewWizardTUI`; nil-cmd guard added
- ✅ `cmd/init_tui_test.go` — 13 unit tests (phase advance/back/skip, tier toggle, quit, input active, window size)
- ✅ Version bumped to `1.1.3`

### Done previous session (v1.1.2)

- ✅ C1: `cmd/root.go` — `applyNamespaceFilters` now uses `filepath.Match` for `--exclude-ns` patterns (glob wildcards actually work now)
- ✅ H1: `cmd/root.go:218` — `filteredScan` now includes `flagExcludeNs != ""`; cache bypassed when `--exclude-ns` is the only filter
- ✅ H2: `pkg/output/table.go` — HPA `tgtVal > 0` guard prevents `+Inf×` in CPU column when target is 0
- ✅ H3: `pkg/notifications/slack.go` — Slack block text truncated at rune 2900, not byte 2900; safe for multi-byte UTF-8
- ✅ M1: `cmd/root.go` — `showDefaultEnvBanner` uses `len([]rune(line1))` for box width; em dash no longer causes 2-char misalignment
- ✅ M2: `pkg/diagnosis/container.go` — `ContainerErrorClassifier` adds `"object_kind": "Pod"` to DetailFields; Kind column no longer shows "-"
- ✅ L1: `pkg/diagnosis/events.go` — `bestEventForObject` guards against empty slice
- ✅ L2: `pkg/config/config.go` — `Validate()` rejects negative `parallel_namespaces`
- ✅ Version bumped to `1.1.2`

### Done previous session (v1.1.1)

- ✅ BUG-01: `cmd/uninstall.go` now calls `cache.DefaultPath()` / `cache.LogPath()` instead of hardcoded paths
- ✅ BUG-02: `pkg/diagnosis/events.go` — `updateFailedKeyRe` extracts key path from `(key: path)` messages
- ✅ UX-01 through UX-05: init defaults, noendpoints UX, WarningEvent Kind column, flag descriptions, UUID service filter

### Reminder: validation order after each change
```
go build -o klarity .   # must compile
go vet ./...            # must pass
go test ./...           # must pass
```

## How the Full Pipeline Fits Together

```
klarity (non-watch, table mode)
  → --history N  →  showHistory() reads ~/.klarity.log, exits
  → load ~/.klarityconfig.yaml
  → apply default_env if set (showDefaultEnvBanner + filterByEnv)
  → warn if >10 clusters and no default_env
  → pkg/cache.Load(~/.klarity_cache)
      stale (age ≥ 5 min) OR --rescan → treat as miss
      hit  → RenderReport(cached findings) + "(from Xs ago · --rescan to force fresh)"
           → gatherFindings() in background goroutine
           → cache.Equal() → "✓ Still current" OR clearScreen + re-render
      miss → doScan() → gatherFindings() → render
  → cache.Save(~/.klarity_cache)  +  cache.AppendLog(~/.klarity.log)

klarity --watch
  → loop: clearScreen → doScan() → sleep(interval)
  → doScan() writes cache + log after each iteration
  → Ctrl-C exits cleanly

gatherFindings()
  → parallel goroutines per env×cluster (semaphore = parallel_clusters)
    → BuildClientsetWithRateLimit(context, api_qps, api_burst)
    → ResolveNamespaces(filter, cfg.Settings.DefaultNsExclude) → []string
    → MatchNamespaces(--namespace patterns, live list)
    → parallel goroutines per namespace (semaphore = parallel_namespaces, default 10)
      → for each ns: ListUnhealthyPods/Deployments/HPAs/Services/Events/
                     Quotas/PVCs/DaemonSets/StatefulSets/Jobs/CronJobs
      → FetchLogs for CrashLoop pods → Summarize → PodIssue.LogSummary
      → ListPVCNames → AllPVCNames[ns]
      → merge under nsMu mutex into shared ScanResults
    → ListUnhealthyNodes (cluster-wide, outside namespace loop)
    → ScanResults{EnvName, ClusterCtx, all scanner outputs, AllPVCNames}
    → RunAll(results, classifiers) → []Finding
```

## Context You'll Need

- `cmd/root.go` — full scan orchestration; `gatherFindings()` / `doScan()` split; cache/log integration; `--history` flag
- `pkg/cache/` — `cache.go` (Cache, Load, Save, Equal) + `log.go` (LogEntry, AppendLog, ReadLog, FilterLog)
- `pkg/output/` — `RenderReport`, `RenderJSON`, `SummaryCounts`; do NOT call lipgloss from JSON path
- `pkg/diagnosis/classifier.go` — Finding, Classifier interface, RunAll
- CLAUDE.md: classifiers return data, output layer is only formatter; never mutate K8s resources

## Previous Session Summary

**2026-04-02 — Session 42: Cache UX polish (v1.1.4)**

| Item | Files | Summary |
|---|---|---|
| `--rescan` flag | `cmd/root.go` | `flagRescan bool`; added to `filteredScan`; bypasses cache |
| 5-min TTL | `cmd/root.go` | `cacheTTLMinutes=5`; stale cache nulled before hit check |
| cached banner | `cmd/root.go` | `"(cached X ago, scanning...)"` → `"(from X ago · --rescan to force fresh)"` |
| footer cleanup | `pkg/output/summary.go` | Removed "Next scan in Xm Ys" countdown; footer = summary counts only |
| tests | `cmd/root_test.go` | 3 new tests: `TestCacheTTL_StaleSkipped`, `TestCacheTTL_FreshUsed`, `TestRescanFlag` |
| Version | `cmd/root.go` | `1.1.3` → `1.1.4` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

**2026-04-01 — Session 41: Full-screen Bubbletea TUI (v1.1.3)**

| Item | Files | Summary |
|---|---|---|
| init TUI | `cmd/init_tui.go` (new) | `initTUIModel` — 5-phase wizard (Groupings/Assign/Tiers/Default/Save), `tea.WithAltScreen()`, phase pills, tip banner, tier toggle |
| config TUI | `cmd/config_tui.go` (new) | `configMenuModel` — single-screen menu, `tea.ExecProcess` for $EDITOR, inline default-env selector |
| init wiring | `cmd/init.go` | `runInit` calls `runNewWizardTUI`; removed `runNewWizard`, `promptDefaultEnv`, `detectedToEnvs`, `selectedToEnvs`, `totalConfigClusters` |
| config wiring | `cmd/config.go` | `RunE: runConfigMenu` added; `runConfigMenu()` launched by `tea.NewProgram`; dispatches show/validate/reinit after TUI exits |
| tests | `cmd/init_tui_test.go` (new) | 13 tests: phase advance, skip-unmatched, back, tier toggle, quit, input-active, window size, helper funcs |
| Version | `cmd/root.go` | `1.1.2` → `1.1.3` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

**2026-04-01 — Session 40: Audit bug fixes (v1.1.2)**

| Item | Files | Summary |
|---|---|---|
| C1 | `cmd/root.go` | `applyNamespaceFilters` uses `filepath.Match` — `--exclude-ns` globs work |
| H1 | `cmd/root.go:218` | `filteredScan` includes `flagExcludeNs` — cache bypassed on `--exclude-ns` |
| H2 | `pkg/output/table.go` | `tgtVal > 0` guard in HPA row — prevents `+Inf×` display |
| H3 | `pkg/notifications/slack.go` | Slack block text truncated at rune 2900, not byte 2900 |
| M1 | `cmd/root.go` | `showDefaultEnvBanner` uses rune count for box width |
| M2 | `pkg/diagnosis/container.go` | `ContainerErrorClassifier` adds `object_kind: "Pod"` |
| L1 | `pkg/diagnosis/events.go` | `bestEventForObject` guards against empty slice |
| L2 | `pkg/config/config.go` | `Validate()` rejects negative `parallel_namespaces` |
| Tests | multiple | 7 new tests covering all fixes |
| Version bump | `cmd/root.go` | `1.1.1` → `1.1.2` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

**2026-04-01 — Session 39: Docs condensation + bug/UX fixes (v1.1.1)**

| Item | Files | Summary |
|---|---|---|
| Docs condensation | `docs/REMEMBER.md`, `docs/HANDOFF.md` | REMEMBER.md condensed to <15k chars; HANDOFF.md to <12k chars |
| BUG-01 | `cmd/uninstall.go` | Use `cache.DefaultPath()` / `cache.LogPath()` instead of hardcoded paths |
| BUG-02 | `pkg/diagnosis/events.go`, `events_test.go` | `updateFailedKeyRe` extracts key path from UpdateFailed messages |
| UX-01 | `cmd/init.go` | All `huh.NewConfirm()` prompts default to `true` |
| UX-02 | `pkg/diagnosis/noendpoints.go` | OneLiner = `"no ready endpoints"` (removed redundant service name prefix) |
| UX-03 | `pkg/output/table.go` | WarningEvent table adds Kind column (uses `object_kind` DetailField) |
| UX-04 | `cmd/root.go` | `--namespace` / `--exclude-ns` descriptions mention glob wildcard quoting |
| UX-05 | `pkg/diagnosis/noendpoints.go`, `noendpoints_test.go` | `isAutoGeneratedServiceName()` skips UUID-derived service names; 3 new tests |
| Version bump | `cmd/root.go` | `1.1.0` → `1.1.1` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (all pass)

**2026-03-31 — Session 38: Wildcard namespace matching + local cluster detection (v1.1.1)**

| Item | Files | Summary |
|---|---|---|
| `MatchNamespaces` | `pkg/kube/namespaces.go` | filepath.Match semantics; dedup by candidate order; passthrough on empty patterns; error on malformed pattern |
| `scanCluster` wiring | `cmd/root.go` | Always resolve live namespace list first; then `MatchNamespaces` filters by `--namespace` patterns |
| `matchLocalCluster` | `pkg/config/detect.go` | Strategy 0; minikube/kind/docker-desktop/rancher-desktop/k3d- → "dev-local"; 13 new tests |
| `klarity update` | `cmd/update.go` | GitHub API → version compare → tarball download + tar/gzip extract → atomic `os.Rename`; injectable `updateHTTPClient` |
| `init --reset` backup | `cmd/init.go` | Copies config to `.bak` before wizard; `cfgPath` scope unified |
| HPA column rename | `pkg/output/table.go` | "CPU Now" → "CPU % of Target"; multiplier when > 200% (`"X% (Y.Y×)"`); red cell when > 150% |
| HPA OneLiner | `pkg/diagnosis/hpa.go` | `≥ 2.0×` overshoot → "CPU at Y.Y× target"; else existing format |
| Version bump | `cmd/root.go` | `1.1.0` → `1.1.1` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (313 tests)

**2026-03-26 — Session 37: Low-priority audit cleanup (v1.1.0)**

| Item | Fix |
|---|---|
| [L1] Deprecated `.Copy()` | `criticalStyle.Copy().Padding(0,1)` → `criticalStyle.Padding(0,1)` in `pkg/output/envtable.go` |
| [L2] Delete dead `ScanAll` | Deleted `ScanFunc`, `ScanAll`, `pkg/kube/client_test.go`; removed `errgroup`/`context`/`config` imports |
| [L3] `RenderEnvTable` tests | 7 tests added in `pkg/output/envtable_test.go` |
| [L4] Redundant `cfg.Validate()` | Removed from `runConfigEdit` — `config.Load()` already validates |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (270 tests)

**2026-03-26 — Session 36: Audit bug fixes (v1.1.0)**

| Bug | Files | Fix |
|---|---|---|
| [C1] Uninstall wrong cache path | `cmd/uninstall.go:45` | `~/.klarity_cache.json` → `~/.klarity_cache` |
| [C1] Uninstall wrong log path | `cmd/uninstall.go:54` | `~/.klarity_log.json` → `~/.klarity.log` |
| [H1] InvalidImageName duplicate | `pkg/diagnosis/imagepull.go:21` | Removed from `imagePullReasons`; `ContainerErrorClassifier` is sole handler |
| [M1] Non-existent `--include-system` | `pkg/output/table.go:331` | Changed hint to valid `--namespace kube-system` |
| [M3] `wrapText` byte vs rune | `pkg/output/table.go:420` | Switched to `[]rune(s)` indexing |
| Version bump | `cmd/root.go` | `1.0.9` → `1.1.0` |

Build: ✅ `go build` | ✅ `go vet` | ✅ `go test` (277 tests)

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
