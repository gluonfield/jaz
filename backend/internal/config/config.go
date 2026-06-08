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
	Mistral    app.MistralConfig
	TTS        app.SpeechConfig
	STT        app.SpeechConfig
}

type ProvidersConfig struct {
	Default string
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
	viper.SetDefault("openai.model", "gpt-5.4-mini")

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
	switch strings.ToLower(cfg.Providers.Default) {
	case "", "openai":
		effort, err := provider.NormalizeReasoningEffort(cfg.OpenAI.ReasoningEffort)
		if err != nil {
			return err
		}
		cfg.Jaz.Provider = app.ProviderConfig{
			Type:            "openai",
			APIKey:          cfg.OpenAI.APIKey,
			Model:           cfg.OpenAI.Model,
			ReasoningEffort: effort,
		}
	case "openrouter":
		effort, err := provider.NormalizeReasoningEffort(cfg.OpenRouter.ReasoningEffort)
		if err != nil {
			return err
		}
		cfg.Jaz.Provider = app.ProviderConfig{
			Type:            "openrouter",
			APIKey:          cfg.OpenRouter.APIKey,
			Model:           cfg.OpenRouter.Model,
			ReasoningEffort: effort,
		}
	case "mock":
		cfg.Jaz.Provider = app.ProviderConfig{Type: "mock"}
	default:
		return fmt.Errorf("unknown default provider %q; valid providers are openai, openrouter, mock", cfg.Providers.Default)
	}
	return nil
}
