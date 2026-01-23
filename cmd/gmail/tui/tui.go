package tui

import (
	"fmt"

	"cli/main/pkg/gmail"
	"cli/main/pkg/gmail/cache"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI with the given emails and deleter
func Run(emails []*cache.CachedEmail, deleter *gmail.Deleter) error {
	if len(emails) == 0 {
		return fmt.Errorf("no emails to display")
	}

	p := tea.NewProgram(
		NewModel(emails, deleter),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
