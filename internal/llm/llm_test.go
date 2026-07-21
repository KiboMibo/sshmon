package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestChatNotConfigured(t *testing.T) {
	_, err := New(config.LLM{}).Chat(context.Background(), "sys", nil)
	if err == nil || !strings.Contains(err.Error(), "не настроен") {
		t.Fatalf("err = %v, want 'не настроен'", err)
	}
}

func TestOpenAIChatSuccessSendsSystemAndAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		var body struct {
			Model    string    `json:"model"`
			Messages []Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Model != "gpt-x" {
			t.Errorf("model = %s", body.Model)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[0].Content != "sys ctx" {
			t.Fatalf("messages = %#v", body.Messages)
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)
	}))
	defer srv.Close()

	c := New(config.LLM{Model: "gpt-x", APIKey: "secret", BaseURL: srv.URL})
	got, err := c.Chat(context.Background(), "sys ctx", []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "hello" {
		t.Fatalf("reply = %q", got)
	}
}

func TestOpenAIChatHTTPErrorIncludesBodySnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()

	_, err := New(config.LLM{Model: "gpt-x", BaseURL: srv.URL}).Chat(context.Background(), "s", nil)
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want http 500 + body", err)
	}
}

func TestOpenAIChatEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	_, err := New(config.LLM{Model: "gpt-x", BaseURL: srv.URL}).Chat(context.Background(), "s", nil)
	if err == nil || !strings.Contains(err.Error(), "пустой ответ") {
		t.Fatalf("err = %v, want 'пустой ответ'", err)
	}
}

func TestAnthropicChatSendsHeadersAndConcatenatesTextParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "key" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		var body struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.MaxTokens != anthropicMaxTokens {
			t.Errorf("max_tokens = %d, want %d", body.MaxTokens, anthropicMaxTokens)
		}
		io.WriteString(w, `{"content":[{"type":"text","text":"a"},{"type":"thinking"},{"type":"text","text":"b"}]}`)
	}))
	defer srv.Close()

	c := New(config.LLM{Provider: "anthropic", Model: "claude", APIKey: "key", BaseURL: srv.URL})
	got, err := c.Chat(context.Background(), "sys", []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "ab" {
		t.Fatalf("reply = %q, want 'ab'", got)
	}
}
