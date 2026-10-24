// recipeprobe runs the production extraction and inference path without
// Telegram. It intentionally prints metrics only, never source or recipe text.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/catalog"
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
	if cfg.RecipeCatalogPath != "" {
		store, err := catalog.Open(context.Background(), cfg.RecipeCatalogPath)
		if err != nil {
			slog.Error("catalog initialization failed")
			return 1
		}
		client = client.WithCatalog(store)
		slog.Info("recipe catalog ready", "records", store.Info.Records)
	}
	r, err := client.Infer(ctx, ev)
	if err != nil {
		slog.Error("probe inference failed", "error", err)
		return 1
	}
	estimates := 0
	for _, ingredient := range r.Ingredients {
		if len(ingredient.ReferenceIDs) > 0 {
			estimates++
		}
	}
	for _, step := range r.Steps {
		if len(step.ReferenceIDs) > 0 {
			estimates++
		}
	}
	fmt.Printf("catalog_reference_count=%d reference_estimates=%d\n", len(r.References), estimates)
	fmt.Printf("recipe_validated=true ingredients=%d steps=%d confidence=%s elapsed_ms=%d\n", len(r.Ingredients), len(r.Steps), r.Confidence, time.Since(started).Milliseconds())
	return 0
}
