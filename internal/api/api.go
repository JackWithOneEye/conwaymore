// Package api holds the JSON types of the HTTP API, shared by the server and
// its clients, so that clients need not import the server's packages.
package api

// Mode says where a game's simulation runs.
type Mode string

const (
	// ModeMulti games run on the server, which streams them to every viewer.
	ModeMulti Mode = "multi"
	// ModeSingle games run in the player's browser; the server only stores their state.
	ModeSingle Mode = "single"
)

// GameInfo describes a game, as GET /api/games and /api/games/:id return it.
type GameInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Mode      Mode   `json:"mode"`
	WorldSize uint   `json:"worldSize"`
	Playing   bool   `json:"playing"`
	Speed     uint32 `json:"speed"`
	Viewers   int    `json:"viewers"`
	Loaded    bool   `json:"loaded"`
	CreatedAt int64  `json:"createdAt"`
	SavedAt   *int64 `json:"savedAt"`
}
