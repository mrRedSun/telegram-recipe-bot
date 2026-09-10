package app

import (
	"context"
	"github.com/mrRedSun/telegram-recipe-bot/internal/recipe"
	"github.com/mrRedSun/telegram-recipe-bot/internal/telegram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
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
	tg := &fakeTG{edits: make(chan string, 1)}
	a := New(Config{Allowed: map[int64]struct{}{7: {}}, Workers: 1, QueueSize: 1, TempRoot: t.TempDir(), JobTimeout: time.Second}, tg, fakeExtract{}, fakeInfer{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = a.Run(ctx); close(done) }()
	select {
	case got := <-tg.edits:
		if got == "" {
			t.Fatal("empty recipe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
	cancel()
	<-done
}
