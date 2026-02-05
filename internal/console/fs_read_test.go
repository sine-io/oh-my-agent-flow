package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSReader_ReadWhitelistedText_AllowsWhitelistedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll(tasks) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks", "prd-foo.md"), []byte("# hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(prd) error: %v", err)
	}

	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	resp, apiErr, status := reader.ReadWhitelistedText("tasks/prd-foo.md")
	if apiErr != nil {
		t.Fatalf("expected success, got error: %+v", *apiErr)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Path != "tasks/prd-foo.md" {
		t.Fatalf("unexpected path: %q", resp.Path)
	}
	if resp.Content != "# hello\n" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if resp.Size <= 0 {
		t.Fatalf("expected positive size, got %d", resp.Size)
	}
	if resp.Truncated {
		t.Fatalf("expected truncated=false")
	}
}

func TestFSReader_ReadWhitelistedText_RejectsNonWhitelistedPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	_, apiErr, status := reader.ReadWhitelistedText("README.md")
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
	if apiErr.Code != "FS_READ_NOT_ALLOWED" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestFSReader_ReadWhitelistedText_PreventsDotDotEscapes(t *testing.T) {
	root := t.TempDir()
	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	_, apiErr, status := reader.ReadWhitelistedText("../progress.txt")
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
	if apiErr.Code != "FS_READ_NOT_ALLOWED" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestFSReader_ReadWhitelistedText_RejectsAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	_, apiErr, status := reader.ReadWhitelistedText("/etc/hosts")
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
	if apiErr.Code != "FS_READ_NOT_ALLOWED" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestFSReader_ReadWhitelistedText_PreventsSymlinkEscapes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll(tasks) error: %v", err)
	}

	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error: %v", err)
	}

	linkPath := filepath.Join(root, "tasks", "prd-escape.md")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	_, apiErr, status := reader.ReadWhitelistedText("tasks/prd-escape.md")
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
	if apiErr.Code != "FS_READ_NOT_ALLOWED" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestFSReader_ReadWhitelistedText_EnforcesMaxReadBytes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 5})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	_, apiErr, status := reader.ReadWhitelistedText("prd.json")
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", status)
	}
	if apiErr.Code != "FS_READ_TOO_LARGE" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestFSReader_ReadWhitelistedText_RequiresUTF8(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "progress.txt"), []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	_, apiErr, status := reader.ReadWhitelistedText("progress.txt")
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if status != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", status)
	}
	if apiErr.Code != "FS_READ_UNSUPPORTED_ENCODING" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestFSReadHandler_WritesJSONResponse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte("{\"ok\":true}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: 1024})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/fs/read?path=prd.json", nil)
	rr := httptest.NewRecorder()
	FSReadHandler(reader).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("unexpected content-type: %q", ct)
	}

	var decoded FSReadResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Path != "prd.json" {
		t.Fatalf("unexpected path: %q", decoded.Path)
	}
	if !strings.Contains(decoded.Content, "\"ok\"") {
		t.Fatalf("unexpected content: %q", decoded.Content)
	}
}
