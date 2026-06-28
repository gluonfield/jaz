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
	_ = viper.BindEnv("jaz.skills.disablesync", "JAZ_SKILLS_DISABLE_SYNC")
	_ = viper.BindEnv("jaz.devices.disablepairing", "JAZ_DISABLE_PAIRING")
	_ = viper.BindEnv("jaz.connections.gmail.oauthclientid", "JAZ_GMAIL_OAUTH_CLIENT_ID")
	_ = viper.BindEnv("jaz.connections.gmail.oauthclientsecret", "JAZ_GMAIL_OAUTH_CLIENT_SECRET")
	viper.SetDefault("jaz.connections.chat.grouphistorylimit", app.DefaultChatGroupHistoryLimit)
	_ = viper.BindEnv("jaz.connections.chat.grouphistorylimit", "JAZ_CHAT_GROUP_HISTORY_LIMIT")
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
	if cfg.Jaz.ModelProviders == nil {
		cfg.Jaz.ModelProviders = map[string]provider.ModelProviderConfig{}
	}
	mergeProviderConfig(cfg.Jaz.ModelProviders, provider.ProviderOpenAI, provider.ModelProviderConfig{
		Type:    provider.ProviderOpenAI,
		BaseURL: modelProviderBaseURL(provider.ProviderOpenAI),
		APIKey:  cfg.OpenAI.APIKey,
	})
	mergeProviderConfig(cfg.Jaz.ModelProviders, provider.ProviderOpenRouter, provider.ModelProviderConfig{
		Type:    provider.ProviderOpenRouter,
		BaseURL: modelProviderBaseURL(provider.ProviderOpenRouter),
		APIKey:  cfg.OpenRouter.APIKey,
	})
	return nil
}

func mergeProviderConfig(providers map[string]provider.ModelProviderConfig, id string, defaults provider.ModelProviderConfig) {
	current := providers[id]
	if current.Type == "" {
		current.Type = defaults.Type
	}
	if current.BaseURL == "" {
		current.BaseURL = defaults.BaseURL
	}
	if current.APIKey == "" {
		current.APIKey = defaults.APIKey
	}
	providers[id] = current
}

func modelProviderBaseURL(id string) string {
	meta, _ := provider.RunnableModelProviderByID(id)
	return meta.BaseURL
}
