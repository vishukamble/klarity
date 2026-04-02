package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// ── Mock HTTP client ─────────────────────────────────────────────────────────

type mockHTTPClient struct {
	statusCode int
	body       string
	err        error
	captured   *http.Request
	capturedBody string
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.captured = req
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		m.capturedBody = string(b)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.body)),
	}, nil
}

// ── Test helpers ─────────────────────────────────────────────────────────────

func sampleFindings() []diagnosis.Finding {
	return []diagnosis.Finding{
		{
			Category:   diagnosis.CategoryOOMKilled,
			Severity:   diagnosis.SeverityCritical,
			EnvName:    "prod",
			ClusterCtx: "prod-us-east-1",
			Namespace:  "payments",
			PodName:    "pay-api-7f8d",
			OneLiner:   "OOMKilled: container exceeded 1Gi limit",
		},
		{
			Category:   diagnosis.CategoryCrashLoop,
			Severity:   diagnosis.SeverityCritical,
			EnvName:    "prod",
			ClusterCtx: "prod-us-east-1",
			Namespace:  "checkout",
			PodName:    "cart-svc-3d1",
			OneLiner:   "FATAL: password auth failed for \"cartdb\"",
		},
		{
			Category:   diagnosis.CategoryPending,
			Severity:   diagnosis.SeverityWarning,
			EnvName:    "staging",
			ClusterCtx: "staging-us-east-1",
			Namespace:  "ml",
			PodName:    "trainer-pod",
			OneLiner:   "Insufficient cpu",
		},
		{
			Category:   diagnosis.CategoryWarningEvent,
			Severity:   diagnosis.SeverityInfo,
			EnvName:    "dev",
			ClusterCtx: "dev-local",
			Namespace:  "default",
			PodName:    "test-pod",
			OneLiner:   "Back-off restarting failed container",
		},
	}
}

func sampleMeta() ScanMeta {
	return ScanMeta{
		Timestamp:    time.Date(2026, 3, 21, 14, 32, 7, 0, time.UTC),
		EnvCount:     3,
		ClusterCount: 5,
	}
}

// ── FormatSummary tests ──────────────────────────────────────────────────────

