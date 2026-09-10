package recipe

import (
	"errors"
	"fmt"
	"html"
	"strings"
)

const messageBudget = 3800

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

// RenderHTML uses only Telegram Bot API-supported HTML. Telegram has no table
// entity, so compact tables are represented by escaped monospaced <pre> blocks.
func RenderHTML(r Recipe) string {
	var b strings.Builder
	truncated := false
	escape := func(s string, n int) string {
		s = strings.TrimSpace(s)
		if len([]rune(s)) > n {
			truncated = true
		}
		return html.EscapeString(limit(s, n))
	}
	add := func(fragment string) bool {
		if b.Len()+len(fragment) > messageBudget {
			truncated = true
			return false
		}
		b.WriteString(fragment)
		return true
	}
	finish := func() string {
		if truncated {
			b.WriteString("\n<i>Recipe shortened to fit Telegram.</i>")
		}
		return b.String()
	}

	add("🍳 <b>" + escape(r.Title, 160) + "</b>\n")
	if r.Summary != "" && !add("<i>"+escape(r.Summary, 500)+"</i>\n") {
		return finish()
	}
	if !add("\n<pre>" + html.EscapeString(overviewTable(r)) + "</pre>\n") {
		return finish()
	}

	if len(r.Warnings) > 0 {
		warningText := boundedList(r.Warnings, 900)
		if !add("\n<blockquote>⚠️ <b>Safety</b>\n" + html.EscapeString(warningText) + "</blockquote>\n") {
			return finish()
		}
	}

	if !add("\n🧺 <b><u>Ingredients</u></b>\n<pre>" + html.EscapeString(ingredientTable(r.Ingredients)) + "</pre>\n") {
		return finish()
	}
	if len(r.Equipment) > 0 {
		if !add("\n🔧 <b><u>Equipment</u></b>\n") {
			return finish()
		}
		for _, item := range r.Equipment {
			if !add("• " + escape(item, 120) + "\n") {
				return finish()
			}
		}
	}

	if !add("\n👩‍🍳 <b><u>Method</u></b>\n") {
		return finish()
	}
	for i, step := range r.Steps {
		line := fmt.Sprintf("<b>%d.</b> %s\n", i+1, escape(step.Instruction, 480))
		var details []string
		if step.Duration != "" {
			details = append(details, "⏱ "+escape(step.Duration, 80))
		}
		if step.Temperature != "" {
			details = append(details, "🌡 "+escape(step.Temperature, 80))
		}
		if step.Confidence != "high" {
			details = append(details, confidenceLabel(step.Confidence))
		}
		if len(details) > 0 {
			line += "   <i>" + strings.Join(details, " · ") + "</i>\n"
		}
		if !add(line) {
			return finish()
		}
	}

	if len(r.Assumptions) > 0 {
		assumptions := boundedList(r.Assumptions, 900)
		if !add("\n<blockquote expandable>🔎 <b>Uncertain or inferred</b>\n" + html.EscapeString(assumptions) + "</blockquote>\n") {
			return finish()
		}
	}
	return finish()
}

func overviewTable(r Recipe) string {
	rows := [][2]string{
		{"YIELD", valueOrUnknown(r.Yield)},
		{"PREP", valueOrUnknown(r.Times.Prep)},
		{"COOK", valueOrUnknown(r.Times.Cook)},
		{"TOTAL", valueOrUnknown(r.Times.Total)},
		{"CONFIDENCE", strings.ToUpper(r.Confidence) + " " + confidenceMark(r.Confidence)},
	}
	var lines []string
	for _, row := range rows {
		lines = append(lines, tableCell(row[0], 10)+"  "+tableCell(row[1], 25))
	}
	return strings.Join(lines, "\n")
}

func ingredientTable(ingredients []Ingredient) string {
	lines := []string{"C  " + tableCell("AMOUNT", 10) + "  INGREDIENT", "-  " + strings.Repeat("-", 10) + "  " + strings.Repeat("-", 22)}
	for _, ingredient := range ingredients {
		amount := strings.TrimSpace(strings.TrimSpace(ingredient.Amount) + " " + strings.TrimSpace(ingredient.Unit))
		if amount == "" {
			amount = "not shown"
		}
		item := strings.TrimSpace(ingredient.Item)
		if ingredient.Preparation != "" {
			item += ", " + strings.TrimSpace(ingredient.Preparation)
		}
		lines = append(lines, confidenceMark(ingredient.Confidence)+"  "+tableCell(amount, 10)+"  "+tableCell(item, 22))
	}
	lines = append(lines, "", "H=high  M=medium  L=low")
	return strings.Join(lines, "\n")
}

func boundedList(items []string, maxRunes int) string {
	var lines []string
	used := 0
	for _, item := range items {
		line := "• " + limit(strings.TrimSpace(item), 260)
		if line == "• " {
			continue
		}
		lineRunes := len([]rune(line))
		if used+lineRunes+1 > maxRunes {
			break
		}
		lines = append(lines, line)
		used += lineRunes + 1
	}
	return strings.Join(lines, "\n")
}

func tableCell(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > width {
		if width > 1 {
			runes = append(runes[:width-1], '…')
		} else {
			runes = runes[:width]
		}
	}
	return string(runes) + strings.Repeat(" ", width-len(runes))
}

func confidenceMark(v string) string {
	switch v {
	case "high":
		return "H"
	case "medium":
		return "M"
	default:
		return "L"
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
