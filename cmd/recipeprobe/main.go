// recipeprobe runs the production extraction and inference path without
// Telegram. It intentionally prints metrics only, never source or recipe text.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/config"
	"github.com/mrRedSun/telegram-recipe-bot/internal/glm"
	"github.com/mrRedSun/telegram-recipe-bot/internal/source"
)

func main() { os.Exit(run()) }

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if len(os.Args) != 2 {
		slog.Error("usage: recipeprobe VIDEO_URL")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.JobTimeout)
	defer cancel()
	dir, err := os.MkdirTemp(cfg.TempRoot, "probe-")
	if err != nil {
		slog.Error("create probe workspace", "error", err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("probe cleanup failed")
		} else {
			slog.Info("probe cleanup completed")
		}
	}()

	started := time.Now()
	extractor := source.Extractor{MaxDuration: cfg.MaxDuration, MaxBytes: cfg.MaxDownloadBytes, TesseractLangs: cfg.TesseractLangs}
	ev, err := extractor.ExtractWithDiagnostics(ctx, os.Args[1], dir, func(stage string) { slog.Info("probe extraction progressed", "stage", stage) }, func(event source.Diagnostic) {
		slog.Info("probe source progressed", "source", event.Source, "stage", event.Stage, "backend", event.Backend, "outcome", event.Outcome, "reason", event.Reason, "exit_code", event.ExitCode, "http_status", event.HTTPStatus, "items", event.Items, "bytes", event.Bytes, "elapsed_ms", event.ElapsedMS)
	})
	if err != nil {
		slog.Error("probe extraction failed", "error", err)
		return 1
	}
	slog.Info("probe evidence extracted", "duration_seconds", int(ev.Duration), "frames", len(ev.Frames), "description_bytes", len(ev.Description), "author_comment_bytes", len(ev.AuthorComments), "transcript_bytes", len(ev.Transcript), "ocr_bytes", len(ev.OCR))

	client := glm.New(cfg.GLMAPIBase, cfg.GLMAPIKey, cfg.GLMModel, cfg.ReasoningEffort, cfg.GLMVision)
	r, err := client.Infer(ctx, ev)
	if err != nil {
		slog.Error("probe inference failed", "error", err)
		return 1
	}
	fmt.Printf("recipe_validated=true ingredients=%d steps=%d confidence=%s elapsed_ms=%d\n", len(r.Ingredients), len(r.Steps), r.Confidence, time.Since(started).Milliseconds())
	return 0
}
