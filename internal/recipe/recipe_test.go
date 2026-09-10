package recipe

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderEscapesHTML(t *testing.T) {
	r := Recipe{Title: "Fish <chips>", Ingredients: []Ingredient{{Item: "A&B", Confidence: "high"}}, Steps: []string{"Cook"}, Confidence: "medium"}
	out := RenderHTML(r)
	if strings.Contains(out, "<chips>") || !strings.Contains(out, "A&amp;B") {
		t.Fatalf("unsafe render: %s", out)
	}
}
func TestRenderBoundsCompleteUTF8Lines(t *testing.T) {
	many := strings.Repeat("🥘 & ingredient ", 600)
	r := Recipe{Title: many, Summary: many, Ingredients: []Ingredient{{Item: many, Notes: many, Confidence: "low"}}, Steps: []string{many}, Assumptions: []string{many}, Confidence: "low"}
	out := RenderHTML(r)
	if len(out) > 4000 || !utf8.ValidString(out) || !strings.HasSuffix(out, "</i>") {
		t.Fatalf("unsafe bounded output: bytes=%d valid=%v suffix=%q", len(out), utf8.ValidString(out), out[len(out)-20:])
	}
}
