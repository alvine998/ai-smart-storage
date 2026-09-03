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

// Ping verifies connectivity to the MiMo API.
// For both pay-as-you-go and Token Plan it first tries GET /models (cheap, no
// credit consumption).  Some Token Plan clusters do not expose /models, so on
// 404 it falls back to a minimal non-streaming POST /chat/completions probe.
// 401/403 are returned verbatim so callers can distinguish invalid keys.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("mimo client not initialized")
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return fmt.Errorf("MIMO_API_KEY not set")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("MIMO_BASE_URL not set")
	}
	// 1) Try the standard OpenAI-compatible /models endpoint.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 {
		return nil
	}
	// 404 → /models not routed on this cluster (seen on some token-plan
	// hosts); fall back to a tiny chat completion probe.
	if resp.StatusCode == http.StatusNotFound && c.IsTokenPlan() {
		_ = resp.Body.Close()
		return c.pingViaChat(ctx)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("mimo API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

// pingViaChat performs a minimal non-streaming chat completion probe that
// consumes ~1 credit on Token Plan.  Used as fallback when /models is 404.
func (c *Client) pingViaChat(ctx context.Context) error {
	payload, _ := json.Marshal(map[string]any{
		"model":       c.model,
		"messages":    []Message{{Role: "user", Content: "ping"}},
		"max_tokens":  1,
		"temperature": 0,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("mimo API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// IsConfigured returns true when mandatory credentials are present.
func (c *Client) IsConfigured() bool {
	return c != nil && strings.TrimSpace(c.apiKey) != "" && strings.TrimSpace(c.baseURL) != ""
}

// IsTokenPlan reports whether this client is configured for the
// MiMo Token Plan (subscription credits) rather than pay-as-you-go.
// Detection is via base URL (contains "token-plan") or key prefix
// (tp- / rtp-), matching the official docs.
func (c *Client) IsTokenPlan() bool {
	if c == nil {
		return false
	}
	if strings.Contains(strings.ToLower(c.baseURL), "token-plan") {
		return true
	}
	key := strings.TrimSpace(c.apiKey)
	return strings.HasPrefix(key, "tp-") || strings.HasPrefix(key, "rtp-")
}

// BillingMode returns a human label for diagnostics/logs.
func (c *Client) BillingMode() string {
	if c.IsTokenPlan() {
		return "Token Plan"
	}
	return "Pay-as-you-go"
}

// Model returns the configured model name (for diagnostics).
func (c *Client) Model() string { return c.model }

// BaseURL returns the configured base URL (for diagnostics).
func (c *Client) BaseURL() string { return c.baseURL }

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
