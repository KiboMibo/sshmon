package sshx

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestHostKeyVerification(t *testing.T) {
	// известный, но отсутствующий файл → молчаливый insecure (true)
	missing := filepath.Join(t.TempDir(), "known_hosts")
	if _, silent := hostKeyVerification(config.Server{}, missing); !silent {
		t.Errorf("отсутствующий known_hosts: ожидали silent insecure=true")
	}

	// существующий (пустой) файл → knownhosts.New успешен, реальная проверка (false)
	empty := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if cb, silent := hostKeyVerification(config.Server{}, empty); silent || cb == nil {
		t.Errorf("пустой known_hosts: ожидали проверку (silent=false, cb!=nil); got silent=%v cb==nil:%v", silent, cb == nil)
	}

	// явный insecure_host_key → не молча (false), пользователь выбрал сам
	if cb, silent := hostKeyVerification(config.Server{InsecureHostKey: true}, missing); silent || cb == nil {
		t.Errorf("insecure_host_key=true: ожидали silent=false, cb!=nil; got silent=%v cb==nil:%v", silent, cb == nil)
	}
}

func TestHostKeyAlgorithms(t *testing.T) {
	// Given: в known_hosts на нестандартном порту записан только ed25519-ключ
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{"[example.com]:7022"}, sshPub)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cb, _ := hostKeyVerification(config.Server{}, path)

	// When: спрашиваем допустимые типы host-key для этого адреса
	// Then: клиент будет просить именно ed25519, а не свой дефолтный ecdsa/rsa
	if got := hostKeyAlgorithms(cb, "example.com:7022"); !slices.Contains(got, ssh.KeyAlgoED25519) {
		t.Errorf("известный хост: ожидали %q в списке, got %v", ssh.KeyAlgoED25519, got)
	}

	// Then: для незнакомого хоста ограничивать нечем — дефолтное поведение
	if got := hostKeyAlgorithms(cb, "other.example.com:22"); got != nil {
		t.Errorf("неизвестный хост: ожидали nil, got %v", got)
	}

	// Then: при insecure_host_key проверки нет, ограничений тоже
	insecure, _ := hostKeyVerification(config.Server{InsecureHostKey: true}, path)
	if got := hostKeyAlgorithms(insecure, "example.com:7022"); got != nil {
		t.Errorf("insecure_host_key=true: ожидали nil, got %v", got)
	}
}
