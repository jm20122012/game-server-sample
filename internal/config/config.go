package config

import (
	"github.com/spf13/viper"
)

type ServerConfig struct {
	DebugLevel string `mapstructure:"debugLevel"`
	ServerPort string `mapstructure:"serverPort"`
}

func LoadConfig() (ServerConfig, error) {
	var cfg ServerConfig

	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		return ServerConfig{}, err
	}

	err = viper.Unmarshal(&cfg)
	if err != nil {
		return ServerConfig{}, err
	}

	return cfg, nil
}
