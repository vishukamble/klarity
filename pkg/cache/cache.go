// Package cache manages the klarity instant-display cache and scan history log.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// Cache holds the result of the last completed scan.
type Cache struct {
	ScannedAt time.Time           `json:"scanned_at"`
	Findings  []diagnosis.Finding `json:"findings"`
}

// DefaultPath returns the path to the cache file (~/.klarity_cache).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".klarity_cache"), nil
}

// Load reads the cache file at path. Returns (nil, nil) if the file does not
// exist. Returns (nil, err) if the file is present but cannot be parsed.
func Load(path string) (*Cache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes the cache to path with mode 0600.
func Save(path string, c *Cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Age returns the duration since the cache was recorded.
func Age(c *Cache) time.Duration {
	return time.Since(c.ScannedAt)
}

// IsStale reports whether the cache is older than threshold.
func IsStale(c *Cache, threshold time.Duration) bool {
	return Age(c) > threshold
}

// Equal reports whether two sets of findings represent the same scan result.
// Both slices are sorted before comparison to account for non-deterministic
// goroutine ordering.
func Equal(a, b []diagnosis.Finding) bool {
	if len(a) != len(b) {
		return false
	}
	aj, err1 := json.Marshal(canonicalFindings(a))
	bj, err2 := json.Marshal(canonicalFindings(b))
	if err1 != nil || err2 != nil {
		return false
	}
	return string(aj) == string(bj)
}

// canonicalFindings returns a deterministically sorted copy of findings.
func canonicalFindings(findings []diagnosis.Finding) []diagnosis.Finding {
	out := make([]diagnosis.Finding, len(findings))
	copy(out, findings)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		ka := string(a.Category) + "\x00" + a.EnvName + "\x00" + a.ClusterCtx + "\x00" + a.Namespace + "\x00" + a.PodName + "\x00" + a.OneLiner
		kb := string(b.Category) + "\x00" + b.EnvName + "\x00" + b.ClusterCtx + "\x00" + b.Namespace + "\x00" + b.PodName + "\x00" + b.OneLiner
		return ka < kb
	})
	return out
}
