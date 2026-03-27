// Package notifications handles sending scan results to external channels.
package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// HTTPClient is the interface used for HTTP calls. Inject a mock in tests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient is the production HTTP client.
var DefaultHTTPClient HTTPClient = &http.Client{Timeout: 10 * time.Second}

// ScanMeta carries metadata about the scan for message formatting.
type ScanMeta struct {
	Timestamp   time.Time
	EnvCount    int
	ClusterCount int
}

// ── Slack Block Kit types ────────────────────────────────────────────────────

type slackBlock struct {
	Type string      `json:"type"`
	Text *slackText  `json:"text,omitempty"`
}

type slackText struct {
	Type string `json:"type"` // mrkdwn | plain_text
	Text string `json:"text"`
}

type slackMessage struct {
	Channel string       `json:"channel,omitempty"` // for bot token posts
	Text    string       `json:"text"`              // fallback for notifications
	Blocks  []slackBlock `json:"blocks"`
}

// ── Error classification ─────────────────────────────────────────────────────

// SlackError wraps an API error with a user-friendly fix suggestion.
type SlackError struct {
	StatusCode int
	APIError   string // raw error from Slack API
	Message    string // user-facing message
}

func (e *SlackError) Error() string { return e.Message }

// classifySlackError maps HTTP status codes and Slack API error strings to
// user-friendly guidance.
func classifySlackError(statusCode int, body string) *SlackError {
	se := &SlackError{StatusCode: statusCode}

	// Parse Slack API JSON error if present.
	var apiResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &apiResp); err == nil && apiResp.Error != "" {
		se.APIError = apiResp.Error
	}

	switch {
	case statusCode == 401 || statusCode == 403:
		se.Message = "Token is invalid or expired. Regenerate at api.slack.com/apps"
	case se.APIError == "channel_not_found":
		se.Message = "Channel doesn't exist or bot hasn't been invited. Run: /invite @klarity in that channel"
	case se.APIError == "not_in_channel":
		se.Message = "Bot is not a member. Run: /invite @klarity in the target channel"
	case se.APIError == "missing_scope":
		se.Message = "Bot is missing chat:write scope. Add it at api.slack.com/apps → OAuth & Permissions"
	case se.APIError == "invalid_payload" || se.APIError == "invalid_payload_json":
		se.Message = "Webhook URL may be malformed. Double-check the URL from api.slack.com"
	case statusCode == 404 || strings.Contains(body, "invalid_token"):
		se.Message = "Webhook URL may be malformed. Double-check the URL from api.slack.com"
	default:
		raw := se.APIError
		if raw == "" {
			raw = strings.TrimSpace(body)
			if len(raw) > 200 {
				raw = raw[:200] + "..."
			}
		}
		se.Message = fmt.Sprintf("Slack API error: %s. Check your configuration at api.slack.com/apps", raw)
	}
	return se
}

// ── Message formatting ───────────────────────────────────────────────────────

