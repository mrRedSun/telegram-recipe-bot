package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/app"
	"github.com/mrRedSun/telegram-recipe-bot/internal/config"
	"github.com/mrRedSun/telegram-recipe-bot/internal/glm"
	"github.com/mrRedSun/telegram-recipe-bot/internal/telegram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	tg := telegram.New(cfg.TelegramAPIBase, cfg.TelegramToken)
	extract := youtube.Extractor{MaxDuration: cfg.MaxDuration, MaxBytes: cfg.MaxDownloadBytes, TesseractLangs: cfg.TesseractLangs}
	ai := glm.New(cfg.GLMAPIBase, cfg.GLMAPIKey, cfg.GLMModel, cfg.ReasoningEffort, cfg.GLMVision)
	a := app.New(app.Config{Allowed: cfg.AllowedUsers, Workers: cfg.Workers, QueueSize: cfg.QueueSize, TempRoot: cfg.TempRoot, JobTimeout: cfg.JobTimeout}, tg, extract, ai)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !a.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	server := &http.Server{Addr: cfg.ListenAddress, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("health server listening", "address", cfg.ListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server failed", "error", err)
			stop()
		}
	}()
	slog.Info("recipe bot starting", "model", cfg.GLMModel, "vision", cfg.GLMVision, "workers", cfg.Workers)
	if err := a.Run(ctx); err != nil {
		slog.Error("bot stopped", "error", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
