// Package sshx — ленивое SSH-подключение с автопереподключением после сбоев.
package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
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

func (c *Client) conn(ctx context.Context) (*ssh.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.c != nil {
		return c.c, nil
	}
	auth, needsPassphrase, cleanup, err := authMethods(c.cfg, c.passphrase)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		if needsPassphrase {
			return nil, ErrPassphraseRequired
		}
		return nil, fmt.Errorf("нет способа аутентификации (key/agent/password)")
	}
	checkHostKey := hostKeyCallback(c.cfg)
	addr := c.cfg.Addr()
	cl, err := dialContext(ctx, addr, &ssh.ClientConfig{
		User:              c.cfg.User,
		Auth:              auth,
		HostKeyCallback:   checkHostKey,
		HostKeyAlgorithms: hostKeyAlgorithms(checkHostKey, addr),
		Timeout:           10 * time.Second,
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

// dialContext подключается с учётом отмены: ssh.Dial контекст игнорирует,
// а рукопожатие блокирует до ответа сервера, поэтому TCP-соединение
// закрывается извне — иначе выход из приложения ждёт таймаута.
func dialContext(ctx context.Context, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	tcp, err := (&net.Dialer{Timeout: cfg.Timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = tcp.Close() })
	defer stop()
	sc, chans, reqs, err := ssh.NewClientConn(tcp, addr, cfg)
	if err != nil {
		_ = tcp.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	return ssh.NewClient(sc, chans, reqs), nil
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
	cl, err := c.conn(ctx)
	if err != nil {
		return "", err
	}
	sess, err := cl.NewSession()
	if err != nil {
		c.drop()
		return "", err
	}
	// Закрытие сессии здесь же разблокирует горутину runCommand, если основной
	// путь вернулся по отмене контекста: Output() на закрытой сессии сразу отдаёт
	// ошибку в буферизованный канал и горутина завершается.
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

// runCommand ждёт результат команды или отмену контекста.
//
// При отмене соединение не рвётся: ssh.Client общий со сборщиком метрик, и выход
// из экрана «Процессы» не должен стоить всему приложению нового handshake.
// Схема безопасна, потому что горутина output() и вызывающий не разделяют ничего,
// кроме буферизованного на 1 канала: вызывающий закрывает свою ssh.Session
// (defer sess.Close() в RunContext) сразу после возврата, Output() на закрытой
// сессии возвращает ошибку, горутина кладёт её в канал, который никто уже не
// читает, и завершается — ни записи в закрытую сессию, ни утечки. Сама
// ssh.Session допускает Close() параллельно с выполняющимся Output().
//
// drop() остаётся только для реальных транспортных ошибок: иначе мёртвый
// ssh.Client остался бы в кэше и все последующие опросы падали бы на нём.
func runCommand(ctx context.Context, output func() ([]byte, error), drop func()) ([]byte, error) {
	result := make(chan commandResult, 1)
	go func() {
		out, err := output()
		result <- commandResult{out: out, err: err}
	}()
	select {
	case res := <-result:
		if isTransportFailure(res.err) {
			drop()
		}
		return res.out, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// isTransportFailure отличает поломку соединения от штатного завершения удалённой
// команды. Ненулевой код возврата — обычное дело для цепочек `a || b` в наших
// командах и не повод закрывать ssh.Client, которым пользуются другие экраны.
func isTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *ssh.ExitError
	return !errors.As(err, &exitErr)
}

func authMethods(cfg config.Server, passphrase []byte) ([]ssh.AuthMethod, bool, func(), error) {
	// Порядок как у openssh: ssh-agent → локальный файл ключа → пароль.
	// Сначала agent, чтобы уже загруженные в ssh-add ключи работали без passphrase-промпта.
	cleanup := func() {}
	var out []ssh.AuthMethod
	agentReachable := false
	expected := publicKeyFromKeyFile(cfg.Key, passphrase)
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			// Соединение с агентом должно жить до конца ssh.Dial: agent-signer'ы
			// вызывают Sign() по этому conn во время handshake. Закрывает вызывающий
			// через cleanup(), иначе "use of closed network connection".
			cleanup = func() { _ = conn.Close() }
			if expected == nil {
				// Не можем вывести публичный ключ cfg.Key — предлагаем все ключи агента
				// (старое поведение). Callback вычисляет signers лениво, при аутентификации.
				out = append(out, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
			} else {
				// Эмуляция openssh IdentitiesOnly yes: агент всё ещё источник,
				// но предлагаем только ключ, совпадающий с cfg.Key, чтобы уложиться
				// в sshd MaxAuthTries при большом количестве загруженных ключей.
				signers, _ := agent.NewClient(conn).Signers()
				filtered := filterAgentSigners(signers, expected)
				if len(filtered) > 0 {
					out = append(out, ssh.PublicKeys(filtered...))
				}
			}
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
							return nil, false, cleanup, ErrInvalidPassphrase
						}
						out = append(out, ssh.PublicKeys(signer))
					}
				}
			}
		}
	}
	if pw := cfg.Pass(); pw != "" {
		out = append(out, ssh.Password(pw))
	}
	return out, needsPassphrase, cleanup, nil
}

