// Package sshx — ленивое SSH-подключение с автопереподключением после сбоев.
package sshx

import (
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
	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		out, err := sess.Output(cmd)
		ch <- res{out, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			if len(r.out) > 0 {
				return string(r.out), nil
			}
			return "", r.err
		}
		return string(r.out), nil
	case <-time.After(timeout):
		c.drop()
		return "", fmt.Errorf("таймаут %s", timeout)
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
