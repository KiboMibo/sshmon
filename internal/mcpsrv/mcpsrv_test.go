package mcpsrv

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kibomibo/sshmon/internal/buildinfo"
	"github.com/kibomibo/sshmon/internal/collect"
)

type fakeCollector struct {
	snap       collect.Snapshot
	tail       string
	tailErr    error
	lastServer string
	lastLines  int
}

func (f *fakeCollector) Snapshot() collect.Snapshot { return f.snap }

func (f *fakeCollector) TailLog(server string, lines int) (string, error) {
	f.lastServer, f.lastLines = server, lines
	return f.tail, f.tailErr
}

// resultText извлекает единственный текстовый блок из результата инструмента.
func resultText(t *testing.T, m map[string]any) (string, bool) {
	t.Helper()
	content, ok := m["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("нет content в результате: %#v", m)
	}
	text, _ := content[0]["text"].(string)
	isErr, _ := m["isError"].(bool)
	return text, isErr
}

func TestHandleInitializeEchoesProtocolAndReportsVersion(t *testing.T) {
	resp := handle(&fakeCollector{}, &request{Method: "initialize", ID: json.RawMessage("1"), Params: json.RawMessage(`{"protocolVersion":"2024-11-05"}`)})
	res, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", resp.Result)
	}
	if res["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	info := res["serverInfo"].(map[string]any)
	if info["version"] != buildinfo.Version {
		t.Errorf("serverInfo.version = %v, want %s", info["version"], buildinfo.Version)
	}
}

func TestHandleInitializeDefaultsProtocolWhenMissing(t *testing.T) {
	resp := handle(&fakeCollector{}, &request{Method: "initialize", ID: json.RawMessage("1")})
	res := resp.Result.(map[string]any)
	if res["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want default %s", res["protocolVersion"], protocolVersion)
	}
}

func TestHandleUnknownMethodReturnsMethodNotFound(t *testing.T) {
	resp := handle(&fakeCollector{}, &request{Method: "frobnicate", ID: json.RawMessage("1")})
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("error = %#v, want code -32601", resp.Error)
	}
}

func TestHandleToolsListReturnsFourTools(t *testing.T) {
	resp := handle(&fakeCollector{}, &request{Method: "tools/list", ID: json.RawMessage("1")})
	res := resp.Result.(map[string]any)
	tools := res["tools"].([]map[string]any)
	if len(tools) != 4 {
		t.Fatalf("tools = %d, want 4", len(tools))
	}
}

func TestCallToolListServers(t *testing.T) {
	col := &fakeCollector{snap: collect.Snapshot{Servers: []collect.Metrics{
		{Name: "web", Online: true},
		{Name: "db", Online: false, Err: "timeout"},
	}}}
	text, isErr := resultText(t, callTool(col, json.RawMessage(`{"name":"list_servers"}`)))
	if isErr {
		t.Fatal("list_servers reported error")
	}
	var rows []struct {
		Name   string `json:"name"`
		Online bool   `json:"online"`
		Err    string `json:"err"`
	}
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		t.Fatalf("unmarshal rows: %v", err)
	}
	if len(rows) != 2 || rows[0].Name != "web" || rows[1].Err != "timeout" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestCallToolGetMetricsByNameAndUnknown(t *testing.T) {
	col := &fakeCollector{snap: collect.Snapshot{Servers: []collect.Metrics{{Name: "web", CPUPct: 12}}}}

	text, isErr := resultText(t, callTool(col, json.RawMessage(`{"name":"get_metrics","arguments":{"server":"web"}}`)))
	if isErr {
		t.Fatal("get_metrics web reported error")
	}
	if !strings.Contains(text, `"Name": "web"`) {
		t.Fatalf("metrics text = %s", text)
	}

	_, isErr = resultText(t, callTool(col, json.RawMessage(`{"name":"get_metrics","arguments":{"server":"nope"}}`)))
	if !isErr {
		t.Fatal("unknown server must report error")
	}
}

func TestCallToolTailLogPassesArgsAndSurfacesError(t *testing.T) {
	col := &fakeCollector{tail: "line1\nline2"}
	text, isErr := resultText(t, callTool(col, json.RawMessage(`{"name":"tail_log","arguments":{"server":"web","lines":50}}`)))
	if isErr || text != "line1\nline2" {
		t.Fatalf("tail_log = %q isErr=%v", text, isErr)
	}
	if col.lastServer != "web" || col.lastLines != 50 {
		t.Fatalf("TailLog got server=%q lines=%d", col.lastServer, col.lastLines)
	}

	col.tailErr = errors.New("ssh down")
	_, isErr = resultText(t, callTool(col, json.RawMessage(`{"name":"tail_log","arguments":{"server":"web"}}`)))
	if !isErr {
		t.Fatal("tail_log error must set isError")
	}

	_, isErr = resultText(t, callTool(col, json.RawMessage(`{"name":"tail_log"}`)))
	if !isErr {
		t.Fatal("missing server must report error")
	}
}

func TestCallToolRejectsMalformedArguments(t *testing.T) {
	// arguments is a JSON string, not an object → unmarshal into struct fails.
	text, isErr := resultText(t, callTool(&fakeCollector{}, json.RawMessage(`{"name":"tail_log","arguments":"oops"}`)))
	if !isErr || !strings.Contains(text, "bad arguments") {
		t.Fatalf("expected bad arguments error, got %q isErr=%v", text, isErr)
	}
}

func TestLoopHandlesRequestsSkipsNotificationsAndReportsParseErrors(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`, // notification: no id, no reply
		``,          // blank line: skipped
		`{not json`, // parse error
	}, "\n")
	var out bytes.Buffer
	if err := loop(strings.NewReader(in), &out, &fakeCollector{}); err != nil {
		t.Fatalf("loop: %v", err)
	}

	var lines []response
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r response
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("decode response %q: %v", l, err)
		}
		lines = append(lines, r)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d responses, want 2 (ping + parse error), out=%s", len(lines), out.String())
	}
	if string(lines[0].ID) != "1" || lines[0].Result == nil {
		t.Errorf("first response = %#v, want ping result for id 1", lines[0])
	}
	if string(lines[1].ID) != "null" || lines[1].Error == nil || lines[1].Error.Code != -32700 {
		t.Errorf("second response = %#v, want parse error with null id", lines[1])
	}
}
