// Package mcpsrv — минимальный MCP-сервер (stdio, NDJSON JSON-RPC 2.0),
// отдаёт метрики серверов любому локальному агенту.
package mcpsrv

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kibomibo/sshmon/internal/collect"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func Serve(ctx context.Context, col *collect.Collector) error {
	done := make(chan error, 1)
	go func() { done <- loop(col) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}

func loop(col *collect.Collector) error {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1024*1024), 1024*1024)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		line := in.Text()
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		if req.ID == nil {
			continue // notification (например notifications/initialized)
		}
		if err := out.Encode(handle(col, &req)); err != nil {
			return err
		}
	}
	return in.Err()
}

func handle(col *collect.Collector, req *request) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-03-26"
		}
		resp.Result = map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sshmon", "version": "0.3.0"},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolList}
	case "tools/call":
		resp.Result = callTool(col, req.Params)
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

var toolList = []map[string]any{
	{
		"name":        "list_servers",
		"description": "Список наблюдаемых серверов и их доступность",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "get_metrics",
		"description": "Метрики сервера: CPU, память, диски, IO, сеть, открытые порты. Без аргумента server — все серверы.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"server": map[string]any{"type": "string", "description": "имя сервера из конфига"}},
		},
	},
	{
		"name":        "get_issues",
		"description": "Обнаруженные проблемы (превышение порогов CPU/памяти/диска, недоступные серверы)",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "tail_log",
		"description": "Последние строки системных логов сервера (journalctl/syslog/logread)",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"server": map[string]any{"type": "string"},
				"lines":  map[string]any{"type": "integer", "default": 200},
			},
			"required": []string{"server"},
		},
	},
}

func callTool(col *collect.Collector, params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return textResult("bad params: "+err.Error(), true)
	}
	var args struct {
		Server string `json:"server"`
		Lines  int    `json:"lines"`
	}
	if len(p.Arguments) > 0 {
		_ = json.Unmarshal(p.Arguments, &args)
	}
	snap := col.Snapshot()
	switch p.Name {
	case "list_servers":
		type row struct {
			Name   string `json:"name"`
			Online bool   `json:"online"`
			Err    string `json:"err,omitempty"`
		}
		var rows []row
		for _, m := range snap.Servers {
			rows = append(rows, row{m.Name, m.Online, m.Err})
		}
		return jsonResult(rows)
	case "get_metrics":
		if args.Server == "" {
			return jsonResult(snap.Servers)
		}
		for _, m := range snap.Servers {
			if m.Name == args.Server {
				return jsonResult(m)
			}
		}
		return textResult(fmt.Sprintf("неизвестный сервер %q", args.Server), true)
	case "get_issues":
		if snap.Issues == nil {
			snap.Issues = []collect.Issue{}
		}
		return jsonResult(snap.Issues)
	case "tail_log":
		if args.Server == "" {
			return textResult("нужен аргумент server", true)
		}
		text, err := col.TailLog(args.Server, args.Lines)
		if err != nil {
			return textResult(err.Error(), true)
		}
		return textResult(text, false)
	default:
		return textResult("unknown tool: "+p.Name, true)
	}
}

func jsonResult(v any) map[string]any {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return textResult(err.Error(), true)
	}
	return textResult(string(b), false)
}

func textResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}
