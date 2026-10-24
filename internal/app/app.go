package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/conversation"
	"github.com/mrRedSun/telegram-recipe-bot/internal/recipe"
	"github.com/mrRedSun/telegram-recipe-bot/internal/source"
	"github.com/mrRedSun/telegram-recipe-bot/internal/telegram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

type Messenger interface {
	Me(context.Context) (telegram.User, error)
	Updates(context.Context, int64) ([]telegram.Update, error)
	Send(context.Context, int64, int64, string) (telegram.Message, error)
	Edit(context.Context, int64, int64, string) error
}

type Extractor interface {
	Extract(context.Context, string, string) (youtube.Evidence, error)
}

type progressExtractor interface {
	ExtractWithProgress(context.Context, string, string, func(string)) (youtube.Evidence, error)
}

type diagnosticExtractor interface {
	ExtractWithDiagnostics(context.Context, string, string, func(string), func(source.Diagnostic)) (youtube.Evidence, error)
}

type Inferer interface {
	Infer(context.Context, youtube.Evidence) (recipe.Recipe, error)
	Converse(context.Context, []conversation.Message) (string, error)
}

type Config struct {
	Allowed                  map[int64]struct{}
	Workers, QueueSize       int
	TempRoot                 string
	JobTimeout               time.Duration
	ConversationMessages     int
	ConversationScopes       int
	ConversationMessageRunes int
}

type jobKind string

const (
	recipeJob       jobKind = "recipe"
	conversationJob jobKind = "conversation"
)

type job struct {
	id         uint64
	kind       jobKind
	chatID     int64
	threadID   int64
	request    string
	statusID   int64
	generation uint64
	mu         sync.Mutex
	cancel     context.CancelFunc
}

type App struct {
	cfg              Config
	tg               Messenger
	extract          Extractor
	infer            Inferer
	conversations    *conversation.Store
	jobs             chan *job
	slots            chan struct{}
	wg               sync.WaitGroup
	conversationMu   sync.Mutex
	conversationJobs map[conversation.Key]map[uint64]*job
	ready            atomic.Bool
	sequence         atomic.Uint64
	bot              telegram.User
}

func New(cfg Config, tg Messenger, extract Extractor, infer Inferer) *App {
	return &App{
		cfg:              cfg,
		tg:               tg,
		extract:          extract,
		infer:            infer,
		conversations:    conversation.NewStore(cfg.ConversationMessages, cfg.ConversationScopes, cfg.ConversationMessageRunes),
		jobs:             make(chan *job, cfg.QueueSize),
		slots:            make(chan struct{}, cfg.QueueSize+cfg.Workers),
		conversationJobs: make(map[conversation.Key]map[uint64]*job),
	}
}

func (a *App) Run(ctx context.Context) error {
	bot, err := a.tg.Me(ctx)
	if err != nil {
		return fmt.Errorf("load Telegram bot identity: %w", err)
	}
	a.bot = bot
	for i := 0; i < a.cfg.Workers; i++ {
		a.wg.Add(1)
		go a.worker(ctx)
	}
	a.ready.Store(true)
	defer func() { a.ready.Store(false); a.wg.Wait() }()
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
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			a.handle(ctx, update)
		}
	}
}

func (a *App) Ready() bool { return a.ready.Load() }

