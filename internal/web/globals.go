package web

import "github.com/JackWithOneEye/conwaymore/internal/games"

type Globals struct {
	GameID    int64
	GameName  string
	Mode      games.Mode
	WorldSize uint
	WSPath    string // empty for a single-player game, which runs in the browser
}
