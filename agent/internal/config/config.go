package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	WarmPoolSizes       map[string]int // template -> pool size (overrides WarmPoolSize)
	// WarmPoolCapabilities lists capabilities to pre-warm alongside the
	// empty-caps baseline. With the default ["sudo"] the pool warms six
	// combos: base/"", base/"sudo", nodejs/"", nodejs/"sudo", python/"",
	// python/"sudo". Set to an empty slice to disable per-capability
	// pre-warming. "git-ssh" and "browser" are intentionally off by
	// default (per-pod NetworkPolicy / 2Gi memory overhead).
	WarmPoolCapabilities []string
	ImagePullPolicy     string
	RateLimitPerMinute  int
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
		// Default is 1 (was 0): ship with warm pool enabled so first
		// sandbox creations don't pay 15-30s of cold-start latency.
		// Operators can still set WARM_POOL_SIZE=0 to disable.
		WarmPoolSize:         envIntOrDefault("WARM_POOL_SIZE", 1),
		WarmPoolSizes:        parseWarmPoolSizes(os.Getenv("WARM_POOL_SIZES")),
		WarmPoolCapabilities: parseCapabilityList(envOrDefault("WARM_POOL_CAPABILITIES", "sudo")),
		RateLimitPerMinute:   envIntOrDefault("RATE_LIMIT_PER_MINUTE", 120),
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

// parseCapabilityList parses a CSV such as "sudo,git-ssh" into a trimmed
// slice with empty entries dropped. Returns an empty (non-nil) slice
// for an empty input so "disabled" is a distinct state from "default".
func parseCapabilityList(raw string) []string {
	out := []string{}
	if raw == "" {
		return out
	}
	for _, c := range strings.Split(raw, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// parseWarmPoolSizes parses "base:3,nodejs:2,gui:1" into a map.
func parseWarmPoolSizes(raw string) map[string]int {
	result := make(map[string]int)
	if raw == "" {
		return result
	}
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) != 2 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || n < 0 {
			continue
		}
		result[strings.TrimSpace(parts[0])] = n
	}
	return result
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
