package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/llm"
)

type chatClient interface {
	Configured() bool
	Chat(context.Context, string, []llm.Message) (string, error)
}

type chatOverlay struct {
	input      textinput.Model
	messages   []llm.Message
	loading    bool
	err        error
	generation uint64
	cancel     context.CancelFunc
}

type chatResultMsg struct {
	generation uint64
	text       string
	err        error
}

func newChatOverlay() chatOverlay {
	input := textinput.New()
	input.Placeholder = "спросить о состоянии серверов"
	return chatOverlay{input: input}
}

func (m *Model) handleChatKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "enter" && strings.TrimSpace(m.chat.input.Value()) != "" && !m.chat.loading {
		return m.startChat()
	}
	var cmd tea.Cmd
	m.chat.input, cmd = m.chat.input.Update(key)
	return cmd
}

func (m *Model) startChat() tea.Cmd {
	m.cancelChat()
	m.request = max(m.request, m.chat.generation) + 1
	m.chat.generation = m.request
	text := strings.TrimSpace(m.chat.input.Value())
	m.chat.input.SetValue("")
	m.chat.messages = append(m.chat.messages, llm.Message{Role: "user", Content: text})
	m.chat.loading, m.chat.err = true, nil
	ctx, cancel := context.WithCancel(context.Background())
	m.chat.cancel = cancel
	generation, client := m.chat.generation, m.chatClient
	messages := append([]llm.Message(nil), m.chat.messages...)
	system := m.chatSystemContext()
	return func() tea.Msg {
		if client == nil || !client.Configured() {
			return chatResultMsg{generation: generation, err: fmt.Errorf("LLM не настроен")}
		}
		reply, err := client.Chat(ctx, system, messages)
		return chatResultMsg{generation: generation, text: reply, err: err}
	}
}

func (m *Model) cancelChat() {
	if m.chat.cancel != nil {
		m.chat.cancel()
		m.chat.cancel = nil
	}
}

func (m Model) chatSystemContext() string {
	return fmt.Sprintf("Active screen: %s\nSelected server: %s\nSubfeature: %s\n\n%s",
		screenLabel(m.screen), m.selectedName(), m.subfeatureContext(), m.snapshot.Text())
}

func (m Model) subfeatureContext() string {
	switch m.screen {
	case screenProcesses:
		return statusContext(m.processes.status, m.processes.err)
	case screenPorts:
		return statusContext(m.ports.status, m.ports.err)
	case screenContainers:
		return statusContext(m.containers.status, m.containers.err)
	case screenLogs:
		return statusContext(m.logs.status, m.logs.err)
	case screenHistory:
		if m.history.err != nil {
			return "error: " + m.history.err.Error()
		}
		return fmt.Sprintf("status=%d", m.history.status)
	default:
		return "ready"
	}
}

func statusContext(status diagnosticsStatus, err error) string {
	labels := [...]string{"idle", "loading", "ready", "stale", "unsupported", "error"}
	text := labels[min(int(status), len(labels)-1)]
	if err != nil {
		text += ": " + err.Error()
	}
	return text
}

func screenLabel(screen screenKind) string {
	return [...]string{"Fleet", "Dashboard", "Processes", "Ports", "History", "Logs", "Containers"}[screen]
}

func (m Model) renderChat() string {
	var lines []string
	for _, message := range m.chat.messages {
		prefix := "Вы: "
		if message.Role == "assistant" {
			prefix = "ИИ: "
		}
		lines = append(lines, prefix+message.Content)
	}
	if m.chat.loading {
		lines = append(lines, "ИИ: думает…")
	}
	if m.chat.err != nil {
		lines = append(lines, "ошибка: "+m.chat.err.Error())
	}
	return "Чат · " + screenLabel(m.screen) + "\n\n" + strings.Join(lines, "\n") + "\n\n" + m.chat.input.View()
}
