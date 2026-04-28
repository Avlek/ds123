package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/openai/openai-go/shared"
	"github.com/spf13/viper"
)

type Config struct {
	APIKey           string `mapstructure:"DEEPSEEK_API_KEY"`
	YandexWeatherKey string `mapstructure:"YANDEX_WEATHR_API_KEY"`
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
	config   *Config
	mu       sync.Mutex
	sessions map[string]*Session
}

var tools = []openai.ChatCompletionToolParam{
	{
		Function: shared.FunctionDefinitionParam{
			Name:        "get_current_time",
			Description: openai.String("Возвращает текущее время на сервере"),
		},
		Type: "function",
	}, {
		Function: shared.FunctionDefinitionParam{
			Name:        "get_weather",
			Description: openai.String("Возвращает текущую погоду в указанном городе по координатам"),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"latitude": map[string]any{
						"type":        "number",
						"description": "Широта города",
					},
					"longitude": map[string]any{
						"type":        "number",
						"description": "Долгота города",
					}},
			},
		},
		Type: "function",
	},
}

type FactWeather struct {
	Temp      float64 `json:"temp"`
	WindSpeed float64 `json:"wind_speed"`
	Humidity  float64 `json:"humidity"`
}

type WeatherResponse struct {
	Fact FactWeather `json:"fact"`
}

type Session struct {
	mu       sync.Mutex
	messages []openai.ChatCompletionMessageParamUnion
}

func getCurrentTime() time.Time {
	return time.Now()
}

func getWeather(ctx context.Context, key string, lat, lon float64) (*WeatherResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.weather.yandex.ru/v2/forecast?lat=%f&lon=%f", lat, lon), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Yandex-Weather-Key", key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body = http.MaxBytesReader(nil, resp.Body, 1<<20)

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Yandex returned %d: %s", resp.StatusCode, body)
	}

	var response WeatherResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
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
	answer := ""

	for range 5 {
		params := openai.ChatCompletionNewParams{
			Messages: messages,
			Tools:    tools,
			Model:    "deepseek-chat",
		}
		chatCompletion, err := ch.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", err
		}
		if len(chatCompletion.Choices[0].Message.ToolCalls) > 0 {
			messages = append(messages, chatCompletion.Choices[0].Message.ToParam())
			var result any
			for _, call := range chatCompletion.Choices[0].Message.ToolCalls {
				switch call.Function.Name {
				case "get_current_time":
					result = getCurrentTime()
				case "get_weather":
					var args struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
					}
					err = json.Unmarshal([]byte(call.Function.Arguments), &args)
					if err != nil {
						result = map[string]string{
							"error": "не смог прочитать аргументы" + err.Error(),
						}
						break
					}
					result, err = getWeather(ctx, ch.config.YandexWeatherKey, args.Latitude, args.Longitude)
					if err != nil {
						result = map[string]string{
							"error": "не смог получить данные" + err.Error(),
						}
					}
				default:
					result = map[string]string{
						"error": "такая функция не найдена",
					}
				}

				content, _ := json.Marshal(result)
				messages = append(messages, openai.ToolMessage(string(content), call.ID))
			}
			continue
		}
		answer = chatCompletion.Choices[0].Message.Content
		sess.messages = append(slices.Clone(messages), openai.AssistantMessage(answer))
		break
	}

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
		config:   config,
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
