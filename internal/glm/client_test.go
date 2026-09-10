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
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"Toast\",\"summary\":\"\",\"servings\":\"1\",\"time\":\"\",\"ingredients\":[{\"quantity\":\"1\",\"item\":\"bread\",\"confidence\":\"high\"}],\"steps\":[\"Toast it\"],\"assumptions\":[],\"warnings\":[],\"confidence\":\"high\"}"}}]}`)
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
}
func TestExtractJSON(t *testing.T) {
	if got := extractJSON("```json\n{\"a\":1}\n```"); !strings.HasPrefix(got, "{") {
		t.Fatal(got)
	}
}
