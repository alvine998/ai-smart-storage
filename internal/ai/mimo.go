package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type Usage struct {
	InputTokens  uint64
	OutputTokens uint64
}

func NewClient(apiKey, baseURL, model string, timeout time.Duration) *Client {
	return &Client{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), model: model, httpClient: &http.Client{Timeout: timeout}}
}

func (c *Client) Stream(ctx context.Context, messages []Message, emit func(string) error) error {
	_, err := c.StreamWithUsage(ctx, messages, emit)
	return err
}

func (c *Client) StreamWithUsage(ctx context.Context, messages []Message, emit func(string) error) (Usage, error) {
	payload, err := json.Marshal(map[string]any{"model": c.model, "messages": messages, "stream": true, "stream_options": map[string]bool{"include_usage": true}})
	if err != nil {
		return Usage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Usage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Usage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Usage{}, fmt.Errorf("mimo returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var usage Usage
	scanner := bufio.NewScanner(resp.Body)
	// Set a larger buffer for streaming responses that may exceed the default 64KB
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     uint64 `json:"prompt_tokens"`
				CompletionTokens uint64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return Usage{}, err
		}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if err := emit(chunk.Choices[0].Delta.Content); err != nil {
				return Usage{}, err
			}
		}
	}
	return usage, scanner.Err()
}
