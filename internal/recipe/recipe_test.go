package recipe

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func sampleRecipe() Recipe {
	return Recipe{
		Title:   "Fish <chips>",
		Summary: "Crisp & quick",
		Yield:   "2 servings",
		Times:   Times{Prep: "10 min", Cook: "20 min", Total: "30 min"},
		Ingredients: []Ingredient{
			{Amount: "2", Unit: "fillets", Item: "Fish & lemon", Preparation: "patted dry", Confidence: "high"},
		},
		Equipment:   []string{"Sheet pan"},
		Steps:       []Step{{Instruction: "Cook <carefully>", Duration: "20 min", Temperature: "200°C", Confidence: "medium"}},
		Assumptions: []string{"The exact fish species is not shown."},
		Warnings:    []string{"Cook fish to safe doneness."},
		Confidence:  "medium",
	}
}

func TestRenderUsesNativeTelegramRichBlocks(t *testing.T) {
	out := RenderRichHTML(sampleRecipe())
	for _, want := range []string{"<h2>", "<table bordered striped compact>", "<caption>🧺 Ingredients</caption>", "<th>Amount</th>", "<blockquote>", "<details>", "<ol>", "<i>Medium</i>", "⏱", "🌡"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render omitted %q: %s", want, out)
		}
	}
	if strings.Contains(out, "<chips>") || strings.Contains(out, "<carefully>") || !strings.Contains(out, "Fish &amp; lemon") {
		t.Fatalf("unsafe render: %s", out)
	}
}

func TestRenderBoundsCompleteUTF8Fragments(t *testing.T) {
	many := strings.Repeat("🥘 & ingredient ", 600)
	r := sampleRecipe()
	r.Title = many
	r.Summary = many
	r.Ingredients[0] = Ingredient{Amount: many, Item: many, Preparation: many, Confidence: "low"}
	r.Steps = []Step{{Instruction: many, Confidence: "low"}}
	r.Assumptions = []string{many}
	out := RenderRichHTML(r)
	if len(out) > 32768 || !utf8.ValidString(out) || !strings.HasSuffix(out, "</footer>") {
		t.Fatalf("unsafe bounded output: bytes=%d valid=%v suffix=%q", len(out), utf8.ValidString(out), out[len(out)-20:])
	}
}

func TestValidateStructuredSteps(t *testing.T) {
	r := sampleRecipe()
	r.Steps[0].Confidence = "maybe"
	if err := r.Validate(); err == nil {
		t.Fatal("accepted invalid step confidence")
	}
}

func TestLowConfidenceUsesRichHighlighting(t *testing.T) {
	r := sampleRecipe()
	r.Ingredients[0].Confidence = "low"
	if out := RenderRichHTML(r); !strings.Contains(out, "<mark>Low</mark>") {
		t.Fatalf("low confidence was not highlighted: %s", out)
	}
}
