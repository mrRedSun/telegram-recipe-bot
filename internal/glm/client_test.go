package glm

import (
	"context"
	"fmt"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInfer(t *testing.T) {
	var auth string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"Toast\",\"summary\":\"\",\"servings\":\"1\",\"time\":\"\",\"ingredients\":[{\"quantity\":\"1\",\"item\":\"bread\",\"confidence\":\"high\"}],\"steps\":[\"Toast it\"],\"assumptions\":[],\"warnings\":[],\"confidence\":\"high\"}"}}]}`)
	}))
	defer s.Close()
	c := New(s.URL, "secret", "glm-5.3", "low", false)
	got, err := c.Infer(context.Background(), youtube.Evidence{Title: "Toast"})
	if err != nil || got.Title != "Toast" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if auth != "Bearer secret" {
		t.Fatalf("bad auth %q", auth)
	}
}
func TestExtractJSON(t *testing.T) {
	if got := extractJSON("```json\n{\"a\":1}\n```"); !strings.HasPrefix(got, "{") {
		t.Fatal(got)
	}
}
