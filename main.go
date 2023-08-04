package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	op "github.com/sashabaranov/go-openai"
)

// https://platform.openai.com/docs/api-reference/chat

var (
	openWeatherMapAPIKEY string
	openAIAPIKEY         string
)

func geocode(location string) ([]byte, error) {
	url := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&format=json", location)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func weather(lat, lon string) ([]byte, error) {
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?units=metric&lat=$%s&lon=%s&appid=%s", lat, lon, openWeatherMapAPIKEY)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func chat(messages []op.ChatCompletionMessage) (op.ChatCompletionResponse, error) {
	client := op.NewClient(openAIAPIKEY)
	paramGeocode := json.RawMessage([]byte(`{
		"type": "object",
		"required": [
			"location"
		],
		"properties": {
			"location": {
				"type": "string",
				"description": "The city, e.g. New York"
			}
		}
	}`))

	paramWeather := json.RawMessage([]byte(`{
		"type": "object",
		"required": [
			"latitude",
			"longitude"
		],
		"properties": {
			"latitude": {
				"type": "number",
				"description": "The latitude"
			},
			"longitude": {
				"type": "number",
				"description": "The longitude"
			}
		}
	}`))

	return client.CreateChatCompletion(
		context.Background(),
		op.ChatCompletionRequest{
			Model:    op.GPT3Dot5Turbo,
			Messages: messages,
			Functions: []op.FunctionDefinition{
				{
					Name:        "geocode",
					Description: "Get the latitude and longitude of a location",
					Parameters:  paramGeocode,
				},
				{
					Name:        "weather",
					Description: "Get the current weather in a given location",
					Parameters:  paramWeather,
				},
			},
		},
	)
}

func invokeFunction(name, argsIn string) ([]byte, error) {
	args := make(map[string]interface{})
	err := json.Unmarshal([]byte(argsIn), &args)

	fmt.Println("DEBUG invoke function ", name, args)

	if err != nil {
		return nil, err
	}
	switch name {
	case "geocode":
		return geocode(args["location"].(string))
	case "weather":
		return weather(args["latitude"].(string), args["longitude"].(string))
	}

	return nil, nil
}

func converse(messages []op.ChatCompletionMessage) (string, error) {
	if len(messages) > 13 {
		return "", errors.New("too in-depth conversation")
	}

	resp, err := chat(messages)
	if err != nil {
		return "", fmt.Errorf("failed chat: %v", err)
	}

	for i, choice := range resp.Choices {
		fmt.Printf("DEBUG choice %d: %+v\n", i, choice.Message)
	}

	message := resp.Choices[0].Message
	if functionCall := message.FunctionCall; functionCall != nil {
		resp, err := invokeFunction(functionCall.Name, functionCall.Arguments)
		if err != nil {
			return "", fmt.Errorf("failed invoke function: %v", err)
		}
		newMessages := append([]op.ChatCompletionMessage{}, messages...)
		newMessages = append(newMessages, message)
		newMessages = append(newMessages, op.ChatCompletionMessage{
			Role:    op.ChatMessageRoleFunction,
			Name:    functionCall.Name,
			Content: string(resp),
		})

		return converse(newMessages)
	}

	return message.Content, nil
}

func query(inquiry string) (string, error) {
	return converse([]op.ChatCompletionMessage{
		{
			Role:    "system",
			Content: "Only use the functions you have been provided with.",
		},
		{
			Role:    "system",
			Content: "Only answer in 50 words or less.",
		},
		{
			Role:    "user",
			Content: inquiry,
		},
	})
}

func init() {
	godotenv.Load(".env")
	openWeatherMapAPIKEY = os.Getenv("OPENWEATHERMAP_API_KEY")
	openAIAPIKEY = os.Getenv("OPENAI_API_KEY")
}

func main() {
	inquiry := ""
	if len(os.Args) > 1 {
		inquiry = strings.Join(os.Args[1:], " ")
	}
	if len(inquiry) < 2 {
		log.Fatal("Supply some inquiry!")
	}
	answer, err := query(inquiry)
	if err != nil {
		log.Fatalf("Failed query answer: %v", err)
	}
	fmt.Println(answer)
}
