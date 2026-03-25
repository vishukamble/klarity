package config

import (
	"regexp"
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
	// preprod must precede prod so word-boundary matching returns "preprod"
	// for names like "ravn-preprod-cus", not "prod".
	{label: "preprod", keywords: []string{"preprod", "pre-prod"}},
	{label: "prod", keywords: []string{"production", "prod"}},
	{label: "staging", keywords: []string{"staging", "stg"}},
	{label: "dev", keywords: []string{"development", "dev"}},
	{label: "qa", keywords: []string{"qa"}},
	{label: "uat", keywords: []string{"uat"}},
	{label: "sandbox", keywords: []string{"sandbox"}},
	{label: "test", keywords: []string{"testing", "test"}},
}

// aksRe matches AKS cluster names of the form aks-{org}-{level}-{rest}.
// Group 1 = org, group 2 = level.
var aksRe = regexp.MustCompile(`^aks-([^-]+)-([^-]+)-`)

// eksRe matches EKS/AWS cluster names of the form {project}-{level}-{rest}.
// Group 1 = project, group 2 = level (must be a known env keyword).
var eksRe = regexp.MustCompile(`^([^-]+)-(preprod|prod|production|staging|stg|dev|development|qa|uat|sandbox|test|testing)-`)

// regionSkipTokens contains location abbreviations that carry no env meaning.
var regionSkipTokens = map[string]bool{
	"cus": true, "eus": true, "wus": true, "ukw": true, "eas": true,
	"seas": true, "neu": true, "we": true, "us": true, "eu": true,
	"ap": true, "sea": true, "aze": true, "aue": true,
	"east": true, "west": true, "central": true, "north": true, "south": true,
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
// buckets using three strategies in priority order:
//  1. AKS pattern: aks-{org}-{level}-*  → "{level}-{org}"
//  2. EKS/AWS pattern: {project}-{level}-* → "{level}-{project}"
//  3. Generic keyword fallback (existing behavior)
//
// Returns (result, true) when every context matched, (result, false) when one
// or more contexts could not be classified.
func DetectEnvironments(contexts []string) (DetectedEnvs, bool) {
	result := DetectedEnvs{
		Envs: make(map[string][]string),
	}

	for _, ctx := range contexts {
		label := matchAKSPattern(ctx)
		if label == "" {
			label = matchEKSPattern(ctx)
		}
		if label == "" {
			label = matchLabel(ctx)
		}

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

// matchAKSPattern matches aks-{org}-{level}-* and returns "{level}-{org}",
// or "" if the context name does not follow the AKS naming convention.
func matchAKSPattern(contextName string) string {
	m := aksRe.FindStringSubmatch(strings.ToLower(contextName))
	if m == nil {
		return ""
	}
	org := m[1]
	level := normalizeLevel(m[2])
	if level == "" {
		return ""
	}
	return level + "-" + org
}

// matchEKSPattern matches {project}-{level}-* and returns "{level}-{project}",
// or "" if the level token is not a known env keyword.
func matchEKSPattern(contextName string) string {
	m := eksRe.FindStringSubmatch(strings.ToLower(contextName))
	if m == nil {
		return ""
	}
	project := m[1]
	level := normalizeLevel(m[2])
	if level == "" {
		return ""
	}
	return level + "-" + project
}

// normalizeLevel maps raw keyword variants to their canonical label, e.g.
// "production" → "prod", "stg" → "staging". Returns "" for unknown tokens.
func normalizeLevel(level string) string {
	for _, entry := range envKeywords {
		for _, kw := range entry.keywords {
			if kw == level {
				return entry.label
			}
		}
	}
	return ""
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

// labelForPart returns the canonical env label if part exactly equals a known
// keyword, or "" otherwise. Used for token-by-token matching in BestGuessGroup.
func labelForPart(part string) string {
	for _, entry := range envKeywords {
		for _, kw := range entry.keywords {
			if part == kw {
				return entry.label
			}
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// BestGuessGroup attempts to infer a group name for a context that matched no
// known pattern. It splits on separators, finds an env keyword token to use as
// the level, and picks the first non-region, non-numeric token as the org.
// Returns "" if no env keyword is found.
func BestGuessGroup(contextName string) string {
	lower := strings.ToLower(contextName)
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})

	var level string
	var org string

	for _, part := range parts {
		if level == "" {
			if l := labelForPart(part); l != "" {
				level = l
				continue
			}
		}
		// Skip region abbreviations, pure numbers, and very short tokens.
		if regionSkipTokens[part] || isAllDigits(part) || len(part) <= 2 {
			continue
		}
		if org == "" {
			org = part
		}
	}

	if level == "" {
		return ""
	}
	if org == "" {
		return level
	}
	return level + "-" + org
}

// HasEnvKeyword reports whether a name contains any known environment keyword
// as a word boundary. Used to detect ambiguous tier assignments.
func HasEnvKeyword(name string) bool {
	lower := strings.ToLower(name)
	for _, entry := range envKeywords {
		for _, kw := range entry.keywords {
			if containsWord(lower, kw) {
				return true
			}
		}
	}
	return false
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

// BuildDetectedConfigWithTiers is like BuildDetectedConfig but applies explicit
// tier overrides instead of inferring from label.
func BuildDetectedConfigWithTiers(selectedClusters map[string][]string, envOrder []string, tierOverrides map[string]string, defaults *Config) *Config {
	cfg := &Config{
		Version:  CurrentVersion,
		Settings: defaults.Settings,
	}

	for _, label := range envOrder {
		contexts, ok := selectedClusters[label]
		if !ok || len(contexts) == 0 {
			continue
		}
		tier, hasTier := tierOverrides[label]
		if !hasTier {
			tier = tierForLabel(label)
		}
		env := Environment{
			Name: label,
			Tier: tier,
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

// tierForLabel assigns a tier based on whether the label contains a
// production-class keyword as a word. "preprod" is treated as critical
// (same risk class as prod). This handles compound names like "prod-intel"
// (critical), "preprod-ravn" (critical), and "dev-ravn" (standard).
func tierForLabel(label string) string {
	lower := strings.ToLower(label)
	if containsWord(lower, "preprod") || containsWord(lower, "prod") || containsWord(lower, "production") {
		return TierCritical
	}
	return TierStandard
}

// InferTier returns the tier for an environment name.
// Exported for use by the init fallback path.
func InferTier(name string) string {
	return tierForLabel(name)
}
