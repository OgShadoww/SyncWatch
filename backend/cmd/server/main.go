package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	httpapi "syncwatch/internal/http"
	"syncwatch/internal/room"
	"syncwatch/internal/storage"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, err := pgxpool.New(dbCtx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	store := storage.Postgres{DB: db}
	if err := store.Init(dbCtx); err != nil {
		return err
	}
	handler, waitSockets := httpapi.New(ctx, room.New(store), env("STATIC_DIR", "../frontend/out"), os.Getenv("DEV_ORIGIN"))
	server := &http.Server{Addr: env("ADDR", ":8080"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	slog.Info("SyncWatch listening", "addr", server.Addr)
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		drain, finish := context.WithTimeout(context.Background(), 10*time.Second)
		defer finish()
		if err := server.Shutdown(drain); err != nil {
			return err
		}
		waitSockets()
		err := <-stopped
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
