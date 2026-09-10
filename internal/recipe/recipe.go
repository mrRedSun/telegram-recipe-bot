package recipe

import (
	"errors"
	"fmt"
	"html"
	"strings"
)

type Ingredient struct {
	Quantity   string `json:"quantity"`
	Item       string `json:"item"`
	Notes      string `json:"notes,omitempty"`
	Confidence string `json:"confidence"`
}
type Recipe struct {
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	Servings    string       `json:"servings"`
	Time        string       `json:"time"`
	Ingredients []Ingredient `json:"ingredients"`
	Steps       []string     `json:"steps"`
	Assumptions []string     `json:"assumptions"`
	Warnings    []string     `json:"warnings"`
	Confidence  string       `json:"confidence"`
}

func (r Recipe) Validate() error {
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("recipe title is empty")
	}
	if len(r.Ingredients) == 0 {
		return errors.New("recipe has no ingredients")
	}
	if len(r.Steps) == 0 {
		return errors.New("recipe has no steps")
	}
	if !confidence(r.Confidence) {
		return fmt.Errorf("invalid confidence %q", r.Confidence)
	}
	for i, in := range r.Ingredients {
		if strings.TrimSpace(in.Item) == "" {
			return fmt.Errorf("ingredient %d has no item", i+1)
		}
		if !confidence(in.Confidence) {
			return fmt.Errorf("ingredient %d has invalid confidence", i+1)
		}
	}
	return nil
}
func confidence(v string) bool { return v == "high" || v == "medium" || v == "low" }

func RenderHTML(r Recipe) string {
	var b strings.Builder
	f := func(s string) string { return html.EscapeString(strings.TrimSpace(s)) }
	fmt.Fprintf(&b, "<b>%s</b>\n", f(r.Title))
	if r.Summary != "" {
		fmt.Fprintf(&b, "%s\n", f(r.Summary))
	}
	meta := []string{}
	if r.Servings != "" {
		meta = append(meta, "Servings: "+f(r.Servings))
	}
	if r.Time != "" {
		meta = append(meta, "Time: "+f(r.Time))
	}
	meta = append(meta, "Confidence: "+f(r.Confidence))
	b.WriteString(strings.Join(meta, " · ") + "\n\n<b>Ingredients</b>\n")
	for _, in := range r.Ingredients {
		line := "• "
		if in.Quantity != "" {
			line += f(in.Quantity) + " "
		}
		line += f(in.Item)
		if in.Notes != "" {
			line += " (" + f(in.Notes) + ")"
		}
		if in.Confidence == "low" {
			line += " ⚠️"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n<b>Method</b>\n")
	for i, step := range r.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, f(step))
	}
	if len(r.Assumptions) > 0 {
		b.WriteString("\n<b>Uncertain / inferred</b>\n")
		for _, v := range r.Assumptions {
			b.WriteString("• " + f(v) + "\n")
		}
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n<b>Safety</b>\n")
		for _, v := range r.Warnings {
			b.WriteString("• " + f(v) + "\n")
		}
	}
	out := b.String()
	if len(out) > 4000 {
		out = out[:3950] + "\n\n<i>Recipe shortened to fit Telegram.</i>"
	}
	return out
}
