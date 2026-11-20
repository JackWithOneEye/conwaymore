package tui

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/games"
	"github.com/JackWithOneEye/conwaymore/internal/server"
	"github.com/stretchr/testify/require"
)

type tuiTestCfg struct {
	dbURL string
}

func (c tuiTestCfg) DBUrl() string              { return c.dbURL }
func (c tuiTestCfg) Port() uint                 { return 0 }
func (c tuiTestCfg) WorldSize() uint            { return 1024 }
func (c tuiTestCfg) MaxGames() int              { return 10 }
func (c tuiTestCfg) MaxLoadedGames() int        { return 10 }
func (c tuiTestCfg) IdleTimeout() time.Duration { return 5 * time.Minute }

func TestTUIConnectResolveAndSave(t *testing.T) {
	f, err := os.CreateTemp("", "tui_test_*.db")
	require.NoError(t, err)
	f.Close()
	defer os.Remove(f.Name())

	cfg := tuiTestCfg{dbURL: f.Name()}
	db := database.NewDatabaseService(cfg)
	defer db.Close()
	mgr := games.NewManager(cfg, db)
	defer mgr.Shutdown(context.Background())

	g1, err := mgr.Create(context.Background(), games.CreateParams{Name: "First", WorldSize: 128})
	require.NoError(t, err)
	g2, err := mgr.Create(context.Background(), games.CreateParams{Name: "Second", WorldSize: 512})
	require.NoError(t, err)

	srv := server.NewServer(cfg, mgr)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	host := strings.TrimPrefix(ts.URL, "http://")

	// gameID=0 auto-selects first game.
	res1 := connectToAPI(host, 0)().(connectionResult)
	require.NoError(t, res1.Err)
	require.True(t, res1.Connected)
	require.Equal(t, g1.ID, res1.GameID)
	require.Equal(t, uint(128), res1.WorldSize)
	defer res1.Conn.CloseNow()

	// Explicit gameID=2 connects to game #2 with its 512 world size.
	res2 := connectToAPI(host, g2.ID)().(connectionResult)
	require.NoError(t, res2.Err)
	require.True(t, res2.Connected)
	require.Equal(t, g2.ID, res2.GameID)
	require.Equal(t, uint(512), res2.WorldSize)
	defer res2.Conn.CloseNow()

	// Manual save via TUI client hits /api/games/2/save.
	saveRes := saveGame(host, g2.ID)().(saveGameResult)
	require.NoError(t, saveRes.Err)
	info2, err := mgr.GetInfo(context.Background(), g2.ID)
	require.NoError(t, err)
	require.NotNil(t, info2.SavedAt)
}
