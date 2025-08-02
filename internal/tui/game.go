package tui

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/coder/websocket"
)

type gameModel struct {
	worldSize    int
	grid         [][]*uint32
	cells        []protocol.Cell
	width        int
	height       int
	running      bool
	saving       bool
	connected    bool
	conn         *websocket.Conn
	apiHost      string
	speed        atomic.Uint32
	err          error
	hasHalfCol   bool   // true if rightmost column should be drawn as half-width
	termWidth    int    // terminal width for responsive layout
	viewportX    int    // viewport offset X (camera position)
	viewportY    int    // viewport offset Y (camera position)
	currentColor uint32 // currently selected color for new cells
	spinner      spinner.Model

	// Pattern placement mode
	placingPattern  bool              // true when in pattern placement mode
	currentPattern  *patterns.Pattern // pattern being placed
	patternCanPlace bool              // true if pattern can be placed at current position
}

func (m *gameModel) Init() tea.Cmd {
	for i := range m.grid {
		m.grid[i] = make([]*uint32, m.width)
	}
	return connectToAPI(m.apiHost)
}

func (m *gameModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case colorSelectedMessage:
		m.currentColor = msg.color
		return m, nil
	case patternSelectedMessage:
		// Enter pattern placement mode
		m.placingPattern = true
		m.currentPattern = msg.pattern
		m.updatePatternCanPlace()
		return m, nil
	case tea.MouseMsg:
		// Disable mouse clicks during pattern placement mode
		if m.placingPattern {
			return m, nil
		}

		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft && m.isConnected() {
			// Calculate grid position from mouse click
			// Account for header lines and frame borders
			headerLines := 1
			if m.err != nil {
				headerLines = 2
			}

			// Check if click is within the grid area
			clickY := msg.Y - headerLines - 1 // Subtract header and top border
			clickX := msg.X - 1               // Subtract left border

			if clickY >= 0 && clickY < m.height && clickX >= 0 {
				// Convert screen coordinates to grid coordinates
				gridX := clickX / 2 // Each cell is 2 characters wide (or 1 for half column)
				if m.hasHalfCol && gridX >= m.width-1 {
					gridX = m.width - 1
				}
				gridY := clickY

				// Convert grid coordinates to world coordinates (apply viewport offset)
				worldX, worldY := m.viewportToWorld(gridX, gridY)

				// Create a cell at this position with the current color
				newCell := protocol.Cell{
					X:      uint16(worldX),
					Y:      uint16(worldY),
					Colour: m.currentColor,
					Age:    1, // New cell starts at age 1
				}

				return m, sendCells(m.conn, []protocol.Cell{newCell})
			}
		}
	case tea.KeyMsg:
		// Handle pattern placement mode first
		if m.placingPattern {
			switch msg.String() {
			case "h", "left":
				m.moveViewport(-1, 0)
			case "j", "down":
				m.moveViewport(0, 1)
			case "k", "up":
				m.moveViewport(0, -1)
			case "l", "right":
				m.moveViewport(1, 0)
			case "H":
				m.moveViewport(-10, 0)
			case "J":
				m.moveViewport(0, 10)
			case "K":
				m.moveViewport(0, -10)
			case "L":
				m.moveViewport(10, 0)
			case "enter":
				if m.patternCanPlace && m.isConnected() {
					// Place the pattern by sending cells to server
					cells := m.getPatternCells()
					m.placingPattern = false
					return m, sendCells(m.conn, cells)
				}
			case "esc":
				// Abort pattern placement
				m.placingPattern = false
			}
			return m, nil
		}

		// Normal game controls
		switch msg.String() {
		case " ":
			if m.isConnected() {
				if m.running {
					return m, sendCommand(m.conn, protocol.Pause)
				}
				return m, sendCommand(m.conn, protocol.Play)
			}
		case "r":
			if m.isConnected() {
				return m, sendCommand(m.conn, protocol.Randomise)
			}
		case "x":
			if m.isConnected() {
				return m, sendCommand(m.conn, protocol.Clear)
			}
		case "n":
			if m.isConnected() {
				return m, sendCommand(m.conn, protocol.Next)
			}
		case "h", "left":
			m.moveViewport(-1, 0)
		case "j", "down":
			m.moveViewport(0, 1)
		case "k", "up":
			m.moveViewport(0, -1)
		case "l", "right":
			m.moveViewport(1, 0)
		case "H":
			m.moveViewport(-10, 0)
		case "J":
			m.moveViewport(0, 10)
		case "K":
			m.moveViewport(0, -10)
		case "L":
			m.moveViewport(10, 0)
		case "S":
			if m.isConnected() {
				speed := m.speed.Add(1)
				return m, sendSpeed(m.conn, uint16(speed))
			}
		case "s":
			speed := m.speed.Load()
			if m.isConnected() && speed > 1 {
				speed -= 1
				m.speed.Store(speed)
				return m, sendSpeed(m.conn, uint16(speed))
			}
		case "ctrl+s":
			if m.isConnected() {
				m.saving = true
				return m, tea.Batch(m.spinner.Tick, saveGame(m.apiHost))
			}
		}
	case quitMessage:
		if m.conn != nil {
			m.conn.Close(websocket.StatusNormalClosure, "")
		}
		return m, nil
	case connectionResult:
		m.connected = msg.Connected
		m.err = msg.Err
		m.conn = msg.Conn
		m.worldSize = int(msg.WorldSize)
		if m.isConnected() {
			// Start listening for messages
			return m, listenForMessages(m.conn)
		}
	case wsMessage:
		if msg.Err != nil {
			m.err = msg.Err
			m.connected = false
			return m, nil
		}

		cells, playing, speed, err := processServerMessage(msg.Data)
		if err != nil {
			m.err = err
		} else {
			m.cells = cells
			m.running = playing
			m.speed.Store(uint32(speed))
			m.updateGrid()
		}

		// Continue listening for messages
		if m.isConnected() {
			return m, listenForMessages(m.conn)
		}
	case saveGameResult:
		if msg.Err != nil {
			m.err = msg.Err
		}
		m.saving = false
	case spinner.TickMsg:
		if m.saving {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case tea.WindowSizeMsg:
		// Update terminal width for responsive header
		m.termWidth = msg.Width

		// Adjust grid size based on terminal size
		if msg.Height > 3 && msg.Width > 6 {
			// Account for: 1 header line + frame borders (2 vertical)
			newHeight := msg.Height - 3
			// Calculate available width for grid content
			availableWidth := msg.Width - 2
			// Each full cell takes 2 chars, but we can use 1 extra char for half-width column
			newWidth := availableWidth / 2
			newHasHalfCol := (availableWidth % 2) == 1

			if newHeight != m.height || newWidth != m.width || newHasHalfCol != m.hasHalfCol {
				m.height = newHeight
				m.width = newWidth
				if newHasHalfCol {
					m.width += 1
				}
				m.hasHalfCol = newHasHalfCol
				m.grid = make([][]*uint32, m.height)
				for i := range m.grid {
					m.grid[i] = make([]*uint32, m.width)
				}
				m.updateGrid()
				// Update pattern collision detection when viewport size changes
				if m.placingPattern {
					m.updatePatternCanPlace()
				}
			}
		}
	}
	return m, nil
}

