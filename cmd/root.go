package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

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
	flagCategory  string
	flagWatch     bool
	flagInterval  int
)

// ── Root command ──────────────────────────────────────────────────────────────

var rootCmd = &cobra.Command{
	Use:   "klarity",
	Short: "Read-only Kubernetes diagnostic CLI",
	Long: `klarity scans multiple clusters and namespaces in parallel,
classifies unhealthy workloads by root cause, and renders categorized
terminal tables with one-line error summaries. Never mutates resources.`,
	RunE:          runScan,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Flags().StringVarP(&flagOutput, "output", "o", "table",
		"Output format: table (default) | json")
	rootCmd.Flags().StringVar(&flagEnv, "env", "",
		"Limit scan to this environment name (e.g. prod)")
	rootCmd.Flags().StringVar(&flagContext, "context", "",
		"Limit scan to this cluster context name")
	rootCmd.Flags().StringVarP(&flagNamespace, "namespace", "n", "",
		"Limit scan to this namespace (across all clusters)")
	rootCmd.Flags().StringVar(&flagCategory, "category", "",
		"Comma-separated list of categories to show (e.g. oom,crashloop,imagepull)")
	rootCmd.Flags().BoolVar(&flagWatch, "watch", false,
		"Continuously scan and refresh the display")
	rootCmd.Flags().IntVar(&flagInterval, "interval", 0,
		"Override scan interval in seconds (default: settings.scan_interval_seconds)")
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

	// Determine effective interval.
	interval := cfg.Settings.ScanIntervalSeconds
	if flagInterval > 0 {
		interval = flagInterval
	}

	// Parse optional --category filter.
	categorySet := parseCategoryFilter(flagCategory)

	// Build the canonical classifier list once.
	classifiers := []diagnosis.Classifier{
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
		diagnosis.EventClassifier{},
	}

	if !flagWatch {
		// Single-shot scan.
		return doScan(context.Background(), cfg, classifiers, categorySet, interval)
	}

	// ── Watch mode ────────────────────────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	for {
		clearScreen()
		if err := doScan(ctx, cfg, classifiers, categorySet, interval); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stdout, "\nWatch mode stopped.")
				return nil
			}
			// Non-fatal scan error: print and continue watching.
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

// doScan executes one full scan cycle and renders the result.
func doScan(
	ctx context.Context,
	cfg *config.Config,
	classifiers []diagnosis.Classifier,
	categorySet map[diagnosis.Category]bool,
	interval int,
) error {
	startTime := time.Now()

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
				findings, errs := scanCluster(ctx, cfg, env, cluster, cs, classifiers)
				mu.Lock()
				allFindings = append(allFindings, findings...)
				scanErrors = append(scanErrors, errs...)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Apply --namespace filter post-scan (cheaper than modifying cluster configs).
	if flagNamespace != "" {
		allFindings = filterByNamespace(allFindings, flagNamespace)
	}

	// Apply --category filter post-classify.
	if len(categorySet) > 0 {
		allFindings = filterByCategory(allFindings, categorySet)
	}

	switch flagOutput {
	case "json":
		if err := output.RenderJSON(allFindings, os.Stdout); err != nil {
			return err
		}
	default:
		output.RenderReport(os.Stdout, allFindings, cfg, startTime, scanErrors)
	}

	// Post to Slack if configured.
	if cfg.Notifications.Slack.Enabled {
		meta := notifications.ScanMeta{
			Timestamp:    startTime,
			EnvCount:     len(cfg.Environments),
			ClusterCount: countConfigClusters(cfg),
		}
		if err := notifications.SendSummary(notifications.DefaultHTTPClient, cfg.Notifications.Slack, allFindings, meta); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Slack notification failed: %v\n", err)
		}
	}

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
) ([]diagnosis.Finding, []string) {
	var errs []string
	prefix := fmt.Sprintf("[%s/%s]", env.Name, cluster.Context)

	// Honour --namespace flag: override cluster's namespace filter.
	nsFilter := cluster.Namespaces
	if flagNamespace != "" {
		nsFilter = config.NamespaceFilter{
			Mode:    config.NamespaceModeInclude,
			Include: []string{flagNamespace},
		}
	}

	namespaces, err := kube.ResolveNamespaces(ctx, cs, nsFilter)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s resolve namespaces: %v", prefix, err)}
	}

	results := diagnosis.ScanResults{
		EnvName:    env.Name,
		ClusterCtx: cluster.Context,
	}

	for _, ns := range namespaces {
		pods, err := kube.ListUnhealthyPods(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s pods/%s: %v", prefix, ns, err))
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
			results.Pods = append(results.Pods, pods...)
		}

		deployments, err := kube.ListUnhealthyDeployments(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s deployments/%s: %v", prefix, ns, err))
		} else {
			results.Deployments = append(results.Deployments, deployments...)
		}

		hpas, err := kube.ListUnhealthyHPAs(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s hpas/%s: %v", prefix, ns, err))
		} else {
			results.HPAs = append(results.HPAs, hpas...)
		}

		services, err := kube.ListServicesWithNoEndpoints(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s services/%s: %v", prefix, ns, err))
		} else {
			results.Services = append(results.Services, services...)
		}

		events, err := kube.ListWarningEvents(ctx, cs, ns, 15*time.Minute)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s events/%s: %v", prefix, ns, err))
		} else {
			results.Events = append(results.Events, events...)
		}

		quotas, err := kube.ListQuotaIssues(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s quotas/%s: %v", prefix, ns, err))
		} else {
			results.Quotas = append(results.Quotas, quotas...)
		}

		pvcs, err := kube.ListPendingPVCs(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s pvcs/%s: %v", prefix, ns, err))
		} else {
			results.PVCs = append(results.PVCs, pvcs...)
		}

		daemonsets, err := kube.ListUnhealthyDaemonSets(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s daemonsets/%s: %v", prefix, ns, err))
		} else {
			results.DaemonSets = append(results.DaemonSets, daemonsets...)
		}

		statefulsets, err := kube.ListUnhealthyStatefulSets(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s statefulsets/%s: %v", prefix, ns, err))
		} else {
			results.StatefulSets = append(results.StatefulSets, statefulsets...)
		}

		jobs, err := kube.ListFailedJobs(ctx, cs, ns, cfg.Settings.ExcludeCompletedJobs)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s jobs/%s: %v", prefix, ns, err))
		} else {
			results.Jobs = append(results.Jobs, jobs...)
		}

		cronJobs, err := kube.ListSuspendedCronJobs(ctx, cs, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s cronjobs/%s: %v", prefix, ns, err))
		} else {
			results.CronJobs = append(results.CronJobs, cronJobs...)
		}
	}

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

// filterByNamespace retains only findings from the given namespace.
func filterByNamespace(findings []diagnosis.Finding, ns string) []diagnosis.Finding {
	var out []diagnosis.Finding
	for _, f := range findings {
		if f.Namespace == ns {
			out = append(out, f)
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

// clearScreen writes ANSI escape codes to move to the top-left and erase the
// display. Works on Linux/macOS terminals; no-ops on Windows cmd.exe.
func clearScreen() {
	fmt.Print("\033[2J\033[H")
}
