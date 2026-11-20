package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/config"
	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/games"
	"github.com/JackWithOneEye/conwaymore/internal/server"
)

func main() {
	cfg := config.NewConfig()
	dbs := database.NewDatabaseService(cfg)
	mgr := games.NewManager(cfg, dbs)
	s := server.NewServer(cfg, mgr)

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.ListenAndServe()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errChan:
		log.Printf("could not serve: %v", err)
	case sig := <-sigChan:
		log.Printf("terminating: %v", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop all running games and autosave their state before shutting down HTTP
	// (which also closes active WebSocket connections cleanly).
	if err := mgr.Shutdown(shutdownCtx); err != nil {
		log.Printf("could not shutdown games manager: %v", err)
	}
	if err := s.Shutdown(shutdownCtx); err != nil {
		log.Printf("could not shutdown server: %v", err)
	}
	if err := dbs.Close(); err != nil {
		log.Printf("could not close database: %v", err)
	}
}
