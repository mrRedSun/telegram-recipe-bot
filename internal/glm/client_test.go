package glm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

func TestInfer(t *testing.T) {
	var auth, requestBody string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"Toast\",\"summary\":\"\",\"yield\":\"1 serving\",\"times\":{\"prep\":\"\",\"cook\":\"\",\"total\":\"\"},\"ingredients\":[{\"amount\":\"1\",\"unit\":\"slice\",\"item\":\"bread\",\"confidence\":\"high\"}],\"equipment\":[],\"steps\":[{\"instruction\":\"Toast it\",\"confidence\":\"high\"}],\"assumptions\":[],\"warnings\":[],\"confidence\":\"high\"}"}}]}`)
	}))
	defer s.Close()
	c := New(s.URL, "secret", "glm-5.3", "low", false)
	got, err := c.Infer(context.Background(), youtube.Evidence{Title: "Toast", AuthorComments: "[Pinned author comment]\nUse sourdough"})
	if err != nil || got.Title != "Toast" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if auth != "Bearer secret" {
		t.Fatalf("bad auth %q", auth)
	}
	if !strings.Contains(requestBody, "UPLOADER-AUTHORED COMMENTS") || !strings.Contains(requestBody, "Use sourdough") {
		t.Fatalf("request omitted author comment evidence: %s", requestBody)
	}
	if !strings.Contains(requestBody, `"max_tokens":16000`) {
		t.Fatalf("request omitted bounded GLM-5.3 output budget: %s", requestBody)
	}
}

func TestInferReportsSafeTruncationDiagnostics(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":","reasoning_content":"private reasoning"},"finish_reason":"length"}],"usage":{"prompt_tokens":321,"completion_tokens":16000}}`)
	}))
	defer s.Close()

	_, err := New(s.URL, "secret", "glm-5.3-flash", "high", false).Infer(context.Background(), youtube.Evidence{Title: "Sensitive title"})
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	got := err.Error()
	for _, want := range []string{`finish_reason="length"`, "content_bytes=9", "reasoning_bytes=17", "prompt_tokens=321", "completion_tokens=16000"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error %q omitted %q", got, want)
		}
	}
	for _, secret := range []string{"private reasoning", "Sensitive title", "secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("error leaked %q: %s", secret, got)
		}
	}
}
func TestExtractJSON(t *testing.T) {
	if got := extractJSON("```json\n{\"a\":1}\n```"); !strings.HasPrefix(got, "{") {
		t.Fatal(got)
	}
}

func TestSystemPromptDefinesStructuredRecipe(t *testing.T) {
	for _, field := range []string{`"yield"`, `"times"`, `"amount"`, `"unit"`, `"equipment"`, `"duration"`, `"temperature"`} {
		if !strings.Contains(systemPrompt, field) {
			t.Fatalf("system prompt omitted %s", field)
		}
	}
}
