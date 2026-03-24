package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/vishukamble/klarity/pkg/cache"
	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
	"github.com/vishukamble/klarity/pkg/kube"
	"github.com/vishukamble/klarity/pkg/logs"
	"github.com/vishukamble/klarity/pkg/notifications"
	"github.com/vishukamble/klarity/pkg/output"
)

// ── Flag variables ────────────────────────────────────────────────────────────

var (
	flagOutput    string
	flagEnv       string
	flagContext   string
	flagNamespace string
	flagExcludeNs string
	flagCategory  string
	flagWatch     bool
	flagInterval  int
	flagHistory   int
)

// ── Root command ──────────────────────────────────────────────────────────────

// Version is the CLI version string, set here for --version flag.
const Version = "1.0.6"

var rootCmd = &cobra.Command{
	Use:     "klarity",
	Short:   "Read-only Kubernetes diagnostic CLI",
	Version: Version,
	Long: `klarity scans multiple clusters and namespaces in parallel,
classifies unhealthy workloads by root cause, and renders categorized
terminal tables with one-line error summaries. Never mutates resources.`,
	RunE:          runScan,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Suppress client-go deprecation warnings (e.g. "v1 Endpoints is deprecated").
	rest.SetDefaultWarningHandler(rest.NoWarnings{})

	rootCmd.Flags().StringVarP(&flagOutput, "output", "o", "table",
		"Output format: table (default) | json")
	rootCmd.Flags().StringVar(&flagEnv, "env", "",
		"Limit scan to this environment name (e.g. prod)")
	rootCmd.Flags().StringVar(&flagContext, "context", "",
		"Limit scan to this cluster context name")
	rootCmd.Flags().StringVarP(&flagNamespace, "namespace", "n", "",
		"Scan only these namespace(s), comma-separated (e.g. payments or payments,analytics)")
	rootCmd.Flags().StringVar(&flagExcludeNs, "exclude-ns", "",
		"Exclude namespace(s) from scan, comma-separated (e.g. build-ns-1,build-ns-2). Ignored if --namespace is also set.")
	rootCmd.Flags().StringVar(&flagCategory, "category", "",
		"Comma-separated list of categories to show (e.g. oom,crashloop,imagepull)")
	rootCmd.Flags().BoolVar(&flagWatch, "watch", false,
		"Continuously scan and refresh the display")
	rootCmd.Flags().IntVar(&flagInterval, "interval", 0,
		"Override scan interval in seconds (default: settings.scan_interval_seconds)")
	rootCmd.Flags().IntVar(&flagHistory, "history", 0,
		"Show scan history (last N scans; --history alone shows last 10)")
	rootCmd.Flag("history").NoOptDefVal = "10"
}

// Execute is the entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ── Scan entrypoint ───────────────────────────────────────────────────────────

