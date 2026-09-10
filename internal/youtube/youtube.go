package youtube

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)

type Evidence struct {
	SourceURL   string
	Title       string
	Description string
	Duration    float64
	Transcript  string
	OCR         string
	Frames      []string
}
type Extractor struct {
	MaxDuration    time.Duration
	MaxBytes       int64
	TesseractLangs string
}
type UnsupportedError struct{ Message string }

func (e *UnsupportedError) Error() string { return e.Message }

func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := urlParse(raw)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	var id string
	switch host {
	case "youtu.be":
		id = strings.Trim(strings.Split(u.Path, "/")[1], " ")
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		if strings.HasPrefix(u.Path, "/shorts/") {
			p := strings.Split(strings.TrimPrefix(u.Path, "/shorts/"), "/")
			id = p[0]
		} else if u.Path == "/watch" {
			id = u.Query().Get("v")
		}
	default:
		return "", errors.New("only youtube.com and youtu.be URLs are accepted")
	}
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid or unsupported YouTube video URL")
	}
	return "https://www.youtube.com/watch?v=" + id, nil
}

// urlParse is separated to keep URL policy obvious and testable.
func urlParse(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("URL must use HTTPS")
	}
	if u.User != nil {
		return nil, errors.New("URL credentials are not allowed")
	}
	return u, nil
}

func (e Extractor) Extract(ctx context.Context, rawURL, dir string) (Evidence, error) {
	canonical, err := Normalize(rawURL)
	if err != nil {
		return Evidence{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Evidence{}, err
	}
	seconds := strconv.Itoa(int(e.MaxDuration.Seconds()))
	megabytes := strconv.FormatInt(e.MaxBytes/(1024*1024), 10) + "M"
	output := filepath.Join(dir, "source.%(ext)s")
	args := []string{"--no-playlist", "--no-progress", "--socket-timeout", "20", "--retries", "2", "--max-filesize", megabytes, "--match-filter", "duration <= " + seconds + " & !is_live", "--write-info-json", "--js-runtimes", "deno", "-f", "bestvideo[height<=720][ext=mp4]/bestvideo[height<=720]/best[height<=720]/best", "-o", output, canonical}
	if out, err := exec.CommandContext(ctx, "yt-dlp", args...).CombinedOutput(); err != nil {
		return Evidence{}, fmt.Errorf("yt-dlp failed: %s", bounded(out))
	}
	// Subtitles are useful evidence but optional. Fetch them separately so a
	// translated-caption rate limit cannot discard a successfully downloaded video.
	subArgs := []string{"--no-playlist", "--no-progress", "--socket-timeout", "20", "--retries", "1", "--skip-download", "--write-subs", "--write-auto-subs", "--sub-langs", "en,uk,ru,en-orig,uk-orig,ru-orig", "--sub-format", "vtt", "--convert-subs", "vtt", "--js-runtimes", "deno", "-o", output, canonical}
	_, _ = exec.CommandContext(ctx, "yt-dlp", subArgs...).CombinedOutput()
	ev := Evidence{SourceURL: canonical}
	infos, _ := filepath.Glob(filepath.Join(dir, "source.info.json"))
	if len(infos) == 0 {
		return ev, errors.New("yt-dlp produced no metadata")
	}
	b, err := os.ReadFile(infos[0])
	if err != nil {
		return ev, err
	}
	var info struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Duration    float64 `json:"duration"`
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return ev, fmt.Errorf("parse metadata: %w", err)
	}
	ev.Title = info.Title
	ev.Description = clip(info.Description, 6000)
	ev.Duration = info.Duration
	if info.Duration > e.MaxDuration.Seconds()+1 {
		return ev, &UnsupportedError{Message: fmt.Sprintf("That video is %.0f seconds; the limit is %.0f seconds.", info.Duration, e.MaxDuration.Seconds())}
	}
	media, err := findMedia(dir)
	if err != nil {
		return ev, err
	}
	st, err := os.Stat(media)
	if err != nil {
		return ev, err
	}
	if st.Size() > e.MaxBytes {
		return ev, errors.New("download exceeded configured size limit")
	}
	ev.Transcript = readVTT(dir)
	framesDir := filepath.Join(dir, "frames")
	if err := os.Mkdir(framesDir, 0700); err != nil {
		return ev, err
	}
	framePattern := filepath.Join(framesDir, "frame-%03d.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", media, "-vf", "fps=1/15,scale=768:-2:force_original_aspect_ratio=decrease", "-frames:v", "12", "-q:v", "5", framePattern)
	if out, err := cmd.CombinedOutput(); err != nil {
		return ev, fmt.Errorf("frame extraction failed: %s", bounded(out))
	}
	ev.Frames, _ = filepath.Glob(filepath.Join(framesDir, "*.jpg"))
	sort.Strings(ev.Frames)
	if len(ev.Frames) == 0 {
		return ev, errors.New("video yielded no frames")
	}
	ev.OCR = ocr(ctx, ev.Frames, e.TesseractLangs)
	return ev, nil
}

func findMedia(dir string) (string, error) {
	items, err := filepath.Glob(filepath.Join(dir, "source.*"))
	if err != nil {
		return "", err
	}
	for _, p := range items {
		if strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".vtt") {
			continue
		}
		return p, nil
	}
	return "", errors.New("yt-dlp produced no media file")
}
func readVTT(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))
	seen := map[string]bool{}
	var lines []string
	tag := regexp.MustCompile(`<[^>]+>`)
	for _, f := range files {
		h, err := os.Open(f)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(h)
		for s.Scan() {
			v := strings.TrimSpace(tag.ReplaceAllString(s.Text(), ""))
			if v == "" || v == "WEBVTT" || strings.Contains(v, "-->") || regexp.MustCompile(`^\d+$`).MatchString(v) {
				continue
			}
			if !seen[v] {
				seen[v] = true
				lines = append(lines, v)
			}
		}
		h.Close()
	}
	return clip(strings.Join(lines, "\n"), 16000)
}
func ocr(ctx context.Context, frames []string, langs string) string {
	seen := map[string]bool{}
	var lines []string
	for _, f := range frames {
		out, err := exec.CommandContext(ctx, "tesseract", f, "stdout", "-l", langs, "--psm", "6").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if len(line) > 2 && !seen[line] {
				seen[line] = true
				lines = append(lines, line)
			}
		}
	}
	return clip(strings.Join(lines, "\n"), 12000)
}
func bounded(b []byte) string { return clip(strings.TrimSpace(string(b)), 1000) }
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
