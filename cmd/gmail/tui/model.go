package tui

import (
	"fmt"
	"time"

	"cli/main/pkg/gmail"
	"cli/main/pkg/gmail/cache"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewState represents the current view in the TUI
type ViewState int

const (
	ViewTable ViewState = iota
	ViewPreview
	ViewDeleting
	ViewDone
)

// Model represents the TUI state
type Model struct {
	table          table.Model
	selected       map[string]bool // messageID -> selected
	emails         []*cache.CachedEmail
	deleter        *gmail.Deleter
	state          ViewState
	preview        *gmail.CachedDeletionPreview
	progress       progress.Model
	progressValue  float64
	totalToDelete  int
	deletedCount   int
	progressCh     <-chan int
	err            error
	width          int
	height         int
	quitting       bool
}

// NewModel creates a new TUI model
func NewModel(emails []*cache.CachedEmail, deleter *gmail.Deleter) Model {
	columns := []table.Column{
		{Title: "✓", Width: 3},
		{Title: "From", Width: 30},
		{Title: "Subject", Width: 50},
		{Title: "Date", Width: 20},
		{Title: "Size", Width: 10},
	}

	rows := make([]table.Row, len(emails))
	for i, email := range emails {
		rows[i] = table.Row{
			"", // Checkbox column
			truncate(email.FromEmail, 30),
			truncate(email.Subject, 50),
			email.Date.Format("2006-01-02 15:04"),
			formatSize(email.Size),
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return Model{
		table:    t,
		selected: make(map[string]bool),
		emails:   emails,
		deleter:  deleter,
		state:    ViewTable,
		progress: progress.New(progress.WithDefaultGradient()),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case ViewTable:
			return m.handleTableInput(msg)
		case ViewPreview:
			return m.handlePreviewInput(msg)
		case ViewDeleting:
			// No input during deletion
			return m, nil
		case ViewDone:
			if msg.String() == "q" || msg.String() == "enter" {
				m.quitting = true
				return m, tea.Quit
			}
		}

	case progressMsg:
		m.deletedCount++
		m.progressValue = float64(m.deletedCount) / float64(m.totalToDelete)

		// Continue listening for more progress updates
		if m.progressCh != nil {
			return m, m.listenForProgress(m.progressCh)
		}
		return m, nil

	case completedMsg:
		m.state = ViewDone
		return m, nil

	case errMsg:
		m.err = msg.err
		m.state = ViewDone
		return m, nil
	}

	if m.state == ViewTable {
		m.table, cmd = m.table.Update(msg)
	}

	return m, cmd
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	switch m.state {
	case ViewTable:
		return m.renderTable()
	case ViewPreview:
		return m.renderPreview()
	case ViewDeleting:
		return m.renderProgress()
	case ViewDone:
		return m.renderDone()
	default:
		return "Unknown state"
	}
}

// handleTableInput handles input in table view
func (m Model) handleTableInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case " ": // Space to toggle selection
		cursor := m.table.Cursor()
		if cursor < len(m.emails) {
			email := m.emails[cursor]
			m.selected[email.ID] = !m.selected[email.ID]
			m.updateTableRows()
		}
		return m, nil

	case "a": // Select all
		for _, email := range m.emails {
			m.selected[email.ID] = true
		}
		m.updateTableRows()
		return m, nil

	case "n": // Select none
		m.selected = make(map[string]bool)
		m.updateTableRows()
		return m, nil

	case "enter": // Proceed to preview
		if len(m.getSelectedIDs()) == 0 {
			return m, nil // Nothing selected
		}

		// Build preview for selected emails
		selectedEmails := m.getSelectedEmails()
		m.preview = &gmail.CachedDeletionPreview{
			EmailCount:  len(selectedEmails),
			Emails:      selectedEmails,
			SafetyLabel: fmt.Sprintf("crab-deleted-%d", time.Now().Unix()),
			GeneratedAt: time.Now(),
		}

		var totalSize int64
		for _, email := range selectedEmails {
			totalSize += email.Size
		}
		m.preview.TotalSize = totalSize

		m.state = ViewPreview
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// handlePreviewInput handles input in preview view
func (m Model) handlePreviewInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc": // Go back to table
		m.state = ViewTable
		return m, nil

	case "y", "enter": // Confirm deletion
		m.state = ViewDeleting
		m.totalToDelete = len(m.preview.Emails)
		m.deletedCount = 0
		m.progressValue = 0
		return m, (&m).startDeletion()

	case "n":
		m.state = ViewTable
		return m, nil
	}

	return m, nil
}

// Messages
type progressMsg struct{}
type errMsg struct{ err error }
type completedMsg struct{}

// startDeletion initiates the deletion process
func (m *Model) startDeletion() tea.Cmd {
	return func() tea.Msg {
		// Get message IDs
		messageIDs := make([]string, len(m.preview.Emails))
		for i, email := range m.preview.Emails {
			messageIDs[i] = email.ID
		}

		// Apply safety label first
		if err := m.deleter.ApplySafetyLabel(messageIDs, m.preview.SafetyLabel); err != nil {
			return errMsg{err: fmt.Errorf("failed to apply safety label: %w", err)}
		}

		// Create progress channel
		progressCh := make(chan int, len(messageIDs))
		m.progressCh = progressCh

		// Start deletion in goroutine
		go func() {
			batchSize := 50 // Process 50 at a time
			if err := m.deleter.ExecuteDeletion(messageIDs, batchSize, progressCh); err != nil {
				// Send error through a different channel mechanism
				// For now, we'll handle errors at the end
			}
		}()

		// Return command that listens for progress updates
		return m.listenForProgress(progressCh)
	}
}

// listenForProgress creates a command that waits for progress updates
func (m Model) listenForProgress(progressCh <-chan int) tea.Cmd {
	return func() tea.Msg {
		// Wait for next progress update
		_, ok := <-progressCh
		if !ok {
			// Channel closed, deletion complete
			return completedMsg{}
		}
		return progressMsg{}
	}
}

// getSelectedIDs returns IDs of selected emails
func (m Model) getSelectedIDs() []string {
	var ids []string
	for id, selected := range m.selected {
		if selected {
			ids = append(ids, id)
		}
	}
	return ids
}

// getSelectedEmails returns selected emails
func (m Model) getSelectedEmails() []*cache.CachedEmail {
	var selected []*cache.CachedEmail
	for _, email := range m.emails {
		if m.selected[email.ID] {
			selected = append(selected, email)
		}
	}
	return selected
}

// updateTableRows updates table rows with selection status
func (m Model) updateTableRows() {
	rows := make([]table.Row, len(m.emails))
	for i, email := range m.emails {
		checkbox := " "
		if m.selected[email.ID] {
			checkbox = "✓"
		}
		rows[i] = table.Row{
			checkbox,
			truncate(email.FromEmail, 30),
			truncate(email.Subject, 50),
			email.Date.Format("2006-01-02 15:04"),
			formatSize(email.Size),
		}
	}
	m.table.SetRows(rows)
}

// Utility functions
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