func TestFormatSummary_HasAllComponents(t *testing.T) {
	msg := FormatSummary(sampleFindings(), sampleMeta())

	// Check fallback text.
	if !strings.Contains(msg.Text, "4 issues") {
		t.Errorf("text should mention issue count, got %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "5 clusters") {
		t.Errorf("text should mention cluster count, got %q", msg.Text)
	}

	// Should have blocks.
	if len(msg.Blocks) < 3 {
		t.Fatalf("expected at least 3 blocks, got %d", len(msg.Blocks))
	}

	// Header block.
	if msg.Blocks[0].Type != "header" {
		t.Errorf("first block should be header, got %s", msg.Blocks[0].Type)
	}

	// Timestamp in section.
	found := false
	for _, b := range msg.Blocks {
		if b.Text != nil && strings.Contains(b.Text.Text, "2026-03-21") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected timestamp in a block")
	}
}

func TestFormatSummary_EnvCounts(t *testing.T) {
	msg := FormatSummary(sampleFindings(), sampleMeta())

	// Find the env counts block.
	var envBlock string
	for _, b := range msg.Blocks {
		if b.Text != nil && strings.Contains(b.Text.Text, "prod") && strings.Contains(b.Text.Text, "issue") {
			envBlock = b.Text.Text
			break
		}
	}
	if envBlock == "" {
		t.Fatal("expected a block with per-env counts")
	}
	if !strings.Contains(envBlock, "prod") || !strings.Contains(envBlock, "2") {
		t.Errorf("env block should show prod: 2 issues, got %q", envBlock)
	}
}

func TestFormatSummary_FindingBlocks(t *testing.T) {
	msg := FormatSummary(sampleFindings(), sampleMeta())

	// Each finding should appear in a block using [namespace] pod format.
	var findingsBlock string
	for _, b := range msg.Blocks {
		if b.Text != nil && strings.Contains(b.Text.Text, "OOMKilled") {
			findingsBlock = b.Text.Text
			break
		}
	}
	if findingsBlock == "" {
		t.Fatal("expected a block containing OOMKilled finding")
	}
	if !strings.Contains(findingsBlock, "[payments]") {
		t.Errorf("should show [namespace], got %q", findingsBlock)
	}
	if !strings.Contains(findingsBlock, "pay-api-7f8d") {
		t.Errorf("should show pod name, got %q", findingsBlock)
	}
}

func TestFormatSummary_AllFindingsIncluded(t *testing.T) {
	findings := make([]diagnosis.Finding, 8)
	for i := range findings {
		findings[i] = diagnosis.Finding{
			Category:   diagnosis.CategoryOOMKilled,
			Severity:   diagnosis.SeverityCritical,
			EnvName:    "prod",
			ClusterCtx: "prod-ctx",
			Namespace:  "ns",
			PodName:    fmt.Sprintf("pod-%d", i),
			OneLiner:   "oom",
		}
	}
	msg := FormatSummary(findings, sampleMeta())

	// All 8 findings should appear in a single category block (same env/cluster/category).
	var categoryBlock string
	for _, b := range msg.Blocks {
		if b.Text != nil && strings.Contains(b.Text.Text, "pod-7") {
			categoryBlock = b.Text.Text
			break
		}
	}
	if categoryBlock == "" {
		t.Error("expected all 8 findings in a block, pod-7 not found")
	}
}

func TestFormatSummary_EmptyFindings(t *testing.T) {
	msg := FormatSummary(nil, sampleMeta())
	if !strings.Contains(msg.Text, "0 issues") {
		t.Errorf("expected 0 issues in text, got %q", msg.Text)
	}
}

func TestFormatTestMessage(t *testing.T) {
	msg := FormatTestMessage()
	if !strings.Contains(msg.Text, "test message") {
		t.Errorf("test message should mention 'test message', got %q", msg.Text)
	}
	if len(msg.Blocks) == 0 {
		t.Error("test message should have blocks")
	}
}

// ── Error classification tests ───────────────────────────────────────────────

func TestClassifySlackError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantSubstr string
	}{
		{
			name:       "401 unauthorized",
			status:     401,
			body:       "invalid_auth",
			wantSubstr: "invalid or expired",
		},
		{
			name:       "403 forbidden",
			status:     403,
			body:       "",
			wantSubstr: "invalid or expired",
		},
		{
			name:       "channel_not_found",
			status:     200,
			body:       `{"ok":false,"error":"channel_not_found"}`,
			wantSubstr: "doesn't exist or bot hasn't been invited",
		},
		{
			name:       "not_in_channel",
			status:     200,
			body:       `{"ok":false,"error":"not_in_channel"}`,
			wantSubstr: "not a member",
		},
		{
			name:       "missing_scope",
			status:     200,
			body:       `{"ok":false,"error":"missing_scope"}`,
			wantSubstr: "chat:write scope",
		},
		{
			name:       "invalid_payload",
			status:     200,
			body:       `{"ok":false,"error":"invalid_payload"}`,
			wantSubstr: "malformed",
		},
		{
			name:       "unknown error",
			status:     500,
			body:       "internal_error",
			wantSubstr: "api.slack.com/apps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifySlackError(tt.status, tt.body)
			if !strings.Contains(err.Message, tt.wantSubstr) {
				t.Errorf("want message containing %q, got %q", tt.wantSubstr, err.Message)
			}
		})
	}
}

// ── SendSummary / TestConnection integration tests (mocked HTTP) ─────────────

func TestTestConnection_WebhookSuccess(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: "ok"}
	cfg := config.SlackConfig{
		Mode:       config.SlackModeWebhook,
		WebhookURL: "https://hooks.slack.com/services/T123/B456/xyz",
	}
	err := TestConnection(mock, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if mock.captured.URL.String() != cfg.WebhookURL {
		t.Errorf("request URL: want %s, got %s", cfg.WebhookURL, mock.captured.URL.String())
	}
}

func TestTestConnection_BotTokenSuccess(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: `{"ok":true}`}
	cfg := config.SlackConfig{
		Mode:     config.SlackModeBotToken,
		BotToken: "xoxb-test-token",
		Channel:  "#sre-alerts",
	}
	err := TestConnection(mock, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if mock.captured.URL.String() != "https://slack.com/api/chat.postMessage" {
		t.Errorf("request URL should be chat.postMessage API")
	}
	if mock.captured.Header.Get("Authorization") != "Bearer xoxb-test-token" {
		t.Error("should set Bearer auth header")
	}
	// Check channel is in payload.
	if !strings.Contains(mock.capturedBody, "#sre-alerts") {
		t.Error("payload should contain channel")
	}
}

