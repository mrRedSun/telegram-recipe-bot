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
	truncated := false
	f := func(s string, n int) string {
		s = strings.TrimSpace(s)
		if len([]rune(s)) > n {
			truncated = true
		}
		return html.EscapeString(limit(s, n))
	}
	add := func(line string) bool {
		if b.Len()+len(line) > 3800 {
			truncated = true
			return false
		}
		b.WriteString(line)
		return true
	}
	finish := func() string {
		if truncated {
			b.WriteString("\n<i>Recipe shortened to fit Telegram.</i>")
		}
		return b.String()
	}
	add("<b>" + f(r.Title, 160) + "</b>\n")
	if r.Summary != "" && !add(f(r.Summary, 600)+"\n") {
		return finish()
	}
	meta := []string{}
	if r.Servings != "" {
		meta = append(meta, "Servings: "+f(r.Servings, 80))
	}
	if r.Time != "" {
		meta = append(meta, "Time: "+f(r.Time, 80))
	}
	meta = append(meta, "Confidence: "+f(r.Confidence, 20))
	if !add(strings.Join(meta, " · ") + "\n\n<b>Ingredients</b>\n") {
		return finish()
	}
	for _, in := range r.Ingredients {
		line := "• "
		if in.Quantity != "" {
			line += f(in.Quantity, 80) + " "
		}
		line += f(in.Item, 180)
		if in.Notes != "" {
			line += " (" + f(in.Notes, 180) + ")"
		}
		if in.Confidence == "low" {
			line += " ⚠️"
		}
		if !add(line + "\n") {
			return finish()
		}
	}
	if !add("\n<b>Method</b>\n") {
		return finish()
	}
	for i, step := range r.Steps {
		if !add(fmt.Sprintf("%d. %s\n", i+1, f(step, 500))) {
			return finish()
		}
	}
	if len(r.Assumptions) > 0 {
		if !add("\n<b>Uncertain / inferred</b>\n") {
			return finish()
		}
		for _, v := range r.Assumptions {
			if !add("• " + f(v, 400) + "\n") {
				return finish()
			}
		}
	}
	if len(r.Warnings) > 0 {
		if !add("\n<b>Safety</b>\n") {
			return finish()
		}
		for _, v := range r.Warnings {
			if !add("• " + f(v, 400) + "\n") {
				return finish()
			}
		}
	}
	return finish()
}

func limit(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
