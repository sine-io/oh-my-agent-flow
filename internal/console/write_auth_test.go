package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateSessionToken(t *testing.T) {
	t.Parallel()

	t1, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}
	t2, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}
	if len(t1) != 32 {
		t.Fatalf("expected 32-char hex token, got %d (%q)", len(t1), t1)
	}
	if t1 == t2 {
		t.Fatalf("expected tokens to differ, got %q and %q", t1, t2)
	}
}

func TestRequireWriteAuth_AllowsValidWrite(t *testing.T) {
	t.Parallel()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := RequireWriteAuth(next, WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234", "http://localhost:1234"},
	})

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ping", nil)
	req.Header.Set("Origin", "http://127.0.0.1:1234")
	req.Header.Set("X-Session-Token", "abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatalf("expected next handler to be called")
	}
}

func TestRequireWriteAuth_RejectsMissingOrigin(t *testing.T) {
	t.Parallel()

	h := RequireWriteAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234"},
	})

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ping", nil)
	req.Header.Set("X-Session-Token", "abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	var got apiError
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got.Code != "ORIGIN_REQUIRED" {
		t.Fatalf("expected ORIGIN_REQUIRED, got %q", got.Code)
	}
}

func TestRequireWriteAuth_RejectsOriginNull(t *testing.T) {
	t.Parallel()

	h := RequireWriteAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234"},
	})

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ping", nil)
	req.Header.Set("Origin", "null")
	req.Header.Set("X-Session-Token", "abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	var got apiError
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got.Code != "ORIGIN_REQUIRED" {
		t.Fatalf("expected ORIGIN_REQUIRED, got %q", got.Code)
	}
}

func TestRequireWriteAuth_RejectsOriginNotAllowed(t *testing.T) {
	t.Parallel()

	h := RequireWriteAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234"},
	})

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ping", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("X-Session-Token", "abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	var got apiError
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got.Code != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("expected ORIGIN_NOT_ALLOWED, got %q", got.Code)
	}
}

func TestRequireWriteAuth_RejectsMissingToken(t *testing.T) {
	t.Parallel()

	h := RequireWriteAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234"},
	})

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ping", nil)
	req.Header.Set("Origin", "http://127.0.0.1:1234")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	var got apiError
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got.Code != "SESSION_TOKEN_REQUIRED" {
		t.Fatalf("expected SESSION_TOKEN_REQUIRED, got %q", got.Code)
	}
}

func TestRequireWriteAuth_RejectsInvalidToken(t *testing.T) {
	t.Parallel()

	h := RequireWriteAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234"},
	})

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ping", nil)
	req.Header.Set("Origin", "http://127.0.0.1:1234")
	req.Header.Set("X-Session-Token", "wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	var got apiError
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got.Code != "SESSION_TOKEN_INVALID" {
		t.Fatalf("expected SESSION_TOKEN_INVALID, got %q", got.Code)
	}
}

func TestRequireWriteAuth_DoesNotProtectNonPostOrNonAPI(t *testing.T) {
	t.Parallel()

	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})

	h := RequireWriteAuth(next, WriteAuthConfig{
		SessionToken:   "abc",
		AllowedOrigins: []string{"http://127.0.0.1:1234"},
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/ping", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/not-api", nil)
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr2.Code, rr2.Body.String())
	}

	if called != 2 {
		t.Fatalf("expected next handler to be called twice, got %d", called)
	}
}
