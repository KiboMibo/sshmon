package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SSHHost — хост из ~/.ssh/config.
type SSHHost struct {
	Alias    string
	HostName string
	User     string
	Port     int
	Key      string
	Group    string // basename include-файла без расширения; "" для главного config
}

// ParseSSHConfig читает ssh-конфиг и все его Include-файлы.
// Хосты из include-файлов получают группу по имени файла
// (~/.ssh/conf.d/prod.conf → группа "prod").
func ParseSSHConfig(path string) ([]SSHHost, error) {
	seen := map[string]bool{}
	return parseSSHFile(path, "", seen)
}

// DefaultSSHConfigPath — ~/.ssh/config.
func DefaultSSHConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "config")
}

func parseSSHFile(path, group string, seen map[string]bool) ([]SSHHost, error) {
	if seen[path] {
		return nil, nil // защита от циклических Include
	}
	seen[path] = true
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out []SSHHost
	var cur []*SSHHost // алиасы текущего Host-блока
	inMatch := false   // Match-блоки пропускаем целиком

	flush := func() {
		for _, h := range cur {
			if h.HostName == "" {
				h.HostName = h.Alias
			}
			out = append(out, *h)
		}
		cur = nil
	}

	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "host":
			flush()
			inMatch = false
			for _, a := range strings.Fields(val) {
				if strings.ContainsAny(a, "*?!") {
					continue // wildcard-паттерны не мониторим
				}
				cur = append(cur, &SSHHost{Alias: a, Group: group})
			}
		case "match":
			flush()
			inMatch = true
		case "include":
			if inMatch || len(cur) > 0 {
				continue // Include внутри блоков не поддерживаем
			}
			for _, pat := range strings.Fields(val) {
				pat = expandHome(pat)
				if !filepath.IsAbs(pat) {
					pat = filepath.Join(filepath.Dir(path), pat)
				}
				files, _ := filepath.Glob(pat)
				for _, f := range files {
					g := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
					hosts, err := parseSSHFile(f, g, seen)
					if err == nil {
						out = append(out, hosts...)
					}
				}
			}
		case "hostname":
			for _, h := range cur {
				h.HostName = val
			}
		case "user":
			for _, h := range cur {
				h.User = val
			}
		case "port":
			if p, err := strconv.Atoi(val); err == nil {
				for _, h := range cur {
					h.Port = p
				}
			}
		case "identityfile":
			for _, h := range cur {
				if h.Key == "" {
					h.Key = val // первый IdentityFile
				}
			}
		}
	}
	flush()
	return out, nil
}

// splitKV делит строку ssh-конфига на ключ и значение:
// поддерживает и "Key value", и "Key=value".
func splitKV(line string) (key, val string, ok bool) {
	if i := strings.IndexAny(line, " \t="); i > 0 {
		return line[:i], strings.Trim(strings.TrimSpace(line[i+1:]), `"`), true
	}
	return "", "", false
}

// Servers превращает ssh-хосты в конфиг sshmon.
func HostsToServers(hosts []SSHHost) []Server {
	var out []Server
	for _, h := range hosts {
		out = append(out, Server{
			Name:  h.Alias,
			Host:  h.HostName,
			Port:  h.Port,
			User:  h.User,
			Key:   h.Key,
			Group: h.Group,
		})
	}
	return out
}
