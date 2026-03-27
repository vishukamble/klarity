package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/notifications"
)

var (
	flagSlackEnv string
	flagSlackAll bool
)

func init() {
	slackCmd := &cobra.Command{
		Use:   "slack",
		Short: "Manage Slack notifications",
	}

	slackCmd.AddCommand(&cobra.Command{
		Use:   "setup",
		Short: "Interactive Slack integration setup — test and save",
		RunE:  runSlackSetup,
	})

	sendCmd := &cobra.Command{
		Use:   "send",
		Short: "Scan and post results to Slack immediately",
		RunE:  runSlackSend,
	}
	sendCmd.Flags().StringVar(&flagSlackEnv, "env", "",
		"Limit scan to environment(s), comma-separated (e.g. prod-intel,prod-ravn)")
	sendCmd.Flags().BoolVar(&flagSlackAll, "all", false,
		"Scan all configured environments (overrides default critical-tier-only behaviour)")
	slackCmd.AddCommand(sendCmd)

	rootCmd.AddCommand(slackCmd)
}

// ── setup ────────────────────────────────────────────────────────────────────

func runSlackSetup(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No existing config found. Run 'klarity init' first to set up environments.")
		return fmt.Errorf("loading config: %w", err)
	}

	// ── Step 1: webhook or bot token? ────────────────────────────────────
	var mode string
	modeSelect := huh.NewSelect[string]().
		Title("How do you want to connect to Slack?").
		Options(
			huh.NewOption("Incoming Webhook URL", config.SlackModeWebhook),
			huh.NewOption("Bot Token (xoxb-...)", config.SlackModeBotToken),
		).
		Value(&mode)

	if err := huh.NewForm(huh.NewGroup(modeSelect)).Run(); err != nil {
		return fmt.Errorf("prompt error: %w", err)
	}

	var slackCfg config.SlackConfig
	slackCfg.Mode = mode

	// ── Step 2: credentials ───────────────────────────────────────────────
	switch mode {
	case config.SlackModeWebhook:
		var webhookURL string
		input := huh.NewInput().
			Title("Paste your Slack webhook URL:").
			Placeholder("https://hooks.slack.com/services/T.../B.../...").
			Validate(func(s string) error {
				if !strings.HasPrefix(s, "https://hooks.slack.com/") {
					return fmt.Errorf("URL must start with https://hooks.slack.com/")
				}
				return nil
			}).
			Value(&webhookURL)
		if err := huh.NewForm(huh.NewGroup(input)).Run(); err != nil {
			return fmt.Errorf("prompt error: %w", err)
		}
		slackCfg.WebhookURL = webhookURL

	case config.SlackModeBotToken:
		var botToken string
		tokenInput := huh.NewInput().
			Title("Paste your bot token:").
			Placeholder("xoxb-...").
			Validate(func(s string) error {
				if !strings.HasPrefix(s, "xoxb-") {
					return fmt.Errorf("bot token must start with xoxb-")
				}
				return nil
			}).
			Value(&botToken)

		var channel string
		channelInput := huh.NewInput().
			Title("Which channel should klarity post to?").
			Placeholder("#sre-alerts").
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("channel name is required")
				}
				return nil
			}).
			Value(&channel)

		if err := huh.NewForm(huh.NewGroup(tokenInput, channelInput)).Run(); err != nil {
			return fmt.Errorf("prompt error: %w", err)
		}
		slackCfg.BotToken = botToken
		slackCfg.Channel = channel
	}

	// ── Step 3: test connection ───────────────────────────────────────────
	fmt.Println("\nSending test message...")
	if err := notifications.TestConnection(notifications.DefaultHTTPClient, slackCfg); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Slack test failed:\n   %v\n", err)
		return fmt.Errorf("slack setup aborted — fix the issue above and re-run 'klarity slack setup'")
	}
	fmt.Println("✅ Test message sent successfully!")

	// ── Step 4: save ─────────────────────────────────────────────────────
	slackCfg.Enabled = true
	cfg.Notifications.Slack = slackCfg
	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\n✅ Slack notifications saved to %s\n", cfgPath)
	fmt.Println("Use 'klarity slack send' to post a report manually.")
	return nil
}

// ── send ─────────────────────────────────────────────────────────────────────

// filterByCriticalTier returns a config copy containing only critical-tier environments.
func filterByCriticalTier(cfg *config.Config) *config.Config {
	out := *cfg
	out.Environments = nil
	for _, env := range cfg.Environments {
		if env.Tier == config.TierCritical {
			out.Environments = append(out.Environments, env)
		}
	}
	return &out
}

func runSlackSend(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !cfg.Notifications.Slack.Enabled {
		fmt.Fprintln(os.Stderr, "Slack is not configured. Run 'klarity slack setup' first.")
		return nil
	}

	// Determine which environments to scan.
	switch {
	case flagSlackEnv != "":
		names := parseCommaSeparated(flagSlackEnv)
		filtered, ferr := filterByEnvs(cfg, names)
		if ferr != nil {
			return ferr
		}
		cfg = filtered
	case flagSlackAll:
		// use full cfg as-is
	default:
		// default: critical-tier only
		cfg = filterByCriticalTier(cfg)
		if len(cfg.Environments) == 0 {
			fmt.Fprintln(os.Stderr, "No critical-tier environments found. Use --all or --env to specify environments.")
			return nil
		}
	}

	fmt.Printf("Scanning %d environment(s) for Slack report...\n", len(cfg.Environments))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	findings, _ := gatherFindings(ctx, cfg, buildClassifiers(), nil, nil)

	if len(findings) == 0 {
		fmt.Println("✅ No issues found — nothing to send")
		return nil
	}

	meta := notifications.ScanMeta{
		Timestamp:    time.Now(),
		EnvCount:     len(cfg.Environments),
		ClusterCount: countConfigClusters(cfg),
	}

	if err := notifications.SendSummary(notifications.DefaultHTTPClient, cfg.Notifications.Slack, findings, meta); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Slack post failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "   Check your token/webhook URL or re-run 'klarity slack setup'.")
		return err
	}

	envCount := len(cfg.Environments)
	fmt.Printf("✅ Posted to Slack — %d issues across %d environment(s)\n", len(findings), envCount)
	return nil
}
