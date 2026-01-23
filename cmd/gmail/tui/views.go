package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	selectedCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			MarginTop(1)
)

// renderTable renders the email selection table
func (m Model) renderTable() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("📧 Email Selection"))
	b.WriteString("\n\n")

	// Selected count
	selectedCount := len(m.getSelectedIDs())
	if selectedCount > 0 {
		b.WriteString(selectedCountStyle.Render(fmt.Sprintf("%d emails selected", selectedCount)))
		b.WriteString("\n\n")
	}

	// Table
	b.WriteString(m.table.View())
	b.WriteString("\n")

	// Help
	help := []string{
		"↑/↓: navigate",
		"space: select/deselect",
		"a: select all",
		"n: select none",
		"enter: preview deletion",
		"q: quit",
	}
	b.WriteString(helpStyle.Render(strings.Join(help, " • ")))

	return b.String()
}

// renderPreview renders the deletion preview
func (m Model) renderPreview() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("🔍 Deletion Preview"))
	b.WriteString("\n\n")

	if m.preview == nil {
		return "No preview available"
	}

	// Summary box
	summary := fmt.Sprintf(
		"%s\n\n"+
			"Total Emails:  %d\n"+
			"Total Size:    %s\n"+
			"Safety Label:  %s\n\n"+
			"%s",
		warningStyle.Render("⚠️  You are about to delete:"),
		m.preview.EmailCount,
		formatSize(m.preview.TotalSize),
		m.preview.SafetyLabel,
		"Emails will be tagged with the safety label before deletion.",
	)
	b.WriteString(boxStyle.Render(summary))
	b.WriteString("\n\n")

	// Sample of emails to delete
	b.WriteString("Preview of emails to delete:\n\n")
	maxPreview := 5
	for i, email := range m.preview.Emails {
		if i >= maxPreview {
			remaining := len(m.preview.Emails) - maxPreview
			b.WriteString(fmt.Sprintf("  ... and %d more emails\n", remaining))
			break
		}
		b.WriteString(fmt.Sprintf(
			"  • %s - %s (%s)\n",
			email.FromEmail,
			truncate(email.Subject, 40),
			formatSize(email.Size),
		))
	}
	b.WriteString("\n")

	// Confirmation prompt
	b.WriteString(warningStyle.Render("Do you want to proceed with deletion?"))
	b.WriteString("\n\n")

	// Help
	help := []string{
		"y/enter: confirm",
		"n/esc: cancel",
		"q: quit",
	}
	b.WriteString(helpStyle.Render(strings.Join(help, " • ")))

	return b.String()
}

// renderProgress renders the deletion progress
func (m Model) renderProgress() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("🗑️  Deleting Emails"))
	b.WriteString("\n\n")

	// Progress info
	b.WriteString(fmt.Sprintf(
		"Deleting %d of %d emails...\n\n",
		m.deletedCount,
		m.totalToDelete,
	))

	// Progress bar
	b.WriteString(m.progress.ViewAs(m.progressValue))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Please wait..."))

	return b.String()
}

// renderDone renders the completion screen
func (m Model) renderDone() string {
	var b strings.Builder

	if m.err != nil {
		// Error occurred
		b.WriteString(titleStyle.Render("❌ Error"))
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render(fmt.Sprintf("Deletion failed: %v", m.err)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Press q or enter to exit"))
	} else {
		// Success
		b.WriteString(titleStyle.Render("✅ Deletion Complete"))
		b.WriteString("\n\n")

		successMsg := fmt.Sprintf(
			"Successfully deleted %d emails\n"+
				"Total size freed: %s",
			m.deletedCount,
			formatSize(m.preview.TotalSize),
		)
		b.WriteString(boxStyle.Render(successStyle.Render(successMsg)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Press q or enter to exit"))
	}

	return b.String()
}
