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
			fmt.Fprintln(cmd.ErrOrStderr(), warning)
			fmt.Println()
		}
	}

	// Stable order for deterministic UX.
	sortStrings(contexts)

	// ── 2. Auto-detect environments (3-strategy) ─────────────────────────
	fmt.Println("Analyzing cluster names...")
	detected, _ := config.DetectEnvironments(contexts)

	defaults := config.DefaultConfig()
	var cfg *config.Config

	cfg, err = runNewWizard(detected, contexts, defaults)
	if err != nil {
		return err
	}

	if cfg == nil {
		// User aborted.
		fmt.Println("Setup cancelled. Run klarity init to start over.")
		return nil
	}

	// ── 3. Guard: at least one environment must have clusters ──────────
	if len(cfg.Environments) == 0 {
		return fmt.Errorf("No environments configured. Run klarity init again and assign at least one cluster to an environment.")
	}

	// ── 4. Save ─────────────────────────────────────────────────────────
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// ── 5. Offer a default environment for large configs ──────────────
	if totalConfigClusters(cfg) > 10 {
		if updated, perr := promptDefaultEnv(cfg); perr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠  Could not set default environment: %v\n", perr)
		} else if updated {
			if serr := config.Save(cfg, cfgPath); serr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠  Could not save default_env: %v\n", serr)
			}
		}
	}

	fmt.Printf("\n✅ Config saved to %s\n", cfgPath)
	fmt.Println("Run `klarity` to scan your environment.")
	fmt.Println("Tip: use --namespace or --exclude-ns to filter scans at runtime")
	return nil
}

// promptDefaultEnv shows a large-config warning and lets the user pick a
// default environment to scan. Returns true if cfg.Settings.DefaultEnv was
// set (caller must re-save). Non-fatal; errors are printed by the caller.
func promptDefaultEnv(cfg *config.Config) (updated bool, err error) {
	total := totalConfigClusters(cfg)
	fmt.Printf("\nYou have %d clusters configured. Running a full scan will take several minutes.\n\n", total)

	// Build select options: one per environment + a "no default" option.
	const noDefault = ""
	opts := make([]huh.Option[string], 0, len(cfg.Environments)+1)
	for _, env := range cfg.Environments {
		tierStr := "standard"
		if env.Tier == config.TierCritical {
			tierStr = "critical"
		}
		label := fmt.Sprintf("%-25s (%s, %d cluster", env.Name, tierStr, len(env.Clusters))
		if len(env.Clusters) != 1 {
			label += "s"
		}
		label += ")"
		opts = append(opts, huh.NewOption(label, env.Name))
	}
	opts = append(opts, huh.NewOption("No default — scan everything", noDefault))

	var chosen string
	sel := huh.NewSelect[string]().
		Title("Would you like to set a default environment?\n  (klarity will scan this by default, use --env to scan others)").
		Options(opts...).
		Value(&chosen)
	if runErr := huh.NewForm(huh.NewGroup(sel)).Run(); runErr != nil {
		return false, fmt.Errorf("prompt error: %w", runErr)
	}

	if chosen == noDefault {
		return false, nil
	}

	cfg.Settings.DefaultEnv = chosen
	fmt.Printf("Default environment set to %q.\n", chosen)
	return true, nil
}

// totalConfigClusters returns the total number of clusters across all environments.
func totalConfigClusters(cfg *config.Config) int {
	n := 0
	for _, e := range cfg.Environments {
		n += len(e.Clusters)
	}
	return n
}

