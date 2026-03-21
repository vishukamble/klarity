package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/notifications"
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

	rootCmd.AddCommand(slackCmd)
}

func runSlackSetup(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	// Load existing config or create a minimal one for the slack section.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No existing config found. Run 'klarity init' first to set up environments.")
		return fmt.Errorf("loading config: %w", err)
	}

	// ── 1. Ask for mode ──────────────────────────────────────────────────
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
	slackCfg.OnIssuesOnly = true
	slackCfg.MinSeverity = config.SlackSeverityAll

	// ── 2. Collect credentials ───────────────────────────────────────────
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

	// ── 3. Test connection ───────────────────────────────────────────────
	fmt.Println("\nSending test message...")
	if err := notifications.TestConnection(notifications.DefaultHTTPClient, slackCfg); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Slack test failed:\n   %v\n", err)
		return fmt.Errorf("slack setup aborted — fix the issue above and re-run 'klarity slack setup'")
	}
	fmt.Println("✅ Test message sent successfully!")

	// ── 4. Ask severity filter ───────────────────────────────────────────
	var minSev string
	sevSelect := huh.NewSelect[string]().
		Title("When should klarity post to Slack?").
		Options(
			huh.NewOption("All issues (any severity)", config.SlackSeverityAll),
			huh.NewOption("High+ only (Warning and Critical)", config.SlackSeverityHigh),
			huh.NewOption("Critical only", config.SlackSeverityCritical),
		).
		Value(&minSev)
	if err := huh.NewForm(huh.NewGroup(sevSelect)).Run(); err != nil {
		return fmt.Errorf("prompt error: %w", err)
	}
	slackCfg.MinSeverity = minSev

	var onIssuesOnly bool
	confirm := huh.NewConfirm().
		Title("Only post when there are findings? (skip quiet scans)").
		Value(&onIssuesOnly)
	if err := huh.NewForm(huh.NewGroup(confirm)).Run(); err != nil {
		return fmt.Errorf("prompt error: %w", err)
	}
	slackCfg.OnIssuesOnly = onIssuesOnly
	slackCfg.Enabled = true

	// ── 5. Save ──────────────────────────────────────────────────────────
	cfg.Notifications.Slack = slackCfg
	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\n✅ Slack notifications saved to %s\n", cfgPath)
	fmt.Println("Scan summaries will be posted after each scan. Disable with:")
	fmt.Println("  notifications.slack.enabled: false  in your config file")
	return nil
}
