package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/ollama"
	server "github.com/CHESSComputing/golib/server"
)

// AIChat represents generic AI chat interface
type AIChat interface {
	Chat(prompt string) (string, error)
}

// OllamaClient represents Ollama AI client
type OllamaClient struct {
	AIConfig *ollama.Config
}

// Chat implementation for OllamaClient
func (o *OllamaClient) Chat(prompt string) (string, error) {
	if o.AIConfig == nil {
		o.AIConfig = &ollama.Config{
			Host:  srvConfig.Config.AIChat.Host,
			Port:  srvConfig.Config.AIChat.Port,
			Model: srvConfig.Config.AIChat.Model,
		}
		log.Printf("configure ai chat with %+v", o.AIConfig)
	}

	client := ollama.NewClient(*o.AIConfig)
	ctx := context.Background()
	stream := true
	response, err := client.SendRequest(ctx, prompt, stream, nil)
	if err != nil {
		return "", err
	}
	return response, nil
}

// TichyClient represents tichy AI client
type TichyClient struct {
}

// Chat implementation for TichyClient
func (o *TichyClient) Chat(prompt string) (string, error) {

	ctx := context.Background()
	if srvConfig.Config.AIChat.Timeout != 0 {
		ctx1, cancel := context.WithTimeout(
			context.Background(),
			time.Duration(srvConfig.Config.AIChat.Timeout)*time.Second)
		defer cancel()
		ctx = ctx1
	}
	resp, err := aitichy(ctx, prompt)
	return resp, err
}

// Request message structure
type TichyMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request payload structure
type TichyRequest struct {
	Messages []TichyMessage `json:"messages"`
}

// Response structures
type TichyResponse struct {
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func aitichy(ctx context.Context, query string) (string, error) {
	// Build request payload
	reqBody := TichyRequest{
		Messages: []TichyMessage{
			{Role: "user", Content: query},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send POST request
	rurl := fmt.Sprintf("http://%s:%v/v1/chat/completions",
		srvConfig.Config.AIChat.Host,
		srvConfig.Config.AIChat.Port)
	req, err := http.NewRequestWithContext(ctx, "POST", rurl, bytes.NewReader(data))
	if err != nil {
		log.Println("ERROR:", err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("ERROR:", err)
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("ERROR:", err)
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var chatResp TichyResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return server.MDStringToHTML(chatResp.Choices[0].Message.Content), nil
}
// wrapper AI chat function to use different AI backend engine
func aichat(prompt string) (string, error) {
	if srvConfig.Config.AIChat.Client == "ollama" {
		client := OllamaClient{}
		return client.Chat(prompt)
	} else if srvConfig.Config.AIChat.Client == "tichy" {
		client := TichyClient{}
		return client.Chat(prompt)
	}
	msg := "FOXDEN is not configured with AI assistance client"
	return "", errors.New(msg)
}