func runScan(cmd *cobra.Command, args []string) error {
	// --history: show scan log and exit (no config or kubeconfig needed).
	if flagHistory > 0 {
		logPath, err := cache.LogPath()
		if err != nil {
			return fmt.Errorf("resolving log path: %w", err)
		}
		return showHistory(flagHistory, flagEnv, logPath)
	}

	cfgPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "No config found. Run 'klarity init' to get started.")
			os.Exit(1)
		}
		return fmt.Errorf("loading config: %w", err)
	}

	// Advisory: warn about kubelogin version regression.
	if warning := kube.CheckKubeloginVersion(); warning != "" {
		fmt.Fprintln(os.Stderr, warning)
		fmt.Fprintln(os.Stderr)
	}

	// Apply default_env when no explicit scope flags are set.
	noScopeFlags := flagEnv == "" && flagContext == ""
	if noScopeFlags && cfg.Settings.DefaultEnv != "" {
		showDefaultEnvBanner(cfg.Settings.DefaultEnv)
		cfg = filterByEnv(cfg, cfg.Settings.DefaultEnv)
		if len(cfg.Environments) == 0 {
			return fmt.Errorf("default_env %q not found in config — update ~/.klarityconfig.yaml or re-run klarity init", cfg.Settings.DefaultEnv)
		}
	} else if noScopeFlags && countConfigClusters(cfg) > 10 {
		n := countConfigClusters(cfg)
		fmt.Fprintf(os.Stderr, "⚠  Scanning %d clusters — this may take several minutes.\n", n)
		if suggestion := suggestDefaultEnv(cfg); suggestion != "" {
			fmt.Fprintf(os.Stderr, "   Tip: set a default environment in ~/.klarityconfig.yaml (default_env: %s)\n", suggestion)
		}
		fmt.Fprintf(os.Stderr, "   or run klarity --env <name> to scan a specific environment.\n\n")
	}

	// Apply --env filter (prune config in place before scanning).
	if flagEnv != "" {
		cfg = filterByEnv(cfg, flagEnv)
		if len(cfg.Environments) == 0 {
			return fmt.Errorf("no environment named %q in config", flagEnv)
		}
	}

	// Apply --context filter.
	if flagContext != "" {
		cfg = filterByContext(cfg, flagContext)
		if len(cfg.Environments) == 0 {
			return fmt.Errorf("no cluster context %q in config", flagContext)
		}
	}

	// Parse namespace filters.
	nsInclude := parseCommaSeparated(flagNamespace)
	nsExclude := parseCommaSeparated(flagExcludeNs)
	if len(nsInclude) > 0 && len(nsExclude) > 0 {
		fmt.Fprintln(os.Stderr, "⚠️  --exclude-ns ignored when --namespace is specified")
		nsExclude = nil
	}

	// Determine effective interval.
	interval := cfg.Settings.ScanIntervalSeconds
	if flagInterval > 0 {
		interval = flagInterval
	}

	// Parse optional --category filter.
	categorySet := parseCategoryFilter(flagCategory)

	// Build the canonical classifier list once.
	classifiers := []diagnosis.Classifier{
		diagnosis.NodeClassifier{},
		diagnosis.OOMClassifier{},
		diagnosis.ImagePullClassifier{},
		diagnosis.CrashLoopClassifier{},
		diagnosis.PendingClassifier{},
		diagnosis.HPAClassifier{},
		diagnosis.NoEndpointsClassifier{},
		diagnosis.QuotaClassifier{},
		diagnosis.PVCClassifier{},
		diagnosis.DaemonSetClassifier{},
		diagnosis.StatefulSetClassifier{},
		diagnosis.JobClassifier{},
		diagnosis.CronJobClassifier{},
		diagnosis.ContainerErrorClassifier{},
		diagnosis.EventClassifier{},
	}

	cachePath, _ := cache.DefaultPath()
	logPath, _ := cache.LogPath()

	// ── Watch mode ────────────────────────────────────────────────────────────
	if flagWatch {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		for {
			clearScreen()
			fmt.Printf("klarity --watch | scanning every %ds | press Ctrl+C to stop\n\n", interval)
			if err := doScan(ctx, cfg, classifiers, categorySet, nsInclude, nsExclude, interval, cachePath, logPath); err != nil {
				if errors.Is(err, context.Canceled) {
					fmt.Fprintln(os.Stdout, "\nWatch mode stopped.")
					return nil
				}
				fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
			}

			select {
			case <-ctx.Done():
				fmt.Fprintln(os.Stdout, "\nWatch mode stopped.")
				return nil
			case <-time.After(time.Duration(interval) * time.Second):
			}
		}
	}

	// ── Single-shot mode: check cache first ───────────────────────────────────
	filteredScan := flagEnv != "" || flagContext != "" || flagNamespace != ""
	if flagOutput != "json" && !filteredScan {
		cachedData, loadErr := cache.Load(cachePath)
		if loadErr != nil {
			// Corrupted cache — remove and fall through to live scan.
			os.Remove(cachePath)
			cachedData = nil
		}

		if cachedData != nil {
			// Show cached results instantly with a "scanning..." label.
			ageStr := formatCacheAge(cache.Age(cachedData))
			fmt.Printf("(cached %s ago, scanning...)\n\n", ageStr)
			output.RenderReport(os.Stdout, cachedData.Findings, cfg, cachedData.ScannedAt, nil)

			// Background scan.
			type bgResult struct {
				findings []diagnosis.Finding
				errs     []string
			}
			ch := make(chan bgResult, 1)
			go func() {
				f, e := gatherFindings(context.Background(), cfg, classifiers, nsInclude, nsExclude)
				ch <- bgResult{f, e}
			}()

			r := <-ch
			if len(categorySet) > 0 {
				r.findings = filterByCategory(r.findings, categorySet)
			}

			// Write fresh cache and log.
			scanTime := time.Now()
			newCache := &cache.Cache{ScannedAt: scanTime, Findings: r.findings}
			_ = cache.Save(cachePath, newCache)
			_ = cache.AppendLog(logPath, buildLogEntry(r.findings, scanTime))

			if cache.Equal(cachedData.Findings, r.findings) {
				fmt.Printf("\n✓ Still current (verified at %s)\n", scanTime.Format("15:04:05"))
			} else {
				clearScreen()
				output.RenderReport(os.Stdout, r.findings, cfg, scanTime, r.errs)
			}

			// Post to Slack if configured.
			postToSlack(cfg, r.findings, scanTime)
			return nil
		}
	}

	// No cache (or JSON mode): scan live.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return doScan(ctx, cfg, classifiers, categorySet, nsInclude, nsExclude, interval, cachePath, logPath)
}

