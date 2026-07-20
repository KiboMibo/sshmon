package sshx

import (
	"errors"
	"strings"
	"testing"
)

// Группа тестов: FriendlyErr должен переводить известные сырые ошибки SSH
// в человекочитаемые подсказки на русском, а неизвестные — возвращать как есть.

func TestFriendlyErr_HostKeyMismatch_ExplainsAndHints(t *testing.T) {
	// Given: сырая ошибка knownhosts: key mismatch из ssh.Dial
	raw := errors.New("ssh: handshake failed: knownhosts: key mismatch")
	// When:FriendlyErr получает её
	got := FriendlyErr(raw)
	// Then: в сообщении есть объяснение причины и обе подсказки
	if !strings.Contains(got, "host-key") {
		t.Fatalf("expected 'host-key' explanation, got %q", got)
	}
	if !strings.Contains(got, "ssh-keygen -R") {
		t.Fatalf("expected 'ssh-keygen -R' hint, got %q", got)
	}
	if !strings.Contains(got, "insecure_host_key") {
		t.Fatalf("expected 'insecure_host_key' hint, got %q", got)
	}
}

func TestFriendlyErr_AuthFailure_ExplainsCredentials(t *testing.T) {
	// Given: ошибка аутентификации
	raw := errors.New("ssh: handshake failed: ssh: unable to authenticate; tried methods [none publickey], no supported methods remain")
	// When:
	got := FriendlyErr(raw)
	// Then: объяснена проблема с ключом/паролем
	if !strings.Contains(got, "ключ") && !strings.Contains(got, "пароль") {
		t.Fatalf("expected auth-related hint, got %q", got)
	}
}

func TestFriendlyErr_Timeout_ExplainsNetwork(t *testing.T) {
	// Given: сетевой таймаут
	raw := errors.New("dial tcp 10.0.0.1:22: i/o timeout")
	// When:
	got := FriendlyErr(raw)
	// Then: упомянута недоступность сети/хоста
	if !strings.Contains(got, "сеть") && !strings.Contains(got, "таймаут") && !strings.Contains(got, "недоступен") {
		t.Fatalf("expected network/timeout hint, got %q", got)
	}
}

func TestFriendlyErr_ConnectionRefused_ExplainsNetwork(t *testing.T) {
	// Given: connection refused
	raw := errors.New("dial tcp 10.0.0.1:22: connect: connection refused")
	// When:
	got := FriendlyErr(raw)
	// Then: упомянут sshd/порт/недоступность
	if !strings.Contains(got, "сеть") && !strings.Contains(got, "недоступен") && !strings.Contains(got, "sshd") && !strings.Contains(got, "порт") {
		t.Fatalf("expected refused hint, got %q", got)
	}
}

func TestFriendlyErr_UnknownError_ReturnsRawText(t *testing.T) {
	// Given: неизвестная ошибка с уникальным текстом
	raw := errors.New("weird bespoke failure xyz123")
	// When:
	got := FriendlyErr(raw)
	// Then: исходный текст сохранён
	if !strings.Contains(got, "weird bespoke failure xyz123") {
		t.Fatalf("expected passthrough of raw text, got %q", got)
	}
}
