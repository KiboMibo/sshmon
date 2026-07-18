// Package sshx — ленивое SSH-подключение с автопереподключением после сбоев.
package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/kibomibo/sshmon/internal/config"
)

type Client struct {
	cfg config.Server
	mu  sync.Mutex
	c   *ssh.Client
}

func New(cfg config.Server) *Client { return &Client{cfg: cfg} }

func (c *Client) conn() (*ssh.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.c != nil {
		return c.c, nil
	}
	auth := authMethods(c.cfg)
	if len(auth) == 0 {
		return nil, fmt.Errorf("нет способа аутентификации (key/agent/password)")
	}
	cl, err := ssh.Dial("tcp", c.cfg.Addr(), &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback(c.cfg),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	c.c = cl
	return cl, nil
}

func (c *Client) drop() {
	c.mu.Lock()
	if c.c != nil {
		c.c.Close()
		c.c = nil
	}
	c.mu.Unlock()
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

func authMethods(cfg config.Server) []ssh.AuthMethod {
	var out []ssh.AuthMethod
	if cfg.Key != "" {
		if b, err := os.ReadFile(cfg.Key); err == nil {
			if signer, err := ssh.ParsePrivateKey(b); err == nil {
				out = append(out, ssh.PublicKeys(signer))
			}
		}
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			out = append(out, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	if cfg.Password != "" {
		out = append(out, ssh.Password(cfg.Password))
	}
	return out
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