// gatherFindings runs the parallel scan across all clusters and returns the
// combined findings and non-fatal error strings. It does not render or write
// anything — that is the caller's responsibility.
func gatherFindings(
	ctx context.Context,
	cfg *config.Config,
	classifiers []diagnosis.Classifier,
	nsInclude []string,
	nsExclude []string,
) ([]diagnosis.Finding, []string) {
	var mu sync.Mutex
	var allFindings []diagnosis.Finding
	var scanErrors []string

	kubeconfigPath := kube.DefaultKubeconfigPath()
	limit := cfg.Settings.ParallelClusters
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup

	for _, env := range cfg.Environments {
		for _, cluster := range env.Clusters {
			env, cluster := env, cluster
			wg.Add(1)
			go func() {
				defer wg.Done()

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()

				cs, err := kube.BuildClientset(kubeconfigPath, cluster.Context)
				if err != nil {
					mu.Lock()
					scanErrors = append(scanErrors, fmt.Sprintf("[%s/%s] %v", env.Name, cluster.Context, err))
					mu.Unlock()
					return
				}
				findings, errs := scanCluster(ctx, cfg, env, cluster, cs, classifiers, nsInclude, nsExclude)
				mu.Lock()
				allFindings = append(allFindings, findings...)
				scanErrors = append(scanErrors, errs...)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	return allFindings, scanErrors
}

// buildLogEntry builds a cache.LogEntry from a completed scan.
func buildLogEntry(findings []diagnosis.Finding, t time.Time) cache.LogEntry {
	envCounts := make(map[string]int)
	for _, f := range findings {
		envCounts[f.EnvName]++
	}
	return cache.LogEntry{
		ScannedAt:    t,
		Environments: envCounts,
		Total:        len(findings),
	}
}

// postToSlack sends a Slack notification if Slack is configured.
func postToSlack(cfg *config.Config, findings []diagnosis.Finding, t time.Time) {
	if !cfg.Notifications.Slack.Enabled {
		return
	}
	meta := notifications.ScanMeta{
		Timestamp:    t,
		EnvCount:     len(cfg.Environments),
		ClusterCount: countConfigClusters(cfg),
	}
	if err := notifications.SendSummary(notifications.DefaultHTTPClient, cfg.Notifications.Slack, findings, meta); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Slack notification failed: %v\n", err)
	}
}

// doScan executes one full scan cycle, writes cache + log, and renders the result.
func doScan(
	ctx context.Context,
	cfg *config.Config,
	classifiers []diagnosis.Classifier,
	categorySet map[diagnosis.Category]bool,
	nsInclude []string,
	nsExclude []string,
	interval int,
	cachePath string,
	logPath string,
) error {
	startTime := time.Now()

	allFindings, scanErrors := gatherFindings(ctx, cfg, classifiers, nsInclude, nsExclude)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Apply --category filter post-classify.
	if len(categorySet) > 0 {
		allFindings = filterByCategory(allFindings, categorySet)
	}

	// Write cache and log after every scan (including watch-mode iterations).
	if cachePath != "" {
		c := &cache.Cache{ScannedAt: startTime, Findings: allFindings}
		_ = cache.Save(cachePath, c)
	}
	if logPath != "" {
		_ = cache.AppendLog(logPath, buildLogEntry(allFindings, startTime))
	}

	switch flagOutput {
	case "json":
		if err := output.RenderJSON(allFindings, os.Stdout, cfg, startTime); err != nil {
			return err
		}
	default:
		output.RenderReport(os.Stdout, allFindings, cfg, startTime, scanErrors)
	}

	// Post to Slack if configured.
	postToSlack(cfg, allFindings, startTime)

	return nil
}

// ── Cluster scan ──────────────────────────────────────────────────────────────

// scanCluster runs all kube scanners for every namespace in the cluster,
// builds ScanResults, runs classifiers, and returns Findings + per-ns errors.
func scanCluster(
	ctx context.Context,
	cfg *config.Config,
	env config.Environment,
	cluster config.Cluster,
	cs kubernetes.Interface,
	classifiers []diagnosis.Classifier,
	nsInclude []string,
	nsExclude []string,
) ([]diagnosis.Finding, []string) {
	var errs []string
	prefix := fmt.Sprintf("[%s/%s]", env.Name, cluster.Context)

	// Honour --namespace flag: override cluster's namespace filter.
	nsFilter := cluster.Namespaces
	if len(nsInclude) > 0 {
		nsFilter = config.NamespaceFilter{
			Mode:    config.NamespaceModeInclude,
			Include: nsInclude,
		}
	}

	namespaces, err := kube.ResolveNamespaces(ctx, cs, nsFilter, cfg.Settings.DefaultNsExclude)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s resolve namespaces: %v", prefix, err)}
	}

	// Apply --exclude-ns filter before any API calls.
	namespaces = applyNamespaceFilters(namespaces, nsInclude, nsExclude)

	results := diagnosis.ScanResults{
		EnvName:     env.Name,
		ClusterCtx:  cluster.Context,
		AllPVCNames: make(map[string][]string),
	}

	// Node scan is cluster-wide (not namespaced).
	nodeIssues, err := kube.ListUnhealthyNodes(ctx, cs)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s nodes: %v", prefix, err))
	} else {
		results.Nodes = nodeIssues
	}

	// Parallelize the namespace loop. Each namespace gets its own goroutine;
	// all resource types within a namespace are still scanned sequentially.
	nsLimit := cfg.Settings.ParallelNamespaces
	if nsLimit < 1 {
		nsLimit = 10 // default for configs that pre-date this field
	}
	nsSem := make(chan struct{}, nsLimit)
	var nsWg sync.WaitGroup
	var nsMu sync.Mutex

	for _, ns := range namespaces {
		ns := ns
		nsWg.Add(1)
		go func() {
			defer nsWg.Done()
			select {
			case nsSem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-nsSem }()

			var localErrs []string

			// pods + log fetch for CrashLoopBackOff
			var localPods []kube.PodIssue
			pods, err := kube.ListUnhealthyPods(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s pods/%s: %v", prefix, ns, err))
			} else {
				for i := range pods {
					if pods[i].Reason == "CrashLoopBackOff" && cfg.Settings.LogTailLines > 0 {
						raw, lerr := logs.FetchLogs(ctx, cs, ns, pods[i].PodName, pods[i].ContainerName,
							int64(cfg.Settings.LogTailLines), true)
						if lerr == nil && raw != "" {
							pods[i].LogSummary = logs.Summarize(raw)
						}
					}
				}
				localPods = pods
			}

			var localDeployments []kube.DeploymentIssue
			deployments, err := kube.ListUnhealthyDeployments(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s deployments/%s: %v", prefix, ns, err))
			} else {
				localDeployments = deployments
			}

			var localHPAs []kube.HPAIssue
			hpas, err := kube.ListUnhealthyHPAs(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s hpas/%s: %v", prefix, ns, err))
			} else {
				localHPAs = hpas
			}

			var localServices []kube.ServiceIssue
			services, err := kube.ListServicesWithNoEndpoints(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s services/%s: %v", prefix, ns, err))
			} else {
				localServices = services
			}

			var localEvents []kube.EventIssue
			events, err := kube.ListWarningEvents(ctx, cs, ns, 15*time.Minute)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s events/%s: %v", prefix, ns, err))
			} else {
				localEvents = events
			}

			var localQuotas []kube.QuotaIssue
			quotas, err := kube.ListQuotaIssues(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s quotas/%s: %v", prefix, ns, err))
			} else {
				localQuotas = quotas
			}

			var localPVCs []kube.PVCIssue
			pvcs, err := kube.ListPendingPVCs(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s pvcs/%s: %v", prefix, ns, err))
			} else {
				localPVCs = pvcs
			}

			var localPVCNames []string
			pvcNames, err := kube.ListPVCNames(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s pvc-names/%s: %v", prefix, ns, err))
			} else {
				localPVCNames = pvcNames
			}

			var localDaemonSets []kube.DaemonSetIssue
			daemonsets, err := kube.ListUnhealthyDaemonSets(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s daemonsets/%s: %v", prefix, ns, err))
			} else {
				localDaemonSets = daemonsets
			}

			var localStatefulSets []kube.StatefulSetIssue
			statefulsets, err := kube.ListUnhealthyStatefulSets(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s statefulsets/%s: %v", prefix, ns, err))
			} else {
				localStatefulSets = statefulsets
			}

			var localJobs []kube.JobIssue
			jobs, err := kube.ListFailedJobs(ctx, cs, ns, cfg.Settings.ExcludeCompletedJobs)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s jobs/%s: %v", prefix, ns, err))
			} else {
				localJobs = jobs
			}

			var localCronJobs []kube.CronJobIssue
			cronJobs, err := kube.ListSuspendedCronJobs(ctx, cs, ns)
			if err != nil {
				localErrs = append(localErrs, fmt.Sprintf("%s cronjobs/%s: %v", prefix, ns, err))
			} else {
				localCronJobs = cronJobs
			}

			// Merge into shared results under the mutex.
			nsMu.Lock()
			results.Pods = append(results.Pods, localPods...)
			results.Deployments = append(results.Deployments, localDeployments...)
			results.HPAs = append(results.HPAs, localHPAs...)
			results.Services = append(results.Services, localServices...)
			results.Events = append(results.Events, localEvents...)
			results.Quotas = append(results.Quotas, localQuotas...)
			results.PVCs = append(results.PVCs, localPVCs...)
			if localPVCNames != nil {
				results.AllPVCNames[ns] = localPVCNames
			}
			results.DaemonSets = append(results.DaemonSets, localDaemonSets...)
			results.StatefulSets = append(results.StatefulSets, localStatefulSets...)
			results.Jobs = append(results.Jobs, localJobs...)
			results.CronJobs = append(results.CronJobs, localCronJobs...)
			errs = append(errs, localErrs...)
			nsMu.Unlock()
		}()
	}
	nsWg.Wait()

	return diagnosis.RunAll(results, classifiers), errs
}

