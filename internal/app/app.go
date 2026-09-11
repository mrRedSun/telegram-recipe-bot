package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/recipe"
	"github.com/mrRedSun/telegram-recipe-bot/internal/telegram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

type Messenger interface {
	Updates(context.Context, int64) ([]telegram.Update, error)
	Send(context.Context, int64, string) (telegram.Message, error)
	Edit(context.Context, int64, int64, string) error
}
type Extractor interface {
	Extract(context.Context, string, string) (youtube.Evidence, error)
}
type progressExtractor interface {
	ExtractWithProgress(context.Context, string, string, func(string)) (youtube.Evidence, error)
}
type Inferer interface {
	Infer(context.Context, youtube.Evidence) (recipe.Recipe, error)
}
type Config struct {
	Allowed            map[int64]struct{}
	Workers, QueueSize int
	TempRoot           string
	JobTimeout         time.Duration
}
type job struct {
	id       uint64
	chatID   int64
	url      string
	statusID int64
}
type App struct {
	cfg      Config
	tg       Messenger
	extract  Extractor
	infer    Inferer
	jobs     chan job
	wg       sync.WaitGroup
	ready    atomic.Bool
	sequence atomic.Uint64
}

func New(cfg Config, tg Messenger, extract Extractor, infer Inferer) *App {
	return &App{cfg: cfg, tg: tg, extract: extract, infer: infer, jobs: make(chan job, cfg.QueueSize)}
}
func (a *App) Run(ctx context.Context) error {
	for i := 0; i < a.cfg.Workers; i++ {
		a.wg.Add(1)
		go a.worker(ctx)
	}
	a.ready.Store(true)
	defer func() { a.ready.Store(false); close(a.jobs); a.wg.Wait() }()
	var offset int64
	for {
		updates, err := a.tg.Updates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Warn("telegram polling failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			a.handle(ctx, u)
		}
	}
}
func (a *App) Ready() bool { return a.ready.Load() }
func (a *App) handle(ctx context.Context, u telegram.Update) {
	m := u.Message
	if m == nil || m.From == nil {
		return
	}
	if _, ok := a.cfg.Allowed[m.From.ID]; !ok {
		return
	}
	text := strings.TrimSpace(m.Text)
	command, argument := parseCommand(text)
	if command == "/start" || command == "/help" {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "Send one YouTube Short URL. In a group, use <code>/recipe URL</code>. I’ll reconstruct the recipe and mark anything uncertain.\n\n/privacy — data handling\n/status — service status")
		return
	}
	if command == "/privacy" {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "The bot keeps no recipe history, usernames, URLs, or videos. Each download and its sampled frames are removed when the job finishes. Telegram and Z.AI still process the data under their own policies.")
		return
	}
	if command == "/status" {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "Recipe bot is ready.")
		return
	}
	request := text
	if command == "/recipe" {
		request = argument
	}
	url, err := youtube.Normalize(request)
	if err != nil {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "❌ Please send one valid youtube.com or youtu.be video URL. In groups, use <code>/recipe URL</code>.")
		return
	}
	id := a.sequence.Add(1)
	status, err := a.tg.Send(ctx, m.Chat.ID, "🕐 <b>Queued</b> — waiting for a recipe worker…")
	if err != nil {
		slog.Warn("send status failed", "job", id, "error", err)
		return
	}
	select {
	case a.jobs <- job{id: id, chatID: m.Chat.ID, url: url, statusID: status.MessageID}:
		slog.Info("recipe job queued", "job", id, "queue_depth", len(a.jobs))
	default:
		slog.Warn("recipe queue full", "job", id, "queue_capacity", cap(a.jobs))
		_ = a.tg.Edit(ctx, m.Chat.ID, status.MessageID, "🚦 <b>Queue full</b> — both workers are busy. Please retry shortly.")
	}
}

func parseCommand(text string) (command, argument string) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", ""
	}
	command = strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
	argument = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	return command, argument
}

