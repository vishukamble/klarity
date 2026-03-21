package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vishukamble/klarity/pkg/config"
)

func init() {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect klarity configuration",
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Pretty-print the current configuration",
		RunE:  runConfigShow,
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		RunE:  runConfigPath,
	})

	rootCmd.AddCommand(configCmd)
}

func runConfigPath(_ *cobra.Command, _ []string) error {
	p, err := config.ConfigPath()
	if err != nil {
		return err
	}
	fmt.Println(p)
	return nil
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "No config found. Run 'klarity init' to get started.")
			return nil
		}
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Printf("Config: %s\n", cfgPath)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Version: %d\n\n", cfg.Version)

	fmt.Println("Settings:")
	fmt.Printf("  parallel_clusters:      %d\n", cfg.Settings.ParallelClusters)
	fmt.Printf("  scan_interval_seconds:  %d\n", cfg.Settings.ScanIntervalSeconds)
	fmt.Printf("  log_tail_lines:         %d\n", cfg.Settings.LogTailLines)
	fmt.Printf("  exclude_completed_jobs: %v\n", cfg.Settings.ExcludeCompletedJobs)
	if len(cfg.Settings.DefaultNsExclude) > 0 {
		fmt.Printf("  default_ns_exclude:     %s\n", strings.Join(cfg.Settings.DefaultNsExclude, ", "))
	}

	if cfg.Notifications.Slack.Enabled {
		fmt.Println("\nNotifications:")
		fmt.Printf("  slack:\n")
		fmt.Printf("    enabled:        true\n")
		fmt.Printf("    mode:           %s\n", cfg.Notifications.Slack.Mode)
		if cfg.Notifications.Slack.Mode == config.SlackModeBotToken && cfg.Notifications.Slack.Channel != "" {
			fmt.Printf("    channel:        %s\n", cfg.Notifications.Slack.Channel)
		}
		fmt.Printf("    on_issues_only: %v\n", cfg.Notifications.Slack.OnIssuesOnly)
		fmt.Printf("    min_severity:   %s\n", cfg.Notifications.Slack.MinSeverity)
	}

	fmt.Printf("\nEnvironments (%d):\n", len(cfg.Environments))
	for _, env := range cfg.Environments {
		tier := env.Tier
		fmt.Printf("\n  %s [%s] — %d cluster(s)\n", env.Name, tier, len(env.Clusters))
		for _, cl := range env.Clusters {
			nsDesc := describeNamespaceFilter(cl.Namespaces)
			fmt.Printf("    • %s  %s\n", cl.Context, nsDesc)
		}
	}
	fmt.Println()
	return nil
}

func describeNamespaceFilter(ns config.NamespaceFilter) string {
	switch ns.Mode {
	case config.NamespaceModeAll:
		return "(all namespaces)"
	case config.NamespaceModeInclude:
		return fmt.Sprintf("(include: %s)", strings.Join(ns.Include, ", "))
	case config.NamespaceModeExclude:
		if len(ns.Exclude) == 0 {
			return "(exclude: none)"
		}
		return fmt.Sprintf("(exclude: %s)", strings.Join(ns.Exclude, ", "))
	default:
		return ""
	}
}
