package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/conversation"
	"github.com/mrRedSun/telegram-recipe-bot/internal/recipe"
	"github.com/mrRedSun/telegram-recipe-bot/internal/source"
	"github.com/mrRedSun/telegram-recipe-bot/internal/telegram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

type fakeTG struct {
	once  sync.Once
	edits chan string
	mu    sync.Mutex
	sent  []string
	input string
}

func (f *fakeTG) Me(context.Context) (telegram.User, error) {
	return telegram.User{ID: 99, IsBot: true, Username: "RecipeBot"}, nil
}

func (f *fakeTG) Updates(ctx context.Context, _ int64) ([]telegram.Update, error) {
	var out []telegram.Update
	f.once.Do(func() {
		text := f.input
		if text == "" {
			text = "https://youtu.be/abcDEF_1234"
		}
		out = []telegram.Update{{UpdateID: 1, Message: &telegram.Message{From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9, Type: "private"}, Text: text}}}
	})
	if out != nil {
		return out, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeTG) Send(_ context.Context, _ int64, _ int64, text string) (telegram.Message, error) {
	f.mu.Lock()
	f.sent = append(f.sent, text)
	f.mu.Unlock()
	return telegram.Message{MessageID: int64(len(f.sent) + 4)}, nil
}

func (f *fakeTG) Edit(_ context.Context, _ int64, _ int64, text string) error {
	if f.edits != nil {
		f.edits <- text
	}
	return nil
}

type fakeExtract struct{}

func (fakeExtract) Extract(_ context.Context, url string, _ string) (youtube.Evidence, error) {
	return youtube.Evidence{SourceURL: url, Title: "Soup", Description: "creator recipe caption"}, nil
}

type fakeInfer struct {
	mu          sync.Mutex
	history     []conversation.Message
	recipeCalls int
	lastSource  string
}

func (f *fakeInfer) Infer(_ context.Context, ev youtube.Evidence) (recipe.Recipe, error) {
	f.mu.Lock()
	f.recipeCalls++
	f.lastSource = ev.SourceURL
	f.mu.Unlock()
	return recipe.Recipe{Title: "Soup", Ingredients: []recipe.Ingredient{{Item: "water", Confidence: "high"}}, Steps: []recipe.Step{{Instruction: "Heat", Confidence: "high"}}, Confidence: "high"}, nil
}

func (f *fakeInfer) Converse(_ context.Context, history []conversation.Message) (string, error) {
	f.mu.Lock()
	f.history = append([]conversation.Message(nil), history...)
	f.mu.Unlock()
	return "Assistant reply", nil
}

func testConfig(temp string) Config {
	return Config{
		Allowed:                  map[int64]struct{}{7: {}},
		Workers:                  1,
		QueueSize:                8,
		TempRoot:                 temp,
		JobTimeout:               time.Second,
		ConversationMessages:     20,
		ConversationScopes:       32,
		ConversationMessageRunes: 4000,
	}
}

func TestEndToEndRecipeMessage(t *testing.T) {
	for name, input := range map[string]string{"youtube": "https://youtu.be/abcDEF_1234", "instagram": "https://www.instagram.com/reel/Abc123/?exln=tracking", "instagram_command": "/recipe https://instagram.com/reel/Abc123/"} {
		t.Run(name, func(t *testing.T) {
			tg := &fakeTG{edits: make(chan string, 16), input: input}
			infer := &fakeInfer{}
			a := New(testConfig(t.TempDir()), tg, fakeExtract{}, infer)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { _ = a.Run(ctx); close(done) }()
			deadline := time.After(2 * time.Second)
			for {
				select {
				case got := <-tg.edits:
					if strings.Contains(got, "<h2>") {
						cancel()
						<-done
						infer.mu.Lock()
						defer infer.mu.Unlock()
						if infer.recipeCalls != 1 {
							t.Fatalf("expected exactly one recipe inference, got %d", infer.recipeCalls)
						}
						if name != "youtube" && infer.lastSource != "https://www.instagram.com/reel/Abc123/" {
							t.Fatalf("wrong recipe source: %s", infer.lastSource)
						}
						return
					}
				case <-deadline:
					cancel()
					<-done
					t.Fatal("timed out waiting for rendered recipe")
				}
			}
		})
	}
}

