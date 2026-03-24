package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogEntry records the summary of a single completed scan.
type LogEntry struct {
	ScannedAt    time.Time      `json:"scanned_at"`
	Environments map[string]int `json:"environments"`
	Total        int            `json:"total"`
}

// LogPath returns the path to the NDJSON scan log (~/.klarity.log).
func LogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".klarity.log"), nil
}

// AppendLog marshals entry as a single JSON line and appends it to path.
// The file is created with mode 0600 if it does not yet exist.
func AppendLog(path string, entry LogEntry) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// ReadLog reads the NDJSON log at path and returns up to last entries, most
// recent last. If last ≤ 0 all entries are returned. Malformed lines are
// silently skipped. Returns (nil, nil) when the file does not exist.
func ReadLog(path string, last int) ([]LogEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var entries []LogEntry
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}

	if last > 0 && len(entries) > last {
		entries = entries[len(entries)-last:]
	}
	return entries, nil
}

// FilterLog returns entries where the named environment had at least one issue.
// If env is empty, all entries are returned unchanged.
func FilterLog(entries []LogEntry, env string) []LogEntry {
	if env == "" {
		return entries
	}
	var out []LogEntry
	for _, e := range entries {
		if count, ok := e.Environments[env]; ok && count > 0 {
			out = append(out, e)
		}
	}
	return out
}
