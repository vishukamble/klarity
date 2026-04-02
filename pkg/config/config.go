// Package config handles loading, saving, and validating ~/.klarityconfig.yaml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion = 1
	DefaultPath    = "~/.klarityconfig.yaml"

	TierCritical = "critical"
	TierStandard = "standard"

	NamespaceModeAll     = "all"
	NamespaceModeInclude = "include"
	NamespaceModeExclude = "exclude"
)

// NamespaceFilter controls which namespaces are scanned for a cluster.
type NamespaceFilter struct {
	// Mode is one of: all | include | exclude
	Mode    string   `yaml:"mode"`
	Include []string `yaml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

// Cluster represents a single kubeconfig context to scan.
type Cluster struct {
	// Context is the kubeconfig context name.
	Context    string          `yaml:"context"`
	Namespaces NamespaceFilter `yaml:"namespaces"`
}

// Environment groups clusters that share the same operational tier.
type Environment struct {
	Name     string    `yaml:"name"`
	Tier     string    `yaml:"tier"` // critical | standard
	Clusters []Cluster `yaml:"clusters"`
}

// Settings holds global scan behaviour parameters.
type Settings struct {
	LogTailLines         int      `yaml:"log_tail_lines"`
	ParallelClusters     int      `yaml:"parallel_clusters"`
	ParallelNamespaces   int      `yaml:"parallel_namespaces"`
	ScanIntervalSeconds  int      `yaml:"scan_interval_seconds"`
	ExcludeCompletedJobs bool     `yaml:"exclude_completed_jobs"`
	DefaultNsExclude     []string `yaml:"default_ns_exclude"`
	DefaultEnv           string   `yaml:"default_env,omitempty"`
	APIQps               float32  `yaml:"api_qps,omitempty"`   // default 50
	APIBurst             int      `yaml:"api_burst,omitempty"` // default 100
}

// Slack auth modes.
const (
	SlackModeWebhook  = "webhook"
	SlackModeBotToken = "bot_token"
)

// SlackConfig holds Slack notification settings.
type SlackConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Mode       string `yaml:"mode"`                  // webhook | bot_token
	WebhookURL string `yaml:"webhook_url,omitempty"` // if mode: webhook
	BotToken   string `yaml:"bot_token,omitempty"`   // if mode: bot_token
	Channel    string `yaml:"channel,omitempty"`     // if mode: bot_token
}

// NotificationsConfig holds all notification channel settings.
type NotificationsConfig struct {
	Slack SlackConfig `yaml:"slack,omitempty"`
}

// Config is the top-level struct that maps to ~/.klarityconfig.yaml.
type Config struct {
	Version       int                 `yaml:"version"`
	Environments  []Environment       `yaml:"environments"`
	Settings      Settings            `yaml:"settings"`
	Notifications NotificationsConfig `yaml:"notifications,omitempty"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: CurrentVersion,
		Settings: Settings{
			LogTailLines:         50,
			ParallelClusters:     4,
			ParallelNamespaces:   10,
			ScanIntervalSeconds:  300,
			ExcludeCompletedJobs: true,
			APIQps:               50,
			APIBurst:             100,
			DefaultNsExclude: []string{
				"kube-system",
				"kube-public",
				"kube-node-lease",
				"default",
			},
		},
	}
}

// ConfigPath expands and returns the canonical config file path.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".klarityconfig.yaml"), nil
}

// Load reads and parses the config file at path.
// If path is empty, ConfigPath() is used.
func Load(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = ConfigPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found at %s — run 'klarity init' to create it: %w", path, err)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("malformed YAML in config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save marshals cfg to YAML and writes it to path (creating the file if needed).
// If path is empty, ConfigPath() is used.
func Save(cfg *Config, path string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if path == "" {
		var err error
		path, err = ConfigPath()
		if err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// Validate checks that the config is semantically valid.
func (c *Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d (expected %d) — re-run 'klarity init' to migrate", c.Version, CurrentVersion)
	}

	if len(c.Environments) == 0 {
		return errors.New("config must define at least one environment")
	}

	for i, env := range c.Environments {
		if env.Name == "" {
			return fmt.Errorf("environment[%d]: name is required", i)
		}
		if env.Tier != TierCritical && env.Tier != TierStandard {
			return fmt.Errorf("environment %q: tier must be %q or %q, got %q", env.Name, TierCritical, TierStandard, env.Tier)
		}
		if len(env.Clusters) == 0 {
			return fmt.Errorf("environment %q: must define at least one cluster", env.Name)
		}
		for j, cl := range env.Clusters {
			if cl.Context == "" {
				return fmt.Errorf("environment %q cluster[%d]: context is required", env.Name, j)
			}
			if err := validateNamespaceFilter(env.Name, cl.Context, cl.Namespaces); err != nil {
				return err
			}
		}
	}

	if c.Settings.ParallelClusters < 1 {
		return errors.New("settings.parallel_clusters must be at least 1")
	}
	if c.Settings.ParallelNamespaces < 0 {
		return errors.New("settings.parallel_namespaces must be >= 0")
	}
	if c.Settings.LogTailLines < 1 {
		return errors.New("settings.log_tail_lines must be at least 1")
	}
	if c.Settings.ScanIntervalSeconds < 1 {
		return errors.New("settings.scan_interval_seconds must be at least 1")
	}

	if err := validateSlackConfig(c.Notifications.Slack); err != nil {
		return err
	}

	return nil
}

func validateSlackConfig(s SlackConfig) error {
	if !s.Enabled {
		return nil
	}
	switch s.Mode {
	case SlackModeWebhook:
		if s.WebhookURL == "" {
			return errors.New("notifications.slack: webhook_url is required when mode is webhook")
		}
	case SlackModeBotToken:
		if s.BotToken == "" {
			return errors.New("notifications.slack: bot_token is required when mode is bot_token")
		}
		if s.Channel == "" {
			return errors.New("notifications.slack: channel is required when mode is bot_token")
		}
	default:
		return fmt.Errorf("notifications.slack: mode must be %q or %q, got %q", SlackModeWebhook, SlackModeBotToken, s.Mode)
	}
	return nil
}

func validateNamespaceFilter(envName, context string, ns NamespaceFilter) error {
	switch ns.Mode {
	case NamespaceModeAll, NamespaceModeInclude, NamespaceModeExclude:
		// valid
	case "":
		return fmt.Errorf("environment %q cluster %q: namespaces.mode is required", envName, context)
	default:
		return fmt.Errorf("environment %q cluster %q: namespaces.mode must be %q, %q, or %q, got %q",
			envName, context, NamespaceModeAll, NamespaceModeInclude, NamespaceModeExclude, ns.Mode)
	}

	if ns.Mode == NamespaceModeInclude && len(ns.Include) == 0 {
		return fmt.Errorf("environment %q cluster %q: namespaces.include must not be empty when mode is %q",
			envName, context, NamespaceModeInclude)
	}

	return nil
}