func TestGroupConversationRequiresExplicitTrigger(t *testing.T) {
	tg := &fakeTG{}
	a := New(testConfig(t.TempDir()), tg, fakeExtract{}, &fakeInfer{})
	a.bot = telegram.User{ID: 99, Username: "RecipeBot"}
	ordinary := telegram.Update{Message: &telegram.Message{From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9, Type: "supergroup"}, Text: "hello everyone"}}
	a.handle(context.Background(), ordinary)
	if len(a.jobs) != 0 || len(tg.sent) != 0 {
		t.Fatalf("ordinary group message triggered work: jobs=%d sent=%#v", len(a.jobs), tg.sent)
	}
	ordinary.Message.Text = "/ask hello"
	a.handle(context.Background(), ordinary)
	queued := <-a.jobs
	if queued.kind != conversationJob || queued.request != "hello" {
		t.Fatalf("unexpected queued job: %#v", queued)
	}
}

func TestGroupMentionAndReplyTriggerAssistant(t *testing.T) {
	for name, message := range map[string]*telegram.Message{
		"mention": {From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9, Type: "group"}, Text: "@RecipeBot hello"},
		"reply":   {From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9, Type: "group"}, Text: "hello", ReplyToMessage: &telegram.Message{From: &telegram.User{ID: 99, IsBot: true}}},
	} {
		t.Run(name, func(t *testing.T) {
			tg := &fakeTG{}
			a := New(testConfig(t.TempDir()), tg, fakeExtract{}, &fakeInfer{})
			a.bot = telegram.User{ID: 99, Username: "RecipeBot"}
			a.handle(context.Background(), telegram.Update{Message: message})
			if queued := <-a.jobs; queued.kind != conversationJob || queued.request != "hello" {
				t.Fatalf("unexpected job: %#v", queued)
			}
		})
	}
}

func TestInstagramDetectionAndRecipeCommandBothQueueRecipes(t *testing.T) {
	tg := &fakeTG{}
	a := New(testConfig(t.TempDir()), tg, fakeExtract{}, &fakeInfer{})
	a.bot = telegram.User{ID: 99, Username: "RecipeBot"}
	message := &telegram.Message{From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9, Type: "private"}, Caption: "look https://instagram.com/p/Abc123/"}
	a.handle(context.Background(), telegram.Update{Message: message})
	if queued := <-a.jobs; queued.kind != recipeJob {
		t.Fatalf("unexpected Instagram job: %#v", queued)
	}
	message.Caption = ""
	message.Text = "/recipe https://instagram.com/p/Abc123/"
	a.handle(context.Background(), telegram.Update{Message: message})
	if len(a.jobs) != 1 {
		t.Fatalf("Instagram /recipe did not queue recipe: jobs=%d", len(a.jobs))
	}
	if queued := <-a.jobs; queued.kind != recipeJob {
		t.Fatalf("wrong command workflow: %s", queued.kind)
	}
}

func TestConversationHistoryIncludesUserAndBotReplies(t *testing.T) {
	tg := &fakeTG{}
	infer := &fakeInfer{}
	a := New(testConfig(t.TempDir()), tg, fakeExtract{}, infer)
	item := job{id: 1, kind: conversationJob, chatID: 9, threadID: 77, request: "hello", statusID: 5, generation: a.conversations.Generation(conversation.Key{ChatID: 9, ThreadID: 77})}
	a.processConversation(context.Background(), &item)
	history := a.conversations.History(conversation.Key{ChatID: 9, ThreadID: 77})
	if len(history) != 2 || history[0].Text != "hello" || history[1].Text != "Assistant reply" {
		t.Fatalf("history = %#v", history)
	}
}