func (m *gameModel) View() string {
	var s strings.Builder
	cellStyleCache := make(map[uint32]lipgloss.Style)

	// Create header with title and status
	title := titleStyle.Render("Conway's Game of Life - Terminal UI")
	if m.saving {
		title += fmt.Sprintf(" %s", m.spinner.View())
	}
	colorSwatch := styleFor(m.currentColor, cellStyleCache).Render("██")

	statusText := ""
	if m.placingPattern {
		placementStatus := "✓ Can Place"
		if !m.patternCanPlace {
			placementStatus = "✗ Cannot Place"
		}
		statusText = fmt.Sprintf("Placing: %s • %s • Viewport: (%d,%d) • [Arrow/hjkl] Move View [Enter] Place [Esc] Cancel",
			m.currentPattern.Name,
			placementStatus,
			m.viewportX, m.viewportY)
	} else {
		statusText = fmt.Sprintf("Size: %dx%d • %s • %s • Speed: %d ms • View: (%d,%d) • Color: %s",
			m.width, m.height,
			runningStatus(m.running),
			connectedStatus(m.connected),
			m.speed.Load(),
			m.viewportX, m.viewportY,
			colorSwatch)
	}

	status := statusStyle.Render(statusText)

	// Calculate available width for status alignment (subtract padding and title width)
	availableWidth := m.termWidth - 2 // 1 char padding on each side
	titleWidth := lipgloss.Width(title)

	// Create header with padding and right-aligned status
	headerWithPadding := lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1).
		Width(m.termWidth)

	headerContent := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(availableWidth-titleWidth-lipgloss.Width(status)).Render(""),
		status)

	headerLine := headerWithPadding.Render(headerContent)
	s.WriteString(headerLine)
	s.WriteString("\n")

	// Show error if present
	if m.err != nil {
		errorText := errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
		s.WriteString(errorText)
		s.WriteString("\n")
	}

	// Build grid using lipgloss styles
	var rows []string
	for y := 0; y < m.height; y++ {
		var row strings.Builder
		for x := 0; x < m.width; x++ {
			color := m.grid[y][x]
			isPatternCell := m.placingPattern && m.isPatternCell(x, y)

			row.WriteString(m.renderCell(x, color, isPatternCell, cellStyleCache))
		}
		rows = append(rows, row.String())
	}

	grid := lipgloss.JoinVertical(lipgloss.Left, rows...)
	framedGrid := frameStyle.Render(grid)
	s.WriteString(framedGrid)

	return s.String()
}

