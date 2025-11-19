package main

import (
	"context"
	"errors"
	"log"
	"time"

	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/ollama"
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