// ── Filter helpers ────────────────────────────────────────────────────────────

// filterByEnv returns a copy of cfg retaining only the named environment.
func filterByEnv(cfg *config.Config, name string) *config.Config {
	out := *cfg
	out.Environments = nil
	for _, e := range cfg.Environments {
		if e.Name == name {
			out.Environments = append(out.Environments, e)
		}
	}
	return &out
}

// filterByContext returns a copy of cfg retaining only clusters matching ctx.
// Environments that end up with no clusters are dropped.
func filterByContext(cfg *config.Config, ctx string) *config.Config {
	out := *cfg
	out.Environments = nil
	for _, e := range cfg.Environments {
		filtered := e
		filtered.Clusters = nil
		for _, cl := range e.Clusters {
			if cl.Context == ctx {
				filtered.Clusters = append(filtered.Clusters, cl)
			}
		}
		if len(filtered.Clusters) > 0 {
			out.Environments = append(out.Environments, filtered)
		}
	}
	return &out
}

// parseCommaSeparated splits a comma-separated string into trimmed tokens.
// Returns nil for empty input.
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(token)
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

// applyNamespaceFilters applies include/exclude filters to a resolved namespace list.
// If nsInclude is non-empty, returns the intersection (include already handled by
// ResolveNamespaces, so this is a no-op). If nsExclude is non-empty, removes those.
func applyNamespaceFilters(namespaces []string, nsInclude, nsExclude []string) []string {
	// Include filter already applied via NamespaceFilter — nothing to do here.
	if len(nsInclude) > 0 {
		return namespaces
	}
	if len(nsExclude) == 0 {
		return namespaces
	}
	excludeSet := make(map[string]bool, len(nsExclude))
	for _, ns := range nsExclude {
		excludeSet[ns] = true
	}
	var out []string
	for _, ns := range namespaces {
		if !excludeSet[ns] {
			out = append(out, ns)
		}
	}
	return out
}

