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
	"strconv"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)

type Evidence struct {
	SourceURL      string
	Title          string
	Description    string
	AuthorComments string
	Duration       float64
	Transcript     string
	OCR            string
	Frames         []string
}
type Extractor struct {
	MaxDuration    time.Duration
	MaxBytes       int64
	TesseractLangs string
	NormalizeURL   func(string) (string, error)
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
		id = strings.Split(strings.Trim(u.Path, "/"), "/")[0]
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
	return e.ExtractWithProgress(ctx, rawURL, dir, nil)
}

// ExtractWithProgress reports completed, non-sensitive extraction stages. The
// callback never receives source text, URLs, filenames, or credentials.
func (e Extractor) ExtractWithProgress(ctx context.Context, rawURL, dir string, progress func(string)) (Evidence, error) {
	return e.ExtractWithDiagnostics(ctx, rawURL, dir, progress, nil)
}

// Both supported platforms share this metadata, download, frame/OCR engine.
func (e Extractor) ExtractWithDiagnostics(ctx context.Context, rawURL, dir string, progress func(string), report func(Diagnostic)) (result Evidence, resultErr error) {
	started := time.Now()
	stage, backend, platform := "url_validation", "yt-dlp", "youtube"
	emit := func(event Diagnostic) {
		if report != nil {
			event.Source = platform
			event.ElapsedMS = time.Since(started).Milliseconds()
			report(event)
		}
	}
	defer func() {
		if resultErr != nil {
			event := diagnose(resultErr)
			event.Outcome = "failed"
			if event.Stage == "" {
				event.Stage = stage
			}
			if event.Backend == "" {
				event.Backend = backend
			}
			emit(event)
		}
	}()
	normalize := e.NormalizeURL
	if normalize == nil {
		normalize = Normalize
	}
	canonical, err := normalize(rawURL)
	if err != nil {
		return result, err
	}
	if strings.HasPrefix(canonical, "https://www.instagram.com/") {
		platform = "instagram"
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return result, err
	}
	stage = "metadata"
	emit(Diagnostic{Stage: stage, Backend: backend, Outcome: "started"})
	metadataArgs := []string{"--ignore-config", "--no-cache-dir", "--no-warnings", "--no-playlist", "--playlist-end", "2", "--socket-timeout", "20", "--retries", "2", "--extractor-retries", "1", "--dump-single-json", "--skip-download", "--js-runtimes", "deno", "--", canonical}
	raw, err := runBounded(ctx, stage, backend, metadataArgs, 2<<20)
	if err != nil {
		return result, err
	}
	selected, err := singleVideoMetadata(raw)
	if err != nil {
		return result, err
	}
	var info struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Duration    float64 `json:"duration"`
		IsLive      bool    `json:"is_live"`
	}
	if json.Unmarshal(selected, &info) != nil {
		return result, errors.New("invalid source metadata")
	}
	if info.IsLive {
		return result, &UnsupportedError{Message: "Live videos are not supported."}
	}
	if info.Duration > e.MaxDuration.Seconds()+1 {
		return result, &UnsupportedError{Message: fmt.Sprintf("That video is %.0f seconds; the limit is %.0f seconds.", info.Duration, e.MaxDuration.Seconds())}
	}
	result = Evidence{SourceURL: canonical, Title: clip(info.Title, 500), Description: clip(info.Description, 6000), Duration: info.Duration}
	snapshot := filepath.Join(dir, "source.info.json")
	if err = os.WriteFile(snapshot, selected, 0600); err != nil {
		return result, err
	}
	emit(Diagnostic{Stage: stage, Backend: backend, Outcome: "succeeded", Bytes: int64(len(selected))})
	stage = "download"
	emit(Diagnostic{Stage: stage, Backend: backend, Outcome: "started"})
	output := filepath.Join(dir, "source.%(ext)s")
	megabytes := strconv.FormatInt(e.MaxBytes/(1024*1024), 10) + "M"
	args := []string{"--ignore-config", "--no-cache-dir", "--no-warnings", "--no-playlist", "--no-progress", "--socket-timeout", "20", "--retries", "2", "--max-filesize", megabytes, "--match-filter", "!is_live", "--js-runtimes", "deno", "-f", "bestvideo[height<=720][ext=mp4]/bestvideo[height<=720]/best[height<=720]/best", "-o", output, "--load-info-json", snapshot}
	if _, err = runBounded(ctx, stage, backend, args, 64<<10); err != nil {
		return result, err
	}
	media, err := findMedia(dir)
	if err != nil {
		return result, err
	}
	stat, err := os.Stat(media)
	if err != nil {
		return result, err
	}
	if stat.Size() > e.MaxBytes {
		return result, &UnsupportedError{Message: "That video exceeds the configured temporary download limit."}
	}
	emit(Diagnostic{Stage: stage, Backend: backend, Outcome: "succeeded", Bytes: stat.Size()})
	reportProgress(progress, "download")
	stage = "captions"
	subArgs := []string{"--ignore-config", "--no-cache-dir", "--no-warnings", "--no-playlist", "--no-progress", "--socket-timeout", "20", "--retries", "1", "--skip-download", "--write-subs", "--write-auto-subs", "--sub-langs", "en,uk,ru,en-orig,uk-orig,ru-orig", "--sub-format", "vtt", "--convert-subs", "vtt", "--js-runtimes", "deno", "-o", output, "--load-info-json", snapshot}
	if _, optionalErr := runBounded(ctx, stage, backend, subArgs, 64<<10); optionalErr != nil {
		event := diagnose(optionalErr)
		event.Outcome = "unavailable"
		emit(event)
	}
	result.Transcript = readVTT(dir)
	reportProgress(progress, "captions")
	stage = "comments"
	if platform == "youtube" {
		commentArgs := []string{"--ignore-config", "--no-cache-dir", "--no-warnings", "--no-playlist", "--no-progress", "--socket-timeout", "20", "--retries", "1", "--skip-download", "--write-info-json", "--write-comments", "--extractor-args", "youtube:comment_sort=top;max_comments=50,40,10,2,2", "--js-runtimes", "deno", "-o", filepath.Join(dir, "comments.%(ext)s"), canonical}
		commentCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		_, optionalErr := runBounded(commentCtx, stage, backend, commentArgs, 64<<10)
		cancel()
		if optionalErr != nil {
			event := diagnose(optionalErr)
			event.Outcome = "unavailable"
			emit(event)
		}
		result.AuthorComments, _ = readAuthorComments(filepath.Join(dir, "comments.info.json"))
	}
	reportProgress(progress, "comments")
	stage, backend = "media_validation", "ffprobe"
	probe, err := runBounded(ctx, stage, backend, []string{"-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "format=duration", "-of", "json", media}, 16<<10)
	if err != nil {
		return result, err
	}
	result.Duration, err = mediaDuration(probe, e.MaxDuration)
	if err != nil {
		return result, err
	}
	stage, backend = "frames", "ffmpeg"
	frames, err := extractFrames(ctx, media, dir, result.Duration)
	if err != nil {
		return result, err
	}
	result.Frames = frames
	emit(Diagnostic{Stage: stage, Backend: backend, Outcome: "succeeded", Items: len(frames)})
	reportProgress(progress, "frames")
	stage, backend = "ocr", "tesseract"
	result.OCR = ocr(ctx, frames, e.TesseractLangs)
	emit(Diagnostic{Stage: stage, Backend: backend, Outcome: "succeeded", Bytes: int64(len(result.OCR))})
	reportProgress(progress, "ocr")
	return result, nil
}

func reportProgress(progress func(string), stage string) {
	if progress != nil {
		progress(stage)
	}
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
func readAuthorComments(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var info struct {
		Comments []struct {
			Text             string `json:"text"`
			AuthorIsUploader bool   `json:"author_is_uploader"`
			IsPinned         bool   `json:"is_pinned"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return "", err
	}
	var pinned, other []string
	seen := map[string]bool{}
	for _, comment := range info.Comments {
		if !comment.AuthorIsUploader {
			continue
		}
		text := strings.TrimSpace(comment.Text)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		if comment.IsPinned {
			pinned = append(pinned, text)
		} else {
			other = append(other, text)
		}
	}
	comments := append(pinned, other...)
	if len(comments) > 5 {
		comments = comments[:5]
	}
	var sections []string
	for i, text := range comments {
		label := "Author comment"
		if i < len(pinned) {
			label = "Pinned author comment"
		}
		sections = append(sections, "["+label+"]\n"+text)
	}
	return clip(strings.Join(sections, "\n\n"), 6000), nil
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
