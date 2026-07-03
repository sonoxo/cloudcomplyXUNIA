package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"cloudcomply/internal/report"
)

var reportFormat string

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate compliance reports (headless / CI mode)",
}

var reportNistCmd = &cobra.Command{
	Use:   "nist",
	Short: "Generate a NIST SP 800-53 findings report",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), fetchTimeout)
		defer cancel()
		findings, _, err := newFetcher(demoMode)(ctx)
		if err != nil {
			return err
		}
		return report.RenderNIST(cmd.OutOrStdout(), findings, reportFormat)
	},
}

func init() {
	reportNistCmd.Flags().StringVar(&reportFormat, "format", "table", "output format: table|json")
	reportCmd.AddCommand(reportNistCmd)
	rootCmd.AddCommand(reportCmd)
}
