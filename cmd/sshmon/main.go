package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/llm"
	"github.com/kibomibo/sshmon/internal/mcpsrv"
	"github.com/kibomibo/sshmon/internal/setup"
	"github.com/kibomibo/sshmon/internal/tui"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "путь к config.yaml")
	headless := flag.Bool("headless", false, "без TUI: сбор метрик + MCP-сервер на stdio")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	col := collect.New(cfg)
	go col.Run(ctx)

	if *headless {
		log.SetOutput(os.Stderr)
		log.Printf("sshmon headless: %d серверов, интервал %s, MCP на stdio", len(cfg.Servers), cfg.Interval)
		if err := mcpsrv.Serve(ctx, col); err != nil {
			log.Fatal(err)
		}
		return
	}

	p := tea.NewProgram(tui.New(col, llm.New(cfg.LLM), cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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

func writeTemplateAndExit(cfgPath string, loadErr error) {
	if werr := config.WriteDefault(cfgPath); werr != nil {
		fmt.Fprintf(os.Stderr, "sshmon: %v\nне удалось создать конфиг: %v\n", loadErr, werr)
	} else {
		fmt.Fprintf(os.Stderr, "Создан конфиг %s — добавьте свои серверы и запустите sshmon снова.\n", cfgPath)
	}
	os.Exit(1)
}
