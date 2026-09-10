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
	if text == "/start" || text == "/help" {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "Send one YouTube Short URL. I’ll reconstruct the recipe and mark anything uncertain.\n\n/privacy — data handling\n/status — service status")
		return
	}
	if text == "/privacy" {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "The bot keeps no recipe history, usernames, URLs, or videos. Each download and its sampled frames are removed when the job finishes. Telegram and Z.AI still process the data under their own policies.")
		return
	}
	if text == "/status" {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "Recipe bot is ready.")
		return
	}
	url, err := youtube.Normalize(text)
	if err != nil {
		_, _ = a.tg.Send(ctx, m.Chat.ID, "Please send one valid youtube.com or youtu.be video URL.")
		return
	}
	status, err := a.tg.Send(ctx, m.Chat.ID, "⏳ Downloading and analyzing the Short…")
	if err != nil {
		slog.Warn("send status failed", "error", err)
		return
	}
	select {
	case a.jobs <- job{chatID: m.Chat.ID, url: url, statusID: status.MessageID}:
	default:
		_ = a.tg.Edit(ctx, m.Chat.ID, status.MessageID, "The bot is busy right now. Please try again shortly.")
	}
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
	id := a.sequence.Add(1)
	dir := filepath.Join(a.cfg.TempRoot, fmt.Sprintf("job-%d", id))
	if err := os.MkdirAll(dir, 0700); err != nil {
		a.fail(parent, j, err)
		return
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("temporary job cleanup failed", "job", id, "error", err)
		}
	}()
	ev, err := a.extract.Extract(ctx, j.url, dir)
	if err != nil {
		a.fail(parent, j, err)
		return
	}
	r, err := a.infer.Infer(ctx, ev)
	if err != nil {
		a.fail(parent, j, err)
		return
	}
	if err := a.tg.Edit(parent, j.chatID, j.statusID, recipe.RenderHTML(r)); err != nil {
		slog.Warn("deliver recipe failed", "job", id, "error", err)
		return
	}
	slog.Info("recipe delivered", "job", id, "confidence", r.Confidence)
}
func (a *App) fail(ctx context.Context, j job, err error) {
	slog.Warn("recipe job failed", "error", err)
	msg := "I couldn’t extract a reliable recipe from that Short. Please check that it is public, under the configured duration limit, and try again."
	var target *youtube.UnsupportedError
	if errors.As(err, &target) {
		msg = html.EscapeString(target.Error())
	}
	_ = a.tg.Edit(ctx, j.chatID, j.statusID, msg)
}
