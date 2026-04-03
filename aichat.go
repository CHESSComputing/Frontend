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
	"net/url"
	"strings"
	"sync"
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
		return "", fmt.Errorf("[Frontend.main.OllamaClient.Chat] client.SendRequest error: %w", err)
	}
	return response, nil
}

// TichyClient represents the tichy AI client.
// It fans out requests concurrently across all configured VectorDB collections
// and aggregates the results into a single response.
type TichyClient struct{}

// Chat sends the prompt to every configured collection concurrently and
// returns a merged response. When no collections are configured the request
// falls back to the server default.
func (o *TichyClient) Chat(user, prompt string) (string, error) {
	ctx := context.Background()
	if srvConfig.Config.AIChat.Timeout != 0 {
		ctx1, cancel := context.WithTimeout(
			context.Background(),
			time.Duration(srvConfig.Config.AIChat.Timeout)*time.Second)
		defer cancel()
		ctx = ctx1
	}

	// Resolve which collections this user may access.
	fuser, err := _foxdenUser.Get(user)
	if err != nil {
		return "", fmt.Errorf("[Frontend.main.TichyClient.Chat] failed to get user: %w", err)
	}
	collections := getVectorDbs(fuser)

	if len(collections) == 0 {
		// No collections configured – fall back to server default.
		resp, err := aitichy(ctx, user, prompt, "")
		if err != nil {
			return resp, fmt.Errorf("[Frontend.main.TichyClient.Chat] aitichy error: %w", err)
		}
		return resp, nil
	}

	// Fan out: one goroutine per collection.
	type result struct {
		collection string
		response   string
		err        error
	}

	results := make(chan result, len(collections))
	var wg sync.WaitGroup

	for _, col := range collections {
		wg.Add(1)
		go func(collection string) {
			defer wg.Done()
			resp, err := aitichy(ctx, user, prompt, collection)
			results <- result{collection: collection, response: resp, err: err}
		}(col)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Aggregate responses.
	var parts []string
	var errs []string
	for r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("collection %q: %v", r.collection, r.err))
			continue
		}
		if r.response != "" {
			if !strings.Contains(r.response, "nodatafound") {
				msg := fmt.Sprintf("<section data-collection=%q>\n<h3>%s knowledge collection</h3>%s\n</section>", r.collection, r.collection, r.response)
				parts = append(parts, msg)
			}
		}
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("all collection queries failed: %s", strings.Join(errs, "; "))
	}

	aggregated := strings.Join(parts, "\n\n")
	if len(errs) > 0 {
		log.Printf("WARN: partial failures in multi-collection chat: %s", strings.Join(errs, "; "))
	}
	return aggregated, nil
}

// ---------------------------------------------------------------------------
// Shared request / response types
// ---------------------------------------------------------------------------

// TichyMessage is a single chat message sent to the tichy server.
type TichyMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TichyRequest is the JSON body sent to /v1/chat/completions.
type TichyRequest struct {
	Messages []TichyMessage `json:"messages"`
}

// TichyResponse is the JSON body received from /v1/chat/completions.
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

// ---------------------------------------------------------------------------
// Access-control helpers
// ---------------------------------------------------------------------------

// getVectorDbs returns the set of Qdrant collections the user may query,
// derived from their group memberships and the configured access rules.
func getVectorDbs(fuser services.User) []string {
	arules := srvConfig.Config.AIChat.AccessRules
	var vdbs []string
	for _, rule := range arules {
		if utils.InList(rule.Group, fuser.Groups) {
			vdbs = append(vdbs, rule.Databases...)
		}
	}
	if len(vdbs) > 0 {
		return utils.List2Set(vdbs)
	}
	return vdbs
}

// getAIToken generates a JWT that embeds user identity and allowed collections.
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

// ---------------------------------------------------------------------------
// Core HTTP call
// ---------------------------------------------------------------------------

// aitichy sends a single chat request to the tichy server.
// When collection is non-empty it is appended as a query parameter so the
// server routes retrieval to that specific Qdrant collection.
func aitichy(ctx context.Context, user, query, collection string) (string, error) {
	// Guard: check group membership (per original logic)
	fuser, err := _foxdenUser.Get(user)
	if err != nil {
		return "", fmt.Errorf("failed to extract user info: %w", err)
	}
	aigroup := srvConfig.Config.AIChat.Group
	if aigroup != "" && !utils.InList(aigroup, fuser.Groups) {
		return "", fmt.Errorf("user %s does not belong to group %s, access prohibited", user, aigroup)
	}

	// Build JSON body.
	reqBody := TichyRequest{
		Messages: []TichyMessage{
			{Role: "user", Content: query},
		},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL, appending ?collection=<name> when provided.
	baseURL := fmt.Sprintf("http://%s:%v/v1/chat/completions",
		srvConfig.Config.AIChat.Host,
		srvConfig.Config.AIChat.Port)
	if collection != "" {
		params := url.Values{}
		params.Set("collection", collection)
		baseURL = baseURL + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(data))
	if err != nil {
		log.Println("ERROR:", err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
	return p.Sanitize(htmlReply), nil
}

// ---------------------------------------------------------------------------
// Top-level dispatcher
// ---------------------------------------------------------------------------

// aichat dispatches to the configured AI backend.
func aichat(user, prompt string) (string, error) {
	switch srvConfig.Config.AIChat.Client {
	case "ollama":
		client := OllamaClient{}
		return client.Chat(user, prompt)
	case "tichy":
		client := TichyClient{}
		return client.Chat(user, prompt)
	}
	return "", errors.New("FOXDEN is not configured with AI assistance client")
}