func (a *App) worker(ctx context.Context) {
	defer a.wg.Done()
	for j := range a.jobs {
		a.process(ctx, j)
	}
}
func (a *App) process(parent context.Context, j job) {
	ctx, cancel := context.WithTimeout(parent, a.cfg.JobTimeout)
	defer cancel()
	started := time.Now()
	dir := filepath.Join(a.cfg.TempRoot, fmt.Sprintf("job-%d", j.id))
	slog.Info("recipe job started", "job", j.id)
	a.progress(parent, j, "download", "⬇️ <b>Step 1/7 — Downloading video</b>\nFetching the Short and its metadata…")
	if err := os.MkdirAll(dir, 0700); err != nil {
		a.fail(parent, j, "workspace", err)
		return
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("temporary job cleanup failed", "job", j.id, "error", err)
		}
	}()
	extractStarted := time.Now()
	var ev youtube.Evidence
	var err error
	if extractor, ok := a.extract.(progressExtractor); ok {
		ev, err = extractor.ExtractWithProgress(ctx, j.url, dir, func(stage string) {
			if message := extractionProgressMessage(stage); message != "" {
				a.progress(parent, j, stage, message)
			}
		})
	} else {
		ev, err = a.extract.Extract(ctx, j.url, dir)
	}
	if err != nil {
		a.fail(parent, j, "extract", err)
		return
	}
	slog.Info("recipe evidence extracted", "job", j.id,
		"elapsed_ms", time.Since(extractStarted).Milliseconds(),
		"duration_seconds", int(ev.Duration), "frames", len(ev.Frames),
		"description_bytes", len(ev.Description), "author_comment_bytes", len(ev.AuthorComments),
		"transcript_bytes", len(ev.Transcript), "ocr_bytes", len(ev.OCR))
	a.progress(parent, j, "inference", "🧠 <b>Step 6/7 — Reconstructing recipe</b>\nGLM-5.3 is comparing the collected evidence…")
	inferStarted := time.Now()
	r, err := a.infer.Infer(ctx, ev)
	if err != nil {
		a.fail(parent, j, "inference", err)
		return
	}
	slog.Info("recipe inference completed", "job", j.id, "elapsed_ms", time.Since(inferStarted).Milliseconds(), "confidence", r.Confidence, "ingredients", len(r.Ingredients), "steps", len(r.Steps))
	a.progress(parent, j, "format", "🧾 <b>Step 7/7 — Formatting</b>\nBuilding the Telegram recipe card…")
	if err := a.tg.Edit(parent, j.chatID, j.statusID, recipe.RenderRichHTML(r)); err != nil {
		slog.Warn("deliver recipe failed", "job", j.id, "stage", "delivery", "error", err)
		return
	}
	slog.Info("recipe delivered", "job", j.id, "elapsed_ms", time.Since(started).Milliseconds(), "confidence", r.Confidence)
}

func (a *App) progress(ctx context.Context, j job, stage, message string) {
	if err := a.tg.Edit(ctx, j.chatID, j.statusID, message); err != nil {
		slog.Warn("progress update failed", "job", j.id, "stage", stage, "error", err)
		return
	}
	slog.Info("recipe job progressed", "job", j.id, "stage", stage)
}

func extractionProgressMessage(stage string) string {
	switch stage {
	case "download":
		return "💬 <b>Step 2/7 — Checking captions</b>\nVideo downloaded; looking for subtitles and transcript…"
	case "captions":
		return "📌 <b>Step 3/7 — Checking creator comments</b>\nCaptions checked; looking for pinned and uploader-authored details…"
	case "comments":
		return "🖼️ <b>Step 4/7 — Sampling frames</b>\nCreator text checked; extracting representative visuals…"
	case "frames":
		return "🔤 <b>Step 5/7 — Reading on-screen text</b>\nFrames sampled; running OCR over visible ingredients and instructions…"
	default:
		return ""
	}
}

func (a *App) fail(ctx context.Context, j job, stage string, err error) {
	slog.Warn("recipe job failed", "job", j.id, "stage", stage, "reason", failureReason(err), "error", err)
	msg := failureMessage(stage, err)
	_ = a.tg.Edit(ctx, j.chatID, j.statusID, msg)
}

func failureReason(err error) string {
	text := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case strings.Contains(text, "glm http 429"):
		return "provider_quota"
	case strings.Contains(text, "invalid recipe json"):
		return "provider_incomplete_json"
	case strings.Contains(text, "recipe validation"):
		return "provider_invalid_recipe"
	case strings.Contains(text, "yt-dlp failed"):
		return "youtube_download"
	case strings.Contains(text, "frame extraction failed") || strings.Contains(text, "video yielded no frames"):
		return "frame_extraction"
	default:
		return "internal"
	}
}

func failureMessage(stage string, err error) string {
	var target *youtube.UnsupportedError
	if errors.As(err, &target) {
		return "🚫 <b>Unsupported video</b> — " + html.EscapeString(target.Error())
	}
	switch failureReason(err) {
	case "timeout":
		return "⏱️ <b>Timed out during " + html.EscapeString(stage) + "</b> — the Short or model took longer than the five-minute job limit. Please retry once."
	case "provider_quota":
		return "💳 <b>GLM quota unavailable</b> — Z.AI rejected the inference request. Check the configured plan endpoint and quota, then retry."
	case "provider_incomplete_json":
		return "🧠 <b>GLM response was incomplete</b> — the model stopped before finishing the recipe JSON. Retry the same Short; the event is logged with safe token diagnostics."
	case "provider_invalid_recipe":
		return "🧩 <b>GLM returned an invalid recipe</b> — required ingredients or steps were missing from its structured response. Please retry."
	case "youtube_download":
		return "📹 <b>YouTube download failed</b> — the video may be private, age/region restricted, removed, or temporarily blocked by YouTube."
	case "frame_extraction":
		return "🖼️ <b>Frame extraction failed</b> — the video downloaded, but no usable visual frames could be decoded."
	default:
		return "⚠️ <b>Recipe processing failed during " + html.EscapeString(stage) + "</b> — the exact technical reason was recorded in the service log."
	}
}
