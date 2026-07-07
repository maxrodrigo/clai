// Package config handles configuration loading and merging from multiple sources.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all resolved settings. Fields are set by merging sources
// in order of precedence (lowest to highest):
//
//  1. Built-in defaults
//  2. Prompt frontmatter defaults (applied by prompt.MergeDefaults)
//  3. User config (~/.config/clai/config.toml)
//  4. Project config (.clai/config.toml)
//  5. Environment variables (CLAI_*)
//  6. CLI flags
type Config struct {
	Model           string                    `mapstructure:"model"`
	ModelSet        bool                      // true if explicitly set by user (config/env/flag)
	Temperature     *float64                  // nil means use provider default
	MaxTokens       int                       `mapstructure:"max_tokens"`
	Think           bool                      `mapstructure:"think"`
	ThinkBudget     int                       `mapstructure:"think_budget"`
	Strategy        string                    `mapstructure:"strategy"`
	ContinueOnError bool                      `mapstructure:"continue_on_error"`
	Providers       map[string]ProviderConfig `mapstructure:"providers"`
}

// ProviderConfig holds settings for a single AI provider.
type ProviderConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Timeout int    `mapstructure:"timeout"`
}

// v is the package-level viper instance.
var v = viper.New()

// initOnce ensures config paths are added to viper only once.
var initOnce sync.Once

// defaultProviders defines built-in provider configurations.
// API keys use ${ENV_VAR} syntax expanded at load time via os.ExpandEnv.
//
//nolint:gosec // G101 false positive: these are env var placeholders, not credentials
var defaultProviders = map[string]ProviderConfig{
	"ollama": {
		BaseURL: "http://localhost:11434/v1",
	},
	"openai": {
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "${OPENAI_API_KEY}",
	},
	"anthropic": {
		BaseURL: "https://api.anthropic.com",
		APIKey:  "${ANTHROPIC_API_KEY}",
	},
	"bedrock": {
		BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
		APIKey:  "${AWS_BEARER_TOKEN_BEDROCK}",
	},
}

func init() {
	// Bind model to env explicitly (no default value — empty means unconfigured).
	_ = v.BindEnv("model")

	for name, pc := range defaultProviders {
		v.SetDefault("providers."+name+".base_url", pc.BaseURL)
		v.SetDefault("providers."+name+".api_key", pc.APIKey)
	}

	v.SetEnvPrefix("CLAI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
}

// BindFlags binds cobra flags to viper. Call this after flags are defined.
func BindFlags(cmd *cobra.Command) {
	_ = v.BindPFlag("model", cmd.PersistentFlags().Lookup("model"))
	_ = v.BindPFlag("temperature", cmd.PersistentFlags().Lookup("temperature"))
	_ = v.BindPFlag("max_tokens", cmd.PersistentFlags().Lookup("max-tokens"))
	_ = v.BindPFlag("think", cmd.PersistentFlags().Lookup("think"))
	_ = v.BindPFlag("strategy", cmd.PersistentFlags().Lookup("strategy"))
	_ = v.BindPFlag("schema", cmd.PersistentFlags().Lookup("schema"))
	_ = v.BindPFlag("dry-run", cmd.PersistentFlags().Lookup("dry-run"))
	_ = v.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))
	_ = v.BindPFlag("no-color", cmd.PersistentFlags().Lookup("no-color"))
	_ = v.BindPFlag("color", cmd.PersistentFlags().Lookup("color"))
}

// Load resolves config from all sources.
func Load() (*Config, error) {
	userDir := Dir()

	// Configure config file paths only once, even if Load is called multiple times.
	initOnce.Do(func() {
		v.SetConfigName("config")
		v.SetConfigType("toml")
		v.AddConfigPath(userDir)
		v.AddConfigPath(".clai")
	})

	// Read user config (errors are not fatal if file doesn't exist).
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	// Merge project config if it exists. We read the file manually to avoid
	// viper's SetConfigFile which has side effects on subsequent ReadInConfig calls.
	projectConfig := filepath.Join(".clai", "config.toml")
	if data, err := os.ReadFile(projectConfig); err == nil {
		if err := v.MergeConfig(bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("reading project config (.clai/config.toml): %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading project config (.clai/config.toml): %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.ModelSet = v.IsSet("model")

	// Resolve temperature: only set if the user explicitly configured it
	// (via flag, env, or config file). nil means "use provider default".
	if v.IsSet("temperature") {
		t := v.GetFloat64("temperature")
		cfg.Temperature = &t
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}

	expandEnvRefs(&cfg)

	return &cfg, nil
}

// expandEnvRefs expands "${ENV_VAR}" references in provider configs.
func expandEnvRefs(cfg *Config) {
	for name, pc := range cfg.Providers {
		pc.APIKey = os.ExpandEnv(pc.APIKey)
		pc.BaseURL = os.ExpandEnv(pc.BaseURL)
		cfg.Providers[name] = pc
	}
}

// Dir returns ~/.config/clai, respecting XDG_CONFIG_HOME.
func Dir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "clai")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "clai")
	}
	return filepath.Join(home, ".config", "clai")
}

// ModelProvider returns the provider prefix from "provider/model-name".
// Returns "" if the model string has no "/" or either side is empty.
func (c *Config) ModelProvider() string {
	provider, model, found := strings.Cut(c.Model, "/")
	if !found || provider == "" || model == "" {
		return ""
	}
	return provider
}

// ModelName returns the model name without provider prefix.
// Returns the whole string if there is no "/" separator.
func (c *Config) ModelName() string {
	_, name, found := strings.Cut(c.Model, "/")
	if found {
		return name
	}
	return c.Model
}

// GetProvider returns the ProviderConfig for the given name.
func (c *Config) GetProvider(name string) (ProviderConfig, error) {
	pc, ok := c.Providers[name]
	if !ok {
		return ProviderConfig{}, fmt.Errorf(
			"unknown provider: %s (add [providers.%s] to your config)", name, name,
		)
	}
	return pc, nil
}

// Flag accessors — expose bound flag values without leaking viper.

// GetBool returns a boolean value from the resolved config (flags + env + file).
func GetBool(key string) bool { return v.GetBool(key) }

// GetString returns a string value from the resolved config (flags + env + file).
func GetString(key string) string { return v.GetString(key) }
