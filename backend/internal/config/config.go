package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"github.com/wins/jaz/backend/internal/app"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	openrouterprovider "github.com/wins/jaz/backend/internal/provider/openrouter"
)

type Config struct {
	Jaz        app.Config
	Providers  ProvidersConfig
	OpenRouter openrouterprovider.Config
	OpenAI     openaiprovider.Config
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

	if configFile := os.Getenv("APPLICATION_CONFIG"); configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("application")
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	bindEnv()
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	return nil
}

func bindEnv() {
	_ = viper.BindEnv("openai.apikey", "OPENAI_API_KEY", "OPENAI_APIKEY")
	_ = viper.BindEnv("openai.model", "OPENAI_MODEL")
	_ = viper.BindEnv("openrouter.apikey", "OPENROUTER_API_KEY", "OPENROUTER_APIKEY")
	_ = viper.BindEnv("openrouter.model", "OPENROUTER_MODEL")
	_ = viper.BindEnv("providers.default", "JAZ_PROVIDER")
}

func applyProvider(cfg *Config) error {
	switch strings.ToLower(cfg.Providers.Default) {
	case "", "openai":
		cfg.Jaz.Provider = app.ProviderConfig{
			Type:   "openai",
			APIKey: cfg.OpenAI.APIKey,
			Model:  cfg.OpenAI.Model,
		}
	case "openrouter":
		cfg.Jaz.Provider = app.ProviderConfig{
			Type:   "openrouter",
			APIKey: cfg.OpenRouter.APIKey,
			Model:  cfg.OpenRouter.Model,
		}
	case "mock":
		cfg.Jaz.Provider = app.ProviderConfig{Type: "mock"}
	default:
		return fmt.Errorf("unknown default provider %q; valid providers are openai, openrouter, mock", cfg.Providers.Default)
	}
	return nil
}
