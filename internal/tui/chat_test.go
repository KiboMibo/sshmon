package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/llm"
)

type fakeChatClient struct {
	system   string
	messages []llm.Message
	reply    string
}

func (f *fakeChatClient) Configured() bool { return true }

func (f *fakeChatClient) Chat(_ context.Context, system string, messages []llm.Message) (string, error) {
	f.system = system
	f.messages = append([]llm.Message(nil), messages...)
	return f.reply, nil
}

func TestChatRequestContainsFleetSelectionScreenAndSubfeatureState(t *testing.T) {
	// Given Chat opened over a stale Processes screen with a selected server.
	client := &fakeChatClient{reply: "проверьте процесс"}
	m := Model{
		screen:     screenProcesses,
		selected:   0,
		snapshot:   collect.Snapshot{Servers: []collect.Metrics{{Name: "web", Online: true}}},
		processes:  processScreen{status: diagnosticsStale, err: context.DeadlineExceeded},
		chatClient: client,
		chat:       newChatOverlay(),
		overlay:    overlayChat,
	}
	m.chat.input.SetValue("что случилось?")

	// When the message is submitted and its asynchronous result is applied.
	m, cmd := updateModel(t, m, key("enter"))
	if cmd == nil {
		t.Fatal("chat did not start asynchronous request")
	}
	m, _ = updateModel(t, m, cmd())

	// Then the system context contains live scope and the reply is visible.
	for _, want := range []string{"web", "Processes", "stale", "context deadline exceeded"} {
		if !strings.Contains(client.system, want) {
			t.Fatalf("system context misses %q: %s", want, client.system)
		}
	}
	if len(client.messages) != 1 || client.messages[0].Content != "что случилось?" {
		t.Fatalf("messages = %#v", client.messages)
	}
	if len(m.chat.messages) == 0 {
		t.Fatalf("chat messages are empty after result, generation=%d loading=%v err=%v", m.chat.generation, m.chat.loading, m.chat.err)
	}
	if got := m.chat.messages[len(m.chat.messages)-1]; got.Role != "assistant" || got.Content != client.reply {
		t.Fatalf("last chat message = %#v", got)
	}
}

func TestClosingChatCancelsRequestAndNewSessionHasNoHistory(t *testing.T) {
	// Given a Chat session with history and an active cancellation function.
	cancelled := false
	m := Model{screen: screenFleet, overlay: overlayChat, chat: newChatOverlay()}
	m.chat.messages = []llm.Message{{Role: "user", Content: "old"}}
	m.chat.cancel = func() { cancelled = true }

	// When Chat closes and is opened again.
	m, _ = updateModel(t, m, key("esc"))
	m, _ = updateModel(t, m, key("c"))

	// Then the request is cancelled and overlay history is not persisted.
	if !cancelled || len(m.chat.messages) != 0 {
		t.Fatalf("cancelled=%v messages=%#v", cancelled, m.chat.messages)
	}
}
