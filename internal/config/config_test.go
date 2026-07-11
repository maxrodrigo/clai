package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/maxrodrigo/clai/internal/testsupport"
	"github.com/spf13/viper"
)

// TestMain points the user config directory at an empty temporary location so
// the suite never reads a developer's real ~/.config/clai/config.toml. Tests
// that need a config file create their own under a temp working directory.
func TestMain(m *testing.M) {
	cleanup := testsupport.IsolateConfig()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func resetViper() {
	v = viper.New()
	initOnce = sync.Once{} // allow re-initialization in tests
	v.SetEnvPrefix("CLAI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.BindEnv("model")
	v.AutomaticEnv()
}

func TestDefaults(t *testing.T) {
	resetViper()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got, want := cfg.Model, ""; got != want {
		t.Errorf("Model = %q, want %q (should be empty when not configured)", got, want)
	}
	if cfg.Temperature != nil {
		t.Errorf("Temperature = %v, want nil (provider default)", *cfg.Temperature)
	}
}

func TestModelProviderSplit(t *testing.T) {
	tests := []struct {
		model    string
		provider string
		name     string
	}{
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"anthropic/claude-3-5-sonnet-20241022", "anthropic", "claude-3-5-sonnet-20241022"},
		{"gpt-4o", "", "gpt-4o"},
		{"openai/", "", ""},
		{"/gpt-4o", "", "gpt-4o"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			cfg := &Config{Model: tt.model}
			if got := cfg.ModelProvider(); got != tt.provider {
				t.Errorf("ModelProvider() = %q, want %q", got, tt.provider)
			}
			if got := cfg.ModelName(); got != tt.name {
				t.Errorf("ModelName() = %q, want %q", got, tt.name)
			}
		})
	}
}

func TestTOMLConfig(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig) //nolint:errcheck

	claiDir := filepath.Join(tmpDir, ".clai")
	if err := os.MkdirAll(claiDir, 0o750); err != nil {
		t.Fatal(err)
	}
	configContent := `
model = "anthropic/claude-3-5-sonnet-20241022"
temperature = 0.5
strategy = "cot"

[providers.anthropic]
base_url = "https://api.anthropic.com"
api_key = "test-key"
`
	if err := os.WriteFile(filepath.Join(claiDir, "config.toml"), []byte(configContent), 0o600); err != nil {
		t.Fatal(err)
	}

	resetViper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if got, want := cfg.Model, "anthropic/claude-3-5-sonnet-20241022"; got != want {
		t.Errorf("Model = %q, want %q", got, want)
	}
	if cfg.Temperature == nil {
		t.Fatal("Temperature = nil, want 0.5")
	}
	if *cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %v, want 0.5", *cfg.Temperature)
	}
	if got, want := cfg.Strategy, "cot"; got != want {
		t.Errorf("Strategy = %q, want %q", got, want)
	}
	if cfg.Providers["anthropic"].APIKey != "test-key" {
		t.Errorf("Providers[anthropic].APIKey = %q, want test-key", cfg.Providers["anthropic"].APIKey)
	}
}

func TestMalformedProjectConfig(t *testing.T) {
	// A malformed project config should return an error, not silently fall back to defaults.
	tmpDir := t.TempDir()
	claiDir := filepath.Join(tmpDir, ".clai")
	if err := os.MkdirAll(claiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	badConfig := filepath.Join(claiDir, "config.toml")
	if err := os.WriteFile(badConfig, []byte("this is not valid toml = = ="), 0o600); err != nil {
		t.Fatal(err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig) //nolint:errcheck

	resetViper()
	_, err = Load()
	if err == nil {
		t.Error("Load() should return error for malformed project config, got nil")
	}
}

func TestEnvOverride(t *testing.T) {
	resetViper()

	t.Setenv("CLAI_MODEL", "anthropic/claude-3-5-sonnet-20241022")
	t.Setenv("CLAI_TEMPERATURE", "0.7")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if got, want := cfg.Model, "anthropic/claude-3-5-sonnet-20241022"; got != want {
		t.Errorf("Model = %q, want %q", got, want)
	}
	if cfg.Temperature == nil {
		t.Fatal("Temperature = nil, want 0.7")
	}
	if *cfg.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", *cfg.Temperature)
	}
}

func TestExpandEnvRefs(t *testing.T) {
	t.Setenv("MY_TEST_API_KEY", "sk-test-123")
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {APIKey: "${MY_TEST_API_KEY}", BaseURL: "https://api.openai.com/v1"},
		},
	}
	expandEnvRefs(cfg)
	if got, want := cfg.Providers["openai"].APIKey, "sk-test-123"; got != want {
		t.Errorf("APIKey = %q, want %q", got, want)
	}
}

func TestGetProvider(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
		},
	}

	t.Run("existing provider", func(t *testing.T) {
		pc, err := cfg.GetProvider("openai")
		if err != nil {
			t.Fatalf("GetProvider() error: %v", err)
		}
		if pc.BaseURL != "https://api.openai.com/v1" {
			t.Errorf("BaseURL = %q, want https://api.openai.com/v1", pc.BaseURL)
		}
	})

	t.Run("missing provider", func(t *testing.T) {
		_, err := cfg.GetProvider("nonexistent")
		if err == nil {
			t.Error("GetProvider(nonexistent) = nil, want error")
		}
	})
}

func TestModelSet_via_env(t *testing.T) {
	resetViper()
	t.Setenv("CLAI_MODEL", "openai/gpt-4o")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.ModelSet {
		t.Error("ModelSet = false, want true when CLAI_MODEL is set")
	}
}

func TestModelSet_not_set(t *testing.T) {
	resetViper()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ModelSet {
		t.Error("ModelSet = true, want false when model is not configured")
	}
}

func TestTemperatureZero_via_env(t *testing.T) {
	resetViper()
	t.Setenv("CLAI_TEMPERATURE", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Temperature == nil {
		t.Fatal("Temperature = nil, want *0.0 when CLAI_TEMPERATURE=0")
	}
	if *cfg.Temperature != 0.0 {
		t.Errorf("Temperature = %v, want 0.0", *cfg.Temperature)
	}
}

func TestTemperature_outOfRange(t *testing.T) {
	tests := []struct {
		name string
		val  string
	}{
		{"below zero", "-0.1"},
		{"above two", "2.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()
			t.Setenv("CLAI_TEMPERATURE", tt.val)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want out-of-range error for temperature=%s", tt.val)
			}
			if !strings.Contains(err.Error(), "out of range") {
				t.Errorf("Load() error = %v, want error mentioning 'out of range'", err)
			}
		})
	}
}
