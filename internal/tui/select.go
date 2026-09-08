package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Choice struct {
	ID    string
	Label string
}

type selectModel struct {
	prompt    string
	choices   []Choice
	cursor    int
	selected  int
	confirmed bool
	cancelled bool
}

func newSelectModel(prompt string, choices []Choice) selectModel {
	return selectModel{
		prompt:   prompt,
		choices:  choices,
		selected: -1,
	}
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.selected = m.cursor
			m.confirmed = true
			return m, tea.Quit
		default:
			n, err := strconv.Atoi(msg.String())
			if err == nil && n >= 1 && n <= len(m.choices) {
				m.cursor = n - 1
				m.selected = n - 1
				m.confirmed = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m selectModel) View() string {
	var b strings.Builder
	b.WriteString(m.prompt)
	b.WriteString("\n\n")
	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		b.WriteString(fmt.Sprintf("%s %d. %s\n", cursor, i+1, choice.Label))
	}
	b.WriteString("\n↑/↓ to move • ")
	if len(m.choices) > 0 && len(m.choices) <= 9 {
		if len(m.choices) == 1 {
			b.WriteString("1 or enter to confirm")
		} else {
			b.WriteString(fmt.Sprintf("1-%d or enter to confirm", len(m.choices)))
		}
	} else {
		b.WriteString("enter to confirm")
	}
	b.WriteString(" • q to cancel\n")
	return b.String()
}

func Select(prompt string, choices []Choice) (Choice, error) {
	if len(choices) == 0 {
		return Choice{}, errors.New("tui: no choices provided")
	}

	program := tea.NewProgram(newSelectModel(prompt, choices))
	result, err := program.Run()
	if err != nil {
		return Choice{}, fmt.Errorf("tui: %w", err)
	}

	final, ok := result.(selectModel)
	if !ok {
		return Choice{}, errors.New("tui: unexpected model type")
	}
	if final.cancelled || !final.confirmed || final.selected < 0 || final.selected >= len(final.choices) {
		return Choice{}, errors.New("tui: selection cancelled")
	}
	return final.choices[final.selected], nil
}
