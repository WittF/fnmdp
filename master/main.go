package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/WittF/fnmdp/master/internal/config"
	"github.com/WittF/fnmdp/master/internal/dedup"
	"github.com/WittF/fnmdp/master/internal/server"
	masterstate "github.com/WittF/fnmdp/master/internal/state"
	"github.com/WittF/fnmdp/master/internal/store"
	"github.com/WittF/fnmdp/master/internal/valkey"
)

func main() {
	debug.SetTraceback("all")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	var st store.Store = store.NewMemory()
	if cfg.PGEnabled() {
		pg, err := store.NewPG(ctx, store.PGConfig{
			Host:      cfg.PGHost,
			Port:      cfg.PGPort,
			User:      cfg.PGUser,
			Password:  cfg.PGPassword,
			Database:  cfg.PGDatabase,
			SSLMode:   cfg.PGSSLMode,
			MaxConns:  cfg.PGMaxConns,
			OpTimeout: cfg.PGOpTimeout,
		})
		if err != nil {
			log.Printf("postgres unavailable, using memory store: %v", err)
		} else {
			st = pg
		}
	}

	var window dedup.Window = dedup.NewMemoryWindow()
	if cfg.ValkeyEnabled() {
		vw, err := valkey.New(ctx, valkey.Config{
			Addr:     cfg.ValkeyAddr,
			Password: cfg.ValkeyPassword,
			DB:       cfg.ValkeyDB,
			TLS:      cfg.ValkeyTLS,
		})
		if err != nil {
			log.Printf("valkey unavailable, using memory L2 window: %v", err)
		} else {
			window = vw
		}
	}

	engine, err := masterstate.New(ctx, st, window, nil)
	if err != nil {
		log.Fatalf("init state: %v", err)
	}
	app := server.New(ctx, cfg, st, engine)
	log.Printf("fnmdp master listening on %s", cfg.ListenAddr)
	if err := app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("server: %v", err)
	}
}
