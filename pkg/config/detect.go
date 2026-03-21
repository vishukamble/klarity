package config

import (
	"strings"
)

// envKeywords maps canonical environment names to the substrings we look for
// inside a kubeconfig context name. Order matters: more-specific keywords
// (production, development) are listed before their shorter aliases so the
// longest match wins when we build the label.
var envKeywords = []struct {
	label    string
	keywords []string
}{
	{label: "prod", keywords: []string{"production", "prod"}},
	{label: "staging", keywords: []string{"staging", "stg"}},
	{label: "dev", keywords: []string{"development", "dev"}},
	{label: "qa", keywords: []string{"qa"}},
	{label: "uat", keywords: []string{"uat"}},
	{label: "sandbox", keywords: []string{"sandbox"}},
	{label: "test", keywords: []string{"testing", "test"}},
}

// DetectedEnvs holds the result of auto-detection.
type DetectedEnvs struct {
	// Envs maps canonical environment label → ordered list of context names.
	Envs map[string][]string
	// Order preserves the order in which environment labels were first seen.
	Order []string
	// Unmatched holds context names that did not match any keyword.
	Unmatched []string
}

// DetectEnvironments classifies kubeconfig context names into environment
// buckets based on well-known keyword patterns.
//
// Returns (result, true) when every context matched at least one keyword,
// (result, false) when one or more contexts could not be classified — the
// caller should fall back to the manual assignment flow in that case.
func DetectEnvironments(contexts []string) (DetectedEnvs, bool) {
	result := DetectedEnvs{
		Envs: make(map[string][]string),
	}

	for _, ctx := range contexts {
		label := matchLabel(ctx)
		if label == "" {
			result.Unmatched = append(result.Unmatched, ctx)
			continue
		}
		if _, exists := result.Envs[label]; !exists {
			result.Order = append(result.Order, label)
		}
		result.Envs[label] = append(result.Envs[label], ctx)
	}

	allMatched := len(result.Unmatched) == 0 && len(contexts) > 0
	return result, allMatched
}

// matchLabel returns the canonical environment label for a context name, or ""
// if no keyword matches.
func matchLabel(contextName string) string {
	lower := strings.ToLower(contextName)
	for _, entry := range envKeywords {
		for _, kw := range entry.keywords {
			// Match as a word boundary: the keyword must be preceded and
			// followed by a non-letter character, or sit at the start/end.
			if containsWord(lower, kw) {
				return entry.label
			}
		}
	}
	return ""
}

// containsWord reports whether s contains kw as a "word" — surrounded by
// non-alphanumeric runes, or at a string boundary. This prevents "prod" from
// matching "reproduced" or "sandbox" from matching "sandboxed-tools".
func containsWord(s, kw string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], kw)
		if pos == -1 {
			return false
		}
		abs := idx + pos
		before := abs == 0 || !isAlphanumeric(rune(s[abs-1]))
		after := abs+len(kw) == len(s) || !isAlphanumeric(rune(s[abs+len(kw)]))
		if before && after {
			return true
		}
		idx = abs + 1
	}
}

func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// BuildDetectedConfig converts a DetectedEnvs (after the user has confirmed /
// deselected clusters) into a Config ready for saving.
//
// selectedClusters: map[envLabel][]contextName — only the clusters the user
// confirmed. tier is assigned: "prod" → critical, everything else → standard.
func BuildDetectedConfig(selectedClusters map[string][]string, envOrder []string, defaults *Config) *Config {
	cfg := &Config{
		Version:  CurrentVersion,
		Settings: defaults.Settings,
	}

	for _, label := range envOrder {
		contexts, ok := selectedClusters[label]
		if !ok || len(contexts) == 0 {
			continue
		}
		env := Environment{
			Name: label,
			Tier: tierForLabel(label),
		}
		for _, ctx := range contexts {
			env.Clusters = append(env.Clusters, Cluster{
				Context: ctx,
				Namespaces: NamespaceFilter{
					Mode:    NamespaceModeAll,
					Exclude: defaults.Settings.DefaultNsExclude,
				},
			})
		}
		cfg.Environments = append(cfg.Environments, env)
	}
	return cfg
}

// BuildManualConfig constructs a Config from manually-entered environment
// names and their assigned clusters.
//
// envNames: ordered slice of environment names entered by the user.
// clustersByEnv: map[envName][]contextName.
func BuildManualConfig(envNames []string, clustersByEnv map[string][]string, defaults *Config) *Config {
	cfg := &Config{
		Version:  CurrentVersion,
		Settings: defaults.Settings,
	}

	for _, name := range envNames {
		contexts, ok := clustersByEnv[name]
		if !ok || len(contexts) == 0 {
			continue
		}
		env := Environment{
			Name: name,
			Tier: tierForLabel(name),
		}
		for _, ctx := range contexts {
			env.Clusters = append(env.Clusters, Cluster{
				Context: ctx,
				Namespaces: NamespaceFilter{
					Mode:    NamespaceModeAll,
					Exclude: defaults.Settings.DefaultNsExclude,
				},
			})
		}
		cfg.Environments = append(cfg.Environments, env)
	}
	return cfg
}

func tierForLabel(label string) string {
	if strings.ToLower(label) == "prod" || strings.ToLower(label) == "production" {
		return TierCritical
	}
	return TierStandard
}