func (a *App) handle(ctx context.Context, update telegram.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	if _, ok := a.cfg.Allowed[message.From.ID]; !ok {
		return
	}
	text := messageText(message)
	command, argument := parseCommand(text)
	if target := commandTarget(text); target != "" && !strings.EqualFold(target, a.bot.Username) {
		return
	}
	key := conversation.Key{ChatID: message.Chat.ID, ThreadID: message.MessageThreadID}
	switch command {
	case "/start", "/help":
		_, _ = a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, "Send a YouTube video/Short or public Instagram Reel URL to get a recipe. You can also chat with me directly. In groups, use <code>/recipe URL</code>, <code>/ask question</code>, mention me, or reply to me.\n\n/reset — clear this chat/topic’s temporary context\n/privacy — data handling\n/status — service status")
		return
	case "/privacy":
		_, _ = a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, "Conversation context is kept only in memory, separately per DM, group, or forum topic, and is limited to the latest 20 messages. <code>/reset</code> clears it and a restart removes it. URLs, messages, context, source media, and identities are not logged. Source video and recipe files use temporary storage and are removed after each job. Telegram, Instagram, YouTube, and Z.AI still process data under their own policies.")
		return
	case "/status":
		_, _ = a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, "Recipe bot and assistant are ready.")
		return
	case "/reset":
		a.resetConversation(key)
		_, _ = a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, "🧹 Temporary conversation context cleared for this chat or topic.")
		return
	case "/recipe":
		a.queueRecipe(ctx, message, argument)
		return
	}

	if url, err := source.Normalize(text); err == nil {
		a.queue(ctx, message, recipeJob, url, "🕐 <b>Queued</b> — waiting for a recipe worker…")
		return
	}

	prompt := text
	group := message.Chat.Type == "group" || message.Chat.Type == "supergroup"
	if group {
		explicit := false
		switch {
		case command == "/ask":
			prompt, explicit = argument, true
		case mentioned(text, a.bot.Username):
			prompt, explicit = removeMention(text, a.bot.Username), true
		case message.ReplyToMessage != nil && message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == a.bot.ID:
			explicit = true
		}
		if !explicit {
			return
		}
	} else if command == "/ask" {
		prompt = argument
	} else if command != "" {
		return
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		_, _ = a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, "Please include a question or a YouTube video/Short or Instagram Reel URL.")
		return
	}
	if recipeURL, ok := source.Find(prompt); ok {
		a.queue(ctx, message, recipeJob, recipeURL, "🕐 <b>Queued</b> — waiting for a recipe worker…")
		return
	}
	a.queue(ctx, message, conversationJob, prompt, "🧠 <b>Thinking…</b>")
}

func (a *App) queueRecipe(ctx context.Context, message *telegram.Message, raw string) {
	url, err := source.Normalize(raw)
	if err != nil {
		_, _ = a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, "❌ Please send one valid YouTube video/Short or Instagram Reel URL after <code>/recipe</code>.")
		return
	}
	a.queue(ctx, message, recipeJob, url, "🕐 <b>Queued</b> — waiting for a recipe worker…")
}

func (a *App) queue(ctx context.Context, message *telegram.Message, kind jobKind, request, statusText string) {
	id := a.sequence.Add(1)
	status, err := a.tg.Send(ctx, message.Chat.ID, message.MessageThreadID, statusText)
	if err != nil {
		slog.Warn("send status failed", "job", id, "kind", kind, "error", err)
		return
	}
	select {
	case a.slots <- struct{}{}:
	default:
		slog.Warn("job queue full", "job", id, "kind", kind, "queue_capacity", a.cfg.QueueSize)
		_ = a.tg.Edit(ctx, message.Chat.ID, status.MessageID, "🚦 <b>Queue full</b> — both workers are busy. Please retry shortly.")
		return
	}
	queued := &job{id: id, kind: kind, chatID: message.Chat.ID, threadID: message.MessageThreadID, request: request, statusID: status.MessageID}
	var key conversation.Key
	if kind == conversationJob {
		key = conversation.Key{ChatID: message.Chat.ID, ThreadID: message.MessageThreadID}
		queued.generation = a.conversations.Generation(key)
		a.registerConversationJob(key, queued)
	}
	select {
	case a.jobs <- queued:
		slog.Info("job queued", "job", id, "kind", kind, "queue_depth", len(a.jobs))
	default:
		<-a.slots
		if kind == conversationJob {
			a.unregisterConversationJob(key, queued.id)
			queued.clearRequest()
		}
		slog.Warn("job queue full", "job", id, "kind", kind, "queue_capacity", cap(a.jobs))
		_ = a.tg.Edit(ctx, message.Chat.ID, status.MessageID, "🚦 <b>Queue full</b> — both workers are busy. Please retry shortly.")
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

func commandTarget(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return ""
	}
	parts := strings.SplitN(fields[0], "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func messageText(message *telegram.Message) string {
	if text := strings.TrimSpace(message.Text); text != "" {
		return text
	}
	return strings.TrimSpace(message.Caption)
}

func mentioned(text, username string) bool {
	if username == "" {
		return false
	}
	pattern := `(?i)(^|[^A-Za-z0-9_])@` + regexp.QuoteMeta(username) + `([^A-Za-z0-9_]|$)`
	return regexp.MustCompile(pattern).MatchString(text)
}

func removeMention(text, username string) string {
	if username == "" {
		return strings.TrimSpace(text)
	}
	pattern := `(?i)@` + regexp.QuoteMeta(username) + `\b`
	return strings.TrimSpace(regexp.MustCompile(pattern).ReplaceAllString(text, ""))
}

func (a *App) worker(ctx context.Context) {
	defer a.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-a.jobs:
			if item == nil {
				continue
			}
			processed := true
			switch item.kind {
			case recipeJob:
				a.processRecipe(ctx, item)
			case conversationJob:
				processed = a.processConversation(ctx, item)
			}
			if !processed {
				a.wg.Add(1)
				go a.retryConversation(ctx, item)
				continue
			}
			<-a.slots
		}
	}
}