// filterAgentSigners ограничивает список signer'ов агента теми, чей публичный ключ
// совпадает с ожидаемым. Это эмуляция openssh IdentitiesOnly yes: при большом числе
// ключей в ssh-agent мы не превысим sshd MaxAuthTries (по умолчанию 6).
//
// nil expected → возвращаются все signers без фильтрации (legacy-поведение).
// Пустой результат после фильтрации → возвращаются все signers (fallback,
// чтобы не молча отключить аутентификацию, если cfg.Key не загружен в агент).
func filterAgentSigners(signers []ssh.Signer, expected ssh.PublicKey) []ssh.Signer {
	if expected == nil {
		return signers
	}
	want := expected.Marshal()
	var out []ssh.Signer
	for _, s := range signers {
		if bytes.Equal(s.PublicKey().Marshal(), want) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return signers
	}
	return out
}

// publicKeyFromKeyFile выводит публичный ключ для приватного файла cfg.Key.
// Пробуем .pub sidecar (работает для зашифрованных ключей), затем сам приватный
// файл (только незашифрованный), затем с passphrase. nil если ничего не сработало.
func publicKeyFromKeyFile(keyPath string, passphrase []byte) ssh.PublicKey {
	if keyPath == "" {
		return nil
	}
	if b, err := os.ReadFile(keyPath + ".pub"); err == nil {
		if pub, _, _, _, err := ssh.ParseAuthorizedKey(b); err == nil {
			return pub
		}
	}
	if b, err := os.ReadFile(keyPath); err == nil {
		if signer, err := ssh.ParsePrivateKey(b); err == nil {
			return signer.PublicKey()
		}
		if len(passphrase) > 0 {
			if signer, err := ssh.ParsePrivateKeyWithPassphrase(b, passphrase); err == nil {
				return signer.PublicKey()
			}
		}
	}
	return nil
}

// FriendlyErr переводит сырые ошибки ssh.Dial/Run в человекочитаемые подсказки.
// Не известные ошибки возвращаются как есть (err.Error()).
func FriendlyErr(err error, srv config.Server) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "knownhosts: key mismatch"):
		return fmt.Sprintf("host-key сервера не совпадает с записью в ~/.ssh/known_hosts — сверьте отпечаток с тем, что показывает хостер; если сервер переустанавливали, выполните `ssh-keygen -R %s` и переподключитесь обычным ssh", knownHostsTarget(srv))
	case strings.Contains(msg, "no common algorithm for host key"):
		return fmt.Sprintf("сервер не предлагает host-key того типа, что записан в ~/.ssh/known_hosts — вероятно сервер переустанавливали; сверьте отпечаток и выполните `ssh-keygen -R %s`", knownHostsTarget(srv))
	case strings.Contains(msg, "knownhosts: key is unknown"):
		return "хост отсутствует в ~/.ssh/known_hosts — подключитесь к нему один раз обычным ssh, сверьте и подтвердите отпечаток, после этого sshmon увидит ключ"
	case strings.Contains(msg, "unable to authenticate"):
		return "не удалось аутентифицироваться — проверьте ключ/пароль и что ssh-agent загружен (ssh-add -l)"
	case strings.Contains(msg, "connection refused"):
		return "сеть: подключение отклонено — проверьте что sshd запущен и порт указан верно"
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "deadline exceeded"):
		return "сеть: таймаут подключения — хост недоступен или порт закрыт firewall"
	}
	return msg
}

func knownHostsTarget(srv config.Server) string {
	switch {
	case srv.Host == "":
		return "<host>"
	case srv.Port == 0 || srv.Port == 22:
		return srv.Host
	default:
		return fmt.Sprintf("[%s]:%d", srv.Host, srv.Port)
	}
}

// Второй результат = true только когда проверка снята молча (known_hosts нет),
// но НЕ при явном insecure_host_key — вызывающий предупреждает лишь в этом случае.
func hostKeyVerification(cfg config.Server, knownHostsPath string) (ssh.HostKeyCallback, bool) {
	if cfg.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), false // явный opt-in в конфиге
	}
	if cb, err := knownhosts.New(knownHostsPath); err == nil {
		return cb, false
	}
	// ponytail: нет known_hosts — принимаем любой ключ, иначе утилита не стартует;
	// апгрейд: TOFU с записью ключа в свой файл
	return ssh.InsecureIgnoreHostKey(), true
}

var insecureHostKeyWarn sync.Once

func hostKeyCallback(cfg config.Server) ssh.HostKeyCallback {
	home, _ := os.UserHomeDir()
	cb, silentInsecure := hostKeyVerification(cfg, home+"/.ssh/known_hosts")
	if silentInsecure {
		insecureHostKeyWarn.Do(func() {
			fmt.Fprintln(os.Stderr, "sshmon: ~/.ssh/known_hosts не найден — host-key не проверяется (риск MITM); создайте файл или задайте insecure_host_key: true осознанно")
		})
	}
	return cb
}

// Без этого Go-клиент просит host-key своего любимого типа (ecdsa/rsa), а не тот,
// что записан в known_hosts, и проверка падает с "key mismatch" на честном сервере.
// Типы вытаскиваются из KeyError.Want: публичного API для этого в x/crypto нет,
// поэтому callback зовётся заведомо чужим ключом. nil = ограничений нет.
func hostKeyAlgorithms(cb ssh.HostKeyCallback, addr string) []string {
	probe, err := ssh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
	if err != nil {
		return nil
	}
	var keyErr *knownhosts.KeyError
	if !errors.As(cb(addr, &net.TCPAddr{IP: net.IPv4zero}, probe), &keyErr) {
		return nil
	}
	var out []string
	for _, known := range keyErr.Want {
		if typ := known.Key.Type(); !slices.Contains(out, typ) {
			out = append(out, typ)
		}
	}
	return out
}
