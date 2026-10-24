package recipe

import (
	"strings"
	"testing"
)

func TestReferenceEstimatesAreCitedAndCannotBecomeHighConfidence(t *testing.T) {
	r := Recipe{Title: "Soup", Confidence: "high", Ingredients: []Ingredient{{Item: "chicken", Amount: "200", Unit: "g", Confidence: "high", ReferenceIDs: []string{"R1"}}}, Steps: []Step{{Instruction: "Simmer", Duration: "20 min", Confidence: "high", ReferenceIDs: []string{"R1"}}}}
	refs := []Reference{{ID: "R1", Title: "Chicken <soup>", URL: "https://example.com/recipe?a=1&b=2", Dataset: "Fixture", License: "CC0-1.0"}, {ID: "R2", Title: "Unused", URL: "https://example.com/unused"}}
	if err := r.BindReferences(refs); err != nil {
		t.Fatal(err)
	}
	if r.Ingredients[0].Confidence != "low" || r.Steps[0].Confidence != "low" || r.Confidence != "medium" || len(r.References) != 1 {
		t.Fatalf("wrong provenance binding: %#v", r)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	html := RenderRichHTML(r)
	for _, want := range []string{"reference estimate", "Chicken &lt;soup&gt;", "CC0-1.0", "a=1&amp;b=2"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(html, "Unused") {
		t.Fatal("unused reference rendered")
	}
}
func TestRejectUnknownReferenceOrUnsafeURL(t *testing.T) {
	r := Recipe{Ingredients: []Ingredient{{ReferenceIDs: []string{"R99"}}}}
	if r.BindReferences(nil) == nil {
		t.Fatal("accepted fabricated reference")
	}
	if (&Recipe{}).BindReferences([]Reference{{ID: "R1", Title: "Bad", URL: "javascript:alert(1)"}}) == nil {
		t.Fatal("accepted unsafe source URL")
	}
}
