// Package sshx — ленивое SSH-подключение с автопереподключением после сбоев.
package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/kibomibo/sshmon/internal/config"
)

var (
	ErrPassphraseRequired = errors.New("требуется passphrase для SSH-ключа")
	ErrInvalidPassphrase  = errors.New("неверная passphrase для SSH-ключа")
)

type Client struct {
	cfg        config.Server
	mu         sync.Mutex
	c          *ssh.Client
	passphrase []byte
}

func New(cfg config.Server) *Client { return &Client{cfg: cfg} }

func (c *Client) conn() (*ssh.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.c != nil {
		return c.c, nil
	}
	auth, needsPassphrase, err := authMethods(c.cfg, c.passphrase)
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		if needsPassphrase {
			return nil, ErrPassphraseRequired
		}
		return nil, fmt.Errorf("нет способа аутентификации (key/agent/password)")
	}
	cl, err := ssh.Dial("tcp", c.cfg.Addr(), &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback(c.cfg),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		if needsPassphrase {
			return nil, fmt.Errorf("%w: альтернативная аутентификация не удалась", ErrPassphraseRequired)
		}
		return nil, err
	}
	c.c = cl
	return cl, nil
}

// SetPassphrase replaces the in-memory key passphrase and resets the connection.
func (c *Client) SetPassphrase(passphrase []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.passphrase)
	c.passphrase = append(c.passphrase[:0], passphrase...)
	c.dropLocked()
}

// Reset closes the cached connection so the next operation dials again.
func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropLocked()
}

func (c *Client) drop() {
	c.mu.Lock()
	c.dropLocked()
	c.mu.Unlock()
}

func (c *Client) dropLocked() {
	if c.c != nil {
		c.c.Close()
		c.c = nil
	}
}

// Run выполняет команду и возвращает stdout с таймаутом.
// Ненулевой exit code с непустым выводом не считается ошибкой:
// в цепочках `a || b` полезный вывод важнее кода возврата.
func (c *Client) Run(cmd string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := c.RunContext(ctx, cmd)
	if errors.Is(err, context.DeadlineExceeded) {
		return "", fmt.Errorf("таймаут %s", timeout)
	}
	return out, err
}

// RunContext выполняет команду до завершения или отмены контекста.
func (c *Client) RunContext(ctx context.Context, cmd string) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	sess, err := cl.NewSession()
	if err != nil {
		c.drop()
		return "", err
	}
	defer sess.Close()
	out, err := runCommand(ctx, func() ([]byte, error) { return sess.Output(cmd) }, c.drop)
	if err != nil {
		if len(out) > 0 {
			return string(out), nil
		}
		return "", err
	}
	return string(out), nil
}

type commandResult struct {
	out []byte
	err error
}

func runCommand(ctx context.Context, output func() ([]byte, error), drop func()) ([]byte, error) {
	result := make(chan commandResult, 1)
	go func() {
		out, err := output()
		result <- commandResult{out: out, err: err}
	}()
	select {
	case res := <-result:
		return res.out, res.err
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

func authMethods(cfg config.Server, passphrase []byte) ([]ssh.AuthMethod, bool, error) {
	// Порядок как у openssh: ssh-agent → локальный файл ключа → пароль.
	// Сначала agent, чтобы уже загруженные в ssh-add ключи работали без passphrase-промпта.
	var out []ssh.AuthMethod
	agentReachable := false
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			out = append(out, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
			agentReachable = true
		}
	}
	needsPassphrase := false
	if cfg.Key != "" {
		if b, err := os.ReadFile(cfg.Key); err == nil {
			signer, parseErr := ssh.ParsePrivateKey(b)
			if parseErr == nil {
				out = append(out, ssh.PublicKeys(signer))
			} else {
				var missing *ssh.PassphraseMissingError
				if errors.As(parseErr, &missing) {
					if len(passphrase) == 0 {
						// Требуем passphrase только если ssh-agent недоступен —
						// иначе пусть сервер сам попробует ключи из агента.
						if !agentReachable {
							needsPassphrase = true
						}
					} else {
						signer, err = ssh.ParsePrivateKeyWithPassphrase(b, passphrase)
						if err != nil {
							return nil, false, ErrInvalidPassphrase
						}
						out = append(out, ssh.PublicKeys(signer))
					}
				}
			}
		}
	}
	if cfg.Password != "" {
		out = append(out, ssh.Password(cfg.Password))
	}
	return out, needsPassphrase, nil
}

// FriendlyErr переводит сырые ошибки ssh.Dial/Run в человекочитаемые подсказки.
// Не известные ошибки возвращаются как есть (err.Error()).
func FriendlyErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "knownhosts: key mismatch"):
		return "host-key сервера не совпадает с записью в ~/.ssh/known_hosts — выполните `ssh-keygen -R <host>` и переподключитесь обычным ssh, либо поставьте insecure_host_key: true в config.yaml"
	case strings.Contains(msg, "unable to authenticate"):
		return "не удалось аутентифицироваться — проверьте ключ/пароль и что ssh-agent загружен (ssh-add -l)"
	case strings.Contains(msg, "connection refused"):
		return "сеть: подключение отклонено — проверьте что sshd запущен и порт указан верно"
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "deadline exceeded"):
		return "сеть: таймаут подключения — хост недоступен или порт закрыт firewall"
	}
	return msg
}

func hostKeyCallback(cfg config.Server) ssh.HostKeyCallback {
	if cfg.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey() // явный opt-in в конфиге
	}
	home, _ := os.UserHomeDir()
	if cb, err := knownhosts.New(home + "/.ssh/known_hosts"); err == nil {
		return cb
	}
	// ponytail: нет known_hosts — принимаем любой ключ, иначе утилита не стартует;
	// апгрейд: TOFU с записью ключа в свой файл
	return ssh.InsecureIgnoreHostKey()
}
