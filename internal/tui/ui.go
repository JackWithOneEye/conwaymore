// Package tui
package tui

import (
	"sync/atomic"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

type quitMessage struct{}

type UIModel struct {
	game              tea.Model
	foreground        tea.Model
	overlay           tea.Model
	foregroundVisible bool
}

func (m *UIModel) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	m.game = &gameModel{
		grid:         make([][]*uint32, 20),
		cells:        []protocol.Cell{},
		width:        40,
		height:       20,
		running:      false,
		saving:       false,
		connected:    false,
		apiHost:      "localhost:8080",
		speed:        atomic.Uint32{},
		termWidth:    80,
		currentColor: 0xFFFFFF, // Default to white
		spinner:      spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205")))),
	}
	cmds = append(cmds, m.game.Init())

	m.foreground = &foregroundModel{}
	cmds = append(cmds, m.foreground.Init())

	m.foregroundVisible = false
	m.overlay = overlay.New(m.foreground, m.game, overlay.Center, overlay.Center, 0, 0)
	cmds = append(cmds, m.overlay.Init())

	return tea.Batch(cmds...)
}

func (m *UIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}

	passToGame := func() {
		gm, gmCmd := m.game.Update(message)
		m.game = gm
		cmds = append(cmds, gmCmd)
	}

	passToForeground := func() {
		fm, fmCmd := m.foreground.Update(message)
		m.foreground = fm
		cmds = append(cmds, fmCmd)
	}

	switch msg := message.(type) {
	case colorSelectedMessage:
		m.foregroundVisible = false
		passToGame()
		return m, tea.Batch(cmds...)
	case patternSelectedMessage:
		m.foregroundVisible = false
		passToGame()
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.game.Update(quitMessage{})
			return m, tea.Quit
		case "esc":
			m.foregroundVisible = false
			// Always pass escape to game in case it's in pattern placement mode
			passToGame()
			return m, tea.Batch(cmds...)
		case "?", "ctrl+p", "ctrl+n":
			m.foregroundVisible = true
			// Sync current color from game to foreground when opening pattern selector
			if msg.String() == "ctrl+n" {
				if gm, ok := m.game.(*gameModel); ok {
					if fm, ok := m.foreground.(*foregroundModel); ok {
						fm.SetCurrentColor(gm.currentColor)
					}
				}
			}
		}
		if !m.foregroundVisible {
			passToGame()
		} else {
			passToForeground()
		}
	case tea.MouseMsg:
		if !m.foregroundVisible {
			passToGame()
		}
	default:
		passToGame()
		passToForeground()
	}

	return m, tea.Batch(cmds...)
}

func (m *UIModel) View() string {
	if m.foregroundVisible {
		return m.overlay.View()
	}
	return m.game.View()
}
