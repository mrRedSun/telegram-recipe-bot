// instagramprobe exercises extraction and bounded media download without sending
// Telegram messages or loading runtime credentials. Output contains metrics only.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/instagram"
)

func main() { os.Exit(run()) }

func run() int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) != 2 {
		slog.Error("usage: instagramprobe INSTAGRAM_URL")
		return 2
	}
	root := os.Getenv("TEMP_ROOT")
	if root == "" {
		root = "/tmp/recipebot"
	}
	dir, err := os.MkdirTemp(root, "instagram-probe-")
	if err != nil {
		slog.Error("Instagram probe workspace failed")
		return 1
	}
	defer func() {
		if os.RemoveAll(dir) != nil {
			slog.Error("Instagram probe cleanup failed")
		} else {
			slog.Info("Instagram probe cleanup completed")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	started := time.Now()
	downloader := instagram.Downloader{MaxBytes: 100 << 20, MaxItems: 20}
	items, err := downloader.DownloadWithDiagnostics(ctx, os.Args[1], dir, func(event instagram.Diagnostic) {
		slog.Info("Instagram probe progressed", "stage", event.Stage, "backend", event.Backend, "outcome", event.Outcome, "reason", event.Reason, "exit_code", event.ExitCode, "http_status", event.HTTPStatus, "item", event.Item, "items", event.Items, "bytes", event.Bytes, "elapsed_ms", event.ElapsedMS)
	})
	if err != nil {
		slog.Error("Instagram probe failed")
		return 1
	}
	var photos, videos int
	var size int64
	for _, item := range items {
		size += item.Size
		if item.Kind == instagram.Photo {
			photos++
		} else {
			videos++
		}
	}
	slog.Info("Instagram probe succeeded", "items", len(items), "photos", photos, "videos", videos, "bytes", size, "elapsed_ms", time.Since(started).Milliseconds())
	return 0
}
