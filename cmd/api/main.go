package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/config"
	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/engine"
	"github.com/JackWithOneEye/conwaymore/internal/server"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.NewConfig()
	dbs := database.NewDatabaseService(cfg)

	seed, err := dbs.GetSeed()
	if err != nil {
		log.Fatalf("could not get seed: %s", err)
	}

	engine := engine.NewEngine(cfg, seed, ctx)

	s := server.NewServer(cfg, dbs, engine, ctx)

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.ListenAndServe()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	select {
	case err := <-errChan:
		log.Printf("could not serve: %v", err)
	case sig := <-sigChan:
		log.Printf("terminating: %v", sig)
	}

	ctx2, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	err = s.Shutdown(ctx2)
	if err != nil {
		log.Printf("could not shutdown server: %v", err)
	}
	err = dbs.Close()
	if err != nil {
		log.Printf("could not close database: %v", err)
	}
}
