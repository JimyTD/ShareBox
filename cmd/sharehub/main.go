package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/syncthing/syncthing/internal/hub"
)

func main() {
	addr := flag.String("addr", ":9527", "listen address")
	dbPath := flag.String("db", "sharehub.db", "sqlite database path")
	flag.Parse()

	textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(textHandler))

	h, err := hub.New(*dbPath, *addr)
	if err != nil {
		slog.Error("failed to create hub", "error", err)
		os.Exit(1)
	}
	defer h.Close()

	if err := h.ListenAndServe(); err != nil {
		slog.Error("hub stopped", "error", err)
		os.Exit(1)
	}
}
