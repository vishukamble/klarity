package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/kube"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard — creates ~/.klarityconfig.yaml",
	Long: `klarity init reads your kubeconfig, auto-detects environments from context
names, and guides you through selecting which clusters to scan. The resulting
configuration is saved to ~/.klarityconfig.yaml.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	// ── 1. Load kubeconfig ──────────────────────────────────────────────
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig, err := loadingRules.Load()
	if err != nil {
		return fmt.Errorf("reading kubeconfig: %w", err)
	}

	fmt.Printf("Reading %s...\n\n", loadingRules.GetLoadingPrecedence()[0])

	contexts := make([]string, 0, len(kubeConfig.Contexts))
	for name := range kubeConfig.Contexts {
		contexts = append(contexts, name)
	}

	if len(contexts) == 0 {
		return fmt.Errorf("no contexts found in kubeconfig — add at least one cluster context and re-run")
	}

	// Check for kubelogin exec credential in any context's auth info.
	if hasKubeloginExec(kubeConfig, contexts) {
		if warning := kube.CheckKubeloginVersion(); warning != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), warning)
			fmt.Println()
		}
	}

	// Stable order for deterministic UX
	sortStrings(contexts)

	// ── 2. Auto-detect environments ─────────────────────────────────────
	detected, allMatched := config.DetectEnvironments(contexts)

	defaults := config.DefaultConfig()
	var cfg *config.Config

	if allMatched {
		cfg, err = runHappyPath(detected, defaults)
	} else {
		cfg, err = runFallbackPath(contexts, defaults)
	}
	if err != nil {
		return err
	}

	if cfg == nil {
		// User aborted.
		fmt.Println("Setup cancelled.")
		return nil
	}

	// ── 3. Save ─────────────────────────────────────────────────────────
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\n✅ Config saved to %s\n", cfgPath)
	fmt.Println("Run `klarity` to scan your environment.")
	return nil
}

// runHappyPath handles the detected-environment flow.
// For each environment, the user chooses whether to include all clusters or
// select a subset via a multi-select.
func runHappyPath(detected config.DetectedEnvs, defaults *config.Config) (*config.Config, error) {
	// Show summary
	fmt.Printf("Detected %d clusters across %d environments:\n\n", totalClusters(detected), len(detected.Order))
	for _, label := range detected.Order {
		clusters := detected.Envs[label]
		fmt.Printf("  %s (%d cluster", label, len(clusters))
		if len(clusters) != 1 {
			fmt.Print("s")
		}
		fmt.Println(")")
		for _, ctx := range clusters {
			fmt.Printf("    ✓ %s\n", ctx)
		}
	}
	fmt.Println()

	selected := make(map[string][]string)

	for _, label := range detected.Order {
		allClusters := detected.Envs[label]

		var scanAll bool
		confirm := huh.NewConfirm().
			Title(fmt.Sprintf("Scan all %d %s cluster(s)?", len(allClusters), label)).
			Value(&scanAll)

		if err := huh.NewForm(huh.NewGroup(confirm)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}

		if scanAll {
			selected[label] = allClusters
			continue
		}

		// Multi-select subset
		opts := make([]huh.Option[string], len(allClusters))
		for i, ctx := range allClusters {
			opts[i] = huh.NewOption(ctx, ctx)
		}
		var chosen []string
		ms := huh.NewMultiSelect[string]().
			Title(fmt.Sprintf("Select %s clusters to scan", label)).
			Options(opts...).
			Value(&chosen)

		if err := huh.NewForm(huh.NewGroup(ms)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}
		selected[label] = chosen
	}

	fmt.Printf("\nNamespaces: scanning all namespaces (excluding %v)\n", defaults.Settings.DefaultNsExclude)
	fmt.Println("To customize, edit ~/.klarityconfig.yaml after setup.")

	return config.BuildDetectedConfig(selected, detected.Order, defaults), nil
}

// runFallbackPath handles the manual env-naming flow when auto-detection fails.
func runFallbackPath(allContexts []string, defaults *config.Config) (*config.Config, error) {
	fmt.Printf("Found %d clusters:\n", len(allContexts))
	for _, ctx := range allContexts {
		fmt.Printf("  • %s\n", ctx)
	}
	fmt.Println("\nCould not detect environments from cluster names.")

	var numEnvs int
	numInput := huh.NewInput().
		Title("How many environments do you want to configure?").
		Validate(func(s string) error {
			n := 0
			if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 1 {
				return fmt.Errorf("enter a number >= 1")
			}
			return nil
		}).
		// We'll parse after the form runs.
		Value(new(string))

	numEnvStr := ""
	numInput.Value(&numEnvStr)

	if err := huh.NewForm(huh.NewGroup(numInput)).Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}
	if _, err := fmt.Sscanf(numEnvStr, "%d", &numEnvs); err != nil || numEnvs < 1 {
		return nil, fmt.Errorf("invalid number of environments: %q", numEnvStr)
	}

	envNames := make([]string, 0, numEnvs)
	clustersByEnv := make(map[string][]string)

	for i := 0; i < numEnvs; i++ {
		var envName string
		nameInput := huh.NewInput().
			Title(fmt.Sprintf("Environment %d name:", i+1)).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("name cannot be empty")
				}
				return nil
			}).
			Value(&envName)

		if err := huh.NewForm(huh.NewGroup(nameInput)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}

		opts := make([]huh.Option[string], len(allContexts))
		for j, ctx := range allContexts {
			opts[j] = huh.NewOption(ctx, ctx)
		}
		var chosen []string
		ms := huh.NewMultiSelect[string]().
			Title(fmt.Sprintf("Select clusters for %s", envName)).
			Options(opts...).
			Value(&chosen)

		if err := huh.NewForm(huh.NewGroup(ms)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}

		envNames = append(envNames, envName)
		clustersByEnv[envName] = chosen
	}

	fmt.Printf("\nNamespaces: scanning all namespaces (excluding %v)\n", defaults.Settings.DefaultNsExclude)
	fmt.Println("To customize, edit ~/.klarityconfig.yaml after setup.")

	return config.BuildManualConfig(envNames, clustersByEnv, defaults), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func totalClusters(d config.DetectedEnvs) int {
	n := 0
	for _, v := range d.Envs {
		n += len(v)
	}
	return n
}

// sortStrings is a simple insertion sort — avoids importing sort just for this.
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// hasKubeloginExec returns true if any context in the kubeconfig uses an exec
// credential plugin whose command is "kubelogin" (indicating AKS clusters).
func hasKubeloginExec(kubeConfig *clientcmdapi.Config, contexts []string) bool {
	for _, ctxName := range contexts {
		ctx, ok := kubeConfig.Contexts[ctxName]
		if !ok || ctx == nil {
			continue
		}
		authInfo, ok := kubeConfig.AuthInfos[ctx.AuthInfo]
		if !ok || authInfo == nil || authInfo.Exec == nil {
			continue
		}
		cmd := filepath.Base(authInfo.Exec.Command)
		if strings.EqualFold(cmd, "kubelogin") {
			return true
		}
	}
	return false
}
