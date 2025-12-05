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

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/ollama"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
	"github.com/CHESSComputing/golib/utils"
	"github.com/microcosm-cc/bluemonday"
)

// AIChat represents generic AI chat interface
type AIChat interface {
	Chat(user, prompt string) (string, error)
}

// OllamaClient represents Ollama AI client
type OllamaClient struct {
	AIConfig *ollama.Config
}

// Chat implementation for OllamaClient
func (o *OllamaClient) Chat(user, prompt string) (string, error) {
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
func (o *TichyClient) Chat(user, prompt string) (string, error) {

	ctx := context.Background()
	if srvConfig.Config.AIChat.Timeout != 0 {
		ctx1, cancel := context.WithTimeout(
			context.Background(),
			time.Duration(srvConfig.Config.AIChat.Timeout)*time.Second)
		defer cancel()
		ctx = ctx1
	}
	resp, err := aitichy(ctx, user, prompt)
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

// helper function to get appropriate set of vector databases
// based on provided FOXDEN user information
func getVectorDbs(fuser services.User) []string {
	vdbs := []string{"foxden"}
	if utils.InList("cmpgrp", fuser.Groups) {
		vdbs = []string{"computing"}
	}
	return vdbs
}

// helper function to generate AI token with appropriate user
// information and list of vector databases to lookup
func getAIToken(fuser services.User) (string, error) {
	customClaims := authz.CustomClaims{
		User:        fuser.Name,
		Application: "FOXDEN",
		VectorDbs:   getVectorDbs(fuser),
	}
	duration := srvConfig.Config.Authz.TokenExpires
	if duration == 0 {
		duration = 7200
	}
	return authz.JWTAccessToken(srvConfig.Config.Authz.ClientID, duration, customClaims)
}

func aitichy(ctx context.Context, user, query string) (string, error) {
	// Build request payload
	reqBody := TichyRequest{
		Messages: []TichyMessage{
			{Role: "user", Content: query},
		},
	}

	// extract user groups and generate proper AIDB collection to query
	fuser, err := _foxdenUser.Get(user)
	if err != nil {
		return "", fmt.Errorf("failed to extract user info: %w", err)
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

	// generate proper token for AI chatbot
	if aitoken, err := getAIToken(fuser); err == nil {
		req.Header.Set("Authorization", "Bearer "+aitoken)
	}

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

	md := chatResp.Choices[0].Message.Content
	htmlReply := server.MDStringToHTML(md)
	p := bluemonday.UGCPolicy()
	safeReply := p.Sanitize(htmlReply)
	return safeReply, nil
}

// wrapper AI chat function to use different AI backend engine
func aichat(user, prompt string) (string, error) {
	if srvConfig.Config.AIChat.Client == "ollama" {
		client := OllamaClient{}
		return client.Chat(user, prompt)
	} else if srvConfig.Config.AIChat.Client == "tichy" {
		client := TichyClient{}
		return client.Chat(user, prompt)
	}
	msg := "FOXDEN is not configured with AI assistance client"
	return "", errors.New(msg)
}
