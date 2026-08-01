package collect

import "testing"

func TestSanitizeLineStripsTerminalControlSequences(t *testing.T) {
	// Given: строки, какие может напечатать в свой лог чужой процесс.
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain text is untouched", "postgres[812]: checkpoint starting", "postgres[812]: checkpoint starting"},
		{"osc 52 clipboard hijack", "before\x1b]52;c;ZXZpbA==\x07after", "beforeafter"},
		{"osc 0 window title", "\x1b]0;pwned\x07log line", "log line"},
		{"osc terminated by st", "a\x1b]52;c;ZXZpbA==\x1b\\b", "ab"},
		{"unterminated osc eats the rest", "ok\x1b]0;title", "ok"},
		{"csi colours", "\x1b[31mERROR\x1b[0m disk full", "ERROR disk full"},
		{"csi cursor move", "line\x1b[2Aoverwrite", "lineoverwrite"},
		{"two byte escape", "a\x1bZb", "ab"},
		{"c1 csi", "a31mb", "a31mb"},
		{"tab becomes a space", "a\tb", "a b"},
		{"line breaks become spaces", "a\r\nb", "a  b"},
		{"bare controls are dropped", "a\x00\x08b\x7f", "ab"},
		{"unicode survives", "юникод ✓", "юникод ✓"},
	}

	// When/Then: санитизация оставляет текст и убирает всё управляющее.
	for _, tc := range cases {
		if got := SanitizeLine(tc.raw); got != tc.want {
			t.Errorf("%s: SanitizeLine(%q) = %q, want %q", tc.name, tc.raw, got, tc.want)
		}
	}
}

func TestLogBufferAppendSanitizesRemoteLines(t *testing.T) {
	// Given: буфер логов, куда строки кладёт экран.
	buffer := NewLogBuffer(10)

	// When: с сервера пришла строка с OSC 52.
	buffer.Append("nginx: \x1b]52;c;ZXZpbA==\x07GET /")

	// Then: до потребителей доедет только текст.
	visible := buffer.Visible()
	if len(visible) != 1 || visible[0] != "nginx: GET /" {
		t.Fatalf("visible = %#v", visible)
	}
}

func TestParsersSanitizeRemoteFields(t *testing.T) {
	// Given: вывод ps, ss и docker с escape-последовательностями в полях.
	processes, err := ParseProcesses("  812  1.0  2.0 /usr/bin/\x1b]52;c;ZXZpbA==\x07evil --flag")
	if err != nil || len(processes) != 1 || processes[0].Command != "/usr/bin/evil --flag" {
		t.Fatalf("processes = %#v err = %v", processes, err)
	}

	ports, err := ParsePorts("tcp LISTEN 0 128 0.0.0.0:80 users:((\"ngi\x1b[31mnx\",pid=7,fd=6))")
	if err != nil || len(ports) != 1 || ports[0].Process != "nginx" {
		t.Fatalf("ports = %#v err = %v", ports, err)
	}

	// Колонки docker разделены табуляцией — она должна пережить санитизацию.
	containers, err := ParseContainers("abc\tapi\x1b[0m-worker\timage\tUp 3 days\t80/tcp\t3 weeks ago", "")
	if err != nil || len(containers) != 1 || containers[0].Name != "api-worker" || containers[0].Status != "Up 3 days" {
		t.Fatalf("containers = %#v err = %v", containers, err)
	}

	units := ParseSystemdUnits("ngi\x1b[1mnx.service loaded active running \x1b]0;title\x07web server")
	if len(units) != 1 || units[0].Name != "nginx.service" || units[0].Description != "web server" {
		t.Fatalf("units = %#v", units)
	}
}
