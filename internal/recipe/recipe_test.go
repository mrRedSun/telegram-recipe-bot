package recipe

import (
	"strings"
	"testing"
)

func TestRenderEscapesHTML(t *testing.T) {
	r := Recipe{Title: "Fish <chips>", Ingredients: []Ingredient{{Item: "A&B", Confidence: "high"}}, Steps: []string{"Cook"}, Confidence: "medium"}
	out := RenderHTML(r)
	if strings.Contains(out, "<chips>") || !strings.Contains(out, "A&amp;B") {
		t.Fatalf("unsafe render: %s", out)
	}
}
