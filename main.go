package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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

	var messages []openai.ChatCompletionMessageParamUnion
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			break
		}
		messages = append(messages, openai.UserMessage(line))
		params := openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    "deepseek-chat",
		}

		chatCompletion, err := client.Chat.Completions.New(context.Background(), params)
		if err != nil {
			fmt.Println(err)
			continue
		}

		answer := chatCompletion.Choices[0].Message.Content
		messages = append(messages, openai.SystemMessage(answer))
		fmt.Println(answer)
	}
}
