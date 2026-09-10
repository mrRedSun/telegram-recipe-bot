package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TelegramToken    string
	TelegramAPIBase  string
	AllowedUsers     map[int64]struct{}
	GLMAPIKey        string
	GLMAPIBase       string
	GLMModel         string
	GLMVision        bool
	ReasoningEffort  string
	Workers          int
	QueueSize        int
	MaxDuration      time.Duration
	MaxDownloadBytes int64
	JobTimeout       time.Duration
	TempRoot         string
	ListenAddress    string
	TesseractLangs   string
}

func Load() (Config, error) {
	c := Config{
		TelegramToken:    strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramAPIBase:  value("TELEGRAM_API_BASE", "https://api.telegram.org"),
		GLMAPIKey:        strings.TrimSpace(os.Getenv("GLM_API_KEY")),
		GLMAPIBase:       strings.TrimRight(value("GLM_API_BASE", "https://api.z.ai/api/paas/v4"), "/"),
		GLMModel:         value("GLM_MODEL", "glm-5.3-flash"),
		ReasoningEffort:  value("GLM_REASONING_EFFORT", "high"),
		Workers:          intValue("WORKERS", 2),
		QueueSize:        intValue("QUEUE_SIZE", 8),
		MaxDuration:      time.Duration(intValue("MAX_VIDEO_SECONDS", 240)) * time.Second,
		MaxDownloadBytes: int64(intValue("MAX_DOWNLOAD_MB", 100)) * 1024 * 1024,
		JobTimeout:       time.Duration(intValue("JOB_TIMEOUT_SECONDS", 300)) * time.Second,
		TempRoot:         value("TEMP_ROOT", "/tmp/recipebot"),
		ListenAddress:    value("LISTEN_ADDRESS", ":8080"),
		TesseractLangs:   value("TESSERACT_LANGS", "eng"),
	}
	c.GLMVision = strings.HasSuffix(strings.ToLower(c.GLMModel), "-flash")
	if raw := strings.TrimSpace(os.Getenv("GLM_VISION_ENABLED")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("GLM_VISION_ENABLED: %w", err)
		}
		c.GLMVision = v
	}
	users, err := parseIDs(os.Getenv("TELEGRAM_ALLOWED_USER_IDS"))
	if err != nil {
		return Config{}, err
	}
	c.AllowedUsers = users
	if c.TelegramToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	if c.GLMAPIKey == "" {
		return Config{}, errors.New("GLM_API_KEY is required")
	}
	if len(c.AllowedUsers) == 0 {
		return Config{}, errors.New("TELEGRAM_ALLOWED_USER_IDS must contain at least one numeric user ID")
	}
	if c.Workers < 1 || c.Workers > 8 {
		return Config{}, errors.New("WORKERS must be between 1 and 8")
	}
	if c.QueueSize < 1 || c.QueueSize > 100 {
		return Config{}, errors.New("QUEUE_SIZE must be between 1 and 100")
	}
	if c.MaxDuration < 15*time.Second || c.MaxDuration > 15*time.Minute {
		return Config{}, errors.New("MAX_VIDEO_SECONDS must be between 15 and 900")
	}
	if c.MaxDownloadBytes < 10*1024*1024 || c.MaxDownloadBytes > 200*1024*1024 {
		return Config{}, errors.New("MAX_DOWNLOAD_MB must be between 10 and 200")
	}
	switch c.ReasoningEffort {
	case "low", "high", "max":
	default:
		return Config{}, errors.New("GLM_REASONING_EFFORT must be low, high, or max")
	}
	return c, nil
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
func intValue(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}
func parseIDs(raw string) (map[int64]struct{}, error) {
	out := make(map[int64]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid TELEGRAM_ALLOWED_USER_IDS entry %q", part)
		}
		out[id] = struct{}{}
	}
	return out, nil
}
