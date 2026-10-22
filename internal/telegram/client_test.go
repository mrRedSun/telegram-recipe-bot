package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEditUsesNativeRichMessage(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/editMessageText" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer server.Close()

	client := New(server.URL, "token")
	if err := client.Edit(context.Background(), 7, 9, "<table><tr><td>Salt</td></tr></table>"); err != nil {
		t.Fatal(err)
	}
	rich, ok := payload["rich_message"].(map[string]any)
	if !ok {
		t.Fatalf("rich_message missing: %#v", payload)
	}
	if _, legacyTextPresent := payload["text"]; legacyTextPresent {
		t.Fatalf("legacy text field present: %#v", payload)
	}
	if rich["html"] != "<table><tr><td>Salt</td></tr></table>" || rich["skip_entity_detection"] != true {
		t.Fatalf("unexpected rich message: %#v", rich)
	}
}

func TestSendIncludesForumThread(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer server.Close()
	if _, err := New(server.URL, "token").Send(context.Background(), 7, 42, "hello"); err != nil {
		t.Fatal(err)
	}
	if payload["message_thread_id"] != float64(42) {
		t.Fatalf("thread id omitted: %#v", payload)
	}
}

func TestSendMediaUsesOrderedAlbumAttachments(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "001.jpg"), filepath.Join(dir, "002.mp4")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("media"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mediaJSON string
	var filenames []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/sendMediaGroup" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(part)
			if part.FormName() == "media" {
				mediaJSON = string(data)
			}
			if part.FileName() != "" {
				filenames = append(filenames, part.FileName())
			}
		}
		fmt.Fprint(w, `{"ok":true,"result":[]}`)
	}))
	defer server.Close()
	client := New(server.URL, "token")
	if err := client.SendMedia(context.Background(), 7, 9, []Media{{Type: "photo", Path: paths[0]}, {Type: "video", Path: paths[1]}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mediaJSON, `"attach://media0"`) || !strings.Contains(mediaJSON, `"attach://media1"`) {
		t.Fatalf("unexpected media JSON: %s", mediaJSON)
	}
	if strings.Join(filenames, ",") != "001.jpg,002.mp4" {
		t.Fatalf("attachment order = %#v", filenames)
	}
}

func TestTransportErrorsNeverExposeBotToken(t *testing.T) {
	const token = "super-secret-token"
	client := New("http://127.0.0.1:1", token)
	client.http.Timeout = 100 * time.Millisecond
	_, err := client.Me(context.Background())
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "/bot") {
		t.Fatalf("transport error exposed request URL: %v", err)
	}
}

func TestRequestConstructionErrorsNeverExposeBotToken(t *testing.T) {
	const token = "secret\ninvalid"
	_, err := New("https://api.telegram.org", token).Me(context.Background())
	if err == nil {
		t.Fatal("expected request construction error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "/bot") {
		t.Fatalf("request error exposed bot token: %v", err)
	}
}

func TestProviderErrorsDoNotEchoSensitiveResponseBodies(t *testing.T) {
	for _, status := range []int{200, 400} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"ok":false,"description":"secret-token private message https://private.example"}`)
			}))
			defer server.Close()
			_, err := New(server.URL, "token").Send(context.Background(), 7, 0, "hello")
			if err == nil {
				t.Fatal("expected provider rejection")
			}
			for _, value := range []string{"secret-token", "private message", "private.example"} {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("provider error leaked response body: %v", err)
				}
			}
		})
	}
}
