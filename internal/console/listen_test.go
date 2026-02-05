package console

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestListenLocal_PortZeroChoosesFreePort(t *testing.T) {
	listener, baseURL, err := ListenLocal(0)
	if err != nil {
		t.Fatalf("ListenLocal(0) error: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	if !strings.HasPrefix(baseURL, "http://127.0.0.1:") {
		t.Fatalf("unexpected baseURL: %q", baseURL)
	}
	if baseURL == "http://127.0.0.1:0" {
		t.Fatalf("expected non-zero port baseURL, got %q", baseURL)
	}
}

func TestListenLocal_ExplicitPortAndInUseError(t *testing.T) {
	listener, baseURL, err := ListenLocal(0)
	if err != nil {
		t.Fatalf("ListenLocal(0) error: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	port := listener.Addr().(*net.TCPAddr).Port
	if !strings.HasSuffix(baseURL, ":"+strconv.Itoa(port)) {
		t.Fatalf("expected baseURL to end with port %d, got %q", port, baseURL)
	}

	_, _, err = ListenLocal(port)
	if err == nil {
		t.Fatalf("expected error when port is already in use")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected 'already in use' error, got: %v", err)
	}
}
