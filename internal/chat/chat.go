package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/avlek/ds123/internal/storage"
	"github.com/avlek/ds123/internal/tools"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Request struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type Response struct {
	Reply string `json:"reply"`
}

type Chat struct {
	Client        openai.Client
	weatherAPIKey string
	mu            sync.Mutex
	sessionLocks  map[string]*sync.Mutex
	Storage       *storage.Storage
}

func New(apiKey, weatherAPIKey string, storage *storage.Storage) *Chat {
	return &Chat{
		Client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL("https://api.deepseek.com"),
		),
		weatherAPIKey: weatherAPIKey,
		mu:            sync.Mutex{},
		sessionLocks:  make(map[string]*sync.Mutex),
		Storage:       storage,
	}
}

func (ch *Chat) SendMessage(ctx context.Context, sessionID string, message string) (string, error) {
	ch.mu.Lock()
	sess, ok := ch.sessionLocks[sessionID]
	if !ok {
		sess = &sync.Mutex{}
		ch.sessionLocks[sessionID] = sess
	}
	ch.mu.Unlock()

	sess.Lock()
	defer sess.Unlock()

	m, err := ch.Storage.GetMessages(ctx, sessionID)
	if err != nil {
		return "", err
	}

	var sessionMessages []openai.ChatCompletionMessageParamUnion
	err = json.Unmarshal(m, &sessionMessages)
	if err != nil {
		return "", err
	}

	userMsg := openai.UserMessage(message)
	messages := append(slices.Clone(sessionMessages), userMsg)
	answer := ""
	good := false

	for range 5 {
		params := openai.ChatCompletionNewParams{
			Messages: messages,
			Tools:    tools.List,
			Model:    "deepseek-chat",
		}
		chatCompletion, err := ch.Client.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", err
		}
		if len(chatCompletion.Choices[0].Message.ToolCalls) > 0 {
			messages = append(messages, chatCompletion.Choices[0].Message.ToParam())
			var result any
			for _, call := range chatCompletion.Choices[0].Message.ToolCalls {
				switch call.Function.Name {
				case "get_current_time":
					result = tools.GetCurrentTime()
				case "get_weather":
					var args struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
					}
					err = json.Unmarshal([]byte(call.Function.Arguments), &args)
					if err != nil {
						result = map[string]string{
							"error": fmt.Sprintf("не смог прочитать аргументы: %s", err.Error()),
						}
						break
					}
					result, err = tools.GetWeather(ctx, ch.weatherAPIKey, args.Latitude, args.Longitude)
					if err != nil {
						result = map[string]string{
							"error": fmt.Sprintf("не смог получить данные: %s", err.Error()),
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
		sessionMessages = append(slices.Clone(messages), openai.AssistantMessage(answer))
		good = true
		break
	}

	if !good {
		return "", fmt.Errorf("модель не вернула финальный ответ за 5 итераций")
	}

	data, err := json.Marshal(sessionMessages)
	if err != nil {
		return "", err
	}
	err = ch.Storage.SaveMessages(ctx, sessionID, data)
	if err != nil {
		return "", err
	}

	return answer, nil
}
