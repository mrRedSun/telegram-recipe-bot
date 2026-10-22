package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRecipeSourcePolicies(t *testing.T) {
	for raw, want := range map[string]string{
		"https://youtube.com/shorts/abcDEF_1234?feature=share": "https://www.youtube.com/watch?v=abcDEF_1234",
		"https://instagram.com/reels/Abc123/?exln=tracking":    "https://www.instagram.com/reel/Abc123/",
	} {
		if got, err := Normalize(raw); err != nil || got != want {
			t.Fatalf("source normalization failed: %q, %v", got, err)
		}
	}
	for _, raw := range []string{"file:///tmp/source", "http://127.0.0.1/x", "https://instagram.example/reel/Abc123/", "https://instagram.com/profile/"} {
		if _, err := Normalize(raw); err == nil {
			t.Fatalf("accepted unsupported source %s", raw)
		}
	}
}

func TestBothPlatformsUseTheSameEvidencePipeline(t *testing.T) {
	bin := t.TempDir()
	tools := map[string]string{
		"yt-dlp": `#!/bin/sh
case "$*" in
 *--dump-single-json*) printf '%s\n' '{"_type":"playlist","entries":[{"_type":"video","title":"Soup","description":"Creator caption: water and salt"}]}' ;;
 *--skip-download*) exit 0 ;;
 *)
 while [ "$#" -gt 0 ]; do
  if [ "$1" = '-o' ]; then shift; output=$1; fi
  shift
 done
 printf '%s' 'video' > "${output%.*}.mp4"
 ;;
esac
`,
		"ffprobe": "#!/bin/sh\nprintf '%s\\n' '{\"format\":{\"duration\":\"24.0\"}}'\n",
		"ffmpeg": `#!/bin/sh
for arg; do output=$arg; done
printf '%s' frame > "${output%/*}/frame-001.jpg"
`,
		"tesseract": "#!/bin/sh\nprintf '%s\\n' 'salt to taste'\n",
	}
	for name, body := range tools {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	for platform, url := range map[string]string{"instagram": "https://instagram.com/reel/Abc123/", "youtube": "https://youtube.com/shorts/abcDEF_1234"} {
		t.Run(platform, func(t *testing.T) {
			var stages []string
			var diagnostics []Diagnostic
			e := Extractor{MaxDuration: time.Minute, MaxBytes: 1024, TesseractLangs: "eng"}
			ev, err := e.ExtractWithDiagnostics(context.Background(), url, t.TempDir(), func(stage string) { stages = append(stages, stage) }, func(d Diagnostic) { diagnostics = append(diagnostics, d) })
			if err != nil {
				t.Fatal(err)
			}
			if ev.Description != "Creator caption: water and salt" || ev.OCR != "salt to taste" || ev.Duration != 24 || len(ev.Frames) != 1 {
				t.Fatalf("incomplete recipe evidence: %#v", ev)
			}
			if !reflect.DeepEqual(stages, []string{"download", "captions", "comments", "frames", "ocr"}) {
				t.Fatalf("different recipe stages: %v", stages)
			}
			for _, d := range diagnostics {
				if d.Source != platform {
					t.Fatalf("wrong source label: %#v", d)
				}
			}
		})
	}
}

func TestExtractorErrorsNeverExposeStderrOrSource(t *testing.T) {
	bin := t.TempDir()
	body := "#!/bin/sh\nprintf '%s\\n' 'HTTP Error 429 private-token https://private.example/source Private123' >&2\nexit 7\n"
	if err := os.WriteFile(filepath.Join(bin, "yt-dlp"), []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var events []Diagnostic
	e := Extractor{MaxDuration: time.Minute, MaxBytes: 1024}
	_, err := e.ExtractWithDiagnostics(context.Background(), "https://instagram.com/reel/Private123/", t.TempDir(), nil, func(d Diagnostic) { events = append(events, d) })
	if err == nil {
		t.Fatal("expected extractor failure")
	}
	text := fmt.Sprint(events) + err.Error()
	for _, private := range []string{"private-token", "private.example", "Private123", "instagram.com"} {
		if strings.Contains(text, private) {
			t.Fatalf("private data reached diagnostic: %s", private)
		}
	}
	last := events[len(events)-1]
	if last.Stage != "metadata" || last.Reason != "rate_limited" || last.ExitCode != 7 || last.HTTPStatus != 429 {
		t.Fatalf("incomplete diagnostics: %#v", last)
	}
}
