// Package report renders NIST findings for the headless CLI path. It never
// sources findings itself — callers pass the data in, keeping the renderer
// agnostic to whether it came from demo data or a live Security Hub call.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"cloudcomply/internal/nist"
)

// RenderNIST writes findings to w in the requested format ("table" or
// "json"; "" defaults to "table").
func RenderNIST(w io.Writer, findings []nist.Finding, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(findings)
	case "table", "":
		return renderTable(w, findings)
	default:
		return fmt.Errorf("unsupported format %q (want: table, json)", format)
	}
}

func renderTable(w io.Writer, findings []nist.Finding) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CONTROL\tTITLE\tSTATUS\tSEVERITY\tACCOUNTS\tRMF_STEP\tMIN_IL")
	for _, f := range findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			f.ControlID, f.Title, f.Status, f.Severity, f.AccountsAffected, f.RMFStep, f.MinImpactLevel)
	}
	return tw.Flush()
}
