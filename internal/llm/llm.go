// Package llm — минимальный чат-клиент: OpenAI-совместимые API и Anthropic.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
)

const (
	requestTimeout     = 2 * time.Minute // весь round-trip чата, включая генерацию
	maxResponseBytes   = 1 << 20         // 1 MiB: защита от неограниченного ответа враждебного апстрима
	anthropicMaxTokens = 2048            // потолок ответа Anthropic (у OpenAI-совместимых берётся дефолт модели)
	errSnippetLen      = 500             // сколько символов тела показывать в ошибке не-200
)

type Message struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

type Client struct {
	cfg config.LLM
	hc  *http.Client
}

func New(cfg config.LLM) *Client {
	return &Client{cfg: cfg, hc: &http.Client{Timeout: requestTimeout}}
}

func (c *Client) Configured() bool { return c.cfg.Model != "" }

func (c *Client) Chat(ctx context.Context, system string, msgs []Message) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("llm не настроен: заполните секцию llm в конфиге (provider, model, api_key/api_key_env)")
	}
	if c.cfg.Provider == "anthropic" {
		return c.anthropic(ctx, system, msgs)
	}
	return c.openai(ctx, system, msgs)
}

func (c *Client) post(ctx context.Context, url string, hdr map[string]string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		s := string(rb)
		if len(s) > errSnippetLen {
			s = s[:errSnippetLen]
		}
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, s)
	}
	return rb, nil
}

func (c *Client) openai(ctx context.Context, system string, msgs []Message) (string, error) {
	base := c.cfg.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	url := strings.TrimSuffix(base, "/") + "/chat/completions"
	all := append([]Message{{Role: "system", Content: system}}, msgs...)
	hdr := map[string]string{}
	if k := c.cfg.Key(); k != "" {
		hdr["Authorization"] = "Bearer " + k
	}
	rb, err := c.post(ctx, url, hdr, map[string]any{"model": c.cfg.Model, "messages": all})
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm: пустой ответ")
	}
	return out.Choices[0].Message.Content, nil
}

func (c *Client) anthropic(ctx context.Context, system string, msgs []Message) (string, error) {
	base := c.cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	url := strings.TrimSuffix(base, "/") + "/v1/messages"
	hdr := map[string]string{
		"x-api-key":         c.cfg.Key(),
		"anthropic-version": "2023-06-01",
	}
	rb, err := c.post(ctx, url, hdr, map[string]any{
		"model": c.cfg.Model, "max_tokens": anthropicMaxTokens, "system": system, "messages": msgs,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, part := range out.Content {
		if part.Type == "text" {
			sb.WriteString(part.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("llm: пустой ответ")
	}
	return sb.String(), nil
}
