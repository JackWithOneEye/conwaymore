package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type env struct {
	DBUrl          string        `mapstructure:"DB_URL"`
	Port           uint          `mapstructure:"PORT"`
	WorldSize      uint          `mapstructure:"WORLD_SIZE"`
	MaxGames       int           `mapstructure:"MAX_GAMES"`
	MaxLoadedGames int           `mapstructure:"MAX_LOADED_GAMES"`
	IdleTimeout    time.Duration `mapstructure:"IDLE_TIMEOUT"`
}

type Config struct {
	env *env
}

var cfgInstance *Config

func NewConfig() *Config {
	if cfgInstance != nil {
		return cfgInstance
	}

	viper.SetDefault("DB_URL", "./test.db")
	viper.SetDefault("PORT", 8080)
	viper.SetDefault("WORLD_SIZE", 1024)
	viper.SetDefault("MAX_GAMES", 50)
	viper.SetDefault("MAX_LOADED_GAMES", 20)
	viper.SetDefault("IDLE_TIMEOUT", 5*time.Minute)

	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Sprintf("error loading config: %s", err))
	}

	var env env
	err = viper.Unmarshal(&env)
	if err != nil {
		panic(fmt.Sprintf("error unmarshaling config: %s", err))
	}
	cfgInstance = &Config{&env}
	return cfgInstance
}

func (c *Config) DBUrl() string {
	return c.env.DBUrl
}

func (c *Config) Port() uint {
	return c.env.Port
}

func (c *Config) WorldSize() uint {
	return c.env.WorldSize
}

func (c *Config) MaxGames() int {
	return c.env.MaxGames
}

func (c *Config) MaxLoadedGames() int {
	return c.env.MaxLoadedGames
}

func (c *Config) IdleTimeout() time.Duration {
	return c.env.IdleTimeout
}
