package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Theme colors
var (
	borderColor   = lipgloss.Color("240")
	headerFg      = lipgloss.Color("#f0f0f0")
	headerBg      = lipgloss.Color("#005577")
	titleFg       = lipgloss.Color("#ffffff")
	statusFg      = lipgloss.Color("#cccccc")
	emptyCellFg   = lipgloss.Color("#666")
	errorFg       = lipgloss.Color("#ff6b6b")
	successFg     = lipgloss.Color("#51cf66")
	modalBorderFg = lipgloss.Color("62")
	modalBg       = lipgloss.Color("235")
	modalFg       = lipgloss.Color("252")
	sliderBg      = lipgloss.Color("234")
)

// Base styles
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(titleFg).
			Bold(true)

	statusStyle = lipgloss.NewStyle().
			Foreground(statusFg).
			AlignHorizontal(lipgloss.Right)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorFg).
			Bold(true)

	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(modalBorderFg).
			Background(modalBg).
			Foreground(modalFg).
			Padding(1, 2)

	emptyCellStyle = lipgloss.NewStyle().
			Width(2).
			SetString("  ")

	halfCellStyle = lipgloss.NewStyle().
			Width(1).
			SetString(" ")
)

// Color picker styles
var (
	sliderRedColor   = lipgloss.Color("196")
	sliderGreenColor = lipgloss.Color("46")
	sliderBlueColor  = lipgloss.Color("21")

	sliderLabelStyle = lipgloss.NewStyle().
				Foreground(modalFg)

	sliderValueStyle = lipgloss.NewStyle().
				Foreground(modalFg).
				Width(4).
				Align(lipgloss.Right)

	previewStyle = lipgloss.NewStyle().
			Height(3).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(modalBorderFg)
)

// styleFor returns a cached lipgloss style for the given color
func styleFor(color uint32, cache map[uint32]lipgloss.Style) lipgloss.Style {
	if s, ok := cache[color]; ok {
		return s
	}
	s := lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("#%06x", color)))
	cache[color] = s
	return s
}

// dimColor reduces the brightness of a color by approximately 50%
func dimColor(color uint32) uint32 {
	r := (color >> 16) & 0xFF
	g := (color >> 8) & 0xFF
	b := color & 0xFF

	// Reduce each component by half
	r = r / 2
	g = g / 2
	b = b / 2

	return (r << 16) | (g << 8) | b
}

// dimStyleFor returns a dimmed version of the style for the given color
func dimStyleFor(color uint32, cache map[uint32]lipgloss.Style) lipgloss.Style {
	dimmedColor := dimColor(color)
	return styleFor(dimmedColor, cache)
}

// hex converts a uint32 color to hex string
func hex(col uint32) string {
	return fmt.Sprintf("#%06x", col)
}

// boolToIcon converts a boolean to a visual indicator
func boolToIcon(b bool) string {
	if b {
		return lipgloss.NewStyle().Foreground(successFg).Render("●")
	}
	return lipgloss.NewStyle().Foreground(errorFg).Render("○")
}

// runningStatus returns a styled status indicator for running state
func runningStatus(running bool) string {
	if running {
		return lipgloss.NewStyle().Foreground(successFg).Render("▶ Running")
	}
	return lipgloss.NewStyle().Foreground(statusFg).Render("⏸ Paused")
}

// connectedStatus returns a styled status indicator for connection state
func connectedStatus(connected bool) string {
	if connected {
		return lipgloss.NewStyle().Foreground(successFg).Render("🔗 Connected")
	}
	return lipgloss.NewStyle().Foreground(errorFg).Render("🔗 Disconnected")
}
