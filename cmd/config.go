package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

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

	configCmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open config in your $EDITOR",
		RunE:  runConfigEdit,
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate config and check cluster contexts",
		RunE:  runConfigValidate,
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
	if cfg.Settings.DefaultEnv != "" {
		fmt.Printf("  default_env:            %s\n", cfg.Settings.DefaultEnv)
	}

	if cfg.Notifications.Slack.Enabled {
		fmt.Println("\nNotifications:")
		fmt.Printf("  slack:\n")
		fmt.Printf("    enabled:        true\n")
		fmt.Printf("    mode:           %s\n", cfg.Notifications.Slack.Mode)
		if cfg.Notifications.Slack.Mode == config.SlackModeBotToken && cfg.Notifications.Slack.Channel != "" {
			fmt.Printf("    channel:        %s\n", cfg.Notifications.Slack.Channel)
		}
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

func runConfigEdit(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "No config found. Run 'klarity init' first.")
		return nil
	}

	editor := findEditor()
	if editor == "" {
		fmt.Fprintf(os.Stderr, "Could not find an editor. Edit manually: %s\n", cfgPath)
		return nil
	}

	cmd := exec.Command(editor, cfgPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Validate after editing (Load calls Validate internally).
	if _, err := config.Load(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Config has errors: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'klarity config edit' to fix, or 'klarity init' to start over.")
		return nil
	}

	fmt.Println("Config updated successfully.")
	return nil
}

// findEditor returns the editor command to use, checking $EDITOR then
// common defaults.
func findEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	for _, candidate := range []string{"nano", "vim", "vi"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func runConfigValidate(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	fmt.Printf("Validating %s...\n\n", cfgPath)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "No config found. Run 'klarity init' to get started.")
			return nil
		}
		return fmt.Errorf("config load error: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Load kubeconfig to check contexts.
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig, loadErr := loadingRules.Load()

	var warnings int
	var found int
	for _, env := range cfg.Environments {
		for _, cl := range env.Clusters {
			if loadErr != nil {
				fmt.Printf("⚠️  %s (could not load kubeconfig: %v)\n", cl.Context, loadErr)
				warnings++
				continue
			}
			if _, ok := kubeConfig.Contexts[cl.Context]; ok {
				fmt.Printf("✓ %s (context found)\n", cl.Context)
				found++
			} else {
				fmt.Printf("⚠️  %s (context not found in kubeconfig)\n", cl.Context)
				warnings++
			}
		}
	}

	fmt.Println()
	if warnings > 0 {
		fmt.Printf("Config is valid with %d warning(s).\n", warnings)
	} else {
		fmt.Printf("Config is valid. %d context(s) found.\n", found)
	}
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