// FormatSummary builds a Slack Block Kit message from scan findings, grouped
// by environment → cluster → category.
func FormatSummary(findings []diagnosis.Finding, meta ScanMeta) slackMessage {
	timestamp := meta.Timestamp.Format("2006-01-02 15:04:05 MST")

	// ── Per-env summary ──────────────────────────────────────────────────
	envCounts := make(map[string]int)
	for _, f := range findings {
		envCounts[f.EnvName]++
	}
	var envNames []string
	for e := range envCounts {
		envNames = append(envNames, e)
	}
	sort.Strings(envNames)
	var envLines []string
	for _, e := range envNames {
		envLines = append(envLines, fmt.Sprintf("• *%s*: %d issue(s)", e, envCounts[e]))
	}

	// ── Group findings by (env, cluster, category) ───────────────────────
	type catKey struct {
		env, cluster string
		cat          diagnosis.Category
	}
	catGroups := make(map[catKey][]diagnosis.Finding)
	var catOrder []catKey
	for _, f := range findings {
		k := catKey{f.EnvName, f.ClusterCtx, f.Category}
		if _, ok := catGroups[k]; !ok {
			catOrder = append(catOrder, k)
		}
		catGroups[k] = append(catGroups[k], f)
	}

	footer := fmt.Sprintf("%d issues found across %d clusters", len(findings), meta.ClusterCount)

	// ── Build blocks ──────────────────────────────────────────────────────
	blocks := []slackBlock{
		{Type: "header", Text: &slackText{Type: "plain_text", Text: "klarity scan results"}},
		{Type: "section", Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("*klarity scan — %s*", timestamp)}},
	}

	if len(envLines) > 0 {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: strings.Join(envLines, "\n")},
		})
	}

	// One section block per category per cluster.
	for _, k := range catOrder {
		fs := catGroups[k]
		var lines []string
		for _, f := range fs {
			resource := f.PodName
			if resource == "" {
				resource = "-"
			}
			lines = append(lines, fmt.Sprintf("`[%s] %s` — %s", f.Namespace, resource, f.OneLiner))
		}
		title := fmt.Sprintf("*%s / %s — %s*", k.env, k.cluster, string(k.cat))
		text := title + "\n" + strings.Join(lines, "\n")
		if len(text) > 2900 {
			text = text[:2900] + "\n_…truncated_"
		}
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: text},
		})
	}

	blocks = append(blocks,
		slackBlock{Type: "divider"},
		slackBlock{Type: "section", Text: &slackText{Type: "mrkdwn", Text: "_" + footer + "_"}},
	)

	return slackMessage{
		Text:   footer,
		Blocks: blocks,
	}
}

// FormatTestMessage builds the test connection message.
func FormatTestMessage() slackMessage {
	return slackMessage{
		Text: "klarity test message — if you see this, Slack is configured correctly.",
		Blocks: []slackBlock{
			{Type: "section", Text: &slackText{
				Type: "mrkdwn",
				Text: ":test_tube: *klarity test message* — if you see this, Slack is configured correctly.",
			}},
		},
	}
}

// ── Sending ──────────────────────────────────────────────────────────────────

// postMessage sends a slackMessage using the given config and HTTP client.
func postMessage(client HTTPClient, cfg config.SlackConfig, msg slackMessage) error {
	var url string
	var authHeader string

	switch cfg.Mode {
	case config.SlackModeWebhook:
		url = cfg.WebhookURL
	case config.SlackModeBotToken:
		url = "https://slack.com/api/chat.postMessage"
		authHeader = "Bearer " + cfg.BotToken
		msg.Channel = cfg.Channel
	default:
		return fmt.Errorf("unknown slack mode: %q", cfg.Mode)
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling slack message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending slack message: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Webhook returns "ok" as plain text on success.
	if cfg.Mode == config.SlackModeWebhook {
		if resp.StatusCode != http.StatusOK || strings.TrimSpace(bodyStr) != "ok" {
			return classifySlackError(resp.StatusCode, bodyStr)
		}
		return nil
	}

	// Bot token returns JSON with {ok: true/false}.
	if resp.StatusCode != http.StatusOK {
		return classifySlackError(resp.StatusCode, bodyStr)
	}
	var apiResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return classifySlackError(resp.StatusCode, bodyStr)
	}
	if !apiResp.OK {
		return classifySlackError(resp.StatusCode, bodyStr)
	}
	return nil
}

// TestConnection sends a test message to verify Slack is configured correctly.
func TestConnection(client HTTPClient, cfg config.SlackConfig) error {
	return postMessage(client, cfg, FormatTestMessage())
}

// SendSummary posts a scan summary to Slack. Returns nil without posting if
// Slack is disabled.
func SendSummary(client HTTPClient, cfg config.SlackConfig, findings []diagnosis.Finding, meta ScanMeta) error {
	if !cfg.Enabled {
		return nil
	}
	msg := FormatSummary(findings, meta)
	return postMessage(client, cfg, msg)
}
