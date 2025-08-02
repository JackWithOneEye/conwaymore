package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type foregroundType int

const (
	Help foregroundType = iota
	ColorPicker
	PatternSelector
)

type foregroundModel struct {
	fgType          foregroundType
	colorPicker     *ColorPickerModel
	patternSelector *PatternSelectorModel
	currentColor    uint32
}

func (h *foregroundModel) Init() tea.Cmd {
	h.colorPicker = NewColorPickerModel()
	h.patternSelector = NewPatternSelectorModel()
	h.currentColor = 0xFFFFFF // Default white
	return nil
}

func (h *foregroundModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			h.fgType = Help
		case "ctrl+p":
			h.fgType = ColorPicker
			// h.colorPicker.Reset()
		case "ctrl+n":
			h.fgType = PatternSelector
			h.patternSelector.SetCurrentColor(h.currentColor)
		}
		if h.fgType == ColorPicker {
			_, cmd := h.colorPicker.Update(message)
			return h, cmd
		}
		if h.fgType == PatternSelector {
			_, cmd := h.patternSelector.Update(message)
			return h, cmd
		}
	}
	return h, nil
}

func (h *foregroundModel) SetCurrentColor(color uint32) {
	h.currentColor = color
	h.patternSelector.SetCurrentColor(color)
}

func (h *foregroundModel) View() string {
	switch h.fgType {
	case Help:
		return renderHelpModal()
	case ColorPicker:
		return h.colorPicker.View()
	case PatternSelector:
		return h.patternSelector.View()
	}
	return ""
}

func renderHelpModal() string {
	helpContent := `Conway's Game of Life - Controls

Game Controls:
  [space]  Play/Pause simulation
  [r]      Randomize grid
  [x]      Clear grid  
  [n]      Next step (when paused)

Speed Control:
  [S]      Decrease speed (+ 1ms)
  [s]      Increase speed (- 1ms)

Viewport Navigation:
  [h/←]    Move viewport left
  [j/↓]    Move viewport down
  [k/↑]    Move viewport up
  [l/→]    Move viewport right
  [H]      Move viewport left (10 units)
  [J]      Move viewport down (10 units)
  [K]      Move viewport up (10 units)
  [L]      Move viewport right (10 units)

Color Controls:
  [Ctrl+p] Open color picker

Pattern Library:
  [Ctrl+n] Open pattern selector

General:
  [?]      Show this help
  [q]      Quit application

Press [Esc] to close this help`

	// Use the shared modal style with proper width
	helpModalStyle := modalStyle.
		Width(45).
		MaxWidth(50).
		Align(lipgloss.Left)

	return helpModalStyle.Render(helpContent)
}
