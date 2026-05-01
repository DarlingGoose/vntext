package cmd

import (
	"fmt"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	tea "github.com/charmbracelet/bubbletea"
)

func PickGameTUI(games []*game.Game) (*game.Game, error) {
	model := newGamePickerModel(games)

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m, ok := finalModel.(gamePickerModel)
	if !ok {
		return nil, nil
	}

	if m.cancelled || m.selected == nil {
		return nil, nil
	}

	return m.selected, nil
}

type gamePickerModel struct {
	games     []*game.Game
	cursor    int
	selected  *game.Game
	cancelled bool
}

func newGamePickerModel(games []*game.Game) gamePickerModel {
	return gamePickerModel{
		games: games,
	}
}

func (m gamePickerModel) Init() tea.Cmd {
	return nil
}

func (m gamePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.cancelled = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.games)-1 {
				m.cursor++
			}

		case "home", "g":
			m.cursor = 0

		case "end", "G":
			if len(m.games) > 0 {
				m.cursor = len(m.games) - 1
			}

		case "enter":
			if len(m.games) > 0 && m.cursor >= 0 && m.cursor < len(m.games) {
				m.selected = m.games[m.cursor]
			}
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m gamePickerModel) View() string {
	if len(m.games) == 0 {
		return "No installed games found.\n"
	}

	var b strings.Builder

	b.WriteString("Select a game to run\n\n")

	for i, g := range m.games {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}

		name := "<unnamed>"
		if g != nil && strings.TrimSpace(g.Name) != "" {
			name = g.Name
		}

		engineName := ""
		if g != nil && strings.TrimSpace(g.EngineName) != "" {
			engineName = fmt.Sprintf(" [%s]", g.EngineName)
		}

		b.WriteString(fmt.Sprintf("%s %s%s\n", cursor, name, engineName))
	}

	b.WriteString("\nenter: run • q/esc: cancel • ↑/↓ or j/k: move\n")

	return b.String()
}
