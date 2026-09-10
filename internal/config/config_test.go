package config

import "testing"

func TestLoadSecureDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("GLM_API_KEY", "glm-key")
	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", "123, 456")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.GLMModel != "glm-5.3-flash" || !c.GLMVision {
		t.Fatalf("unexpected GLM defaults: %#v", c)
	}
	if len(c.AllowedUsers) != 2 {
		t.Fatalf("allowed users = %d", len(c.AllowedUsers))
	}
}

func TestLoadRejectsOpenAccess(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("GLM_API_KEY", "glm-key")
	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected allowlist error")
	}
}
