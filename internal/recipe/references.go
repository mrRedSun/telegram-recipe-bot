package recipe

import (
	"errors"
	"html"
	"net/url"
	"strings"
)

// Reference metadata is attached by the application, never accepted from model output.
type Reference struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Dataset string `json:"dataset"`
	License string `json:"license"`
}

func validReference(r Reference) bool {
	u, err := url.Parse(r.URL)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && len(r.URL) <= 600 && len(r.Title) <= 240 && len(r.Dataset) <= 200 && len(r.License) <= 100 && r.Title != "" && (r.ID == "R1" || r.ID == "R2" || r.ID == "R3")
}

func (r *Recipe) BindReferences(available []Reference) error {
	if len(available) > 3 {
		return errors.New("recipe reference limit exceeded")
	}
	known := map[string]Reference{}
	for _, ref := range available {
		if !validReference(ref) {
			return errors.New("invalid recipe reference")
		}
		if _, duplicate := known[ref.ID]; duplicate {
			return errors.New("duplicate recipe reference")
		}
		known[ref.ID] = ref
	}
	used := map[string]bool{}
	check := func(ids []string) error {
		if len(ids) > 3 {
			return errors.New("recipe reference limit exceeded")
		}
		for _, id := range ids {
			if _, ok := known[id]; !ok {
				return errors.New("unknown recipe reference")
			}
			used[id] = true
		}
		return nil
	}
	for i := range r.Ingredients {
		if err := check(r.Ingredients[i].ReferenceIDs); err != nil {
			return err
		}
		if len(r.Ingredients[i].ReferenceIDs) > 0 {
			r.Ingredients[i].Confidence = "low"
		}
	}
	for i := range r.Steps {
		if err := check(r.Steps[i].ReferenceIDs); err != nil {
			return err
		}
		if len(r.Steps[i].ReferenceIDs) > 0 {
			r.Steps[i].Confidence = "low"
		}
	}
	r.References = nil
	for _, ref := range available {
		if used[ref.ID] {
			r.References = append(r.References, ref)
		}
	}
	if len(r.References) > 0 && r.Confidence == "high" {
		r.Confidence = "medium"
	}
	return nil
}
func (r Recipe) validateReferences() error {
	if len(r.References) > 0 && r.Confidence == "high" {
		return errors.New("reference estimates cannot have high confidence")
	}
	for _, item := range r.Ingredients {
		if len(item.ReferenceIDs) > 0 && item.Confidence != "low" {
			return errors.New("reference ingredient must have low confidence")
		}
	}
	for _, step := range r.Steps {
		if len(step.ReferenceIDs) > 0 && step.Confidence != "low" {
			return errors.New("reference step must have low confidence")
		}
	}
	copy := r
	copy.Ingredients = append([]Ingredient(nil), r.Ingredients...)
	copy.Steps = append([]Step(nil), r.Steps...)
	return copy.BindReferences(r.References)
}
func referencesHTML(refs []Reference) string {
	if len(refs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<hr/><h3>📚 Recipe references</h3><p><i>Reference estimates are marked above and need your judgement when cooking.</i></p><ul>")
	for _, ref := range refs {
		if !validReference(ref) {
			continue
		}
		title := []rune(ref.Title)
		if len(title) > 100 {
			title = title[:100]
		}
		b.WriteString("<li>[" + strings.TrimPrefix(ref.ID, "R") + "] <a href=\"" + html.EscapeString(ref.URL) + "\">" + html.EscapeString(string(title)) + "</a> · " + html.EscapeString(ref.Dataset) + " · " + html.EscapeString(ref.License) + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

func referenceEstimateLabel(ids []string) string {
	var numbers []string
	for _, id := range ids {
		if id == "R1" || id == "R2" || id == "R3" {
			numbers = append(numbers, strings.TrimPrefix(id, "R"))
		}
	}
	return "reference estimate [" + strings.Join(numbers, ", ") + "]"
}
