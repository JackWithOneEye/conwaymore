package tui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/lrucache"
	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/coder/websocket"
)

// tickMsg is sent every 1/30th second to trigger UI updates
type tickMsg struct{}

// sgrPrefixCache caches just the SGR prefix for a given color (no reset), used by RLE renderer
var sgrPrefixCache = lrucache.NewLruCache[uint64, string](2048)

const emptyCell uint32 = 0xffffffff

type gameModel struct {
	worldSize    int
	grid         [][]uint32 // value grid: 0 = empty, non-zero = color
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

	// Performance optimizations
	pendingData    []byte // latest WebSocket message data, processed on tick
	lastUpdate     time.Time
	currentCells   map[uint64]uint32         // reused across frames to avoid allocation
	prevCells      map[uint64]uint32         // cache of previous frame's cells for diff rendering (key: x<<32|y)
	rowDirty       []bool                    // tracks which rows need re-rendering
	renderedRows   []string                  // cached rendered row strings
	cellStyleCache map[uint32]lipgloss.Style // reused across frames
}

// tick returns a command that sends a tickMsg every 1/30th second (30 FPS)
func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// getSGRPrefix returns the ANSI SGR prefix for a color without reset, cached and bounded
func getSGRPrefix(color uint32) string {
	key := uint64(color)
	if s, ok := sgrPrefixCache.Get(key); ok {
		return s
	}
	r := (color >> 16) & 0xff
	g := (color >> 8) & 0xff
	b := color & 0xff
	prefix := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	sgrPrefixCache.Add(key, prefix)
	return prefix
}

// markAllRowsDirty marks all rows as needing re-rendering
func (m *gameModel) markAllRowsDirty() {
	for i := range m.rowDirty {
		m.rowDirty[i] = true
	}
}

func (m *gameModel) Init() tea.Cmd {
	m.setDimensions(m.height, m.width, m.hasHalfCol)
	m.lastUpdate = time.Now()
	m.prevCells = make(map[uint64]uint32)
	m.currentCells = make(map[uint64]uint32)
	m.cellStyleCache = make(map[uint32]lipgloss.Style)
	// Initialize all rows as dirty for first render
	m.markAllRowsDirty()
	return tea.Batch(connectToAPI(m.apiHost), tick())
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
		m.markAllRowsDirty() // Redraw all rows to show dimmed effect
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
					m.markAllRowsDirty() // Remove dimmed effect
					return m, sendCells(m.conn, cells)
				}
			case "esc":
				// Abort pattern placement
				m.placingPattern = false
				m.markAllRowsDirty() // Remove dimmed effect
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
			m.conn = nil
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

		// Cache the latest data instead of processing immediately
		m.pendingData = msg.Data

		// Continue listening for messages
		if m.isConnected() {
			return m, listenForMessages(m.conn)
		}
	case tickMsg:
		// Process pending data at 30 FPS
		if m.pendingData != nil {
			cells, playing, speed, err := processServerMessage(m.pendingData)
			if err != nil {
				m.err = err
			} else {
				m.cells = cells
				m.running = playing
				m.speed.Store(uint32(speed))
				m.updateGrid()
				m.lastUpdate = time.Now()
			}
			m.pendingData = nil // Clear pending data
		}

		// Continue ticking
		return m, tick()
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
				m.setDimensions(newHeight, newWidth, newHasHalfCol)
				clear(m.prevCells)
				// Mark all rows dirty when terminal size changes
				m.markAllRowsDirty()
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

	// Create header with title and status
	title := titleStyle.Render("Conway's Game of Life - Terminal UI")
	if m.saving {
		title += fmt.Sprintf(" %s", m.spinner.View())
	}
	colorSwatch := styleFor(m.currentColor, m.cellStyleCache).Render("██")

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

	// Build grid using dirty row tracking to avoid rebuilding unchanged rows
	for y := 0; y < m.height; y++ {
		if m.rowDirty[y] || m.placingPattern {
			m.renderedRows[y] = m.renderRowRLE(y)
			m.rowDirty[y] = false
		}
	}

	grid := lipgloss.JoinVertical(lipgloss.Left, m.renderedRows...)
	// Ensure the grid has a fixed width so the frame doesn't collapse when rows are empty or unchanged
	gridWidthChars := m.width * 2
	if m.hasHalfCol {
		gridWidthChars = (m.width-1)*2 + 1
	}
	grid = lipgloss.NewStyle().Width(gridWidthChars).Render(grid)
	framedGrid := frameStyle.Render(grid)
	s.WriteString(framedGrid)

	return s.String()
}

