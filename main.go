package main

import (
	"context"
	"fmt"
	"log"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/spf13/viper"
)

type Config struct {
	APIKey string `mapstructure:"DEEPSEEK_API_KEY"`
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

	client := openai.NewClient(
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL("https://api.deepseek.com"),
	)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello, DeepSeek!"),
		},
		Model: "deepseek-chat",
	}

	chatCompletion, err := client.Chat.Completions.New(context.Background(), params)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(chatCompletion.Choices[0].Message.Content)
}
