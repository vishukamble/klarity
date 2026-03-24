package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeEntry(total int, envs map[string]int) LogEntry {
	return LogEntry{
		ScannedAt:    time.Now().UTC(),
		Environments: envs,
		Total:        total,
	}
}

func TestAppendLog_CreatesFileAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.log")

	e1 := makeEntry(3, map[string]int{"prod": 2, "dev": 1})
	if err := AppendLog(path, e1); err != nil {
		t.Fatalf("first append: %v", err)
	}

	e2 := makeEntry(0, map[string]int{"prod": 0})
	if err := AppendLog(path, e2); err != nil {
		t.Fatalf("second append: %v", err)
	}

	entries, err := ReadLog(path, 0)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].Total != 3 {
		t.Errorf("entry[0].Total = %d, want 3", entries[0].Total)
	}
	if entries[1].Total != 0 {
		t.Errorf("entry[1].Total = %d, want 0", entries[1].Total)
	}
}

func TestReadLog_MissingFile(t *testing.T) {
	entries, err := ReadLog(filepath.Join(t.TempDir(), "nofile.log"), 10)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if entries != nil {
		t.Fatalf("expected nil entries for missing file, got: %v", entries)
	}
}

func TestReadLog_Last10Of50(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.log")

	for i := 0; i < 50; i++ {
		e := makeEntry(i, map[string]int{"prod": i})
		if err := AppendLog(path, e); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	entries, err := ReadLog(path, 10)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("want 10 entries, got %d", len(entries))
	}
	// Last 10 should be entries 40–49.
	if entries[0].Total != 40 {
		t.Errorf("first of last-10 total = %d, want 40", entries[0].Total)
	}
	if entries[9].Total != 49 {
		t.Errorf("last of last-10 total = %d, want 49", entries[9].Total)
	}
}

func TestReadLog_SkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.log")

	// Write one valid + one malformed line.
	if err := AppendLog(path, makeEntry(5, map[string]int{"prod": 5})); err != nil {
		t.Fatal(err)
	}
	// Append garbage directly.
	f, _ := openAppend(path)
	f.WriteString("not valid json\n")
	f.Close()

	entries, err := ReadLog(path, 0)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 valid entry (malformed skipped), got %d", len(entries))
	}
}

func TestFilterLog_NoFilter(t *testing.T) {
	entries := []LogEntry{
		makeEntry(1, map[string]int{"prod": 1}),
		makeEntry(0, map[string]int{"prod": 0}),
	}
	out := FilterLog(entries, "")
	if len(out) != 2 {
		t.Errorf("empty filter should return all entries, got %d", len(out))
	}
}

func TestFilterLog_EnvWithIssues(t *testing.T) {
	entries := []LogEntry{
		makeEntry(2, map[string]int{"prod": 2, "dev": 0}),
		makeEntry(0, map[string]int{"prod": 0, "dev": 0}),
		makeEntry(1, map[string]int{"prod": 0, "dev": 1}),
	}
	out := FilterLog(entries, "prod")
	if len(out) != 1 {
		t.Fatalf("want 1 entry with prod issues, got %d", len(out))
	}
	if out[0].Total != 2 {
		t.Errorf("total = %d, want 2", out[0].Total)
	}
}

func TestFilterLog_EnvNotPresent(t *testing.T) {
	entries := []LogEntry{
		makeEntry(3, map[string]int{"prod": 3}),
	}
	out := FilterLog(entries, "staging")
	if len(out) != 0 {
		t.Errorf("env not in map should yield 0 results, got %d", len(out))
	}
}

// openAppend opens path for appending — helper for TestReadLog_SkipsMalformedLines.
func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
}
