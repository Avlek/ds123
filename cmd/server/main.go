package main

import (
	"context"
	"log"

	"github.com/avlek/ds123/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spf13/viper"
)

type Config struct {
	APIKey             string `mapstructure:"DEEPSEEK_API_KEY"`
	YandexWeatherKey   string `mapstructure:"YANDEX_WEATHR_API_KEY"`
	DBConnectionString string `mapstructure:"DB_CONNECTION_STRING"`
}

func LoadConfig() (*Config, error) {
	var config Config

	viper.SetConfigFile(".env")
	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}

	err = viper.Unmarshal(&config)
	return &config, err
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := pgxpool.New(context.Background(), config.DBConnectionString)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	srv := server.New(pool, config.APIKey, config.YandexWeatherKey)
	log.Fatal(srv.Run())
}
