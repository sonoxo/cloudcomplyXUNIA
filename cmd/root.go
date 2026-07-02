// Package cmd wires the Cobra command tree. It contains no business logic —
// commands delegate to internal/tui, internal/nist, and internal/report.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cloudcomply/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "cloudcomply",
	Short: "Assess AWS Organizations against NIST SP 800-53 / RMF controls",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

// Execute runs the CLI. version/commit/date come from ldflags set at build
// time (see .goreleaser.yml); main.go passes them through unchanged.
func Execute(version, commit, date string) error {
	rootCmd.Version = fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	return rootCmd.Execute()
}
