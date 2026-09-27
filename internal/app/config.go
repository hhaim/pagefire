package app

import (
	"strings"

	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

// Config holds all application configuration.
type Config struct {
	Port           int          `koanf:"port"`
	DatabaseURL    string       `koanf:"database_url"`
	DataDir        string       `koanf:"data_dir"`
	LogLevel       string       `koanf:"log_level"`
	Engine         EngineConfig `koanf:"engine"`
}

type EngineConfig struct {
	IntervalSeconds int `koanf:"interval_seconds"`
}

// LoadConfig loads configuration from environment variables with PAGEFIRE_ prefix.
// Precedence: env vars > defaults.
func LoadConfig() (*Config, error) {
	k := koanf.New(".")

	// Defaults
	k.Set("port", 3000)
	k.Set("data_dir", ".")
	k.Set("log_level", "info")
	k.Set("engine.interval_seconds", 5)

	// Environment variables: PAGEFIRE_PORT, PAGEFIRE_DATABASE_URL, etc.
	// Known prefixes are mapped to nested struct fields.
	nestedPrefixes := []string{"engine_"}

	err := k.Load(env.Provider("PAGEFIRE_", ".", func(s string) string {
		key := strings.ToLower(strings.TrimPrefix(s, "PAGEFIRE_"))
		for _, prefix := range nestedPrefixes {
			if strings.HasPrefix(key, prefix) {
				// Replace only the first underscore to create nesting dot
				// e.g. engine_interval_seconds → engine.interval_seconds
				return strings.Replace(key, "_", ".", 1)
			}
		}
		return key
	}), nil)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, err
	}

	// Default SQLite path if no DATABASE_URL set
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = cfg.DataDir + "/pagefire.db"
	}

	return &cfg, nil
}