func TestParseGroupRecipeCommand(t *testing.T) {
	command, argument := parseCommand("/recipe@RecipeBot https://youtube.com/shorts/d9kcEzeoFXY?si=test")
	if command != "/recipe" || argument != "https://youtube.com/shorts/d9kcEzeoFXY?si=test" {
		t.Fatalf("command=%q argument=%q", command, argument)
	}
}

func TestExtractionProgressMessages(t *testing.T) {
	for _, stage := range []string{"download", "captions", "comments", "frames"} {
		if got := extractionProgressMessage(stage); got == "" || !strings.Contains(got, "Step") {
			t.Fatalf("stage %q has no progress message: %q", stage, got)
		}
	}
	if got := extractionProgressMessage("ocr"); got != "" {
		t.Fatalf("terminal extraction stage should flow directly to inference: %q", got)
	}
}

func TestFailureMessagesAreConcrete(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "Timed out during inference"},
		{errors.New("GLM HTTP 429"), "GLM quota unavailable"},
		{errors.New("GLM returned invalid recipe JSON"), "GLM response was incomplete"},
		{errors.New("yt-dlp failed"), "Video download failed"},
		{errors.New("frame extraction failed"), "Frame extraction failed"},
	}
	for _, tt := range tests {
		if got := failureMessage("inference", tt.err); !strings.Contains(got, tt.want) {
			t.Errorf("failureMessage(%q)=%q, want %q", tt.err, got, tt.want)
		}
	}
}

type blockingInfer struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingInfer) Infer(context.Context, youtube.Evidence) (recipe.Recipe, error) {
	return recipe.Recipe{Title: "Soup", Ingredients: []recipe.Ingredient{{Item: "water", Confidence: "high"}}, Steps: []recipe.Step{{Instruction: "Heat", Confidence: "high"}}, Confidence: "high"}, nil
}

func (b *blockingInfer) Converse(ctx context.Context, _ []conversation.Message) (string, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-b.release:
		return "reply", nil
	}
}

type signalingExtract struct{ called chan struct{} }

func (s signalingExtract) Extract(context.Context, string, string) (youtube.Evidence, error) {
	select {
	case s.called <- struct{}{}:
	default:
	}
	return youtube.Evidence{Title: "Soup"}, nil
}

func TestResetCancelsInFlightConversation(t *testing.T) {
	tg := &fakeTG{edits: make(chan string, 8)}
	infer := &blockingInfer{started: make(chan struct{}, 1), release: make(chan struct{})}
	a := New(testConfig(t.TempDir()), tg, fakeExtract{}, infer)
	key := conversation.Key{ChatID: 9, ThreadID: 3}
	item := &job{id: 1, kind: conversationJob, chatID: 9, threadID: 3, request: "private prompt", statusID: 5, generation: a.conversations.Generation(key)}
	a.registerConversationJob(key, item)
	done := make(chan struct{})
	go func() {
		a.processConversation(context.Background(), item)
		close(done)
	}()
	select {
	case <-infer.started:
	case <-time.After(time.Second):
		t.Fatal("conversation did not start")
	}
	a.resetConversation(key)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reset did not cancel the in-flight conversation")
	}
	if got := a.conversations.History(key); len(got) != 0 {
		t.Fatalf("history survived reset: %#v", got)
	}
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.request != "" {
		t.Fatal("queued prompt survived reset")
	}
}