// getPatternCells converts the current pattern to protocol.Cell slice for placement
func (m *gameModel) getPatternCells() []protocol.Cell {
	positions := m.getPatternPositions()
	var cells []protocol.Cell

	for _, pos := range positions {
		// Convert grid coordinates to world coordinates (apply viewport offset)
		worldX, worldY := m.viewportToWorld(pos.x, pos.y)

		cell := protocol.Cell{
			X:      uint16(worldX),
			Y:      uint16(worldY),
			Colour: m.currentColor,
		}

		cells = append(cells, cell)
	}

	return cells
}

// getPatternCenterOffset calculates the offset needed to center the pattern
func (m *gameModel) getPatternCenterOffset() (offsetX, offsetY uint16) {
	if !m.placingPattern {
		return 0, 0
	}

	// Calculate viewport center
	viewportCenterX := m.width / 2
	viewportCenterY := m.height / 2

	// Calculate offset to center the pattern
	offsetX = uint16(viewportCenterX) - (m.currentPattern.CenterX)
	offsetY = uint16(viewportCenterY) - (m.currentPattern.CenterY)

	return offsetX, offsetY
}

// getPatternPositions returns all pattern cell positions in viewport coordinates
func (m *gameModel) getPatternPositions() []struct{ x, y int } {
	if !m.placingPattern {
		return nil
	}

	// Get offset to properly center the pattern by its center cell
	offsetX, offsetY := m.getPatternCenterOffset()

	var positions []struct{ x, y int }
	for _, ps := range m.currentPattern.Cells {
		// Calculate absolute position with proper centering offset
		absoluteX := int(offsetX + ps.X)
		absoluteY := int(offsetY + ps.Y)
		positions = append(positions, struct{ x, y int }{absoluteX, absoluteY})
	}

	return positions
}

// isConnected checks if the model is connected and has a valid connection
func (m *gameModel) isConnected() bool {
	return m.connected && m.conn != nil
}

