package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vishukamble/klarity/pkg/diagnosis"
)

func TestLoad_MissingFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil cache for missing file, got: %+v", c)
	}
}

func TestLoad_CorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(path, []byte("not valid json{{"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
	if c != nil {
		t.Fatalf("expected nil cache for corrupted file, got: %+v", c)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	c := &Cache{
		ScannedAt: now,
		Findings: []diagnosis.Finding{
			{
				Category:  diagnosis.CategoryOOMKilled,
				Severity:  diagnosis.SeverityCritical,
				EnvName:   "prod",
				ClusterCtx: "aks-prod",
				Namespace: "payments",
				PodName:   "api-123",
				OneLiner:  "OOM killed",
			},
		},
	}

	path := filepath.Join(t.TempDir(), "cache")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil cache")
	}
	if !loaded.ScannedAt.Equal(now) {
		t.Errorf("ScannedAt = %v, want %v", loaded.ScannedAt, now)
	}
	if len(loaded.Findings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(loaded.Findings))
	}
	if loaded.Findings[0].PodName != "api-123" {
		t.Errorf("PodName = %q, want api-123", loaded.Findings[0].PodName)
	}
}

func TestAge(t *testing.T) {
	scannedAt := time.Now().Add(-5 * time.Minute)
	c := &Cache{ScannedAt: scannedAt}
	age := Age(c)
	if age < 4*time.Minute || age > 6*time.Minute {
		t.Errorf("Age = %v, want ~5m", age)
	}
}

func TestIsStale(t *testing.T) {
	recent := &Cache{ScannedAt: time.Now().Add(-10 * time.Minute)}
	old := &Cache{ScannedAt: time.Now().Add(-2 * time.Hour)}

	if IsStale(recent, time.Hour) {
		t.Error("recent cache should not be stale with 1h threshold")
	}
	if !IsStale(old, time.Hour) {
		t.Error("2h-old cache should be stale with 1h threshold")
	}
}

func TestEqual_EmptySlices(t *testing.T) {
	if !Equal(nil, nil) {
		t.Error("nil == nil should be true")
	}
	if !Equal([]diagnosis.Finding{}, []diagnosis.Finding{}) {
		t.Error("empty slices should be equal")
	}
}

func TestEqual_DifferentLengths(t *testing.T) {
	a := []diagnosis.Finding{{OneLiner: "a"}}
	if Equal(a, nil) {
		t.Error("slices of different length should not be equal")
	}
}

func TestEqual_SameFindings(t *testing.T) {
	f := diagnosis.Finding{
		Category:  diagnosis.CategoryCrashLoop,
		Severity:  diagnosis.SeverityCritical,
		EnvName:   "prod",
		ClusterCtx: "aks-prod",
		Namespace: "default",
		PodName:   "pod-1",
		OneLiner:  "panic: nil pointer",
	}
	a := []diagnosis.Finding{f}
	b := []diagnosis.Finding{f}
	if !Equal(a, b) {
		t.Error("identical findings should be equal")
	}
}

func TestEqual_OrderIndependent(t *testing.T) {
	f1 := diagnosis.Finding{PodName: "pod-a", OneLiner: "err a"}
	f2 := diagnosis.Finding{PodName: "pod-b", OneLiner: "err b"}
	a := []diagnosis.Finding{f1, f2}
	b := []diagnosis.Finding{f2, f1} // reversed order
	if !Equal(a, b) {
		t.Error("Equal should be order-independent")
	}
}

func TestEqual_DifferentContent(t *testing.T) {
	a := []diagnosis.Finding{{PodName: "pod-a", OneLiner: "err a"}}
	b := []diagnosis.Finding{{PodName: "pod-a", OneLiner: "err b"}} // different OneLiner
	if Equal(a, b) {
		t.Error("findings with different content should not be equal")
	}
}
