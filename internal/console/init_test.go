package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInitHandler_CreatesAndCopiesSkills(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "ralph-prd-generator"), 0o755); err != nil {
		t.Fatalf("MkdirAll(generator) error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "ralph-prd-converter"), 0o755); err != nil {
		t.Fatalf("MkdirAll(converter) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "ralph-prd-generator", "SKILL-codex.md"), []byte("generator\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(generator) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "ralph-prd-converter", "SKILL-codex.md"), []byte("converter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(converter) error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/init", nil)
	rr := httptest.NewRecorder()
	InitHandler(InitConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded InitResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// Dest files must exist and match sources.
	genDest := filepath.Join(root, ".codex", "skills", "ralph-prd-generator", "SKILL.md")
	convDest := filepath.Join(root, ".codex", "skills", "ralph-prd-converter", "SKILL.md")
	if got, err := os.ReadFile(genDest); err != nil || string(got) != "generator\n" {
		t.Fatalf("unexpected generator dest content: err=%v content=%q", err, string(got))
	}
	if got, err := os.ReadFile(convDest); err != nil || string(got) != "converter\n" {
		t.Fatalf("unexpected converter dest content: err=%v content=%q", err, string(got))
	}

	if len(decoded.Created) == 0 {
		t.Fatalf("expected some created entries, got none")
	}
	if len(decoded.Overwritten) != 0 {
		t.Fatalf("expected no overwritten entries on first run, got %+v", decoded.Overwritten)
	}
}

func TestInitHandler_OverwritesWhenDifferent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "ralph-prd-generator"), 0o755); err != nil {
		t.Fatalf("MkdirAll(generator) error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "ralph-prd-converter"), 0o755); err != nil {
		t.Fatalf("MkdirAll(converter) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "ralph-prd-generator", "SKILL-codex.md"), []byte("new generator\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(generator) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "ralph-prd-converter", "SKILL-codex.md"), []byte("new converter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(converter) error: %v", err)
	}

	// Pre-create dest files with different content.
	if err := os.MkdirAll(filepath.Join(root, ".codex", "skills", "ralph-prd-generator"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dest generator) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "skills", "ralph-prd-generator", "SKILL.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(dest generator) error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".codex", "skills", "ralph-prd-converter"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dest converter) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "skills", "ralph-prd-converter", "SKILL.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(dest converter) error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/init", nil)
	rr := httptest.NewRecorder()
	InitHandler(InitConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded InitResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(decoded.Overwritten) == 0 {
		t.Fatalf("expected overwritten entries, got none (created=%+v warnings=%+v)", decoded.Created, decoded.Warnings)
	}
}

func TestInitHandler_ReturnsNotFoundWhenSourceMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "ralph-prd-generator"), 0o755); err != nil {
		t.Fatalf("MkdirAll(generator) error: %v", err)
	}
	// converter skill intentionally missing
	if err := os.WriteFile(filepath.Join(root, "skills", "ralph-prd-generator", "SKILL-codex.md"), []byte("generator\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(generator) error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/init", nil)
	rr := httptest.NewRecorder()
	InitHandler(InitConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Code != "NOT_FOUND" {
		t.Fatalf("unexpected code: %q", decoded.Code)
	}
}
