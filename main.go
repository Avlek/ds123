package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
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

type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	Reply string `json:"reply"`
}

type Chat struct {
	client   openai.Client
	mu       sync.Mutex
	sessions map[string]*Session
}

type Session struct {
	mu       sync.Mutex
	messages []openai.ChatCompletionMessageParamUnion
}

func (ch *Chat) SendMessage(ctx context.Context, sessionID string, message string) (string, error) {
	ch.mu.Lock()
	sess, ok := ch.sessions[sessionID]
	if !ok {
		sess = &Session{
			mu:       sync.Mutex{},
			messages: []openai.ChatCompletionMessageParamUnion{},
		}
		ch.sessions[sessionID] = sess
	}
	ch.mu.Unlock()

	sess.mu.Lock()
	defer sess.mu.Unlock()

	userMsg := openai.UserMessage(message)
	messages := append(slices.Clone(sess.messages), userMsg)

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    "deepseek-chat",
	}
	chatCompletion, err := ch.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}
	answer := chatCompletion.Choices[0].Message.Content
	sess.messages = append(sess.messages, userMsg, openai.AssistantMessage(answer))

	return answer, nil
}

func (ch *Chat) Stream() {
	var messages []openai.ChatCompletionMessageParamUnion
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			break
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		messages = append(messages, openai.UserMessage(line))
		params := openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    "deepseek-chat",
		}

		answer := ""
		stream := ch.client.Chat.Completions.NewStreaming(context.Background(), params)
		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) > 0 {
				chunkText := chunk.Choices[0].Delta.Content
				fmt.Print(chunkText)
				answer += chunkText
			}
		}

		if err := stream.Err(); err != nil {
			fmt.Println(err)
			continue
		}

		messages = append(messages, openai.AssistantMessage(answer))
		fmt.Println()
	}
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()
	defer e.Close()

	chat := Chat{
		client: openai.NewClient(
			option.WithAPIKey(config.APIKey),
			option.WithBaseURL("https://api.deepseek.com"),
		),
		mu:       sync.Mutex{},
		sessions: make(map[string]*Session),
	}

	e.POST("/chat", func(c echo.Context) error {
		var params ChatRequest
		err2 := c.Bind(&params)
		if err2 != nil {
			return c.NoContent(http.StatusBadRequest)
		}

		reply, err2 := chat.SendMessage(c.Request().Context(), params.SessionID, params.Message)
		if err2 != nil {
			return c.NoContent(http.StatusInternalServerError)
		}

		return c.JSON(http.StatusOK, ChatResponse{Reply: reply})
	})

	ctx2, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	serverErr := make(chan error, 1)
	go func() {
		if err2 := e.Start(":8080"); err2 != nil && !errors.Is(err2, http.ErrServerClosed) {
			serverErr <- err2
		}
		close(serverErr)
	}()

	select {
	case <-ctx2.Done():
		log.Println("signal received, shutting down")
	case err2 := <-serverErr:
		log.Println("server crashed:", err2)
	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err = e.Shutdown(shutdownCtx); err != nil {
		log.Println("shutdown error:", err)
	}

	//chat.Stream()
}
