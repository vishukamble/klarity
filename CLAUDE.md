# klarity

Read-only Kubernetes diagnostic CLI written in Go. Scans multiple clusters/namespaces in parallel via `client-go`, classifies unhealthy workloads by root cause, renders categorized terminal tables with one-line error summaries. Never mutates resources.

## Key Commands

```bash
go build -o klarity .                # build
go test ./...                        # run all tests
go test ./pkg/diagnosis/...          # test classifiers only
go vet ./...                         # static analysis
golangci-lint run                    # lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
go run main.go init                  # test onboarding wizard
go run main.go                       # test full scan (requires kubeconfig with reachable clusters)
```

## Validation — IMPORTANT

After ANY code change, ALWAYS run in this exact order:
1. `go build -o klarity .` — must compile
2. `go vet ./...` — must pass
3. `go test ./...` — must pass
4. If you added a new error classifier in `pkg/diagnosis/`, you MUST add table tests covering edge cases

Do NOT skip validation. Do NOT commit code that doesn't compile.

## Architecture

```
klarity/
├── cmd/           # Cobra CLI commands (root.go, init.go, config.go)
├── pkg/
│   ├── config/    # Config structs, load/save ~/.klarityconfig.yaml, env auto-detection
│   ├── kube/      # client-go wrappers — one file per resource type (pods.go, deployments.go, etc.)
│   ├── diagnosis/ # Error classification engine — one file per category (oom.go, crashloop.go, etc.)
│   ├── logs/      # Log tail + one-line root cause extraction (language-aware: Java, Python, Go)
│   └── output/    # Terminal table rendering (lipgloss), color/tier theming, JSON output
├── main.go
└── go.mod
```

## Critical Rules

- **NEVER generate code that creates, updates, patches, or deletes Kubernetes resources.** This tool is strictly read-only. Only `Get`, `List`, and pod log reads are allowed. If you find yourself importing `k8s.io/client-go/kubernetes/typed/*/v1` write methods, stop.
- Config file is `~/.klarityconfig.yaml` — NOT `.klarity.yaml`, NOT `klarity.yml`. Be consistent.
- Environment is the top-level grouping in config, not cluster. Structure: `environments[] → clusters[] → namespaces{}`.
- All Kubernetes API calls MUST go through `pkg/kube/` — never call client-go directly from `cmd/` or `diagnosis/`.
- `settings.default_ns_exclude` (default: `kube-system`, `kube-public`, `kube-node-lease`, `default`) is applied by `ResolveNamespaces()` when mode=`all` and the cluster has no explicit `exclude` list. It is ignored for mode=`include` or mode=`exclude`, and overridden when the cluster specifies its own exclude list.
- Error classifiers in `pkg/diagnosis/` must implement a common interface. Each classifier returns structured results, not formatted strings.
- The `pkg/output/` layer is the ONLY place that formats for terminal display. Classifiers return data, output renders it.
- Parallel cluster scanning uses goroutines with `WaitGroup + semaphore` (not errgroup). Respect `settings.parallel_clusters` from config as concurrency limit.
- Log parsing in `pkg/logs/` extracts one-line summaries. MUST handle: Java (Caused by), Python (last traceback line), Go (panic/fatal error), generic (FATAL/Error first match), fallback (last non-empty line).

## Caveats

- `client-go` requires matching `k8s.io/api` and `k8s.io/apimachinery` versions. Pin all three to the same release. Version mismatch causes subtle compile errors.
- When testing with mock kubeconfigs, use `k8s.io/client-go/kubernetes/fake` — don't spin up real clusters in unit tests.
- The `charmbracelet/huh` library (for onboarding wizard forms) requires a TTY. Tests that exercise the wizard must mock stdin or test the config logic separately.
- Lipgloss rendering includes ANSI codes. When `--output json` is set, ensure NO lipgloss/color functions are called. Check output mode early.
- Context names in kubeconfig can contain special characters (dots, slashes). Sanitize when using as map keys or in display.

## Dependencies

Core: `k8s.io/client-go`, `github.com/spf13/cobra`, `gopkg.in/yaml.v3`
TUI: `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/huh`
Tables: `github.com/charmbracelet/lipgloss/table` (prefer over tablewriter)
Lint: `golangci-lint`

## Further Documentation

- @docs/REMEMBER.md — current project state, what's implemented, what's next
- @docs/HANDOFF.md — session continuity notes for picking up where we left off
