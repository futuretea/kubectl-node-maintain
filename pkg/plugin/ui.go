package plugin

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

func createList(items []list.Item, title string, width, height int) list.Model {
	l := list.New(items, list.NewDefaultDelegate(), width, height)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Title = title
	titleStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1)
	l.Styles.Title = titleStyle
	return l
}
