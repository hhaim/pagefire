package app

import (
	"os"
	"testing"
)

func TestLoadConfig_EnvVarMapping(t *testing.T) {
	// Verify that flat env vars map correctly — underscores within field names
	// are preserved (not replaced with dots).
	t.Setenv("PAGEFIRE_PORT", "4000")
	t.Setenv("PAGEFIRE_DATABASE_URL", "/tmp/test.db")
	t.Setenv("PAGEFIRE_LOG_LEVEL", "debug")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Port != 4000 {
		t.Errorf("Port: got %d, want 4000", cfg.Port)
	}
	if cfg.DatabaseURL != "/tmp/test.db" {
		t.Errorf("DatabaseURL: got %q, want %q", cfg.DatabaseURL, "/tmp/test.db")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadConfig_NestedEnvVars(t *testing.T) {
	// Verify that the engine prefix maps to nested configuration.
	t.Setenv("PAGEFIRE_ENGINE_INTERVAL_SECONDS", "10")

	// Clear vars that might interfere from other tests
	os.Unsetenv("PAGEFIRE_PORT")
	os.Unsetenv("PAGEFIRE_DATABASE_URL")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Engine.IntervalSeconds != 10 {
		t.Errorf("Engine.IntervalSeconds: got %d, want 10", cfg.Engine.IntervalSeconds)
	}
}
