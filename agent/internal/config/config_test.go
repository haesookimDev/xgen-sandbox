package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Unset all relevant env vars to ensure defaults
	envVars := []string{
		"AGENT_LISTEN_ADDR", "PREVIEW_DOMAIN", "AGENT_EXTERNAL_URL",
		"SANDBOX_NAMESPACE", "SIDECAR_IMAGE", "RUNTIME_BASE_IMAGE",
		"IMAGE_PULL_POLICY", "DEFAULT_TIMEOUT", "MAX_TIMEOUT",
		"WARM_POOL_SIZE", "API_KEY", "JWT_SECRET",
	}
	for _, key := range envVars {
		t.Setenv(key, "")
	}

	cfg := Load()

	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr: expected :8080, got %q", cfg.ListenAddr)
	}
	if cfg.PreviewDomain != "preview.localhost" {
		t.Errorf("PreviewDomain: expected preview.localhost, got %q", cfg.PreviewDomain)
	}
	if cfg.ExternalURL != "http://localhost:8080" {
		t.Errorf("ExternalURL: expected http://localhost:8080, got %q", cfg.ExternalURL)
	}
	if cfg.SandboxNamespace != "xgen-sandboxes" {
		t.Errorf("SandboxNamespace: expected xgen-sandboxes, got %q", cfg.SandboxNamespace)
	}
	if cfg.DefaultTimeout != 1*time.Hour {
		t.Errorf("DefaultTimeout: expected 1h, got %v", cfg.DefaultTimeout)
	}
	if cfg.MaxTimeout != 24*time.Hour {
		t.Errorf("MaxTimeout: expected 24h, got %v", cfg.MaxTimeout)
	}
	if cfg.WarmPoolSize != 1 {
		t.Errorf("WarmPoolSize: expected 1 (warm pool now on by default), got %d", cfg.WarmPoolSize)
	}
	if len(cfg.WarmPoolCapabilities) != 1 || cfg.WarmPoolCapabilities[0] != "sudo" {
		t.Errorf("WarmPoolCapabilities: expected [sudo] default, got %v", cfg.WarmPoolCapabilities)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey: expected empty string (no default), got %q", cfg.APIKey)
	}
	if cfg.JWTSecret != "" {
		t.Errorf("JWTSecret: expected empty string (no default), got %q", cfg.JWTSecret)
	}
}

func TestLoad_CustomEnvVars(t *testing.T) {
	t.Setenv("AGENT_LISTEN_ADDR", ":9090")
	t.Setenv("WARM_POOL_SIZE", "5")
	t.Setenv("DEFAULT_TIMEOUT", "30m")
	t.Setenv("PREVIEW_DOMAIN", "preview.example.com")

	cfg := Load()

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr: expected :9090, got %q", cfg.ListenAddr)
	}
	if cfg.WarmPoolSize != 5 {
		t.Errorf("WarmPoolSize: expected 5, got %d", cfg.WarmPoolSize)
	}
	if cfg.DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout: expected 30m, got %v", cfg.DefaultTimeout)
	}
	if cfg.PreviewDomain != "preview.example.com" {
		t.Errorf("PreviewDomain: expected preview.example.com, got %q", cfg.PreviewDomain)
	}
}

func TestLoad_InvalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("WARM_POOL_SIZE", "notanumber")

	cfg := Load()

	if cfg.WarmPoolSize != 1 {
		t.Errorf("WarmPoolSize: expected 1 (default), got %d", cfg.WarmPoolSize)
	}
}

func TestLoad_WarmPoolCapabilitiesCustom(t *testing.T) {
	t.Setenv("WARM_POOL_CAPABILITIES", "sudo, git-ssh ,")
	cfg := Load()
	want := []string{"sudo", "git-ssh"}
	if len(cfg.WarmPoolCapabilities) != len(want) {
		t.Fatalf("WarmPoolCapabilities: got %v, want %v", cfg.WarmPoolCapabilities, want)
	}
	for i, c := range want {
		if cfg.WarmPoolCapabilities[i] != c {
			t.Errorf("WarmPoolCapabilities[%d]: got %q want %q", i, cfg.WarmPoolCapabilities[i], c)
		}
	}
}

func TestLoad_WarmPoolCapabilitiesDisabled(t *testing.T) {
	t.Setenv("WARM_POOL_CAPABILITIES", "")
	cfg := Load()
	// Empty value means "no caps to pre-warm" — should default to "sudo"
	// via envOrDefault fallback; explicit disabling uses a placeholder we
	// accept as empty after trimming, so this is a regression check.
	if len(cfg.WarmPoolCapabilities) != 1 || cfg.WarmPoolCapabilities[0] != "sudo" {
		t.Errorf("empty WARM_POOL_CAPABILITIES should fall back to default [sudo], got %v", cfg.WarmPoolCapabilities)
	}
}

func TestLoad_InvalidDurationFallsBackToDefault(t *testing.T) {
	t.Setenv("DEFAULT_TIMEOUT", "badvalue")

	cfg := Load()

	if cfg.DefaultTimeout != 1*time.Hour {
		t.Errorf("DefaultTimeout: expected 1h (default), got %v", cfg.DefaultTimeout)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_KEY_EXISTS", "custom_value")

	if v := envOrDefault("TEST_KEY_EXISTS", "fallback"); v != "custom_value" {
		t.Errorf("expected custom_value, got %q", v)
	}
	if v := envOrDefault("TEST_KEY_NOT_EXISTS_12345", "fallback"); v != "fallback" {
		t.Errorf("expected fallback, got %q", v)
	}
}

func TestEnvIntOrDefault(t *testing.T) {
	t.Setenv("TEST_INT_VALID", "42")
	t.Setenv("TEST_INT_INVALID", "abc")

	if v := envIntOrDefault("TEST_INT_VALID", 0); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if v := envIntOrDefault("TEST_INT_INVALID", 10); v != 10 {
		t.Errorf("expected 10 (fallback), got %d", v)
	}
	if v := envIntOrDefault("TEST_INT_MISSING_12345", 99); v != 99 {
		t.Errorf("expected 99 (fallback), got %d", v)
	}
}

func TestEnvDurationOrDefault(t *testing.T) {
	t.Setenv("TEST_DUR_VALID", "5m")
	t.Setenv("TEST_DUR_INVALID", "nope")

	if v := envDurationOrDefault("TEST_DUR_VALID", time.Hour); v != 5*time.Minute {
		t.Errorf("expected 5m, got %v", v)
	}
	if v := envDurationOrDefault("TEST_DUR_INVALID", time.Hour); v != time.Hour {
		t.Errorf("expected 1h (fallback), got %v", v)
	}
	if v := envDurationOrDefault("TEST_DUR_MISSING_12345", 2*time.Hour); v != 2*time.Hour {
		t.Errorf("expected 2h (fallback), got %v", v)
	}
}

func TestValidate_EmptySecrets(t *testing.T) {
	cfg := &Config{JWTSecret: "", APIKey: ""}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty secrets")
	}
}

func TestValidate_InsecureDefaults(t *testing.T) {
	cfg := &Config{JWTSecret: "xgen-dev-jwt-secret-change-in-production", APIKey: "some-key"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for insecure JWT secret")
	}

	cfg = &Config{JWTSecret: "secure-secret-32-bytes-long!!!!!", APIKey: "xgen_dev_key"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for insecure API key")
	}
}

func TestValidate_SecureConfig(t *testing.T) {
	cfg := &Config{JWTSecret: "my-production-secret-key-here!!!", APIKey: "prod-api-key-12345"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for secure config, got: %v", err)
	}
}
