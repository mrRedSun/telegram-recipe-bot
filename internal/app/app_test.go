package app

import (
	"context"
	"errors"
	"github.com/mrRedSun/telegram-recipe-bot/internal/recipe"
	"github.com/mrRedSun/telegram-recipe-bot/internal/telegram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTG struct {
	once  sync.Once
	edits chan string
}

func (f *fakeTG) Updates(ctx context.Context, _ int64) ([]telegram.Update, error) {
	var out []telegram.Update
	f.once.Do(func() {
		out = []telegram.Update{{UpdateID: 1, Message: &telegram.Message{From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: 9}, Text: "https://youtu.be/abcDEF_1234"}}}
	})
	if out != nil {
		return out, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (f *fakeTG) Send(context.Context, int64, string) (telegram.Message, error) {
	return telegram.Message{MessageID: 5}, nil
}
func (f *fakeTG) Edit(_ context.Context, _ int64, _ int64, text string) error {
	f.edits <- text
	return nil
}

type fakeExtract struct{}

func (fakeExtract) Extract(context.Context, string, string) (youtube.Evidence, error) {
	return youtube.Evidence{Title: "Soup"}, nil
}

type fakeInfer struct{}

func (fakeInfer) Infer(context.Context, youtube.Evidence) (recipe.Recipe, error) {
	return recipe.Recipe{Title: "Soup", Ingredients: []recipe.Ingredient{{Item: "water", Confidence: "high"}}, Steps: []recipe.Step{{Instruction: "Heat", Confidence: "high"}}, Confidence: "high"}, nil
}
func TestEndToEndMessage(t *testing.T) {
	tg := &fakeTG{edits: make(chan string, 16)}
	a := New(Config{Allowed: map[int64]struct{}{7: {}}, Workers: 1, QueueSize: 1, TempRoot: t.TempDir(), JobTimeout: time.Second}, tg, fakeExtract{}, fakeInfer{})
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
				return
			}
		case <-deadline:
			cancel()
			<-done
			t.Fatal("timed out waiting for rendered recipe")
		}
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
		{errors.New("yt-dlp failed"), "YouTube download failed"},
		{errors.New("frame extraction failed"), "Frame extraction failed"},
	}
	for _, tt := range tests {
		if got := failureMessage("inference", tt.err); !strings.Contains(got, tt.want) {
			t.Errorf("failureMessage(%q)=%q, want %q", tt.err, got, tt.want)
		}
	}
}
