package recipe

import (
	"errors"
	"fmt"
	"html"
	"strings"
)

// Rich messages allow 32,768 UTF-8 characters. A byte budget below that limit
// is deliberately conservative and also covers markup overhead.
const richMessageBudget = 30000

type Times struct {
	Prep  string `json:"prep"`
	Cook  string `json:"cook"`
	Total string `json:"total"`
}

type Ingredient struct {
	Amount      string `json:"amount"`
	Unit        string `json:"unit"`
	Item        string `json:"item"`
	Preparation string `json:"preparation,omitempty"`
	Confidence  string `json:"confidence"`
}

type Step struct {
	Instruction string `json:"instruction"`
	Duration    string `json:"duration,omitempty"`
	Temperature string `json:"temperature,omitempty"`
	Confidence  string `json:"confidence"`
}

type Recipe struct {
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	Yield       string       `json:"yield"`
	Times       Times        `json:"times"`
	Ingredients []Ingredient `json:"ingredients"`
	Equipment   []string     `json:"equipment"`
	Steps       []Step       `json:"steps"`
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
	if len(r.Ingredients) > 40 {
		return errors.New("recipe has too many ingredients")
	}
	if len(r.Steps) == 0 {
		return errors.New("recipe has no steps")
	}
	if len(r.Steps) > 30 || len(r.Equipment) > 20 || len(r.Assumptions) > 20 || len(r.Warnings) > 20 {
		return errors.New("recipe has too many entries")
	}
	if !confidence(r.Confidence) {
		return fmt.Errorf("invalid confidence %q", r.Confidence)
	}
	for i, ingredient := range r.Ingredients {
		if strings.TrimSpace(ingredient.Item) == "" {
			return fmt.Errorf("ingredient %d has no item", i+1)
		}
		if !confidence(ingredient.Confidence) {
			return fmt.Errorf("ingredient %d has invalid confidence", i+1)
		}
	}
	for i, step := range r.Steps {
		if strings.TrimSpace(step.Instruction) == "" {
			return fmt.Errorf("step %d has no instruction", i+1)
		}
		if !confidence(step.Confidence) {
			return fmt.Errorf("step %d has invalid confidence", i+1)
		}
	}
	return nil
}

func confidence(v string) bool { return v == "high" || v == "medium" || v == "low" }

// RenderRichHTML returns Telegram Bot API Rich HTML, not legacy sendMessage
// HTML. Tables, headings, lists, details, dividers, and footers are native rich
// message blocks parsed by editMessageText's rich_message field.
func RenderRichHTML(r Recipe) string {
	var b strings.Builder
	truncated := false
	add := func(block string) bool {
		if b.Len()+len(block) > richMessageBudget {
			truncated = true
			return false
		}
		b.WriteString(block)
		return true
	}
	field := func(s string, maxRunes int) string {
		value, wasTruncated := escapedField(s, maxRunes)
		truncated = truncated || wasTruncated
		return value
	}
	finish := func() string {
		if truncated {
			b.WriteString("<footer>Recipe shortened to fit Telegram.</footer>")
		}
		return b.String()
	}

	add("<h2>🍳 " + field(r.Title, 180) + "</h2>")
	if r.Summary != "" && !add("<p><i>"+field(r.Summary, 600)+"</i></p>") {
		return finish()
	}
	overview, shortened := overviewTableHTML(r)
	truncated = truncated || shortened
	if !add(overview) {
		return finish()
	}

	if len(r.Warnings) > 0 {
		warnings, shortened := quotationHTML("⚠️ Safety", r.Warnings, 2500)
		truncated = truncated || shortened
		if !add(warnings) {
			return finish()
		}
	}
	add("<hr/>")

	ingredients, shortened := ingredientTableHTML(r.Ingredients, 9000)
	truncated = truncated || shortened
	if !add(ingredients) {
		return finish()
	}

	steps, shortened := stepsHTML(r.Steps, 9000)
	truncated = truncated || shortened
	if !add(steps) {
		return finish()
	}

	if len(r.Equipment) > 0 {
		equipment, shortened := listSectionHTML("🔧 Equipment", r.Equipment, 3000)
		truncated = truncated || shortened
		if !add(equipment) {
			return finish()
		}
	}

	if len(r.Assumptions) > 0 {
		assumptions, shortened := detailsHTML("🔎 Uncertain or inferred", r.Assumptions, 2800)
		truncated = truncated || shortened
		if !add(assumptions) {
			return finish()
		}
	}
	return finish()
}

func overviewTableHTML(r Recipe) (string, bool) {
	rows := [][2]string{
		{"Yield", valueOrUnknown(r.Yield)},
		{"Prep", valueOrUnknown(r.Times.Prep)},
		{"Cook", valueOrUnknown(r.Times.Cook)},
		{"Total", valueOrUnknown(r.Times.Total)},
	}
	var b strings.Builder
	b.WriteString(`<table bordered striped compact><caption>Overview</caption>`)
	shortened := false
	for _, row := range rows {
		value, cut := escapedField(row[1], 120)
		shortened = shortened || cut
		b.WriteString("<tr><th align=\"left\">" + row[0] + "</th><td align=\"left\">" + value + "</td></tr>")
	}
	b.WriteString("<tr><th align=\"left\">Confidence</th><td align=\"left\">" + confidenceHTML(r.Confidence) + "</td></tr></table>")
	return b.String(), shortened
}

