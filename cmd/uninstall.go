package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove klarity binary and config",
	Long: `Removes the klarity binary from PATH and deletes ~/.klarityconfig.yaml.
Also clears the scan cache (~/.klarity_cache.json).`,
	RunE: runUninstall,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home dir: %w", err)
	}

	removed := 0
	failed := 0

	// 1. Remove config file.
	configPath := filepath.Join(homeDir, ".klarityconfig.yaml")
	if err := removeFile(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ config:  %v\n", err)
		failed++
	} else {
		fmt.Printf("  ✓ removed config:  %s\n", configPath)
		removed++
	}

	// 2. Remove cache file.
	cacheFile := filepath.Join(homeDir, ".klarity_cache.json")
	if err := removeFile(cacheFile); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  ✗ cache:   %v\n", err)
	} else if err == nil {
		fmt.Printf("  ✓ removed cache:   %s\n", cacheFile)
		removed++
	}

	// 3. Remove scan log file.
	logFile := filepath.Join(homeDir, ".klarity_log.json")
	if err := removeFile(logFile); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  ✗ log:     %v\n", err)
	} else if err == nil {
		fmt.Printf("  ✓ removed log:     %s\n", logFile)
		removed++
	}

	// 4. Remove binary (best-effort — locate via PATH).
	binaryPath, err := exec.LookPath("klarity")
	if err == nil {
		// Resolve symlinks so we remove the real file.
		if resolved, err := filepath.EvalSymlinks(binaryPath); err == nil {
			binaryPath = resolved
		}
		if err := os.Remove(binaryPath); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ binary:  %s — %v\n", binaryPath, err)
			fmt.Fprintf(os.Stderr, "    (try: sudo rm %s)\n", binaryPath)
			failed++
		} else {
			fmt.Printf("  ✓ removed binary:  %s\n", binaryPath)
			removed++
		}
	} else {
		// Binary not in PATH — check common install locations.
		candidates := []string{
			filepath.Join(homeDir, "go", "bin", "klarity"),
			filepath.Join(homeDir, ".local", "bin", "klarity"),
			"/usr/local/bin/klarity",
		}
		found := false
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				if err := os.Remove(p); err != nil {
					fmt.Fprintf(os.Stderr, "  ✗ binary:  %s — %v\n", p, err)
					failed++
				} else {
					fmt.Printf("  ✓ removed binary:  %s\n", p)
					removed++
				}
				found = true
				break
			}
		}
		if !found {
			fmt.Println("  - binary not found in PATH or common locations (already removed?)")
		}
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("Uninstall completed with %d error(s). %d item(s) removed.\n", failed, removed)
		return fmt.Errorf("uninstall incomplete — %s", pluralise(failed, "item"))
	}
	fmt.Printf("klarity uninstalled. %s removed.\n", pluralise(removed, "item"))
	fmt.Println("Run 'klarity init' after reinstalling to reconfigure.")
	return nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // already gone — not an error
	}
	return err
}

func pluralise(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, strings.TrimSuffix(noun, "s"))
}