// filterByCategory retains only findings whose category is in the set.
func filterByCategory(findings []diagnosis.Finding, set map[diagnosis.Category]bool) []diagnosis.Finding {
	var out []diagnosis.Finding
	for _, f := range findings {
		if set[f.Category] {
			out = append(out, f)
		}
	}
	return out
}

// categoryAliases maps short CLI names → diagnosis.Category.
var categoryAliases = map[string]diagnosis.Category{
	"node":         diagnosis.CategoryNodeIssue,
	"nodes":        diagnosis.CategoryNodeIssue,
	"oom":          diagnosis.CategoryOOMKilled,
	"oomkilled":    diagnosis.CategoryOOMKilled,
	"imagepull":    diagnosis.CategoryImagePull,
	"image":        diagnosis.CategoryImagePull,
	"crashloop":    diagnosis.CategoryCrashLoop,
	"crash":        diagnosis.CategoryCrashLoop,
	"pending":      diagnosis.CategoryPending,
	"hpa":          diagnosis.CategoryHPACeiling,
	"noendpoints":  diagnosis.CategoryNoEndpoints,
	"endpoints":    diagnosis.CategoryNoEndpoints,
	"quota":        diagnosis.CategoryQuotaExhausted,
	"pvc":          diagnosis.CategoryPVCPending,
	"job":          diagnosis.CategoryJobFailed,
	"jobs":         diagnosis.CategoryJobFailed,
	"cronjob":      diagnosis.CategoryCronJobSuspended,
	"cronjobs":     diagnosis.CategoryCronJobSuspended,
	"daemonset":    diagnosis.CategoryDaemonSetDegraded,
	"statefulset":  diagnosis.CategoryStatefulSetDegraded,
	"event":        diagnosis.CategoryWarningEvent,
	"events":       diagnosis.CategoryWarningEvent,
}