func ingredientTableHTML(ingredients []Ingredient, maxBytes int) (string, bool) {
	const open = `<table bordered striped compact><caption>🧺 Ingredients</caption><tr><th>C</th><th>Amount</th><th>Ingredient</th><th>Preparation</th></tr>`
	const close = `</table>`
	var b strings.Builder
	b.WriteString(open)
	shortened := false
	for _, ingredient := range ingredients {
		amount := strings.TrimSpace(strings.TrimSpace(ingredient.Amount) + " " + strings.TrimSpace(ingredient.Unit))
		if amount == "" {
			amount = "not shown"
		}
		amountHTML, amountCut := escapedField(amount, 80)
		itemHTML, itemCut := escapedField(ingredient.Item, 220)
		preparationHTML, preparationCut := escapedField(ingredient.Preparation, 180)
		shortened = shortened || amountCut || itemCut || preparationCut
		row := "<tr><td align=\"center\">" + confidenceHTML(ingredient.Confidence) + "</td><td>" + amountHTML + "</td><td>" + itemHTML + "</td><td>" + preparationHTML + "</td></tr>"
		if b.Len()+len(row)+len(close) > maxBytes {
			shortened = true
			break
		}
		b.WriteString(row)
	}
	b.WriteString(close)
	return b.String(), shortened
}

func quotationHTML(title string, items []string, maxBytes int) (string, bool) {
	const close = `</blockquote>`
	var b strings.Builder
	b.WriteString("<blockquote><b>" + html.EscapeString(title) + "</b>")
	shortened := false
	for _, item := range items {
		value, cut := escapedField(item, 320)
		shortened = shortened || cut
		line := "<br>• " + value
		if b.Len()+len(line)+len(close) > maxBytes {
			shortened = true
			break
		}
		b.WriteString(line)
	}
	b.WriteString(close)
	return b.String(), shortened
}

func listSectionHTML(title string, items []string, maxBytes int) (string, bool) {
	return titledListHTML("<h3>"+html.EscapeString(title)+"</h3><ul>", "</ul>", items, maxBytes)
}

func detailsHTML(summary string, items []string, maxBytes int) (string, bool) {
	return titledListHTML("<details><summary>"+html.EscapeString(summary)+"</summary><ul>", "</ul></details>", items, maxBytes)
}

func titledListHTML(open, close string, items []string, maxBytes int) (string, bool) {
	var b strings.Builder
	b.WriteString(open)
	shortened := false
	for _, item := range items {
		value, cut := escapedField(item, 320)
		shortened = shortened || cut
		line := "<li>" + value + "</li>"
		if b.Len()+len(line)+len(close) > maxBytes {
			shortened = true
			break
		}
		b.WriteString(line)
	}
	b.WriteString(close)
	return b.String(), shortened
}

func stepsHTML(steps []Step, maxBytes int) (string, bool) {
	const open = `<h3>👩‍🍳 Method</h3><ol>`
	const close = `</ol>`
	var b strings.Builder
	b.WriteString(open)
	shortened := false
	for _, step := range steps {
		instruction, instructionCut := escapedField(step.Instruction, 520)
		shortened = shortened || instructionCut
		var metadata []string
		if step.Duration != "" {
			duration, cut := escapedField(step.Duration, 80)
			shortened = shortened || cut
			metadata = append(metadata, "⏱ "+duration)
		}
		if step.Temperature != "" {
			temperature, cut := escapedField(step.Temperature, 80)
			shortened = shortened || cut
			metadata = append(metadata, "🌡 "+temperature)
		}
		if step.Confidence != "high" {
			metadata = append(metadata, confidenceLabel(step.Confidence))
		}
		line := "<li>" + instruction
		if len(metadata) > 0 {
			line += "<br><i>" + strings.Join(metadata, " · ") + "</i>"
		}
		line += "</li>"
		if b.Len()+len(line)+len(close) > maxBytes {
			shortened = true
			break
		}
		b.WriteString(line)
	}
	b.WriteString(close)
	return b.String(), shortened
}

func escapedField(s string, maxRunes int) (string, bool) {
	s = strings.TrimSpace(s)
	cut := len([]rune(s)) > maxRunes
	return html.EscapeString(limit(s, maxRunes)), cut
}

func confidenceHTML(v string) string {
	switch v {
	case "high":
		return "High"
	case "medium":
		return "<i>Medium</i>"
	default:
		return "<mark>Low</mark>"
	}
}

func confidenceLabel(v string) string {
	if v == "medium" {
		return "🟡 medium confidence"
	}
	return "🔴 low confidence"
}

func valueOrUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "not shown"
	}
	return s
}

func limit(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
