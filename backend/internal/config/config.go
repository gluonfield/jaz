package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	openrouterprovider "github.com/wins/jaz/backend/internal/provider/openrouter"
)

type Config struct {
	Jaz        app.Config
	OpenRouter openrouterprovider.Config
	OpenAI     openaiprovider.Config
	Mistral    app.MistralConfig
	TTS        app.SpeechConfig
	STT        app.SpeechConfig
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
	_ = viper.BindEnv("openrouter.apikey", "OPENROUTER_API_KEY")
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
	cfg.Jaz.ModelProviders = map[string]app.ProviderConfig{
		"openai": {
			Type:    "openai",
			BaseURL: nativeProviderBaseURL("openai"),
			APIKey:  cfg.OpenAI.APIKey,
		},
		"openrouter": {
			Type:    "openrouter",
			BaseURL: nativeProviderBaseURL("openrouter"),
			APIKey:  cfg.OpenRouter.APIKey,
		},
	}
	return nil
}

func nativeProviderBaseURL(id string) string {
	meta, _ := provider.NativeProviderByID(id)
	return meta.BaseURL
}
