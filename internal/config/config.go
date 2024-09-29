package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type env struct {
	DBUrl     string `mapstructure:"DB_URL"`
	Port      uint   `mapstructure:"PORT"`
	WorldSize uint   `mapstructure:"WORLD_SIZE"`
}

type Config struct {
	env *env
}

var cfgInstance *Config

func NewConfig() *Config {
	if cfgInstance != nil {
		return cfgInstance
	}

	viper.AddConfigPath(".")
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
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
