package patterns

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"path"
	"strconv"
	"strings"
)

type PatternCell struct {
	X uint16 `json:"x"`
	Y uint16 `json:"y"`
}

// Pattern represents a Conway's Game of Life pattern
type Pattern struct {
	Name    string        `json:"name"`
	Cells   []PatternCell `json:"cells"`
	CenterX uint16        `json:"center_x"`
	CenterY uint16        `json:"center_y"`
}

//go:embed data/*.rle
var patternFiles embed.FS

// Patterns maps each pattern key (the file name without extension) to its pattern.
var Patterns map[string]*Pattern

func init() {
	entries, err := patternFiles.ReadDir("data")
	if err != nil {
		panic(err)
	}
	Patterns = make(map[string]*Pattern, len(entries))
	for _, entry := range entries {
		data, err := patternFiles.ReadFile(path.Join("data", entry.Name()))
		if err != nil {
			panic(err)
		}
		key := strings.TrimSuffix(entry.Name(), ".rle")
		p, err := parseRLE(data)
		if err != nil {
			panic(fmt.Sprintf("pattern %s: %s", entry.Name(), err))
		}
		if p.Name == "" {
			p.Name = key
		}
		Patterns[key] = p
	}
}

// parseRLE parses a pattern in the run length encoded format used by LifeWiki and Golly
// (https://conwaylife.com/wiki/Run_Length_Encoded).
func parseRLE(data []byte) (*Pattern, error) {
	p := &Pattern{}
	var width, height int
	headerSeen := false
	var body strings.Builder

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
		case strings.HasPrefix(line, "#"):
			if name, ok := strings.CutPrefix(line, "#N"); ok {
				p.Name = strings.TrimSpace(name)
			}
		case !headerSeen:
			headerSeen = true
			for _, field := range strings.Split(line, ",") {
				k, v, _ := strings.Cut(field, "=")
				k, v = strings.TrimSpace(k), strings.TrimSpace(v)
				if k != "x" && k != "y" {
					continue
				}
				n, err := strconv.Atoi(v)
				if err != nil {
					return nil, fmt.Errorf("invalid header %q: %w", line, err)
				}
				if k == "x" {
					width = n
				} else {
					height = n
				}
			}
		default:
			body.WriteString(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("missing or invalid header (need x and y)")
	}

	x, y, count := 0, 0, 0
loop:
	for _, c := range body.String() {
		if c >= '0' && c <= '9' {
			count = count*10 + int(c-'0')
			continue
		}
		n := max(count, 1)
		count = 0
		switch c {
		case 'b':
			x += n
		case 'o':
			if x+n > width || y >= height {
				return nil, fmt.Errorf("cell row %d, columns %d-%d outside of %dx%d bounds", y, x, x+n-1, width, height)
			}
			for range n {
				p.Cells = append(p.Cells, PatternCell{X: uint16(x), Y: uint16(y)})
				x++
			}
		case '$':
			x = 0
			y += n
		case '!':
			break loop
		default:
			return nil, fmt.Errorf("unexpected character %q", c)
		}
	}

	p.CenterX = uint16(width / 2)
	p.CenterY = uint16(height / 2)
	return p, nil
}
