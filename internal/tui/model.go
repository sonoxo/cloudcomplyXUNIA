package tui

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"cloudcomply/internal/nist"
)

type view int

const (
	viewDashboard view = iota
	viewFindings
)

type model struct {
	// shared
	currentView view
	quitting    bool

	// dashboard
	orgName         string
	accountCount    int
	complianceScore int
	selected        int
	menuItems       []string
	message         string

	// findings browser
	findings      []nist.Finding
	findingsTable table.Model
	families      []string
	familyIdx     int
	familyFilter  string
	impactLevels  []string
	impactIdx     int
	impactFilter  string
}

func initialModel() model {
	findings := nist.DemoFindings()
	families := []string{"ALL", "AC", "AU", "CM", "IA", "SC", "SI"}
	impactLevels := []string{"ALL", string(nist.IL2), string(nist.IL4), string(nist.IL5), string(nist.IL6)}

	return model{
		currentView:     viewDashboard,
		orgName:         "Acme Federal Org",
		accountCount:    47,
		complianceScore: nist.ComplianceScore(findings),
		selected:        0,
		menuItems: []string{
			"Run Full NIST Compliance Scan",
			"Browse Findings by Control Family",
			"Generate Threat Model",
			"View Best Practices Report",
			"Quit",
		},
		findings:      findings,
		findingsTable: buildTable(findings, "ALL", "ALL"),
		families:      families,
		familyIdx:     0,
		familyFilter:  "ALL",
		impactLevels:  impactLevels,
		impactIdx:     0,
		impactFilter:  "ALL",
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.currentView {
	case viewDashboard:
		return m.updateDashboard(msg)
	case viewFindings:
		return m.updateFindings(msg)
	}
	return m, nil
}

func (m model) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.menuItems)-1 {
				m.selected++
			}
		case "enter":
			return m.handleMenuSelection()
		}
	}
	return m, nil
}

func (m model) handleMenuSelection() (tea.Model, tea.Cmd) {
	switch m.selected {
	case 0:
		m.message = "Scan complete (demo mode). Results loaded below."
	case 1:
		m.currentView = viewFindings
		m.message = ""
	case 2:
		m.message = "Threat modeling wizard — coming soon."
	case 3:
		m.message = "Best practices report — coming soon."
	case 4:
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateFindings(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q", "esc":
			m.currentView = viewDashboard
			return m, nil
		case "left", "h":
			if m.familyIdx > 0 {
				m.familyIdx--
				m.familyFilter = m.families[m.familyIdx]
				m.findingsTable = buildTable(m.findings, m.familyFilter, m.impactFilter)
			}
			return m, nil
		case "right", "l":
			if m.familyIdx < len(m.families)-1 {
				m.familyIdx++
				m.familyFilter = m.families[m.familyIdx]
				m.findingsTable = buildTable(m.findings, m.familyFilter, m.impactFilter)
			}
			return m, nil
		case "[":
			if m.impactIdx > 0 {
				m.impactIdx--
				m.impactFilter = m.impactLevels[m.impactIdx]
				m.findingsTable = buildTable(m.findings, m.familyFilter, m.impactFilter)
			}
			return m, nil
		case "]":
			if m.impactIdx < len(m.impactLevels)-1 {
				m.impactIdx++
				m.impactFilter = m.impactLevels[m.impactIdx]
				m.findingsTable = buildTable(m.findings, m.familyFilter, m.impactFilter)
			}
			return m, nil
		}
	}

	// Pass remaining key events (up/down/etc.) to the table.
	var cmd tea.Cmd
	m.findingsTable, cmd = m.findingsTable.Update(msg)
	return m, cmd
}
