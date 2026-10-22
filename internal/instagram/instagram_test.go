package instagram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestNormalizeAndFind(t *testing.T) {
	got, err := Normalize("https://instagram.com/reels/Abc_123/?utm_source=test")
	if err != nil || got != "https://www.instagram.com/reel/Abc_123/" {
		t.Fatalf("Normalize() = %q, %v", got, err)
	}
	if found, ok := Find("please fetch https://www.instagram.com/p/Abc-123/ thanks"); !ok || found != "https://www.instagram.com/p/Abc-123/" {
		t.Fatalf("Find() = %q, %v", found, ok)
	}
}

func TestNormalizeRejectsProfilesAndLookalikes(t *testing.T) {
	for _, raw := range []string{
		"https://instagram.com/example/",
		"https://instagram.example/p/Abc123/",
		"https://user:pass@instagram.com/p/Abc123/",
		"file:///tmp/media",
	} {
		if _, err := Normalize(raw); err == nil {
			t.Fatalf("Normalize(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestDownloadPreservesCarouselOrder(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, make([]byte, 600)...)
	mp4 := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2'}, make([]byte, 600)...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/one":
			_, _ = w.Write(jpeg)
		case "/two":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(mp4)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	resolve := func(context.Context, string) ([]string, error) {
		return []string{server.URL + "/one", server.URL + "/two"}, nil
	}
	d := Downloader{MaxBytes: 100 << 20, MaxItems: 20, Resolve: resolve, HTTP: server.Client(), AllowURL: func(u *url.URL) bool { return u.Host == strings.TrimPrefix(server.URL, "http://") }}
	items, err := d.Download(context.Background(), "https://instagram.com/p/Abc123/", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Kind != Photo || items[1].Kind != Video {
		t.Fatalf("items = %#v", items)
	}
	for index, item := range items {
		if _, err := os.Stat(item.Path); err != nil {
			t.Fatalf("item %d missing: %v", index, err)
		}
		if !strings.Contains(item.Path, fmt.Sprintf("%03d", index+1)) {
			t.Fatalf("item order not reflected in path: %s", item.Path)
		}
	}
}

func TestDownloadRejectsTooManyItems(t *testing.T) {
	resolve := func(context.Context, string) ([]string, error) {
		return []string{"https://x.cdninstagram.com/1", "https://x.cdninstagram.com/2"}, nil
	}
	d := Downloader{MaxBytes: 100 << 20, MaxItems: 1, Resolve: resolve}
	_, err := d.Download(context.Background(), "https://instagram.com/p/Abc123/", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadRejectsUnexpectedHosts(t *testing.T) {
	for _, target := range []string{"http://x.cdninstagram.com/a", "https://cdninstagram.com.evil.example/a", "https://127.0.0.1/a", "https://user:pass@x.fbcdn.net/a"} {
		d := Downloader{MaxBytes: 100 << 20, Resolve: func(context.Context, string) ([]string, error) { return []string{target}, nil }}
		if _, err := d.Download(context.Background(), "https://instagram.com/p/Abc123/", t.TempDir()); err == nil {
			t.Fatalf("accepted unexpected media target: %s", target)
		}
	}
}

func TestDownloadEnforcesCombinedSizeLimit(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, make([]byte, 600)...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(jpeg) }))
	defer server.Close()
	d := Downloader{MaxBytes: 1000, Resolve: func(context.Context, string) ([]string, error) { return []string{server.URL, server.URL}, nil }, HTTP: server.Client(), AllowURL: func(u *url.URL) bool { return u.Host == strings.TrimPrefix(server.URL, "http://") }}
	if _, err := d.Download(context.Background(), "https://instagram.com/p/Abc123/", t.TempDir()); err == nil {
		t.Fatal("download exceeded the total byte budget")
	}
}

func TestDownloadRejectsRedirectToUnexpectedHost(t *testing.T) {
	contacted := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer source.Close()
	d := Downloader{MaxBytes: 100 << 20, Resolve: func(context.Context, string) ([]string, error) { return []string{source.URL}, nil }, HTTP: source.Client(), AllowURL: func(u *url.URL) bool { return u.Host == strings.TrimPrefix(source.URL, "http://") }}
	if _, err := d.Download(context.Background(), "https://instagram.com/p/Abc123/", t.TempDir()); err == nil {
		t.Fatal("accepted redirect to unexpected host")
	}
	if contacted {
		t.Fatal("contacted rejected redirect host")
	}
}

func TestResolverRequestsDirectVideosWithoutCredentials(t *testing.T) {
	dir := t.TempDir()
	// Stand in for the extractor to verify the subprocess contract without network access.
	script := "#!/bin/sh\ncase \"$*\" in *'--ignore-config -o extractor.instagram.videos=merged'*'--range 1-21 -g https://www.instagram.com/p/Abc123/'*) ;; *) exit 1;; esac\nprintf '%s\\n' 'https://x.cdninstagram.com/first.jpg' 'https://x.cdninstagram.com/second.mp4'\n"
	if err := os.WriteFile(dir+"/gallery-dl", []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	urls, err := resolveWithGalleryDL(context.Background(), "https://www.instagram.com/p/Abc123/", 20)
	if err != nil || len(urls) != 2 || !strings.HasSuffix(urls[1], "second.mp4") {
		t.Fatalf("resolver output=%v error=%v", urls, err)
	}
}
