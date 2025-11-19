package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	srvConfig "github.com/CHESSComputing/golib/config"
)

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
	log.Println("### call tichy server", rurl, string(data))
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

	return chatResp.Choices[0].Message.Content, nil
}
