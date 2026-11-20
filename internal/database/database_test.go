package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type testDBConfig struct {
	url       string
	worldSize uint
}

func (c testDBConfig) DBUrl() string   { return c.url }
func (c testDBConfig) WorldSize() uint { return c.worldSize }

func TestLegacyMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	_, err = raw.Exec("CREATE TABLE conway (id INTEGER PRIMARY KEY AUTOINCREMENT, seed BLOB NOT NULL)")
	require.NoError(t, err)
	legacySeed := []byte{1, 0, 50, 0, 0, 0}
	_, err = raw.Exec("INSERT INTO conway (id, seed) VALUES (1, ?)", legacySeed)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	dbs := NewDatabaseService(testDBConfig{url: dbPath, worldSize: 512})
	defer dbs.Close()

	games, err := dbs.ListGames(context.Background())
	require.NoError(t, err)
	require.Len(t, games, 1)
	require.Equal(t, int64(1), games[0].ID)
	require.Equal(t, "Default", games[0].Name)
	require.Equal(t, "multi", games[0].Mode)
	require.Equal(t, uint(512), games[0].WorldSize)
	require.Equal(t, legacySeed, games[0].Seed)
	require.NotNil(t, games[0].SavedAt)

	// Confirm legacy table was dropped and the schema is current.
	raw2, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer raw2.Close()

	var version int
	require.NoError(t, raw2.QueryRow("PRAGMA user_version").Scan(&version))
	require.Equal(t, 2, version)

	var name string
	err = raw2.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='conway'").Scan(&name)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestGameCRUDAndLimits(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crud.db")
	dbs := NewDatabaseService(testDBConfig{url: dbPath, worldSize: 1024})
	defer dbs.Close()

	ctx := context.Background()

	g1, err := dbs.CreateGame(ctx, "Alpha", "multi", 128, nil, 2)
	require.NoError(t, err)
	require.Equal(t, int64(1), g1.ID)
	require.Equal(t, "Alpha", g1.Name)
	require.Equal(t, uint(128), g1.WorldSize)
	require.Nil(t, g1.SavedAt)

	g2, err := dbs.CreateGame(ctx, "Beta", "single", 2048, []byte{0, 0, 100, 0, 0, 0}, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), g2.ID)
	got2, err := dbs.GetGame(ctx, g2.ID)
	require.NoError(t, err)
	require.Equal(t, "single", got2.Mode)

	// Max games cap enforced.
	_, err = dbs.CreateGame(ctx, "Gamma", "multi", 256, nil, 2)
	require.ErrorIs(t, err, ErrMaxGamesReached)

	// Rename.
	require.NoError(t, dbs.RenameGame(ctx, g1.ID, "Alpha Renamed"))
	got, err := dbs.GetGame(ctx, g1.ID)
	require.NoError(t, err)
	require.Equal(t, "Alpha Renamed", got.Name)

	// Save seed.
	seed := []byte{1, 0, 25, 0, 0, 0}
	require.NoError(t, dbs.SaveGameSeed(ctx, g1.ID, seed, 123456789))
	got, err = dbs.GetGame(ctx, g1.ID)
	require.NoError(t, err)
	require.Equal(t, seed, got.Seed)
	require.NotNil(t, got.SavedAt)
	require.Equal(t, int64(123456789), *got.SavedAt)

	// Delete and check not found.
	require.NoError(t, dbs.DeleteGame(ctx, g1.ID))
	_, err = dbs.GetGame(ctx, g1.ID)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, dbs.DeleteGame(ctx, g1.ID), ErrNotFound)
	require.ErrorIs(t, dbs.RenameGame(ctx, g1.ID, "Nope"), ErrNotFound)

	// Unknown mode rejected by SQLite CHECK constraint.
	_, err = dbs.CreateGame(ctx, "BadMode", "solo", 256, nil, 10)
	require.Error(t, err)

	// Invalid world size rejected by SQLite CHECK constraint.
	_, err = dbs.CreateGame(ctx, "TooSmall", "multi", 64, nil, 10)
	require.Error(t, err)
	_, err = dbs.CreateGame(ctx, "NotPow2", "multi", 300, nil, 10)
	require.Error(t, err)
	_, err = dbs.CreateGame(ctx, "TooBig", "multi", 4096, nil, 10)
	require.Error(t, err)
}

// TestV1Migration upgrades a database written by the multi-game release, whose
// games table has no mode column: its games become multiplayer games.
func TestV1Migration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v1.db")

	raw, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	_, err = raw.Exec(`
		CREATE TABLE games (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			world_size INTEGER NOT NULL,
			seed       BLOB,
			created_at INTEGER NOT NULL,
			saved_at   INTEGER
		);
		INSERT INTO games (name, world_size, seed, created_at) VALUES ('Old', 256, x'000064000000', 1);
		PRAGMA user_version = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	dbs := NewDatabaseService(testDBConfig{url: dbPath, worldSize: 1024})
	defer dbs.Close()

	got, err := dbs.GetGame(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "Old", got.Name)
	require.Equal(t, "multi", got.Mode)
	require.Equal(t, []byte{0, 0, 100, 0, 0, 0}, got.Seed)
}
