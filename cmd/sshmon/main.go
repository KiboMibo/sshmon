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

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/history"
	"github.com/kibomibo/sshmon/internal/llm"
	"github.com/kibomibo/sshmon/internal/mcpsrv"
	"github.com/kibomibo/sshmon/internal/setup"
	"github.com/kibomibo/sshmon/internal/tui"
)

var version = "0.2.1"

func writeVersion(w io.Writer, v string) {
	fmt.Fprintf(w, "sshmon %s\n", v)
}

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "путь к config.yaml")
	headless := flag.Bool("headless", false, "без TUI: сбор метрик + MCP-сервер на stdio")
	importFlag := flag.Bool("import", false, "добавить серверы из ~/.ssh/config в существующий конфиг")
	versionFlag := flag.Bool("version", false, "показать версию и выйти")
	flag.Parse()
	if *versionFlag {
		writeVersion(os.Stdout, version)
		return
	}
	if *importFlag && *headless {
		fmt.Fprintln(os.Stderr, "sshmon: --import нельзя использовать вместе с --headless")
		os.Exit(1)
	}

	cfg, err := config.Load(*cfgPath)
	loaded := err == nil
	if err != nil {
		missing := errors.Is(err, fs.ErrNotExist)
		empty := errors.Is(err, config.ErrNoServers)
		if (missing || empty) && !*headless {
			cfg = firstRun(*cfgPath, empty)
		}
		if cfg == nil {
			if missing {
				writeTemplateAndExit(*cfgPath, err)
			}
			fmt.Fprintf(os.Stderr, "sshmon: %v\n", err)
			os.Exit(1)
		}
	}
	if *importFlag && loaded {
		cfg = importServers(*cfgPath, cfg)
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
		if err := mcpsrv.Serve(ctx, col); err != nil {
			log.Fatal(err)
		}
		return
	}

	model := tui.New(col, llm.New(cfg.LLM), cfg).WithHistory(historyService)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
// Возвращает загруженный конфиг или nil (тогда вызывающий пишет шаблон).
func firstRun(cfgPath string, populate bool) *config.Config {
	hosts, err := config.ParseSSHConfig(config.DefaultSSHConfigPath())
	if err != nil || len(hosts) == 0 {
		return nil // нет ssh-конфига — fallback на шаблон
	}
	servers, err := setup.Run(hosts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshmon: %v\n", err)
		os.Exit(1)
	}
	if len(servers) == 0 {
		fmt.Fprintln(os.Stderr, "Ничего не выбрано — выход. Конфиг не создан.")
		os.Exit(1)
	}
	var saveErr error
	if populate {
		saveErr = config.PopulateServers(cfgPath, servers)
	} else {
		saveErr = config.WriteWithServers(cfgPath, servers)
	}
	if saveErr != nil {
		fmt.Fprintf(os.Stderr, "sshmon: не удалось сохранить конфиг: %v\n", saveErr)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Создан конфиг %s (%d серверов).\n", cfgPath, len(servers))
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshmon: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func importServers(cfgPath string, cfg *config.Config) *config.Config {
	hosts, err := config.ParseSSHConfig(config.DefaultSSHConfigPath())
	if err != nil || len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "sshmon: в ~/.ssh/config нет хостов для импорта")
		return cfg
	}
	hosts = config.RemainingHosts(hosts, cfg.Servers)
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "Все хосты из ~/.ssh/config уже есть в конфиге.")
		return cfg
	}

	servers, err := setup.Run(hosts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshmon: ошибка выбора серверов: %v\n", err)
		os.Exit(1)
	}
	if len(servers) == 0 {
		fmt.Fprintln(os.Stderr, "Ничего не выбрано — конфиг не изменён.")
		return cfg
	}

	added, err := config.AddServers(cfgPath, servers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshmon: не удалось сохранить конфиг: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Добавлено серверов: %d.\n", added)
	updated, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshmon: не удалось загрузить обновлённый конфиг: %v\n", err)
		os.Exit(1)
	}
	return updated
}

func writeTemplateAndExit(cfgPath string, loadErr error) {
	if werr := config.WriteDefault(cfgPath); werr != nil {
		fmt.Fprintf(os.Stderr, "sshmon: %v\nне удалось создать конфиг: %v\n", loadErr, werr)
	} else {
		fmt.Fprintf(os.Stderr, "Создан конфиг %s — добавьте свои серверы и запустите sshmon снова.\n", cfgPath)
	}
	os.Exit(1)
}
