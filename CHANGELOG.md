# Changelog

## v1.0.4 — 2026-03-23

### Fixed
- `klarity init`: Single-cluster environments now auto-select without requiring manual space-toggle
- `klarity init`: Empty cluster selection now re-prompts instead of fatal exit

### Added
- Instant cache display — `klarity` shows last scan results immediately, refreshes in background
- Persistent scan log — every scan appended to `~/.klarity.log`
- `--history` flag — `klarity --history` shows last 10 scans, `--history 20` for more, `--history --env prod` to filter
- Classifier: Go structured log pattern `"command failed" err=[missing flags]` now produces clean one-line summary
- Hint when `kube-system` is excluded and control plane pods may be unhealthy

### Internal
- 260 tests passing
- `pkg/cache/` — new cache and log packages with full test coverage

## v1.0.3 — prior release

### Initial public release
- Multi-cluster scanning via kubeconfig contexts
- Environment auto-detection from cluster names
- CrashLoopBackOff, OOMKilled, ImagePullBackOff, Pending pod classification
- One-line log summarizer (Java, Python, Go, generic)
- Watch mode, JSON output, `--history` flag
- `klarity init` onboarding wizard
