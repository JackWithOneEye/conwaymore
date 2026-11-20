package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	ErrNotFound        = errors.New("game not found")
	ErrMaxGamesReached = errors.New("maximum number of games reached")
)

type DatabaseConfig interface {
	DBUrl() string
}

type worldSizeProvider interface {
	WorldSize() uint
}

type GameRecord struct {
	ID        int64
	Name      string
	WorldSize uint
	Seed      []byte
	CreatedAt int64
	SavedAt   *int64
}

type DatabaseService interface {
	Close() error
	CreateGame(ctx context.Context, name string, worldSize uint, seed []byte, maxGames int) (*GameRecord, error)
	GetGame(ctx context.Context, id int64) (*GameRecord, error)
	ListGames(ctx context.Context) ([]GameRecord, error)
	RenameGame(ctx context.Context, id int64, name string) error
	SaveGameSeed(ctx context.Context, id int64, seed []byte, savedAt int64) error
	DeleteGame(ctx context.Context, id int64) error
}

type service struct {
	cfg DatabaseConfig
	db  *sql.DB
}

func NewDatabaseService(cfg DatabaseConfig) DatabaseService {
	db, err := sql.Open("sqlite3", cfg.DBUrl())
	if err != nil {
		panic(fmt.Sprintf("could not open database %s: %v", cfg.DBUrl(), err))
	}
	// Serialize SQLite access through a single connection to avoid SQLITE_BUSY
	// errors when multiple games autosave concurrently.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		panic(fmt.Sprintf("could not configure busy_timeout: %v", err))
	}

	defaultWorldSize := uint(1024)
	if wp, ok := cfg.(worldSizeProvider); ok {
		ws := wp.WorldSize()
		if ws >= 128 && ws <= 2048 && (ws&(ws-1)) == 0 {
			defaultWorldSize = ws
		}
	}

	s := &service{cfg: cfg, db: db}
	if err := s.migrate(defaultWorldSize); err != nil {
		panic(fmt.Sprintf("could not migrate database %s: %v", cfg.DBUrl(), err))
	}

	return s
}

func (s *service) migrate(defaultWorldSize uint) error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= 1 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS games (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
			world_size INTEGER NOT NULL
			           CHECK (world_size BETWEEN 128 AND 2048
			                  AND (world_size & (world_size - 1)) = 0),
			seed       BLOB,
			created_at INTEGER NOT NULL,
			saved_at   INTEGER
		)
	`)
	if err != nil {
		return fmt.Errorf("create games table: %w", err)
	}

	// Migrate legacy single-game conway table if present.
	var legacyName string
	err = tx.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='conway'").Scan(&legacyName)
	if err == nil {
		var legacySeed []byte
		seedErr := tx.QueryRow("SELECT seed FROM conway ORDER BY id DESC LIMIT 1").Scan(&legacySeed)
		if seedErr == nil {
			now := time.Now().UnixMilli()
			_, err = tx.Exec(
				"INSERT INTO games (name, world_size, seed, created_at, saved_at) VALUES (?, ?, ?, ?, ?)",
				"Default", defaultWorldSize, legacySeed, now, now,
			)
			if err != nil {
				return fmt.Errorf("migrate legacy seed: %w", err)
			}
		} else if !errors.Is(seedErr, sql.ErrNoRows) {
			return fmt.Errorf("read legacy seed: %w", seedErr)
		}

		if _, err := tx.Exec("DROP TABLE conway"); err != nil {
			return fmt.Errorf("drop legacy conway table: %w", err)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check legacy conway table: %w", err)
	}

	if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	return tx.Commit()
}

func (s *service) Close() error {
	log.Printf("disconnected from database: %s", s.cfg.DBUrl())
	return s.db.Close()
}

func (s *service) CreateGame(ctx context.Context, name string, worldSize uint, seed []byte, maxGames int) (*GameRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if maxGames > 0 {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM games").Scan(&count); err != nil {
			return nil, err
		}
		if count >= maxGames {
			return nil, ErrMaxGamesReached
		}
	}

	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(
		ctx,
		"INSERT INTO games (name, world_size, seed, created_at, saved_at) VALUES (?, ?, ?, ?, NULL)",
		name, worldSize, seed, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &GameRecord{
		ID:        id,
		Name:      name,
		WorldSize: worldSize,
		Seed:      seed,
		CreatedAt: now,
	}, nil
}

func (s *service) GetGame(ctx context.Context, id int64) (*GameRecord, error) {
	var rec GameRecord
	var savedAt sql.NullInt64
	err := s.db.QueryRowContext(
		ctx,
		"SELECT id, name, world_size, seed, created_at, saved_at FROM games WHERE id = ?",
		id,
	).Scan(&rec.ID, &rec.Name, &rec.WorldSize, &rec.Seed, &rec.CreatedAt, &savedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if savedAt.Valid {
		v := savedAt.Int64
		rec.SavedAt = &v
	}
	return &rec, nil
}

func (s *service) ListGames(ctx context.Context) ([]GameRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT id, name, world_size, seed, created_at, saved_at FROM games ORDER BY id ASC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []GameRecord
	for rows.Next() {
		var rec GameRecord
		var savedAt sql.NullInt64
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.WorldSize, &rec.Seed, &rec.CreatedAt, &savedAt); err != nil {
			return nil, err
		}
		if savedAt.Valid {
			v := savedAt.Int64
			rec.SavedAt = &v
		}
		games = append(games, rec)
	}
	return games, rows.Err()
}

func (s *service) RenameGame(ctx context.Context, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE games SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *service) SaveGameSeed(ctx context.Context, id int64, seed []byte, savedAt int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE games SET seed = ?, saved_at = ? WHERE id = ?", seed, savedAt, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *service) DeleteGame(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM games WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
