package history

import (
	"net"
	"strconv"
	"strings"

	"github.com/kibomibo/sshmon/internal/config"
)

func ServerKey(server config.Server) string {
	user := server.User
	if user == "" {
		user = "root"
	}
	port := server.Port
	if port == 0 {
		port = 22
	}
	address := net.JoinHostPort(strings.ToLower(server.Host), strconv.Itoa(port))
	return user + "@" + address
}
