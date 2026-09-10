//go:build integration

package youtube

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveExtraction(t *testing.T) {
	raw := os.Getenv("TEST_YOUTUBE_URL")
	if raw == "" {
		t.Skip("TEST_YOUTUBE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	ev, err := (Extractor{MaxDuration: 4 * time.Minute, MaxBytes: 100 << 20, TesseractLangs: "eng"}).Extract(ctx, raw, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Title == "" || ev.Duration <= 0 || len(ev.Frames) == 0 {
		t.Fatalf("incomplete evidence: title=%q duration=%v frames=%d", ev.Title, ev.Duration, len(ev.Frames))
	}
	t.Logf("extracted title=%q duration=%.0fs frames=%d transcript_bytes=%d ocr_bytes=%d", ev.Title, ev.Duration, len(ev.Frames), len(ev.Transcript), len(ev.OCR))
}
