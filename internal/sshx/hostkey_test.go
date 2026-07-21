package sshx

import (
	"os"
	"path/filepath"
	"testing"

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
