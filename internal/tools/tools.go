package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

var List = []openai.ChatCompletionToolParam{
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

func GetCurrentTime() time.Time {
	return time.Now()
}

func GetWeather(ctx context.Context, key string, lat, lon float64) (*WeatherResponse, error) {
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
