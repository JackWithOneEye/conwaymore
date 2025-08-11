package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type DatabaseConfig interface {
	DBUrl() string
}

type DatabaseService interface {
	Close() error
	GetSeed() ([]byte, error)
	WriteSeed(ctx context.Context, seed []byte) error
}

type service struct {
	cfg DatabaseConfig
	db  *sql.DB
}

func NewDatabaseService(cfg DatabaseConfig) DatabaseService {
	db, err := sql.Open("sqlite3", cfg.DBUrl())
	if err != nil {
		panic(fmt.Sprintf("could not open database %s", err))
	}

	s := &service{cfg, db}

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS conway (id INTEGER PRIMARY KEY AUTOINCREMENT, seed BLOB NOT NULL)")
	if err != nil {
		panic(fmt.Sprintf("could not initialise database %s", err))
	}

	return s
}

func (s *service) Close() error {
	log.Printf("disconnected from database: %s", s.cfg.DBUrl())
	return s.db.Close()
}

func (s *service) GetSeed() ([]byte, error) {
	rows, err := s.db.Query("SELECT seed FROM conway ORDER BY id DESC LIMIT 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var seed []byte
	for rows.Next() {
		if err := rows.Scan(&seed); err != nil {
			return nil, err
		}
	}
	return seed, rows.Err()
}

func (s *service) WriteSeed(ctx context.Context, seed []byte) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO conway (id, seed) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET seed=?", seed, seed)
	if err != nil {
		return err
	}

	return nil
}