func TestSameScopeConversationDoesNotBlockRecipeWorker(t *testing.T) {
	tg := &fakeTG{edits: make(chan string, 32)}
	infer := &blockingInfer{started: make(chan struct{}, 2), release: make(chan struct{})}
	extract := signalingExtract{called: make(chan struct{}, 1)}
	cfg := testConfig(t.TempDir())
	cfg.Workers = 2
	a := New(cfg, tg, extract, infer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < cfg.Workers; i++ {
		a.wg.Add(1)
		go a.worker(ctx)
	}
	key := conversation.Key{ChatID: 9}
	generation := a.conversations.Generation(key)
	first := &job{id: 1, kind: conversationJob, chatID: 9, request: "one", statusID: 1, generation: generation}
	second := &job{id: 2, kind: conversationJob, chatID: 9, request: "two", statusID: 2, generation: generation}
	recipeItem := &job{id: 3, kind: recipeJob, chatID: 9, request: "https://www.youtube.com/watch?v=abcDEF_1234", statusID: 3}
	for _, item := range []*job{first, second} {
		a.registerConversationJob(key, item)
	}
	for _, item := range []*job{first, second, recipeItem} {
		a.slots <- struct{}{}
		a.jobs <- item
	}
	select {
	case <-infer.started:
	case <-time.After(time.Second):
		t.Fatal("first conversation did not start")
	}
	select {
	case <-extract.called:
	case <-time.After(2 * time.Second):
		t.Fatal("recipe remained blocked behind the same conversation scope")
	}
	close(infer.release)
	cancel()
	a.wg.Wait()
}

func TestCommandForAnotherBotIsIgnored(t *testing.T) {
	tg := &fakeTG{}
	a := New(testConfig(t.TempDir()), tg, fakeExtract{}, &fakeInfer{})
	a.bot = telegram.User{ID: 99, Username: "RecipeBot"}
	message := &telegram.Message{From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9, Type: "group"}, Text: "/ask@OtherBot hello"}
	a.handle(context.Background(), telegram.Update{Message: message})
	if len(a.jobs) != 0 || len(tg.sent) != 0 {
		t.Fatalf("command for another bot triggered work: jobs=%d sent=%#v", len(a.jobs), tg.sent)
	}
}

func TestConversationJobsPreserveArrivalOrder(t *testing.T) {
	tg := &fakeTG{}
	infer := &fakeInfer{}
	a := New(testConfig(t.TempDir()), tg, fakeExtract{}, infer)
	key := conversation.Key{ChatID: 9}
	generation := a.conversations.Generation(key)
	first := &job{id: 1, kind: conversationJob, chatID: 9, request: "first", generation: generation}
	second := &job{id: 2, kind: conversationJob, chatID: 9, request: "second", generation: generation}
	a.registerConversationJob(key, first)
	a.registerConversationJob(key, second)
	if a.processConversation(context.Background(), second) {
		t.Fatal("later job overtook the first request")
	}
	if !a.processConversation(context.Background(), first) {
		t.Fatal("first request was deferred")
	}
	if !a.processConversation(context.Background(), second) {
		t.Fatal("second request did not resume")
	}
	history := a.conversations.History(key)
	if len(history) != 4 || history[0].Text != "first" || history[2].Text != "second" {
		t.Fatalf("incorrect conversation order: %#v", history)
	}
}

type diagnosticSource struct{}

func (diagnosticSource) Extract(context.Context, string, string) (youtube.Evidence, error) {
	panic("diagnostic source path was skipped")
}
func (diagnosticSource) ExtractWithDiagnostics(_ context.Context, _ string, _ string, _ func(string), report func(source.Diagnostic)) (youtube.Evidence, error) {
	report(source.Diagnostic{Source: "instagram", Stage: "metadata", Backend: "yt-dlp", Outcome: "failed", Reason: "login_required", ExitCode: 4})
	return youtube.Evidence{}, errors.New("source command failed (metadata: login_required)")
}
func TestSourceStageDiagnosticsIncludeAnonymousJobOnly(t *testing.T) {
	var logs bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(prior)
	a := New(testConfig(t.TempDir()), &fakeTG{}, diagnosticSource{}, &fakeInfer{})
	a.processRecipe(context.Background(), &job{id: 42, kind: recipeJob, chatID: 987654321, request: "https://instagram.com/reel/Private123/"})
	text := logs.String()
	for _, expected := range []string{`"job":42`, `"source":"instagram"`, `"stage":"metadata"`, `"backend":"yt-dlp"`, `"reason":"login_required"`, `"exit_code":4`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing diagnostic %s", expected)
		}
	}
	for _, private := range []string{"987654321", "Private123", "instagram.com"} {
		if strings.Contains(text, private) {
			t.Fatalf("private request data reached logs: %s", private)
		}
	}
}
