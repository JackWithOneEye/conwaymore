package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type patternSelectedMessage struct {
	pattern *patterns.Pattern
}

type PatternSelectorModel struct {
	patterns     []*patterns.Pattern
	selected     int
	scrollOffset int
	viewHeight   int
	currentColor uint32 // current color for pattern preview
}

func NewPatternSelectorModel() *PatternSelectorModel {
	patternList := make([]*patterns.Pattern, len(patterns.Patterns))
	i := 0
	for _, pattern := range patterns.Patterns {
		patternList[i] = pattern
		i += 1
	}
	slices.SortFunc(patternList, func(a, b *patterns.Pattern) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return &PatternSelectorModel{
		patterns:     patternList,
		selected:     0,
		scrollOffset: 0,
		viewHeight:   20,       // Increased view height for larger modal
		currentColor: 0xFFFFFF, // Default white color
	}
}

func (m *PatternSelectorModel) Init() tea.Cmd {
	return nil
}

func (m *PatternSelectorModel) SetCurrentColor(color uint32) {
	m.currentColor = color
}

// renderPatternPreview creates a visual preview of the pattern
func (m *PatternSelectorModel) renderPatternPreview(pattern *patterns.Pattern) string {
	// Calculate available space: right panel (65) - padding (2) - border (2) = 61, divided by 2 for double-width chars = 30.5
	const previewWidth = 30
	// Available height: viewHeight (20) + 6 - padding (2) - border (2) = 22
	const previewHeight = 22

	// Create grid for preview
	grid := make([][]bool, previewHeight)
	for i := range grid {
		grid[i] = make([]bool, previewWidth)
	}

	// Center the pattern in the preview
	offsetX := previewWidth/2 - pattern.CenterX
	offsetY := previewHeight/2 - pattern.CenterY

	// Fill the grid with pattern cells
	for _, ps := range pattern.Cells {
		gridX := ps.X + offsetX
		gridY := ps.Y + offsetY

		if gridX < previewWidth && gridY < previewHeight {
			grid[gridY][gridX] = true
		}
	}

	// Render the grid
	var preview strings.Builder
	cellStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%06x", m.currentColor)))

	for y := range previewHeight {
		for x := range previewWidth {
			if grid[y][x] {
				preview.WriteString(cellStyle.Render("██"))
			} else {
				preview.WriteString("  ")
			}
		}
		if y < previewHeight-1 {
			preview.WriteString("\n")
		}
	}

	return lipgloss.NewStyle().
		Width(previewWidth * 2).
		Height(previewHeight).
		Render(preview.String())
}

func (m *PatternSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.moveUp()
		case "down", "j":
			m.moveDown()
		case "enter":
			if len(m.patterns) > 0 {
				return m, func() tea.Msg {
					return patternSelectedMessage{pattern: m.patterns[m.selected]}
				}
			}
		}
	}
	return m, nil
}

func (m *PatternSelectorModel) moveUp() {
	if m.selected > 0 {
		m.selected--
		// Adjust scroll offset if needed
		if m.selected < m.scrollOffset {
			m.scrollOffset = m.selected
		}
	}
}

func (m *PatternSelectorModel) moveDown() {
	if m.selected < len(m.patterns)-1 {
		m.selected++
		// Adjust scroll offset if needed
		if m.selected >= m.scrollOffset+m.viewHeight {
			m.scrollOffset = m.selected - m.viewHeight + 1
		}
	}
}

func (m *PatternSelectorModel) View() string {
	if len(m.patterns) == 0 {
		return modalStyle.Width(50).Render("No patterns available")
	}

	// Left panel: pattern list
	var listContent strings.Builder
	listContent.WriteString("Select a Pattern\n")

	// Show up arrow if there are items above
	if m.scrollOffset > 0 {
		listContent.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Align(lipgloss.Center).
			Width(1).
			Render("↑") + "\n")
	} else {
		listContent.WriteString("\n")
	}

	// Calculate visible range
	start := m.scrollOffset
	end := min(m.scrollOffset+m.viewHeight, len(m.patterns))

	// Show patterns in the visible range
	for i := start; i < end; i++ {
		pattern := m.patterns[i]
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(modalFg)

		if i == m.selected {
			prefix = "▶ "
			style = style.Foreground(lipgloss.Color("#51cf66")).Bold(true)
		}

		listContent.WriteString(style.Render(prefix+pattern.Name) + "\n")
	}

	// Show down arrow if there are items below
	if m.scrollOffset+m.viewHeight < len(m.patterns) {
		listContent.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Align(lipgloss.Center).
			Width(1).
			Render("↓"))
	}

	listContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("\n\nPress [Enter] to select, [Esc] to close"))

	// Create left panel with pattern list
	leftPanel := lipgloss.NewStyle().
		Width(40).
		Height(m.viewHeight + 6).
		Background(modalBg).
		Render(listContent.String())

	// Create divider between panels
	divider := lipgloss.NewStyle().
		Width(1).
		Height(m.viewHeight + 6).
		Background(modalBg).
		Foreground(modalBorderFg).
		Render(strings.Repeat("│", m.viewHeight+6))

	// Right panel: pattern preview with border and expanded width
	var rightPanel string
	if len(m.patterns) > 0 && m.selected < len(m.patterns) {
		selectedPattern := m.patterns[m.selected]
		preview := m.renderPatternPreview(selectedPattern)

		rightPanel = lipgloss.NewStyle().
			Width(65).
			Height(m.viewHeight + 6).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(modalBorderFg).
			Background(modalBg).
			Padding(1).
			Render(preview)
	} else {
		rightPanel = lipgloss.NewStyle().
			Width(65).
			Height(m.viewHeight+6).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(modalBorderFg).
			Background(modalBg).
			Padding(1).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No Preview Available")
	}

	// Combine left panel, divider, and right panel horizontally
	combinedContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, divider, rightPanel)

	return modalStyle.
		Width(120).               // Increased width to accommodate expanded preview and divider
		Height(m.viewHeight + 8). // Slightly increased height
		Render(combinedContent)
}
