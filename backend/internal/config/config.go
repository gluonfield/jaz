package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	openrouterprovider "github.com/wins/jaz/backend/internal/provider/openrouter"
)

type Config struct {
	Jaz        app.Config
	Providers  ProvidersConfig
	OpenRouter openrouterprovider.Config
	OpenAI     openaiprovider.Config
	Anthropic  AnthropicConfig
	Mistral    app.MistralConfig
	TTS        app.SpeechConfig
	STT        app.SpeechConfig
}

type ProvidersConfig struct {
	Default string
}

type AnthropicConfig struct {
	APIKey          string
	Model           string
	ReasoningEffort string
}

func Load() (Config, error) {
	cfg, err := LoadConfig[Config]()
	if err != nil {
		return Config{}, err
	}
	if err := applyProvider(&cfg); err != nil {
		return Config{}, err
	}
	cfg.Jaz.Voice = app.VoiceConfig{TTS: cfg.TTS, STT: cfg.STT, Mistral: cfg.Mistral}
	if cfg.Jaz.Voice.Mistral.APIKey == "" {
		cfg.Jaz.Voice.Mistral.APIKey = os.Getenv("MISTRAL_API_KEY")
	}
	return cfg, nil
}

func LoadConfig[T any]() (T, error) {
	if err := Init(); err != nil {
		var zero T
		return zero, err
	}
	var cfg T
	if err := viper.Unmarshal(&cfg); err != nil {
		var zero T
		return zero, fmt.Errorf("unable to decode config: %w", err)
	}
	return cfg, nil
}

func Init() error {
	_ = godotenv.Load()

	viper.SetDefault("providers.default", "openrouter")
	viper.SetDefault("openrouter.model", "openai/gpt-5.4-mini")
	viper.SetDefault("openrouter.reasoningeffort", "medium")
	viper.SetDefault("openai.model", "gpt-5.4-mini")
	viper.SetDefault("openai.reasoningeffort", "medium")
	viper.SetDefault("anthropic.model", "claude-sonnet-4-5")
	viper.SetDefault("anthropic.reasoningeffort", "medium")

	explicitConfig := false
	if configFile := os.Getenv("APPLICATION_CONFIG"); configFile != "" {
		explicitConfig = true
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("application")
		viper.AddConfigPath(".")
		viper.AddConfigPath("backend")
		viper.SetConfigType("yaml")
	}
	_ = viper.BindEnv("openai.apikey", "OPENAI_API_KEY")
	_ = viper.BindEnv("openai.model", "OPENAI_MODEL")
	_ = viper.BindEnv("openai.reasoningeffort", "OPENAI_REASONING_EFFORT")
	_ = viper.BindEnv("openrouter.apikey", "OPENROUTER_API_KEY")
	_ = viper.BindEnv("openrouter.model", "OPENROUTER_MODEL")
	_ = viper.BindEnv("openrouter.reasoningeffort", "OPENROUTER_REASONING_EFFORT")
	_ = viper.BindEnv("anthropic.apikey", "ANTHROPIC_API_KEY")
	_ = viper.BindEnv("anthropic.model", "ANTHROPIC_MODEL")
	_ = viper.BindEnv("anthropic.reasoningeffort", "ANTHROPIC_REASONING_EFFORT")
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !explicitConfig && errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("error reading config file: %w", err)
	}
	return nil
}

func applyProvider(cfg *Config) error {
	openAIEffort, err := provider.NormalizeReasoningEffort(cfg.OpenAI.ReasoningEffort)
	if err != nil {
		return err
	}
	openRouterEffort, err := provider.NormalizeReasoningEffort(cfg.OpenRouter.ReasoningEffort)
	if err != nil {
		return err
	}
	anthropicEffort, err := provider.NormalizeReasoningEffort(cfg.Anthropic.ReasoningEffort)
	if err != nil {
		return err
	}
	cfg.Jaz.ModelProviders = map[string]app.ProviderConfig{
		"openai": {
			Type:            "openai",
			BaseURL:         nativeProviderBaseURL("openai"),
			APIKey:          cfg.OpenAI.APIKey,
			Model:           cfg.OpenAI.Model,
			ReasoningEffort: openAIEffort,
		},
		"openrouter": {
			Type:            "openrouter",
			BaseURL:         nativeProviderBaseURL("openrouter"),
			APIKey:          cfg.OpenRouter.APIKey,
			Model:           cfg.OpenRouter.Model,
			ReasoningEffort: openRouterEffort,
		},
		"anthropic": {
			Type:            "anthropic",
			BaseURL:         nativeProviderBaseURL("anthropic"),
			APIKey:          cfg.Anthropic.APIKey,
			Model:           cfg.Anthropic.Model,
			ReasoningEffort: anthropicEffort,
		},
	}
	switch strings.ToLower(cfg.Providers.Default) {
	case "", "openai":
		cfg.Jaz.Provider = cfg.Jaz.ModelProviders["openai"]
	case "openrouter":
		cfg.Jaz.Provider = cfg.Jaz.ModelProviders["openrouter"]
	case "anthropic":
		cfg.Jaz.Provider = cfg.Jaz.ModelProviders["anthropic"]
	case "mock":
		cfg.Jaz.Provider = app.ProviderConfig{Type: "mock"}
	default:
		return fmt.Errorf("unknown default provider %q; valid providers are openai, openrouter, anthropic, mock", cfg.Providers.Default)
	}
	return nil
}

func nativeProviderBaseURL(id string) string {
	meta, _ := provider.NativeProviderByID(id)
	return meta.BaseURL
}