func (a *App) retryConversation(ctx context.Context, item *job) {
	defer a.wg.Done()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		item.clearRequest()
		a.unregisterConversationJob(conversation.Key{ChatID: item.chatID, ThreadID: item.threadID}, item.id)
		<-a.slots
	case <-timer.C:
		select {
		case <-ctx.Done():
			item.clearRequest()
			a.unregisterConversationJob(conversation.Key{ChatID: item.chatID, ThreadID: item.threadID}, item.id)
			<-a.slots
		case a.jobs <- item:
		}
	}
}

func (a *App) processConversation(parent context.Context, item *job) bool {
	key := conversation.Key{ChatID: item.chatID, ThreadID: item.threadID}
	if !a.firstConversationJob(key, item.id) {
		return false
	}
	unlock, ok := a.conversations.TryLock(key)
	if !ok {
		return false
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(parent, a.cfg.JobTimeout)
	item.mu.Lock()
	if item.request == "" {
		item.mu.Unlock()
		cancel()
		a.unregisterConversationJob(key, item.id)
		_ = a.tg.Edit(parent, item.chatID, item.statusID, "🧹 <b>Cancelled by /reset</b>")
		return true
	}
	request := item.request
	item.cancel = cancel
	item.mu.Unlock()
	defer func() {
		cancel()
		item.mu.Lock()
		item.request = ""
		item.cancel = nil
		item.mu.Unlock()
		a.unregisterConversationJob(key, item.id)
	}()
	if a.conversations.Generation(key) != item.generation {
		_ = a.tg.Edit(parent, item.chatID, item.statusID, "🧹 <b>Cancelled by /reset</b>")
		return true
	}
	history := a.conversations.History(key)
	userMessage := conversation.Message{Role: "user", Text: clipRunes(request, a.cfg.ConversationMessageRunes)}
	request = ""
	if keep := a.cfg.ConversationMessages - 1; keep >= 0 && len(history) > keep {
		history = append([]conversation.Message(nil), history[len(history)-keep:]...)
	}
	history = append(history, userMessage)
	started := time.Now()
	answer, err := a.infer.Converse(ctx, history)
	if a.conversations.Generation(key) != item.generation {
		_ = a.tg.Edit(parent, item.chatID, item.statusID, "🧹 <b>Cancelled by /reset</b>")
		return true
	}
	if err != nil {
		slog.Warn("conversation failed", "job", item.id, "reason", assistantFailureReason(err))
		_ = a.tg.Edit(parent, item.chatID, item.statusID, assistantFailureMessage(err))
		return true
	}
	if err := a.tg.Edit(ctx, item.chatID, item.statusID, renderAssistant(answer)); err != nil {
		slog.Warn("deliver conversation failed", "job", item.id, "error", err)
		return true
	}
	a.conversations.AppendIfGeneration(key, item.generation, userMessage, conversation.Message{Role: "assistant", Text: answer})
	slog.Info("conversation delivered", "job", item.id, "elapsed_ms", time.Since(started).Milliseconds(), "response_bytes", len(answer))
	return true
}

func (j *job) clearRequest() {
	j.mu.Lock()
	j.request = ""
	if j.cancel != nil {
		j.cancel()
	}
	j.mu.Unlock()
}

func (a *App) registerConversationJob(key conversation.Key, item *job) {
	a.conversationMu.Lock()
	defer a.conversationMu.Unlock()
	jobs := a.conversationJobs[key]
	if jobs == nil {
		jobs = make(map[uint64]*job)
		a.conversationJobs[key] = jobs
	}
	jobs[item.id] = item
}

// firstConversationJob preserves arrival order while busy scopes yield workers.
func (a *App) firstConversationJob(key conversation.Key, id uint64) bool {
	a.conversationMu.Lock()
	defer a.conversationMu.Unlock()
	for queuedID := range a.conversationJobs[key] {
		if queuedID < id {
			return false
		}
	}
	return true
}

func (a *App) unregisterConversationJob(key conversation.Key, id uint64) {
	a.conversationMu.Lock()
	defer a.conversationMu.Unlock()
	jobs := a.conversationJobs[key]
	delete(jobs, id)
	if len(jobs) == 0 {
		delete(a.conversationJobs, key)
	}
}

func (a *App) resetConversation(key conversation.Key) {
	a.conversations.Reset(key)
	a.conversationMu.Lock()
	jobs := make([]*job, 0, len(a.conversationJobs[key]))
	for _, item := range a.conversationJobs[key] {
		jobs = append(jobs, item)
	}
	a.conversationMu.Unlock()
	for _, item := range jobs {
		item.clearRequest()
	}
}

func renderAssistant(answer string) string {
	escaped := html.EscapeString(strings.TrimSpace(answer))
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return "<p>" + escaped + "</p>"
}

func assistantFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if strings.Contains(strings.ToLower(err.Error()), "glm http 429") {
		return "provider_quota"
	}
	return "provider"
}

