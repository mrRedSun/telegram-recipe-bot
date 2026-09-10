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

func TestRenderUsesTelegramRichHTMLAndTables(t *testing.T) {
	out := RenderHTML(sampleRecipe())
	for _, want := range []string{"<pre>", "AMOUNT", "<u>Ingredients</u>", "<blockquote>", "<blockquote expandable>", "⏱", "🌡"} {
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
	out := RenderHTML(r)
	if len(out) > 4096 || !utf8.ValidString(out) || !strings.HasSuffix(out, "</i>") {
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
