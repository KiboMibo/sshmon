package sshx

import (
	"errors"
	"strings"
	"testing"

	"github.com/kibomibo/sshmon/internal/config"
)

// Группа тестов: FriendlyErr должен переводить известные сырые ошибки SSH
// в человекочитаемые подсказки на русском, а неизвестные — возвращать как есть.

func TestFriendlyErr_HostKeyMismatch_ExplainsAndHints(t *testing.T) {
	// Given: сырая ошибка knownhosts: key mismatch из ssh.Dial
	raw := errors.New("ssh: handshake failed: knownhosts: key mismatch")
	// When:FriendlyErr получает её вместе с конфигом сервера
	got := FriendlyErr(raw, config.Server{Host: "emarb.example.ru", Port: 7022})
	// Then: в сообщении есть объяснение причины и готовая команда с адресом сервера
	if !strings.Contains(got, "host-key") {
		t.Fatalf("expected 'host-key' explanation, got %q", got)
	}
	if !strings.Contains(got, "ssh-keygen -R [emarb.example.ru]:7022") {
		t.Fatalf("expected 'ssh-keygen -R' hint with real target, got %q", got)
	}
}

func TestFriendlyErr_NoCommonHostKeyAlgorithm_PointsAtKnownHosts(t *testing.T) {
	// Given: сервер не отдаёт host-key того типа, что записан в known_hosts
	raw := errors.New("ssh: handshake failed: ssh: no common algorithm for host key; client offered: [ssh-ed25519], server offered: [rsa-sha2-512]")
	// When:
	got := FriendlyErr(raw, config.Server{Host: "emarb.example.ru", Port: 22})
	// Then: подсказка ведёт в known_hosts и даёт команду без скобок для порта 22
	if !strings.Contains(got, "known_hosts") {
		t.Fatalf("expected known_hosts hint, got %q", got)
	}
	if !strings.Contains(got, "ssh-keygen -R emarb.example.ru") {
		t.Fatalf("expected plain-host target for port 22, got %q", got)
	}
}

func TestFriendlyErr_UnknownHost_PointsAtKnownHosts(t *testing.T) {
	// Given: хоста нет в known_hosts вовсе
	raw := errors.New("ssh: handshake failed: knownhosts: key is unknown")
	// When:
	got := FriendlyErr(raw, config.Server{Host: "emauth.example.ru", Port: 7022})
	// Then: сказано куда смотреть и что сделать, без совета отключить проверку
	if !strings.Contains(got, "known_hosts") {
		t.Fatalf("expected known_hosts hint, got %q", got)
	}
	if strings.Contains(got, "insecure_host_key") {
		t.Fatalf("unexpected advice to disable verification, got %q", got)
	}
}

func TestFriendlyErr_AuthFailure_ExplainsCredentials(t *testing.T) {
	// Given: ошибка аутентификации
	raw := errors.New("ssh: handshake failed: ssh: unable to authenticate; tried methods [none publickey], no supported methods remain")
	// When:
	got := FriendlyErr(raw, config.Server{})
	// Then: объяснена проблема с ключом/паролем
	if !strings.Contains(got, "ключ") && !strings.Contains(got, "пароль") {
		t.Fatalf("expected auth-related hint, got %q", got)
	}
}

func TestFriendlyErr_Timeout_ExplainsNetwork(t *testing.T) {
	// Given: сетевой таймаут
	raw := errors.New("dial tcp 10.0.0.1:22: i/o timeout")
	// When:
	got := FriendlyErr(raw, config.Server{})
	// Then: упомянута недоступность сети/хоста
	if !strings.Contains(got, "сеть") && !strings.Contains(got, "таймаут") && !strings.Contains(got, "недоступен") {
		t.Fatalf("expected network/timeout hint, got %q", got)
	}
}

func TestFriendlyErr_ConnectionRefused_ExplainsNetwork(t *testing.T) {
	// Given: connection refused
	raw := errors.New("dial tcp 10.0.0.1:22: connect: connection refused")
	// When:
	got := FriendlyErr(raw, config.Server{})
	// Then: упомянут sshd/порт/недоступность
	if !strings.Contains(got, "сеть") && !strings.Contains(got, "недоступен") && !strings.Contains(got, "sshd") && !strings.Contains(got, "порт") {
		t.Fatalf("expected refused hint, got %q", got)
	}
}

func TestFriendlyErr_UnknownError_ReturnsRawText(t *testing.T) {
	// Given: неизвестная ошибка с уникальным текстом
	raw := errors.New("weird bespoke failure xyz123")
	// When:
	got := FriendlyErr(raw, config.Server{})
	// Then: исходный текст сохранён
	if !strings.Contains(got, "weird bespoke failure xyz123") {
		t.Fatalf("expected passthrough of raw text, got %q", got)
	}
}
