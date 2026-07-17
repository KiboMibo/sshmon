package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/llm"
	"github.com/kibomibo/sshmon/internal/mcpsrv"
	"github.com/kibomibo/sshmon/internal/tui"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "путь к config.yaml")
	headless := flag.Bool("headless", false, "без TUI: сбор метрик + MCP-сервер на stdio")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshmon: %v\nПример конфига: config.example.yaml в репозитории\n", err)
		os.Exit(1)
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
