// Package tui implements the Bubble Tea interactive dashboard — the default
// entrypoint when cloudcomply is run with no subcommand.
package tui

import tea "github.com/charmbracelet/bubbletea"

// Run launches the full-screen interactive dashboard and blocks until the
// user quits. fetch supplies the findings and org summary — from demo data
// or a live AWS call — asynchronously on startup.
func Run(fetch FindingsFetcher) error {
	p := tea.NewProgram(initialModel(fetch), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
