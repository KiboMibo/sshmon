package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/buildinfo"
	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/history"
	"github.com/kibomibo/sshmon/internal/llm"
	"github.com/kibomibo/sshmon/internal/mcpsrv"
	"github.com/kibomibo/sshmon/internal/setup"
	"github.com/kibomibo/sshmon/internal/tui"
)

func writeVersion(w io.Writer, v string) {
	fmt.Fprintf(w, "sshmon %s\n", v)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run содержит весь жизненный цикл процесса. Возврат ошибки (вместо os.Exit
// из вложенных хелперов) гарантирует, что defer-cleanup ниже — остановка
// сбора и flush истории — выполнится на всех путях выхода, включая ошибки
// headless MCP и TUI.
func run() error {
	cfgPath := flag.String("config", config.DefaultPath(), "путь к config.yaml")
	headless := flag.Bool("headless", false, "без TUI: сбор метрик + MCP-сервер на stdio")
	importFlag := flag.Bool("import", false, "добавить серверы из ~/.ssh/config в существующий конфиг")
	versionFlag := flag.Bool("version", false, "показать версию и выйти")
	flag.Parse()
	if *versionFlag {
		writeVersion(os.Stdout, buildinfo.Version)
		return nil
	}
	if *importFlag && *headless {
		return errors.New("sshmon: --import нельзя использовать вместе с --headless")
	}

	cfg, err := config.Load(*cfgPath)
	loaded := err == nil
	if err != nil {
		missing := errors.Is(err, fs.ErrNotExist)
		empty := errors.Is(err, config.ErrNoServers)
		if (missing || empty) && !*headless {
			c, ferr := firstRun(*cfgPath, empty)
			if ferr != nil {
				return ferr
			}
			cfg = c
		}
		if cfg == nil {
			if missing {
				return writeTemplate(*cfgPath, err)
			}
			return fmt.Errorf("sshmon: %v", err)
		}
	}
	if *importFlag && loaded {
		c, ferr := importServers(*cfgPath, cfg)
		if ferr != nil {
			return ferr
		}
		cfg = c
	}

	if w := config.SecretPermWarning(*cfgPath, cfg); w != "" {
		fmt.Fprintln(os.Stderr, w)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	col := collect.New(cfg)
	historyService := openHistory(cfg.History, os.Stderr)
	var runtime sync.WaitGroup
	if historyService != nil {
		runtime.Add(1)
		go func() {
			defer runtime.Done()
			maintainHistory(ctx, historyService, os.Stderr)
		}()
	}
	runtime.Add(1)
	go func() {
		defer runtime.Done()
		if historyService != nil {
			col.RunWithSink(ctx, col.HistorySink(historyService))
			return
		}
		col.Run(ctx)
	}()
	defer func() {
		stop()
		runtime.Wait()
		if historyService != nil {
			if err := historyService.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "sshmon: не удалось закрыть историю: %v\n", err)
			}
		}
	}()

	if *headless {
		log.SetOutput(os.Stderr)
		log.Printf("sshmon headless: %d серверов, интервал %s, MCP на stdio", len(cfg.Servers), cfg.Interval)
		return mcpsrv.Serve(ctx, col)
	}

	model := tui.New(col, llm.New(cfg.LLM), cfg).WithHistory(historyService)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func openHistory(cfg config.History, stderr io.Writer) *history.Service {
	service, err := history.OpenService(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "sshmon: история недоступна: %v\n", err)
		return nil
	}
	return service
}

func maintainHistory(ctx context.Context, service *history.Service, stderr io.Writer) {
	maintain := func() {
		if err := service.Maintain(ctx, time.Now()); err != nil && ctx.Err() == nil {
			fmt.Fprintf(stderr, "sshmon: обслуживание истории: %v\n", err)
		}
	}
	maintain()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := service.Maintain(ctx, now); err != nil && ctx.Err() == nil {
				fmt.Fprintf(stderr, "sshmon: обслуживание истории: %v\n", err)
			}
		}
	}
}

// firstRun предлагает выбрать серверы из ~/.ssh/config для нового или пустого конфига.
// Возвращает (nil, nil), когда ssh-конфига нет — тогда вызывающий пишет шаблон.
func firstRun(cfgPath string, populate bool) (*config.Config, error) {
	hosts, err := config.ParseSSHConfig(config.DefaultSSHConfigPath())
	if err != nil || len(hosts) == 0 {
		return nil, nil // нет ssh-конфига — fallback на шаблон
	}
	servers, err := setup.Run(hosts)
	if err != nil {
		return nil, fmt.Errorf("sshmon: %w", err)
	}
	if len(servers) == 0 {
		return nil, errors.New("Ничего не выбрано — выход. Конфиг не создан.")
	}
	var saveErr error
	if populate {
		saveErr = config.PopulateServers(cfgPath, servers)
	} else {
		saveErr = config.WriteWithServers(cfgPath, servers)
	}
	if saveErr != nil {
		return nil, fmt.Errorf("sshmon: не удалось сохранить конфиг: %w", saveErr)
	}
	fmt.Fprintf(os.Stderr, "Создан конфиг %s (%d серверов).\n", cfgPath, len(servers))
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("sshmon: %w", err)
	}
	return cfg, nil
}

func importServers(cfgPath string, cfg *config.Config) (*config.Config, error) {
	hosts, err := config.ParseSSHConfig(config.DefaultSSHConfigPath())
	if err != nil || len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "sshmon: в ~/.ssh/config нет хостов для импорта")
		return cfg, nil
	}
	hosts = config.RemainingHosts(hosts, cfg.Servers)
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "Все хосты из ~/.ssh/config уже есть в конфиге.")
		return cfg, nil
	}

	servers, err := setup.Run(hosts)
	if err != nil {
		return nil, fmt.Errorf("sshmon: ошибка выбора серверов: %w", err)
	}
	if len(servers) == 0 {
		fmt.Fprintln(os.Stderr, "Ничего не выбрано — конфиг не изменён.")
		return cfg, nil
	}

	added, err := config.AddServers(cfgPath, servers)
	if err != nil {
		return nil, fmt.Errorf("sshmon: не удалось сохранить конфиг: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Добавлено серверов: %d.\n", added)
	updated, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("sshmon: не удалось загрузить обновлённый конфиг: %w", err)
	}
	return updated, nil
}

// writeTemplate создаёт конфиг-шаблон и возвращает ошибку, сигнализирующую
// вызывающему завершиться с кодом 1 (пользователь должен вписать серверы).
func writeTemplate(cfgPath string, loadErr error) error {
	if werr := config.WriteDefault(cfgPath); werr != nil {
		return fmt.Errorf("sshmon: %v\nне удалось создать конфиг: %v", loadErr, werr)
	}
	return fmt.Errorf("Создан конфиг %s — добавьте свои серверы и запустите sshmon снова.", cfgPath)
}