func TestTestConnection_WebhookFailure(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 403, body: "invalid_token"}
	cfg := config.SlackConfig{
		Mode:       config.SlackModeWebhook,
		WebhookURL: "https://hooks.slack.com/services/bad",
	}
	err := TestConnection(mock, cfg)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var se *SlackError
	if ok := isSlackError(err, &se); !ok {
		t.Fatalf("expected SlackError, got %T", err)
	}
}

func TestTestConnection_BotTokenAPIError(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: `{"ok":false,"error":"channel_not_found"}`}
	cfg := config.SlackConfig{
		Mode:     config.SlackModeBotToken,
		BotToken: "xoxb-test",
		Channel:  "#nonexistent",
	}
	err := TestConnection(mock, cfg)
	if err == nil {
		t.Fatal("expected error for channel_not_found")
	}
	if !strings.Contains(err.Error(), "doesn't exist") {
		t.Errorf("error should mention channel doesn't exist, got: %v", err)
	}
}

func TestSendSummary_Disabled(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: "ok"}
	cfg := config.SlackConfig{Enabled: false}
	err := SendSummary(mock, cfg, sampleFindings(), sampleMeta())
	if err != nil {
		t.Fatalf("disabled should not error: %v", err)
	}
	if mock.captured != nil {
		t.Error("should not have made HTTP request when disabled")
	}
}

func TestSendSummary_PostsWithNoFindings(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: "ok"}
	cfg := config.SlackConfig{
		Enabled:    true,
		Mode:       config.SlackModeWebhook,
		WebhookURL: "https://hooks.slack.com/services/T/B/x",
	}
	// SendSummary always posts regardless of findings count.
	err := SendSummary(mock, cfg, nil, sampleMeta())
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if mock.captured == nil {
		t.Error("should have made HTTP request even with no findings")
	}
}

func TestSendSummary_PostsWhenFindings(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: "ok"}
	cfg := config.SlackConfig{
		Enabled:    true,
		Mode:       config.SlackModeWebhook,
		WebhookURL: "https://hooks.slack.com/services/T/B/x",
	}
	err := SendSummary(mock, cfg, sampleFindings(), sampleMeta())
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if mock.captured == nil {
		t.Fatal("should have made HTTP request with findings")
	}
}

func TestSendSummary_AllFindingsPosted(t *testing.T) {
	mock := &mockHTTPClient{statusCode: 200, body: "ok"}
	cfg := config.SlackConfig{
		Enabled:    true,
		Mode:       config.SlackModeWebhook,
		WebhookURL: "https://hooks.slack.com/services/T/B/x",
	}

	// All 4 sample findings (Critical, Critical, Warning, Info) should be posted.
	err := SendSummary(mock, cfg, sampleFindings(), sampleMeta())
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if mock.captured == nil {
		t.Fatal("should have posted")
	}

	var msg slackMessage
	if err := json.NewDecoder(bytes.NewReader([]byte(mock.capturedBody))).Decode(&msg); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !strings.Contains(msg.Text, "4 issues") {
		t.Errorf("payload text should mention 4 issues, got %q", msg.Text)
	}
}

func TestTruncateUTF8(t *testing.T) {
	// Build a string where a multi-byte rune (×, U+00D7, 2 UTF-8 bytes) sits at
	// rune position 2899. A raw byte-index truncation at byte 2900 would split it.
	// The rune-based truncation must produce valid UTF-8.
	prefix := strings.Repeat("a", 2899)
	text := prefix + "×rest of the text"
	runes := []rune(text)
	if len(runes) > 2900 {
		text = string(runes[:2900]) + "\n_…truncated_"
	}
	if !utf8.ValidString(text) {
		t.Errorf("truncated string is not valid UTF-8")
	}
	if strings.Contains(text, "rest") {
		t.Errorf("truncated string should not contain text past truncation point")
	}
}

// ── helper ───────────────────────────────────────────────────────────────────

// isSlackError is a helper to type-assert errors in tests.
func isSlackError(err error, target **SlackError) bool {
	if se, ok := err.(*SlackError); ok {
		*target = se
		return true
	}
	return false
}
