package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the agent configuration.
type Config struct {
	ListenAddr          string
	PreviewDomain       string
	ExternalURL         string
	SandboxNamespace    string
	SidecarImage        string
	RuntimeBaseImage    string
	DefaultTimeout      time.Duration
	MaxTimeout          time.Duration
	WarmPoolSize        int
	ImagePullPolicy     string
	APIKey              string // single API key for Phase 1; replaced by DB lookup later
	JWTSecret           string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		ListenAddr:       envOrDefault("AGENT_LISTEN_ADDR", ":8080"),
		PreviewDomain:    envOrDefault("PREVIEW_DOMAIN", "preview.localhost"),
		ExternalURL:      envOrDefault("AGENT_EXTERNAL_URL", "http://localhost:8080"),
		SandboxNamespace: envOrDefault("SANDBOX_NAMESPACE", "xgen-sandboxes"),
		SidecarImage:     envOrDefault("SIDECAR_IMAGE", "ghcr.io/xgen-sandbox/sidecar:latest"),
		RuntimeBaseImage: envOrDefault("RUNTIME_BASE_IMAGE", "ghcr.io/xgen-sandbox/runtime-base:latest"),
		ImagePullPolicy:  envOrDefault("IMAGE_PULL_POLICY", ""),
		DefaultTimeout:   envDurationOrDefault("DEFAULT_TIMEOUT", 1*time.Hour),
		MaxTimeout:       envDurationOrDefault("MAX_TIMEOUT", 24*time.Hour),
		WarmPoolSize:     envIntOrDefault("WARM_POOL_SIZE", 0),
		APIKey:           envOrDefault("API_KEY", ""),
		JWTSecret:        envOrDefault("JWT_SECRET", ""),
	}
}

// knownInsecureSecrets contains default dev secrets that must not be used in production.
var knownInsecureSecrets = []string{
	"xgen-dev-jwt-secret-change-in-production",
	"xgen_dev_key",
}

// Validate checks that critical configuration values are set and secure.
func (c *Config) Validate() error {
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET environment variable is required")
	}
	if c.APIKey == "" {
		return fmt.Errorf("API_KEY environment variable is required")
	}
	for _, insecure := range knownInsecureSecrets {
		if c.JWTSecret == insecure {
			return fmt.Errorf("JWT_SECRET must not use the default dev value")
		}
		if c.APIKey == insecure {
			return fmt.Errorf("API_KEY must not use the default dev value")
		}
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
