package main

import (
	"context"
	"log"

	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/ollama"
)

var aiconfig *ollama.Config

func aichat(prompt string) (string, error) {
	if aiconfig == nil {
		aiconfig = &ollama.Config{
			Host:  srvConfig.Config.AIChat.Host,
			Port:  srvConfig.Config.AIChat.Port,
			Model: srvConfig.Config.AIChat.Model,
		}
		log.Printf("configure ai chat with %+v", aiconfig)
	}

	client := ollama.NewClient(*aiconfig)
	ctx := context.Background()
	stream := true
	response, err := client.SendRequest(ctx, prompt, stream, nil)
	if err != nil {
		return "", err
	}
	return response, nil
}
