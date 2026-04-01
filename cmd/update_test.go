package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParseLatestVersion verifies that fetchLatestVersion correctly parses the
// tag_name field from a GitHub API response and strips the leading "v".
func TestParseLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.2.0"})
	}))
	defer server.Close()

	got, err := fetchLatestVersion(http.DefaultClient, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.2.0" {
		t.Errorf("want %q, got %q", "1.2.0", got)
	}
}

// TestParseLatestVersion_NoV checks a tag_name without a "v" prefix still works.
func TestParseLatestVersion_NoV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "2.0.0"})
	}))
	defer server.Close()

	got, err := fetchLatestVersion(http.DefaultClient, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2.0.0" {
		t.Errorf("want %q, got %q", "2.0.0", got)
	}
}

// TestParseLatestVersion_NonOK checks that a non-200 HTTP response is an error.
func TestParseLatestVersion_NonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := fetchLatestVersion(http.DefaultClient, server.URL)
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

// TestAlreadyUpToDate verifies that when the current version equals the latest,
// doUpdate prints a success message and makes exactly one HTTP request (no download).
func TestAlreadyUpToDate(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v" + Version})
	}))
	defer server.Close()

	err := doUpdate(http.DefaultClient, Version, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the API check should have been made — no download.
	if requestCount != 1 {
		t.Errorf("expected 1 API request, got %d", requestCount)
	}
}

// TestBuildDownloadURL checks the URL format for several OS/arch combinations.
func TestBuildDownloadURL(t *testing.T) {
	tests := []struct {
		version string
		goOS    string
		goArch  string
		want    string
	}{
		{
			version: "1.2.0",
			goOS:    "linux",
			goArch:  "amd64",
			want:    "https://github.com/vishukamble/klarity/releases/download/v1.2.0/klarity_linux_amd64.tar.gz",
		},
		{
			version: "1.2.0",
			goOS:    "darwin",
			goArch:  "arm64",
			want:    "https://github.com/vishukamble/klarity/releases/download/v1.2.0/klarity_darwin_arm64.tar.gz",
		},
		{
			version: "2.0.0",
			goOS:    "linux",
			goArch:  "arm64",
			want:    "https://github.com/vishukamble/klarity/releases/download/v2.0.0/klarity_linux_arm64.tar.gz",
		},
		{
			version: "1.0.0",
			goOS:    "darwin",
			goArch:  "amd64",
			want:    "https://github.com/vishukamble/klarity/releases/download/v1.0.0/klarity_darwin_amd64.tar.gz",
		},
	}
	for _, tc := range tests {
		t.Run(tc.goOS+"_"+tc.goArch, func(t *testing.T) {
			got := buildDownloadURL(tc.version, tc.goOS, tc.goArch)
			if got != tc.want {
				t.Errorf("want %q\n got %q", tc.want, got)
			}
		})
	}
}
