package collect

import (
	"context"
	"errors"
	"strings"
)

type SystemdUnit struct {
	Name        string
	Load        string
	Active      string
	Sub         string
	Description string
}

// systemdDiscoveryCommand — ограниченный список запущенных сервисов, когда юниты не заданы в конфиге.
const systemdDiscoveryCommand = "systemctl list-units --type=service --state=running --no-pager --no-legend --plain 2>/dev/null | head -n 100"

func (c *Collector) systemdUnitsCommand(configured []string) (string, error) {
	if len(configured) == 0 {
		return systemdDiscoveryCommand, nil
	}
	for _, name := range configured {
		if !safeLogName.MatchString(name) {
			return "", errors.New("недопустимое имя systemd unit")
		}
	}
	return "systemctl list-units --all --no-pager --no-legend --plain " + strings.Join(configured, " "), nil
}

func (c *Collector) SystemdUnits(ctx context.Context, server string, configured []string) ([]SystemdUnit, error) {
	command, err := c.systemdUnitsCommand(configured)
	if err != nil {
		return nil, err
	}
	client, err := c.clientFor(server)
	if err != nil {
		return nil, err
	}
	raw, err := client.RunContext(ctx, command)
	if err != nil {
		return nil, err
	}
	return ParseSystemdUnits(raw), nil
}

func ParseSystemdUnits(raw string) []SystemdUnit {
	var units []SystemdUnit
	for line := range strings.SplitSeq(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		units = append(units, SystemdUnit{
			Name:        fields[0],
			Load:        fields[1],
			Active:      fields[2],
			Sub:         fields[3],
			Description: strings.Join(fields[4:], " "),
		})
	}
	return units
}
