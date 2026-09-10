package glm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mrRedSun/telegram-recipe-bot/internal/recipe"
	"github.com/mrRedSun/telegram-recipe-bot/internal/youtube"
)

type Client struct {
	base, key, model, effort string
	vision                   bool
	http                     *http.Client
}

func New(base, key, model, effort string, vision bool) *Client {
	return &Client{base: strings.TrimRight(base, "/"), key: key, model: model, effort: effort, vision: vision, http: &http.Client{Timeout: 180 * time.Second}}
}

type content struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}
type imageURL struct {
	URL string `json:"url"`
}
type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}
type request struct {
	Model           string         `json:"model"`
	Messages        []message      `json:"messages"`
	Thinking        map[string]any `json:"thinking"`
	ReasoningEffort string         `json:"reasoning_effort"`
	Temperature     float64        `json:"temperature"`
	MaxTokens       int            `json:"max_tokens"`
	ResponseFormat  map[string]any `json:"response_format"`
}
type apiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Infer(ctx context.Context, ev youtube.Evidence) (recipe.Recipe, error) {
	evidence := fmt.Sprintf("SOURCE TITLE:\n%s\n\nSOURCE DESCRIPTION:\n%s\n\nCAPTIONS/TRANSCRIPT:\n%s\n\nOCR FROM SAMPLED FRAMES:\n%s\n\nDURATION SECONDS: %.0f", ev.Title, ev.Description, ev.Transcript, ev.OCR, ev.Duration)
	parts := []content{{Type: "text", Text: evidence + "\n\nExtract the recipe using the system rules."}}
	if c.vision {
		for _, path := range ev.Frames {
			b, err := os.ReadFile(path)
			if err != nil {
				return recipe.Recipe{}, err
			}
			parts = append(parts, content{Type: "image_url", ImageURL: &imageURL{URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)}})
		}
	}
	userContent := any(parts)
	if !c.vision {
		userContent = evidence + "\n\nExtract the recipe using the system rules."
	}
	reqBody := request{Model: c.model, Messages: []message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userContent}}, Thinking: map[string]any{"type": "enabled"}, ReasoningEffort: c.effort, Temperature: 1, MaxTokens: 5000, ResponseFormat: map[string]any{"type": "json_object"}}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return recipe.Recipe{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return recipe.Recipe{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return recipe.Recipe{}, fmt.Errorf("GLM request: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return recipe.Recipe{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return recipe.Recipe{}, fmt.Errorf("GLM HTTP %d: %s", res.StatusCode, safe(raw))
	}
	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return recipe.Recipe{}, fmt.Errorf("decode GLM response: %w", err)
	}
	if ar.Error != nil {
		return recipe.Recipe{}, errors.New("GLM: " + ar.Error.Message)
	}
	if len(ar.Choices) == 0 {
		return recipe.Recipe{}, errors.New("GLM returned no choices")
	}
	jsonText := extractJSON(ar.Choices[0].Message.Content)
	var result recipe.Recipe
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return recipe.Recipe{}, fmt.Errorf("GLM returned invalid recipe JSON: %w", err)
	}
	if err := result.Validate(); err != nil {
		return recipe.Recipe{}, fmt.Errorf("GLM recipe validation: %w", err)
	}
	return result, nil
}
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
func safe(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

const systemPrompt = `You are a careful culinary analyst. Reconstruct a usable recipe only from the supplied YouTube Short evidence (metadata, captions, OCR, and optionally sampled frames).

Rules:
- Never invent exact quantities, temperatures, timings, ingredients, or allergens. If not evidenced, use "to taste", "as needed", "not shown", or a range and explain it in assumptions.
- Distinguish observed facts from plausible inference. Low-confidence ingredients must have confidence "low" and an assumption.
- Preserve the source language when practical, but produce clear English culinary instructions.
- Call out raw meat/egg, cross-contamination, food allergy, and uncertain doneness concerns when relevant. Do not make medical claims.
- If the evidence is insufficient to form a recipe, still return the identifiable components and steps, with overall confidence "low" and explicit missing details.
- Return JSON only with exactly this shape:
{"title":"string","summary":"string","servings":"string","time":"string","ingredients":[{"quantity":"string","item":"string","notes":"string","confidence":"high|medium|low"}],"steps":["string"],"assumptions":["string"],"warnings":["string"],"confidence":"high|medium|low"}`
