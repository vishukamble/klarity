package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/output"
)

var envCmd = &cobra.Command{
	Use:     "env",
	Aliases: []string{"ls"},
	Short:   "List configured environments and their clusters",
	Long: `klarity env lists every environment in ~/.klarityconfig.yaml with its tier,
cluster count, and the full set of kubeconfig context names.

Critical (prod-class) environments are highlighted in red.`,
	RunE: runEnvList,
}

func init() {
	rootCmd.AddCommand(envCmd)
}

func runEnvList(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "No config found. Run 'klarity init' to get started.")
			os.Exit(1)
		}
		return fmt.Errorf("loading config: %w", err)
	}

	noColor := !term.IsTerminal(int(os.Stdout.Fd()))
	fmt.Println(output.RenderEnvTable(cfg.Environments, noColor))
	return nil
}