// parseCategoryFilter converts "oom,crashloop" → set of Category values.
// Returns nil (no filter) if the input string is empty.
func parseCategoryFilter(s string) map[diagnosis.Category]bool {
	if s == "" {
		return nil
	}
	set := make(map[diagnosis.Category]bool)
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(strings.ToLower(token))
		if cat, ok := categoryAliases[token]; ok {
			set[cat] = true
		} else {
			// Treat as literal category name (e.g. "OOMKilled").
			set[diagnosis.Category(token)] = true
		}
	}
	return set
}

// ── Watch mode helpers ────────────────────────────────────────────────────────

// countConfigClusters returns the total number of clusters across all environments.
func countConfigClusters(cfg *config.Config) int {
	n := 0
	for _, e := range cfg.Environments {
		n += len(e.Clusters)
	}
	return n
}

// showDefaultEnvBanner prints the framed banner shown when default_env is active.
func showDefaultEnvBanner(envName string) {
	line1 := fmt.Sprintf("  klarity scan — scanning %s (default)", envName)
	line2 := "  Use --env to scan other environments"
	width := len(line1)
	if len(line2) > width {
		width = len(line2)
	}
	bar := strings.Repeat("═", width+2)
	fmt.Printf("╔%s╗\n", bar)
	fmt.Printf("║%-*s  ║\n", width, line1)
	fmt.Printf("║%-*s  ║\n", width, line2)
	fmt.Printf("╚%s╝\n\n", bar)
}

// suggestDefaultEnv returns the name of the first critical-tier environment,
// falling back to the first environment in the config. Returns "" for empty configs.
func suggestDefaultEnv(cfg *config.Config) string {
	for _, env := range cfg.Environments {
		if env.Tier == config.TierCritical {
			return env.Name
		}
	}
	if len(cfg.Environments) > 0 {
		return cfg.Environments[0].Name
	}
	return ""
}

// clearScreen writes ANSI escape codes to move to the top-left and erase the
// display. Works on Linux/macOS terminals; no-ops on Windows cmd.exe.
func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

// formatCacheAge returns a human-readable duration string (e.g. "5m30s" or "42s").
func formatCacheAge(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) - m*60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// showHistory prints the last N scan log entries to stdout.
func showHistory(last int, envFilter, logPath string) error {
	entries, err := cache.ReadLog(logPath, last)
	if err != nil {
		return fmt.Errorf("reading history: %w", err)
	}

	entries = cache.FilterLog(entries, envFilter)

	if len(entries) == 0 {
		fmt.Println("No scan history found. Run klarity to start recording scans.")
		return nil
	}

	fmt.Printf("klarity scan history — last %d scan(s)\n\n", len(entries))

	for _, e := range entries {
		line := e.ScannedAt.Local().Format("  2006-01-02 15:04")
		if e.Total == 0 {
			line += "  0 issues  ✓ clean"
		} else {
			line += fmt.Sprintf("  %d issues", e.Total)
			// Sort env names for consistent display order.
			envNames := make([]string, 0, len(e.Environments))
			for name := range e.Environments {
				envNames = append(envNames, name)
			}
			sort.Strings(envNames)
			for _, name := range envNames {
				count := e.Environments[name]
				if count > 0 {
					line += fmt.Sprintf("  %s: %d", name, count)
				}
			}
		}
		fmt.Println(line)
	}
	return nil
}
