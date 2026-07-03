// Package tui implements the Bubble Tea interactive dashboard — the default
// entrypoint when cloudcomply is run with no subcommand.
package tui

import tea "github.com/charmbracelet/bubbletea"

// Run launches the full-screen interactive dashboard and blocks until the
// user quits.
func Run() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