// isPatternCell checks if the given grid position contains a pattern cell
func (m *gameModel) isPatternCell(gridX, gridY int) bool {
	positions := m.getPatternPositions()
	for _, pos := range positions {
		if pos.x == gridX && pos.y == gridY {
			return true
		}
	}
	return false
}

// moveViewport moves the viewport by the given delta and handles wrapping
func (m *gameModel) moveViewport(deltaX, deltaY int) {
	m.viewportX += deltaX
	if m.viewportX < 0 {
		m.viewportX += m.worldSize
	} else if m.viewportX >= m.worldSize {
		m.viewportX -= m.worldSize
	}

	m.viewportY += deltaY
	if m.viewportY < 0 {
		m.viewportY += m.worldSize
	} else if m.viewportY >= m.worldSize {
		m.viewportY -= m.worldSize
	}

	m.updateGrid()
	if m.placingPattern {
		m.updatePatternCanPlace()
	}
}

// renderCell renders a single cell with appropriate styling
func (m *gameModel) renderCell(x int, color *uint32, isPatternCell bool, cellStyleCache map[uint32]lipgloss.Style) string {
	var style lipgloss.Style

	if isPatternCell {
		if m.patternCanPlace {
			style = styleFor(m.currentColor, cellStyleCache)
		} else {
			style = dimStyleFor(m.currentColor, cellStyleCache)
		}
	} else if color != nil {
		if m.placingPattern {
			style = dimStyleFor(*color, cellStyleCache)
		} else {
			style = styleFor(*color, cellStyleCache)
		}
	} else {
		// Empty cell
		if m.hasHalfCol && x == m.width-1 {
			return halfCellStyle.Render()
		}
		return emptyCellStyle.Render()
	}

	if m.hasHalfCol && x == m.width-1 {
		return style.Render("█")
	}
	return style.Render("██")
}

func (m *gameModel) updateGrid() {
	// Clear the grid - use max uint32 as sentinel for dead cells
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			m.grid[y][x] = nil
		}
	}

	// Set cells from server data, applying viewport offset
	for _, cell := range m.cells {
		// Convert world coordinates to viewport coordinates
		screenX, screenY := m.worldToViewport(int(cell.X), int(cell.Y))

		// Only render if cell is within the visible viewport
		if screenX >= 0 && screenX < m.width && screenY >= 0 && screenY < m.height {
			m.grid[screenY][screenX] = &cell.Colour
		}
	}
}

// updatePatternCanPlace checks if the current pattern can be placed at the current position
func (m *gameModel) updatePatternCanPlace() {
	if !m.placingPattern {
		return
	}

	m.patternCanPlace = true
	positions := m.getPatternPositions()

	for _, pos := range positions {
		// Only check for overlap with existing cells within viewport bounds
		// Allow patterns to extend beyond viewport - they just won't be visible
		if pos.x >= 0 && pos.x < m.width && pos.y >= 0 && pos.y < m.height {
			// Check if there's already a cell at this position
			if m.grid[pos.y][pos.x] != nil {
				m.patternCanPlace = false
				return
			}
		}
	}
}

// viewportToWorld converts viewport coordinates to world coordinates
func (m *gameModel) viewportToWorld(viewportX, viewportY int) (worldX, worldY int) {
	worldX = (viewportX + m.viewportX) % m.worldSize
	worldY = (viewportY + m.viewportY) % m.worldSize
	return worldX, worldY
}

// worldToViewport converts world coordinates to viewport coordinates
func (m *gameModel) worldToViewport(worldX, worldY int) (viewportX, viewportY int) {
	viewportX = worldX - m.viewportX
	if viewportX < 0 {
		viewportX += m.worldSize
	}

	viewportY = worldY - m.viewportY
	if viewportY < 0 {
		viewportY += m.worldSize
	}

	return viewportX, viewportY
}
