package source

import (
	"context"
	"errors"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/instagram"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

type Diagnostic = youtube.Diagnostic

type Extractor struct {
	MaxDuration    time.Duration
	MaxBytes       int64
	TesseractLangs string
}

func Normalize(raw string) (string, error) {
	if canonical, err := youtube.Normalize(raw); err == nil {
		return canonical, nil
	}
	if canonical, err := instagram.Normalize(raw); err == nil {
		return canonical, nil
	}
	return "", errors.New("send one public YouTube video/Short or Instagram Reel URL")
}

func Find(text string) (string, bool) {
	if canonical, err := Normalize(text); err == nil {
		return canonical, true
	}
	return instagram.Find(text)
}

func (e Extractor) Extract(ctx context.Context, raw, dir string) (youtube.Evidence, error) {
	return e.ExtractWithDiagnostics(ctx, raw, dir, nil, nil)
}
func (e Extractor) ExtractWithProgress(ctx context.Context, raw, dir string, progress func(string)) (youtube.Evidence, error) {
	return e.ExtractWithDiagnostics(ctx, raw, dir, progress, nil)
}
func (e Extractor) ExtractWithDiagnostics(ctx context.Context, raw, dir string, progress func(string), report func(Diagnostic)) (youtube.Evidence, error) {
	engine := youtube.Extractor{MaxDuration: e.MaxDuration, MaxBytes: e.MaxBytes, TesseractLangs: e.TesseractLangs, NormalizeURL: Normalize}
	return engine.ExtractWithDiagnostics(ctx, raw, dir, progress, report)
}
