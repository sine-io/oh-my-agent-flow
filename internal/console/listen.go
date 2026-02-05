package console

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const bindHost = "127.0.0.1"

func ListenLocal(port int) (net.Listener, string, error) {
	if port < 0 || port > 65535 {
		return nil, "", fmt.Errorf("invalid --port %d (must be 0..65535)", port)
	}

	addr := net.JoinHostPort(bindHost, strconv.Itoa(port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			return nil, "", fmt.Errorf("port %d is already in use", port)
		}
		return nil, "", fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://%s:%d", bindHost, actualPort)
	return listener, baseURL, nil
}
