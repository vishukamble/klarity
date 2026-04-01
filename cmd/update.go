package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	githubAPILatest    = "https://api.github.com/repos/vishukamble/klarity/releases/latest"
	githubDownloadBase = "https://github.com/vishukamble/klarity/releases/download"
)

// updateHTTPClient is injectable for testing.
type updateHTTPClient interface {
	Get(url string) (*http.Response, error)
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update klarity to the latest version",
		Long:  `Downloads and replaces the current binary with the latest release from GitHub.`,
		RunE:  runUpdate,
	})
}

func runUpdate(_ *cobra.Command, _ []string) error {
	return doUpdate(&http.Client{}, Version, githubAPILatest)
}

// doUpdate is the testable core of the update flow.
// apiURL is the GitHub releases API endpoint (injectable for tests).
func doUpdate(client updateHTTPClient, currentVersion, apiURL string) error {
	// 1. Fetch latest version.
	latestVersion, err := fetchLatestVersion(client, apiURL)
	if err != nil {
		return err
	}

	// 2. Already up to date?
	if latestVersion == currentVersion {
		fmt.Printf("✓ klarity v%s is already the latest version.\n", currentVersion)
		return nil
	}

	fmt.Printf("Update available: v%s → v%s\n", currentVersion, latestVersion)
	fmt.Println("Downloading...")

	// 3. Detect current binary path.
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("detecting binary path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolving symlink: %w", err)
	}

	// 4. Build download URL from OS/arch.
	downloadURL := buildDownloadURL(latestVersion, runtime.GOOS, runtime.GOARCH)

	// 5. Download to temp dir, extract binary.
	tmpDir, err := os.MkdirTemp("", "klarity-update-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	newBinaryPath, err := downloadAndExtract(client, downloadURL, tmpDir)
	if err != nil {
		return err
	}

	// 6. Atomically replace the current binary.
	if err := replaceBinary(exePath, newBinaryPath); err != nil {
		return err
	}

	fmt.Printf("✓ klarity updated to v%s\n", latestVersion)
	return nil
}

// githubRelease is the subset of the GitHub API response we care about.
type githubRelease struct {
	TagName string `json:"tag_name"`
}

// fetchLatestVersion calls the GitHub releases API and returns the latest
// version string (without the leading "v").
func fetchLatestVersion(client updateHTTPClient, apiURL string) (string, error) {
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("could not reach GitHub — check your connection")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d — visit https://github.com/vishukamble/klarity/releases to update manually", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parsing GitHub API response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// buildDownloadURL constructs the tarball URL for the given version, OS, and arch.
func buildDownloadURL(version, goOS, goArch string) string {
	return fmt.Sprintf("%s/v%s/klarity_%s_%s.tar.gz", githubDownloadBase, version, goOS, goArch)
}

// downloadAndExtract downloads the tarball from url, extracts the "klarity"
// binary into tmpDir, and returns its path.
func downloadAndExtract(client updateHTTPClient, url, tmpDir string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d — visit https://github.com/vishukamble/klarity/releases", resp.StatusCode)
	}

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("decompressing update: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tarball: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "klarity" {
			continue
		}

		outPath := filepath.Join(tmpDir, "klarity")
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return "", fmt.Errorf("writing update binary: %w", err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return "", fmt.Errorf("writing update binary: %w", err)
		}
		f.Close()
		return outPath, nil
	}

	return "", fmt.Errorf("binary 'klarity' not found in update tarball")
}

// replaceBinary atomically replaces exePath with the binary at newBinaryPath.
// It writes to a temp file in the same directory first to ensure os.Rename is
// on the same filesystem (required for atomicity).
func replaceBinary(exePath, newBinaryPath string) error {
	dir := filepath.Dir(exePath)
	tmpPath := filepath.Join(dir, ".klarity-update-tmp")

	src, err := os.Open(newBinaryPath)
	if err != nil {
		return fmt.Errorf("reading new binary: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied — try: sudo klarity update")
		}
		return fmt.Errorf("preparing update: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing update: %w", err)
	}
	dst.Close()

	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied — try: sudo klarity update")
		}
		return fmt.Errorf("replacing binary: %w", err)
	}

	return nil
}
