package console

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const DefaultMaxReadBytes int64 = 2 << 20 // 2 MiB

type FSReadConfig struct {
	ProjectRoot string
	MaxBytes    int64
}

type FSReader struct {
	rootAbs  string
	rootReal string
	maxBytes int64
}

type FSReadResponse struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
}

func NewFSReader(cfg FSReadConfig) (*FSReader, error) {
	if cfg.ProjectRoot == "" {
		return nil, errors.New("project root is required")
	}

	rootAbs, err := filepath.Abs(cfg.ProjectRoot)
	if err != nil {
		return nil, err
	}

	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, err
	}

	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxReadBytes
	}

	return &FSReader{
		rootAbs:  rootAbs,
		rootReal: rootReal,
		maxBytes: maxBytes,
	}, nil
}

func FSReadHandler(reader *FSReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get("path")
		resp, apiErr, status := reader.ReadWhitelistedText(relPath)
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (r *FSReader) ReadWhitelistedText(relPath string) (FSReadResponse, *APIError, int) {
	if relPath == "" {
		return FSReadResponse{}, &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "path query parameter is required.",
			Hint:    "Provide a project-relative path, e.g. tasks/prd-foo.md",
		}, http.StatusBadRequest
	}
	if strings.Contains(relPath, "\x00") {
		return FSReadResponse{}, &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "path must not contain NUL bytes.",
		}, http.StatusBadRequest
	}
	if strings.Contains(relPath, "\\") {
		return FSReadResponse{}, &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "path must use forward slashes (/).",
			Hint:    "Use a project-relative path like tasks/prd-foo.md (not Windows-style backslashes).",
		}, http.StatusBadRequest
	}

	clean := filepath.Clean(relPath)
	if clean == "." || clean == string(filepath.Separator) {
		return FSReadResponse{}, &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "path must point to a file.",
		}, http.StatusBadRequest
	}
	if filepath.IsAbs(clean) {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_NOT_ALLOWED",
			Message: "Absolute paths are not allowed.",
			Hint:    "Provide a path relative to the project root.",
		}, http.StatusForbidden
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_NOT_ALLOWED",
			Message: "Path escapes project root.",
			Hint:    "Remove '..' segments and use a project-relative path.",
		}, http.StatusForbidden
	}

	if !isReadWhitelisted(clean) {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_NOT_ALLOWED",
			Message: "Path not allowed.",
			Hint:    "Only tasks/prd-*.md, prd.json, and progress.txt are readable.",
		}, http.StatusForbidden
	}

	absPath := filepath.Join(r.rootAbs, clean)
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return FSReadResponse{}, &APIError{
				Code:    "FS_READ_NOT_FOUND",
				Message: "File not found.",
			}, http.StatusNotFound
		}
		return FSReadResponse{}, &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "Failed to resolve path.",
			Hint:    "Check file permissions and try again.",
		}, http.StatusInternalServerError
	}

	if !isWithinRoot(r.rootReal, realPath) {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_NOT_ALLOWED",
			Message: "Path escapes project root.",
			Hint:    "Symlink escapes are not allowed; ensure the file resolves within the project root.",
		}, http.StatusForbidden
	}

	info, err := os.Stat(realPath)
	if err != nil {
		if os.IsNotExist(err) {
			return FSReadResponse{}, &APIError{
				Code:    "FS_READ_NOT_FOUND",
				Message: "File not found.",
			}, http.StatusNotFound
		}
		return FSReadResponse{}, &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "Failed to stat file.",
		}, http.StatusInternalServerError
	}
	if !info.Mode().IsRegular() {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_NOT_ALLOWED",
			Message: "Only regular files are readable.",
		}, http.StatusForbidden
	}
	if info.Size() > r.maxBytes {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_TOO_LARGE",
			Message: "File is too large to read.",
			Hint:    "Reduce the file size or increase the server's max read limit.",
		}, http.StatusRequestEntityTooLarge
	}

	content, size, err := readFileUpTo(realPath, r.maxBytes)
	if err != nil {
		if errors.Is(err, errFileTooLarge) {
			return FSReadResponse{}, &APIError{
				Code:    "FS_READ_TOO_LARGE",
				Message: "File is too large to read.",
			}, http.StatusRequestEntityTooLarge
		}
		return FSReadResponse{}, &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "Failed to read file.",
		}, http.StatusInternalServerError
	}
	if !utf8.Valid(content) {
		return FSReadResponse{}, &APIError{
			Code:    "FS_READ_UNSUPPORTED_ENCODING",
			Message: "File is not valid UTF-8 text.",
			Hint:    "Convert the file to UTF-8 and retry.",
		}, http.StatusUnsupportedMediaType
	}

	return FSReadResponse{
		Path:      clean,
		Content:   string(content),
		Size:      size,
		Truncated: false,
	}, nil, http.StatusOK
}

func isReadWhitelisted(relPath string) bool {
	if relPath == "prd.json" || relPath == "progress.txt" {
		return true
	}
	if !strings.HasPrefix(relPath, "tasks/prd-") || !strings.HasSuffix(relPath, ".md") {
		return false
	}
	rest := strings.TrimPrefix(relPath, "tasks/prd-")
	return !strings.Contains(rest, "/")
}

func isWithinRoot(rootReal, candidateReal string) bool {
	rel, err := filepath.Rel(rootReal, candidateReal)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var errFileTooLarge = errors.New("file too large")

func readFileUpTo(path string, maxBytes int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := info.Size()

	lr := io.LimitReader(f, maxBytes+1)
	buf, err := io.ReadAll(lr)
	if err != nil {
		return nil, size, err
	}
	if int64(len(buf)) > maxBytes {
		return nil, size, errFileTooLarge
	}
	return buf, size, nil
}
