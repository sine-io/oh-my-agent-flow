package console

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectLocalhostTo127_RedirectsLocalhost(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected redirect, but next handler was called")
	})
	handler := RedirectLocalhostTo127("127.0.0.1:1234", next)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:1234/some/path?x=y", nil)
	req.Host = "localhost:1234"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "http://127.0.0.1:1234/some/path?x=y" {
		t.Fatalf("expected Location header %q, got %q", "http://127.0.0.1:1234/some/path?x=y", got)
	}
}

func TestRedirectLocalhostTo127_Allows127(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := RedirectLocalhostTo127("127.0.0.1:1234", next)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	req.Host = "127.0.0.1:1234"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatalf("expected next handler to be called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}