// runNewWizard implements the multi-phase init wizard:
//
//	Phase 1 — Show proposed groupings, ask Accept/edit/cancel.
//	Phase 2 — Handle unmatched clusters one at a time.
//	Phase 3 — Tier confirmation (only when names are ambiguous).
//	Phase 4 — Final summary and save confirmation.
func runNewWizard(detected config.DetectedEnvs, allContexts []string, defaults *config.Config) (*config.Config, error) {
	// ── Phase 1: Display proposed groupings ─────────────────────────────
	printBar()
	fmt.Println("Proposed groupings")
	printBar()

	for _, label := range detected.Order {
		clusters := detected.Envs[label]
		tier := config.InferTier(label)
		tierStr := "standard"
		if tier == config.TierCritical {
			tierStr = "critical"
		}
		fmt.Printf("  %-25s (%s)   %d cluster", label, tierStr, len(clusters))
		if len(clusters) != 1 {
			fmt.Print("s")
		}
		fmt.Println()
	}

	if len(detected.Unmatched) > 0 {
		word := "cluster"
		if len(detected.Unmatched) != 1 {
			word = "clusters"
		}
		fmt.Printf("\nUnmatched (%d %s):\n", len(detected.Unmatched), word)
		for _, ctx := range detected.Unmatched {
			suggestion := config.BestGuessGroup(ctx)
			if suggestion != "" {
				fmt.Printf("  • %-35s → suggested: %s\n", ctx, suggestion)
			} else {
				fmt.Printf("  • %-35s → no suggestion\n", ctx)
			}
		}
	}
	printBar()

	if len(detected.Order) == 0 && len(detected.Unmatched) == 0 {
		return nil, fmt.Errorf("no clusters to configure")
	}

	var choice string
	sel := huh.NewSelect[string]().
		Title("Accept these groupings?").
		Options(
			huh.NewOption("Yes, accept all", "yes"),
			huh.NewOption("Edit/rename groups manually", "edit"),
			huh.NewOption("No, cancel setup", "no"),
		).
		Value(&choice)
	if err := huh.NewForm(huh.NewGroup(sel)).Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}

	switch choice {
	case "no":
		return nil, nil
	case "edit":
		// Fall back to the manual naming flow.
		return runFallbackPath(allContexts, defaults)
	}

	// Build working selected map from detected envs.
	selected := make(map[string][]string, len(detected.Order))
	order := make([]string, len(detected.Order))
	copy(order, detected.Order)
	for _, label := range detected.Order {
		selected[label] = detected.Envs[label]
	}

	// ── Phase 2: Handle unmatched clusters ──────────────────────────────
	for _, ctx := range detected.Unmatched {
		suggestion := config.BestGuessGroup(ctx)

		title := ctx + " → no suggestion"
		if suggestion != "" {
			title = fmt.Sprintf("%s → suggested group: %s", ctx, suggestion)
		}

		acceptLabel := "Accept (enter name)"
		if suggestion != "" {
			acceptLabel = fmt.Sprintf("Accept (%s)", suggestion)
		}

		var action string
		actionSel := huh.NewSelect[string]().
			Title(title).
			Options(
				huh.NewOption(acceptLabel, "accept"),
				huh.NewOption("Rename (enter custom group name)", "rename"),
				huh.NewOption("Skip (exclude from config)", "skip"),
			).
			Value(&action)
		if err := huh.NewForm(huh.NewGroup(actionSel)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}

		if action == "skip" {
			continue
		}

		groupName := suggestion
		if action == "rename" || groupName == "" {
			hint := strings.Join(order, ", ")
			var nameInput string
			prompt := huh.NewInput().
				Title(fmt.Sprintf("Group name for %s (existing groups: %s):", ctx, hint)).
				Value(&nameInput).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name cannot be empty")
					}
					return nil
				})
			if err := huh.NewForm(huh.NewGroup(prompt)).Run(); err != nil {
				return nil, fmt.Errorf("prompt error: %w", err)
			}
			groupName = strings.TrimSpace(nameInput)
		}

		if _, exists := selected[groupName]; !exists {
			order = append(order, groupName)
		}
		selected[groupName] = append(selected[groupName], ctx)
	}

	// ── Phase 3: Tier confirmation (only when ambiguous) ─────────────────
	// Show only if at least one env name carries no recognisable env keyword
	// (e.g. a custom group name like "intel-team").
	ambiguous := false
	for _, label := range order {
		if len(selected[label]) > 0 && !config.HasEnvKeyword(label) {
			ambiguous = true
			break
		}
	}

	if ambiguous {
		fmt.Println("\nTier assignments (critical = prod environments, shown first):")
		for _, label := range order {
			if len(selected[label]) == 0 {
				continue
			}
			tier := config.InferTier(label)
			tierStr := "standard"
			if tier == config.TierCritical {
				tierStr = "critical ✓"
			} else {
				tierStr = "standard ✓"
			}
			fmt.Printf("  %-25s → %s\n", label, tierStr)
		}

		var changeTiers bool
		confirm := huh.NewConfirm().
			Title("Change any tier assignments?").
			Value(&changeTiers)
		if err := huh.NewForm(huh.NewGroup(confirm)).Run(); err != nil {
			return nil, fmt.Errorf("prompt error: %w", err)
		}

		if changeTiers {
			tierOverrides := make(map[string]string, len(order))
			for _, label := range order {
				if len(selected[label]) == 0 {
					continue
				}
				var newTier string
				tierSel := huh.NewSelect[string]().
					Title(fmt.Sprintf("Tier for %s:", label)).
					Options(
						huh.NewOption("standard", config.TierStandard),
						huh.NewOption("critical (prod environments)", config.TierCritical),
					).
					Value(&newTier)
				if err := huh.NewForm(huh.NewGroup(tierSel)).Run(); err != nil {
					return nil, fmt.Errorf("prompt error: %w", err)
				}
				tierOverrides[label] = newTier
			}
			return config.BuildDetectedConfigWithTiers(selected, order, tierOverrides, defaults), nil
		}
	}

	// ── Phase 4: Final summary and save confirmation ─────────────────────
	fmt.Println()
	printBar()
	fmt.Println("Config summary")
	printBar()
	for _, label := range order {
		clusters := selected[label]
		if len(clusters) == 0 {
			continue
		}
		tier := config.InferTier(label)
		tierStr := "standard"
		if tier == config.TierCritical {
			tierStr = "critical"
		}
		fmt.Printf("  %-25s (%s)   %d cluster", label, tierStr, len(clusters))
		if len(clusters) != 1 {
			fmt.Print("s")
		}
		fmt.Println()
	}
	fmt.Printf("Namespaces: all (excluding %s)\n", strings.Join(defaults.Settings.DefaultNsExclude, ", "))
	printBar()

	cfgPath, _ := config.ConfigPath()
	var confirmSave bool
	savePrompt := huh.NewConfirm().
		Title(fmt.Sprintf("Save config to %s?", cfgPath)).
		Value(&confirmSave)
	if err := huh.NewForm(huh.NewGroup(savePrompt)).Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}

	if !confirmSave {
		return nil, nil
	}

	return config.BuildDetectedConfig(selected, order, defaults), nil
}

// runFallbackPath handles the manual env-naming flow when the user selects
// "Edit/rename groups manually" from the wizard, or as a legacy fallback.
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
	var confirmSave bool
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
