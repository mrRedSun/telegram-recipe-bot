package instagram

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	photoLimit = int64(10 * 1024 * 1024)
	videoLimit = int64(50 * 1024 * 1024)
)

var (
	shortcodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{5,64}$`)
	urlPattern       = regexp.MustCompile(`https?://[^\s<>]+`)
)

type Kind string

const (
	Photo Kind = "photo"
	Video Kind = "video"
)

type Media struct {
	Path string
	Kind Kind
	Size int64
}

type UnsupportedError struct{ Message string }

func (e *UnsupportedError) Error() string { return e.Message }

type Resolver func(context.Context, string) ([]string, error)

type Downloader struct {
	MaxBytes int64
	MaxItems int
	Resolve  Resolver
	HTTP     *http.Client
	AllowURL func(*url.URL) bool
}

func Normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return "", errors.New("invalid Instagram URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("Instagram URL must use HTTPS")
	}
	if u.User != nil {
		return "", errors.New("URL credentials are not allowed")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != "instagram.com" && host != "www.instagram.com" {
		return "", errors.New("only instagram.com post and Reel URLs are accepted")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || (parts[0] != "p" && parts[0] != "reel" && parts[0] != "reels") || !shortcodePattern.MatchString(parts[1]) {
		return "", errors.New("only direct public Instagram post and Reel URLs are accepted")
	}
	kind := parts[0]
	if kind == "reels" {
		kind = "reel"
	}
	return "https://www.instagram.com/" + kind + "/" + parts[1] + "/", nil
}

func Find(text string) (string, bool) {
	for _, candidate := range urlPattern.FindAllString(text, -1) {
		candidate = strings.TrimRight(candidate, ".,;:!?)]}\"'")
		if normalized, err := Normalize(candidate); err == nil {
			return normalized, true
		}
	}
	return "", false
}

func (d Downloader) Download(ctx context.Context, rawURL, dir string) ([]Media, error) {
	canonical, err := Normalize(rawURL)
	if err != nil {
		return nil, err
	}
	if d.MaxItems < 1 {
		d.MaxItems = 20
	}
	resolve := d.Resolve
	if resolve == nil {
		resolve = func(ctx context.Context, target string) ([]string, error) {
			return resolveWithGalleryDL(ctx, target, d.MaxItems)
		}
	}
	mediaURLs, err := resolve(ctx, canonical)
	if err != nil {
		return nil, err
	}
	if len(mediaURLs) == 0 {
		return nil, &UnsupportedError{Message: "Instagram returned no downloadable public media. The post may require login or be rate-limited."}
	}
	if len(mediaURLs) > d.MaxItems {
		return nil, &UnsupportedError{Message: fmt.Sprintf("That Instagram post has more than the supported %d media items.", d.MaxItems)}
	}
	mediaDir := filepath.Join(dir, "instagram")
	if err := os.MkdirAll(mediaDir, 0700); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 90 * time.Second}
	if d.HTTP != nil {
		copy := *d.HTTP
		client = &copy
	}
	redirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !d.allowed(req.URL) {
			return errors.New("Instagram media redirect was rejected")
		}
		if redirect != nil {
			return redirect(req, via)
		}
		return nil
	}
	var total int64
	items := make([]Media, 0, len(mediaURLs))
	for index, mediaURL := range mediaURLs {
		u, err := url.Parse(strings.TrimSpace(mediaURL))
		if err != nil || !d.allowed(u) {
			return nil, errors.New("Instagram extractor returned an unexpected media host")
		}
		item, err := d.downloadOne(ctx, client, u.String(), mediaDir, index+1, d.MaxBytes-total)
		if err != nil {
			return nil, err
		}
		total += item.Size
		if total > d.MaxBytes {
			return nil, &UnsupportedError{Message: "That Instagram post exceeds the configured temporary download limit."}
		}
		items = append(items, item)
	}
	return items, nil
}

func (d Downloader) downloadOne(ctx context.Context, client *http.Client, mediaURL, dir string, index int, remaining int64) (Media, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return Media{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TelegramRecipeBot/1.0)")
	res, err := client.Do(req)
	if err != nil {
		return Media{}, &UnsupportedError{Message: "Instagram media could not be downloaded. The public link may be expired or rate-limited."}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Media{}, &UnsupportedError{Message: "Instagram media could not be downloaded. The public link may require login or be rate-limited."}
	}
	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(res.Body, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return Media{}, errors.New("read Instagram media header")
	}
	prefix = prefix[:n]
	contentType := strings.Split(http.DetectContentType(prefix), ";")[0]
	var kind Kind
	var extension string
	var perFileLimit int64
	switch contentType {
	case "image/jpeg":
		kind, extension, perFileLimit = Photo, ".jpg", photoLimit
	case "image/png":
		kind, extension, perFileLimit = Photo, ".png", photoLimit
	case "video/mp4":
		kind, extension, perFileLimit = Video, ".mp4", videoLimit
	default:
		return Media{}, &UnsupportedError{Message: "Instagram returned a media format that Telegram cannot upload as a photo or video."}
	}
	if remaining < perFileLimit {
		perFileLimit = remaining
	}
	if perFileLimit <= 0 {
		return Media{}, &UnsupportedError{Message: "That Instagram post exceeds the configured temporary download limit."}
	}
	path := filepath.Join(dir, fmt.Sprintf("%03d%s", index, extension))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Media{}, err
	}
	written, copyErr := io.Copy(file, io.MultiReader(bytes.NewReader(prefix), io.LimitReader(res.Body, perFileLimit+1-int64(len(prefix)))))
	closeErr := file.Close()
	if copyErr != nil {
		return Media{}, errors.New("download Instagram media")
	}
	if closeErr != nil {
		return Media{}, closeErr
	}
	if written > perFileLimit {
		label := "photo"
		if kind == Video {
			label = "video"
		}
		return Media{}, &UnsupportedError{Message: "An Instagram " + label + " exceeds Telegram's hosted Bot API upload limit."}
	}
	return Media{Path: path, Kind: kind, Size: written}, nil
}

func (d Downloader) allowed(u *url.URL) bool {
	if d.AllowURL != nil {
		return d.AllowURL(u)
	}
	if u == nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return host == "cdninstagram.com" || strings.HasSuffix(host, ".cdninstagram.com") || host == "fbcdn.net" || strings.HasSuffix(host, ".fbcdn.net")
}

func resolveWithGalleryDL(ctx context.Context, target string, maxItems int) ([]string, error) {
	args := []string{"--ignore-config", "-o", "extractor.instagram.videos=merged", "--http-timeout", "20", "--retries", "2", "--range", "1-" + strconv.Itoa(maxItems+1), "-g", target}
	cmd := exec.CommandContext(ctx, "gallery-dl", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &limitedWriter{Writer: &stdout, Remaining: 1 << 20}
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, &UnsupportedError{Message: "Instagram could not expose this post anonymously. It may be private, login-gated, removed, or rate-limited."}
	}
	var urls []string
	scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	scanner.Buffer(make([]byte, 4096), 128*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			urls = append(urls, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("parse Instagram extractor output")
	}
	if len(urls) > maxItems {
		return nil, &UnsupportedError{Message: fmt.Sprintf("That Instagram post has more than the supported %d media items.", maxItems)}
	}
	return urls, nil
}

type limitedWriter struct {
	Writer    io.Writer
	Remaining int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.Remaining {
		return 0, errors.New("extractor output exceeded limit")
	}
	n, err := w.Writer.Write(p)
	w.Remaining -= int64(n)
	return n, err
}
