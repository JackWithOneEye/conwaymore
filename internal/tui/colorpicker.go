package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const coarseAdjustment = 51

type colorSelectedMessage struct {
	color uint32
}

type ColorPickerModel struct {
	r            uint8 // Red value (0-255)
	g            uint8 // Green value (0-255)
	b            uint8 // Blue value (0-255)
	activeSlider int   // 0=R, 1=G, 2=B
}

func NewColorPickerModel() *ColorPickerModel {
	return &ColorPickerModel{
		r:            255,
		g:            255,
		b:            255,
		activeSlider: 0,
	}
}

func (c *ColorPickerModel) Init() tea.Cmd {
	return nil
}

func (c *ColorPickerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			// Cycle through sliders (R -> G -> B -> R)
			c.activeSlider--
			if c.activeSlider < 0 {
				c.activeSlider = 2
			}
		case "down", "j":
			// Cycle through sliders (R -> G -> B -> R)
			c.activeSlider++
			if c.activeSlider > 2 {
				c.activeSlider = 0
			}
		case "left", "h":
			// Decrease current slider value
			switch c.activeSlider {
			case 0: // Red
				if c.r > 0 {
					c.r--
				}
			case 1: // Green
				if c.g > 0 {
					c.g--
				}
			case 2: // Blue
				if c.b > 0 {
					c.b--
				}
			}
		case "right", "l":
			// Increase current slider value
			switch c.activeSlider {
			case 0: // Red
				if c.r < 255 {
					c.r++
				}
			case 1: // Green
				if c.g < 255 {
					c.g++
				}
			case 2: // Blue
				if c.b < 255 {
					c.b++
				}
			}
		case "H":
			// Coarse decrease
			switch c.activeSlider {
			case 0: // Red
				if c.r >= coarseAdjustment {
					c.r -= coarseAdjustment
				} else {
					c.r = 0
				}
			case 1: // Green
				if c.g >= coarseAdjustment {
					c.g -= coarseAdjustment
				} else {
					c.g = 0
				}
			case 2: // Blue
				if c.b >= coarseAdjustment {
					c.b -= coarseAdjustment
				} else {
					c.b = 0
				}
			}
		case "L":
			// Coarse increase
			switch c.activeSlider {
			case 0: // Red
				if c.r <= 255-coarseAdjustment {
					c.r += coarseAdjustment
				} else {
					c.r = 255
				}
			case 1: // Green
				if c.g <= 255-coarseAdjustment {
					c.g += coarseAdjustment
				} else {
					c.g = 255
				}
			case 2: // Blue
				if c.b <= 255-coarseAdjustment {
					c.b += coarseAdjustment
				} else {
					c.b = 255
				}
			}
		case "enter":
			// Convert RGB to uint32 and return
			color := (uint32(c.r) << 16) | (uint32(c.g) << 8) | uint32(c.b)
			return c, func() tea.Msg {
				return colorSelectedMessage{color: color}
			}
		}
	}
	return c, nil
}

func (c *ColorPickerModel) View() string {
	return renderRGBSliderModal(c.r, c.g, c.b, c.activeSlider)
}

// Reset resets the color picker to its initial state
func (c *ColorPickerModel) Reset() {
	c.r = 255
	c.g = 255
	c.b = 255
	c.activeSlider = 0
}

func renderRGBSliderModal(r, g, b uint8, activeSlider int) string {
	const modalWidth = 75

	// Use the shared modal style
	colorPickerModalStyle := modalStyle.Width(modalWidth)

	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(modalFg).
		Bold(true).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render("Color Picker"))
	content.WriteString("\n\n")

	// Calculate available width for content (accounting for modal padding and border)
	availableWidth := modalWidth - 6 // More conservative calculation for padding/border

	// Render sliders with improved styling
	content.WriteString(renderSliderRow("R", r, activeSlider == 0, sliderRedColor, availableWidth))
	content.WriteString(renderSliderRow("G", g, activeSlider == 1, sliderGreenColor, availableWidth))
	content.WriteString(renderSliderRow("B", b, activeSlider == 2, sliderBlueColor, availableWidth))
	content.WriteString("\n")

	// Show color preview with fixed styling
	previewColor := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
	colorPreviewStyle := lipgloss.NewStyle().
		Background(previewColor).
		Width(availableWidth).
		Height(3).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(modalBorderFg)

	content.WriteString("Preview:\n")
	content.WriteString(colorPreviewStyle.Render(""))

	// RGB values display
	rgbStyle := lipgloss.NewStyle().
		Foreground(modalFg).
		Align(lipgloss.Center)
	content.WriteString("\n")
	content.WriteString(rgbStyle.Render(fmt.Sprintf("RGB(%d,%d,%d) • %s", r, g, b, hex((uint32(r)<<16)|(uint32(g)<<8)|uint32(b)))))
	content.WriteString("\n\n")

	// Controls section with better formatting
	controlsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)

	controlsText := fmt.Sprintf(`Controls:
  [k/↑] [j/↓]  Switch slider
  [h/←] [l/→]  Adjust value (±1)
  [H]   [L]    Coarse adjust (±%d)
  [Enter]      Select color
  [Esc]        Close picker`, coarseAdjustment)

	content.WriteString(controlsStyle.Render(controlsText))

	return colorPickerModalStyle.Render(content.String())
}

func renderSliderRow(label string, value uint8, isActive bool, color lipgloss.Color, availableWidth int) string {
	// Create label with selection indicator using styles
	labelText := fmt.Sprintf("%s:", label)
	activeIndicator := "  "
	if isActive {
		activeIndicator = "▶ "
		labelText = lipgloss.NewStyle().
			Foreground(color).
			Bold(true).
			Render(labelText)
	} else {
		labelText = sliderLabelStyle.Render(labelText)
	}

	// Create value text with proper styling
	valueText := sliderValueStyle.Render(fmt.Sprintf("%3d", value))

	// Calculate slider width more accurately
	labelPart := activeIndicator + labelText
	labelWidth := lipgloss.Width(labelPart)
	valueWidth := lipgloss.Width(valueText)
	sliderWidth := availableWidth - labelWidth - valueWidth - 4 // More conservative spacing

	// Create slider using background colors instead of characters
	sliderText := renderSliderWithWidth(value, color, sliderWidth)

	// Join horizontally with proper spacing
	return lipgloss.JoinHorizontal(lipgloss.Top,
		labelPart, " ", sliderText, " ", valueText) + "\n"
}

func renderSliderWithWidth(value uint8, color lipgloss.Color, sliderWidth int) string {
	if sliderWidth < 4 {
		sliderWidth = 4 // Minimum slider width
	}

	filled := int(float64(value) / 255.0 * float64(sliderWidth-2)) // -2 for brackets
	unfilled := (sliderWidth - 2) - filled

	// Use background colors for a cleaner look
	filledStyle := lipgloss.NewStyle().Background(color)
	unfilledStyle := lipgloss.NewStyle().Background(sliderBg)

	filledBar := filledStyle.Render(strings.Repeat(" ", filled))
	unfilledBar := unfilledStyle.Render(strings.Repeat(" ", unfilled))

	// Create the slider with brackets
	bracketStyle := lipgloss.NewStyle().Foreground(modalBorderFg)
	return bracketStyle.Render("[") + filledBar + unfilledBar + bracketStyle.Render("]")
}
