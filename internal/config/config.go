package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL        string
	HostID             string
	EnabledVendors     []string
	Sink               string
	ClaudeProjectsRoot string
	CodexSessionsRoot  string
	OpenCodeDBPath     string
	CopilotDBPath      string
	CopilotForceLegacy bool
	GeminiTmpRoot      string
	AntigravityRoot       string
	CursorStateDB          string
	CursorUsageReportsDir  string
	LogFormat              string
}

func Load(envFiles ...string) (*Config, error) {
	for _, f := range envFiles {
		_ = godotenv.Load(f)
	}
	// Prefer .env in cwd if present
	_ = godotenv.Load()

	c := &Config{
		DatabaseURL:        firstEnv("DATABASE_URL"),
		HostID:             firstEnv("HOST_ID", "HOST"),
		Sink:               strings.ToLower(firstEnv("SINK", "TOKEN_USAGE_SINK")),
		ClaudeProjectsRoot: firstEnv("CLAUDE_PROJECTS_ROOT"),
		CodexSessionsRoot:  firstEnv("CODEX_SESSIONS_ROOT"),
		OpenCodeDBPath:     firstEnv("OPENCODE_DB_PATH"),
		CopilotDBPath:      firstEnv("COPILOT_DB_PATH"),
		CopilotForceLegacy: strings.EqualFold(firstEnv("COPILOT_FORCE_LEGACY_PREMIUM_REQUESTS"), "true"),
		GeminiTmpRoot:      firstEnv("GEMINI_TMP_ROOT"),
		AntigravityRoot:      firstEnv("ANTIGRAVITY_ROOT"),
		CursorStateDB:         firstEnv("CURSOR_STATE_DB"),
		CursorUsageReportsDir: firstEnv("CURSOR_USAGE_REPORTS_DIR"),
		LogFormat:             firstEnv("LOG_FORMAT"),
	}
	if c.Sink == "" {
		c.Sink = "postgres"
	}
	if c.LogFormat == "" {
		c.LogFormat = "text"
	}
	raw := firstEnv("ENABLED_VENDORS")
	if raw == "" {
		raw = "claude-code"
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			c.EnabledVendors = append(c.EnabledVendors, p)
		}
	}
	if c.HostID == "" {
		return nil, fmt.Errorf("HOST_ID is required")
	}
	if c.Sink == "postgres" && c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required when SINK=postgres")
	}
	if len(c.EnabledVendors) == 0 {
		return nil, fmt.Errorf("ENABLED_VENDORS must list at least one adapter")
	}
	return c, nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
