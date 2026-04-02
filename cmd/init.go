package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/kube"
)

var flagReset bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard — creates ~/.klarityconfig.yaml",
	Long: `klarity init reads your kubeconfig, auto-detects environments from context
names, and guides you through selecting which clusters to scan. The resulting
configuration is saved to ~/.klarityconfig.yaml.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&flagReset, "reset", false, "Overwrite existing config (skips the overwrite guard)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	// ── 0. Overwrite guard + backup ─────────────────────────────────────
	cfgPath, pathErr := config.ConfigPath()
	if pathErr == nil {
		if _, statErr := os.Stat(cfgPath); statErr == nil {
			if !flagReset {
				fmt.Printf("Config already exists at %s\n", cfgPath)
				fmt.Println("Run 'klarity init --reset' to overwrite it.")
				return nil
			}
			// --reset: back up the existing config before overwriting.
			bakPath := cfgPath + ".bak"
			if err := copyFile(cfgPath, bakPath); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Could not back up config: %v\n", err)
			} else {
				fmt.Printf("Previous config backed up to %s\n", bakPath)
			}
		}
	}

	// ── 1. Load kubeconfig ──────────────────────────────────────────────
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	fmt.Printf("Reading %s... ", loadingRules.GetLoadingPrecedence()[0])
	kubeConfig, err := loadingRules.Load()
	if err != nil {
		return fmt.Errorf("reading kubeconfig: %w", err)
	}

	contexts := make([]string, 0, len(kubeConfig.Contexts))
	for name := range kubeConfig.Contexts {
		contexts = append(contexts, name)
	}
	fmt.Printf("found %d clusters.\n", len(contexts))

	if len(contexts) == 0 {
		return fmt.Errorf("no contexts found in kubeconfig — add at least one cluster context and re-run")
	}

	// Check for kubelogin exec credential in any context's auth info.
	if hasKubeloginExec(kubeConfig, contexts) {
		if warning := kube.CheckKubeloginVersion(); warning != "" {
			if cmd != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), warning)
			} else {
				fmt.Fprintln(os.Stderr, warning)
			}
			fmt.Println()
		}
	}

	// Stable order for deterministic UX.
	sortStrings(contexts)

	// ── 2. Auto-detect environments ─────────────────────────────────────
	fmt.Println("Analyzing cluster names...")
	detected, _ := config.DetectEnvironments(contexts)
	defaults := config.DefaultConfig()

	// ── 3. Run full-screen TUI wizard ────────────────────────────────────
	cfg, err := runNewWizardTUI(detected, defaults, true)
	if err != nil {
		return err
	}
	if cfg == nil {
		fmt.Println("Setup cancelled. Run klarity init to start over.")
		return nil
	}

	// ── 4. Guard: at least one environment must have clusters ────────────
	if len(cfg.Environments) == 0 {
		return fmt.Errorf("no environments configured — run 'klarity init' again and assign at least one cluster")
	}

	// ── 5. Save ──────────────────────────────────────────────────────────
	if pathErr != nil {
		// cfgPath was undetermined at startup (unusual) — re-resolve now.
		cfgPath, err = config.ConfigPath()
		if err != nil {
			return err
		}
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\n✅ Config saved to %s\n", cfgPath)
	fmt.Println("Run `klarity` to scan your environment.")
	fmt.Println("Tip: use --namespace or --exclude-ns to filter scans at runtime")
	return nil
}

// runFallbackPath handles manual env-naming when the user wants full control
// over cluster assignments. Kept for future use as an "Edit groupings" path.
func runFallbackPath(allContexts []string, defaults *config.Config) (*config.Config, error) {
	fmt.Printf("Found %d cluster", len(allContexts))
	if len(allContexts) != 1 {
		fmt.Print("s")
	}
	fmt.Println(":")
	for _, ctx := range allContexts {
		fmt.Printf("  • %s\n", ctx)
	}
	fmt.Println("\nManual environment assignment mode.")

	// ── Ask how many environments ────────────────────────────────────────
	var numEnvStr string
	numInput := huh.NewInput().
		Title("How many environments do you want to configure? (1-10)").
		Validate(func(s string) error {
			var n int
			if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 1 || n > 10 {
				return fmt.Errorf("enter a number between 1 and 10")
			}
			return nil
		}).
		Value(&numEnvStr)

	if err := huh.NewForm(huh.NewGroup(numInput)).Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}

	var numEnvs int
	if _, err := fmt.Sscanf(numEnvStr, "%d", &numEnvs); err != nil || numEnvs < 1 {
		return nil, fmt.Errorf("invalid number of environments: %q", numEnvStr)
	}

	// ── Collect env names + cluster assignments ──────────────────────────
	envNames := make([]string, 0, numEnvs)
	clustersByEnv := make(map[string][]string)

	for i := 0; i < numEnvs; i++ {
		var envName string
		nameInput := huh.NewInput().
			Title(fmt.Sprintf("Environment %d name:", i+1)).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return fmt.Errorf("name cannot be empty")
				}
				return nil
			}).
			Value(&envName)

		if err := huh.NewForm(huh.NewGroup(nameInput)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}
		envName = strings.TrimSpace(envName)

		if len(allContexts) == 1 {
			fmt.Printf("  ✓ Only one cluster available — auto-selected: %s\n", allContexts[0])
		}
		chosen, err := assignClusters(allContexts, func(available []string) ([]string, error) {
			opts := make([]huh.Option[string], len(available))
			for j, ctx := range available {
				opts[j] = huh.NewOption(ctx, ctx)
			}
			var sel []string
			ms := huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("Select clusters for %s:", envName)).
				Options(opts...).
				Value(&sel)
			if runErr := huh.NewForm(huh.NewGroup(ms)).Run(); runErr != nil {
				return nil, fmt.Errorf("prompt error: %w", runErr)
			}
			return sel, nil
		})
		if err != nil {
			return nil, err
		}

		envNames = append(envNames, envName)
		clustersByEnv[envName] = chosen
	}

	if len(envNames) == 0 {
		return nil, fmt.Errorf("no environments have clusters assigned — at least one environment must have a cluster")
	}

	// ── Build and validate ───────────────────────────────────────────────
	envs, err := buildEnvironmentsFromInput(envNames, clustersByEnv)
	if err != nil {
		return nil, err
	}

	// ── Confirmation summary ─────────────────────────────────────────────
	fmt.Println("\nReady to save:")
	for _, env := range envs {
		clusterWord := "cluster"
		if len(env.Clusters) != 1 {
			clusterWord = "clusters"
		}
		ctxNames := make([]string, len(env.Clusters))
		for i, cl := range env.Clusters {
			ctxNames[i] = cl.Context
		}
		fmt.Printf("  %s (%d %s): %s\n", env.Name, len(env.Clusters), clusterWord, strings.Join(ctxNames, ", "))
	}
	fmt.Println()

	cfgPath, _ := config.ConfigPath()
	confirmSave := true
	confirmPrompt := huh.NewConfirm().
		Title(fmt.Sprintf("Save to %s?", cfgPath)).
		Value(&confirmSave)

	if err := huh.NewForm(huh.NewGroup(confirmPrompt)).Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}

	if !confirmSave {
		fmt.Println("Setup cancelled. Run klarity init to start over.")
		return nil, nil
	}

	fmt.Printf("\nNamespaces: scanning all namespaces (excluding %v)\n", defaults.Settings.DefaultNsExclude)
	fmt.Println("To customize, edit ~/.klarityconfig.yaml after setup.")

	cfg := &config.Config{
		Version:      config.CurrentVersion,
		Settings:     defaults.Settings,
		Environments: envs,
	}
	return cfg, nil
}

// buildEnvironmentsFromInput constructs config.Environment slices from
// user-supplied names and cluster assignments. This is the testable core
// of the fallback path — no huh/TTY dependency.
func buildEnvironmentsFromInput(names []string, assignments map[string][]string) ([]config.Environment, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("at least one environment name is required")
	}

	var envs []config.Environment
	for _, rawName := range names {
		// Look up with original key before trimming.
		contexts := assignments[rawName]

		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, fmt.Errorf("environment name cannot be empty")
		}

		// Also try trimmed key if original didn't match.
		if len(contexts) == 0 && name != rawName {
			contexts = assignments[name]
		}
		if len(contexts) == 0 {
			return nil, fmt.Errorf("environment %q has no clusters assigned", name)
		}

		env := config.Environment{
			Name: name,
			Tier: config.InferTier(name),
		}
		for _, ctx := range contexts {
			env.Clusters = append(env.Clusters, config.Cluster{
				Context: ctx,
				Namespaces: config.NamespaceFilter{
					Mode: config.NamespaceModeAll,
				},
			})
		}
		envs = append(envs, env)
	}
	return envs, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// sortStrings is a simple insertion sort — avoids importing sort just for this.
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// printBar prints a visual separator line.
func printBar() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// assignClusters returns the cluster selection for an environment.
// If only one cluster is available it is returned immediately without calling
// promptFn. Otherwise promptFn is called in a loop until a non-empty list is
// returned, printing a re-prompt message on each empty selection.
func assignClusters(available []string, promptFn func([]string) ([]string, error)) ([]string, error) {
	if len(available) == 1 {
		return available, nil
	}
	for {
		chosen, err := promptFn(available)
		if err != nil {
			return nil, err
		}
		if len(chosen) > 0 {
			return chosen, nil
		}
		fmt.Println("  ✗ No clusters selected — please select at least one.")
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

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