func assistantFailureMessage(err error) string {
	switch assistantFailureReason(err) {
	case "timeout":
		return "⏱️ <b>Assistant timed out</b> — please retry with a shorter request."
	case "provider_quota":
		return "💳 <b>GLM quota unavailable</b> — Z.AI rejected the request."
	default:
		return "⚠️ <b>Assistant request failed</b> — please retry shortly."
	}
}

func (a *App) processRecipe(parent context.Context, item *job) {
	ctx, cancel := context.WithTimeout(parent, a.cfg.JobTimeout)
	defer cancel()
	started := time.Now()
	dir := filepath.Join(a.cfg.TempRoot, fmt.Sprintf("job-%d", item.id))
	slog.Info("recipe job started", "job", item.id)
	a.progress(parent, item, "download", "⬇️ <b>Step 1/7 — Downloading video</b>\nFetching the source video and its caption/metadata…")
	if err := os.MkdirAll(dir, 0700); err != nil {
		a.failRecipe(parent, item, "workspace", err)
		return
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("temporary job cleanup failed", "job", item.id, "kind", item.kind, "error", err)
		}
	}()
	extractStarted := time.Now()
	var evidence youtube.Evidence
	var err error
	progress := func(stage string) {
		if message := extractionProgressMessage(stage); message != "" {
			a.progress(parent, item, stage, message)
		}
	}
	if extractor, ok := a.extract.(diagnosticExtractor); ok {
		evidence, err = extractor.ExtractWithDiagnostics(ctx, item.request, dir, progress, func(event source.Diagnostic) {
			level := slog.LevelInfo
			if event.Outcome == "failed" {
				level = slog.LevelWarn
			}
			slog.Log(ctx, level, "recipe source progressed", "job", item.id, "source", event.Source, "stage", event.Stage, "backend", event.Backend, "outcome", event.Outcome, "reason", event.Reason, "exit_code", event.ExitCode, "http_status", event.HTTPStatus, "items", event.Items, "bytes", event.Bytes, "elapsed_ms", event.ElapsedMS)
		})
	} else if extractor, ok := a.extract.(progressExtractor); ok {
		evidence, err = extractor.ExtractWithProgress(ctx, item.request, dir, progress)
	} else {
		evidence, err = a.extract.Extract(ctx, item.request, dir)
	}
	if err != nil {
		a.failRecipe(parent, item, "extract", err)
		return
	}
	slog.Info("recipe evidence extracted", "job", item.id,
		"elapsed_ms", time.Since(extractStarted).Milliseconds(),
		"duration_seconds", int(evidence.Duration), "frames", len(evidence.Frames),
		"description_bytes", len(evidence.Description), "author_comment_bytes", len(evidence.AuthorComments),
		"transcript_bytes", len(evidence.Transcript), "ocr_bytes", len(evidence.OCR))
	a.progress(parent, item, "inference", "🧠 <b>Step 6/7 — Reconstructing recipe</b>\nGLM is comparing the collected evidence…")
	inferStarted := time.Now()
	result, err := a.infer.Infer(ctx, evidence)
	if err != nil {
		a.failRecipe(parent, item, "inference", err)
		return
	}
	slog.Info("recipe inference completed", "job", item.id, "elapsed_ms", time.Since(inferStarted).Milliseconds(), "confidence", result.Confidence, "ingredients", len(result.Ingredients), "steps", len(result.Steps), "references", len(result.References))
	a.progress(parent, item, "format", "🧾 <b>Step 7/7 — Formatting</b>\nBuilding the Telegram recipe card…")
	if err := a.tg.Edit(parent, item.chatID, item.statusID, recipe.RenderRichHTML(result)); err != nil {
		slog.Warn("deliver recipe failed", "job", item.id, "stage", "delivery", "error", err)
		return
	}
	slog.Info("recipe delivered", "job", item.id, "elapsed_ms", time.Since(started).Milliseconds(), "confidence", result.Confidence)
}