// renderRowRLE renders a single row using run-length emission of ANSI sequences to reduce SGR count
func (m *gameModel) renderRowRLE(y int) string {
	var b strings.Builder
	// Rough capacity: 2 chars per cell + some ANSI overhead
	b.Grow(m.width*2 + 64)

	currentColor := emptyCell
	currentHalf := false
	runLen := 0

	flush := func() {
		if runLen == 0 {
			return
		}
		if currentColor == emptyCell {
			if currentHalf {
				b.WriteString(strings.Repeat(" ", runLen))
			} else {
				b.WriteString(strings.Repeat("  ", runLen))
			}
		} else {
			b.WriteString(getSGRPrefix(currentColor))
			if currentHalf {
				b.WriteString(strings.Repeat("█", runLen))
			} else {
				b.WriteString(strings.Repeat("██", runLen))
			}
			b.WriteString("\x1b[0m")
		}
		runLen = 0
	}

	for x := 0; x < m.width; x++ {
		displayColor := emptyCell
		if m.placingPattern && m.isPatternCell(x, y) {
			if m.patternCanPlace {
				displayColor = m.currentColor
			} else {
				displayColor = dimColor(m.currentColor)
			}
		} else {
			cell := m.grid[y][x]
			if cell != emptyCell {
				if m.placingPattern {
					displayColor = dimColor(cell)
				} else {
					displayColor = cell
				}
			}
		}
		half := m.hasHalfCol && x == m.width-1

		if runLen == 0 {
			currentColor = displayColor
			currentHalf = half
			runLen = 1
			continue
		}
		if displayColor == currentColor && half == currentHalf {
			runLen++
		} else {
			flush()
			currentColor = displayColor
			currentHalf = half
			runLen = 1
		}
	}
	flush()

	return b.String()
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

func (m *gameModel) setDimensions(height, width int, hasHalfCol bool) {
	m.height = height
	m.width = width
	if hasHalfCol {
		m.width += 1
	}
	m.hasHalfCol = hasHalfCol
	m.grid = make([][]uint32, m.height)
	for i := range m.grid {
		m.grid[i] = make([]uint32, m.width)
		for j := range m.width {
			m.grid[i][j] = emptyCell
		}
	}
	m.rowDirty = make([]bool, m.height)
	m.renderedRows = make([]string, m.height)
}

func (m *gameModel) updateGrid() {
	// Clear current cells map for reuse (avoids allocation)
	clear(m.currentCells)

	// Reset dirty row tracking
	for i := range m.rowDirty {
		m.rowDirty[i] = false
	}

	// Process cells and filter to visible viewport
	for _, cell := range m.cells {
		// Convert world coordinates to viewport coordinates (handles wrap-around correctly)
		screenX, screenY := m.worldToViewport(int(cell.X), int(cell.Y))

		// Only track cells within the visible viewport
		if screenX >= 0 && screenX < m.width && screenY >= 0 && screenY < m.height {
			key := uint64(screenX)<<32 | uint64(screenY)
			m.currentCells[key] = cell.Colour
		}
	}

	// Clear cells that are no longer alive (differential update)
	for key := range m.prevCells {
		if _, exists := m.currentCells[key]; !exists {
			x := int(key >> 32)
			y := int(key & 0xFFFFFFFF)
			if x >= 0 && x < m.width && y >= 0 && y < m.height {
				m.grid[y][x] = emptyCell
				m.rowDirty[y] = true
			}
		}
	}

	// Update cells that changed or are new
	for key, color := range m.currentCells {
		if prevColor, exists := m.prevCells[key]; !exists || prevColor != color {
			x := int(key >> 32)
			y := int(key & 0xFFFFFFFF)
			if x >= 0 && x < m.width && y >= 0 && y < m.height {
				m.grid[y][x] = color // Direct value assignment, no heap allocation
				m.rowDirty[y] = true
			}
		}
	}

	// Swap current and previous for next frame (avoid map reallocation)
	m.prevCells, m.currentCells = m.currentCells, m.prevCells
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
			m.patternCanPlace = m.grid[pos.y][pos.x] == emptyCell
			m.rowDirty[pos.y] = true
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
