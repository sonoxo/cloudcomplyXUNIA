package cmd

import (
	"github.com/spf13/cobra"

	"cloudcomply/internal/nist"
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
		return report.RenderNIST(cmd.OutOrStdout(), nist.DemoFindings(), reportFormat)
	},
}

func init() {
	reportNistCmd.Flags().StringVar(&reportFormat, "format", "table", "output format: table|json")
	reportCmd.AddCommand(reportNistCmd)
	rootCmd.AddCommand(reportCmd)
}