func (a *App) progress(ctx context.Context, item *job, stage, message string) {
	if err := a.tg.Edit(ctx, item.chatID, item.statusID, message); err != nil {
		slog.Warn("progress update failed", "job", item.id, "stage", stage, "error", err)
		return
	}
	slog.Info("recipe job progressed", "job", item.id, "stage", stage)
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

func (a *App) failRecipe(ctx context.Context, item *job, stage string, err error) {
	reason := failureReason(err)
	slog.Warn("recipe job failed", "job", item.id, "stage", stage, "reason", reason)
	_ = a.tg.Edit(ctx, item.chatID, item.statusID, failureMessage(stage, err))
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
	case strings.Contains(text, "source command failed (metadata"), strings.Contains(text, "source command failed (download"), strings.Contains(text, "yt-dlp failed"):
		return "source_download"
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
		return "⏱️ <b>Timed out during " + html.EscapeString(stage) + "</b> — the video or model took longer than the five-minute job limit. Please retry once."
	case "provider_quota":
		return "💳 <b>GLM quota unavailable</b> — Z.AI rejected the inference request. Check the configured plan endpoint and quota, then retry."
	case "provider_incomplete_json":
		return "🧠 <b>GLM response was incomplete</b> — the model stopped before finishing the recipe JSON. Retry the same video; the event is logged with safe token diagnostics."
	case "provider_invalid_recipe":
		return "🧩 <b>GLM returned an invalid recipe</b> — required ingredients or steps were missing from its structured response. Please retry."
	case "source_download":
		return "📹 <b>Video download failed</b> — the source may be private, login-gated, removed, or temporarily unavailable."
	case "frame_extraction":
		return "🖼️ <b>Frame extraction failed</b> — the video downloaded, but no usable visual frames could be decoded."
	default:
		return "⚠️ <b>Recipe processing failed during " + html.EscapeString(stage) + "</b> — a privacy-safe failure category was recorded in the service log."
	}
}

func clipRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
