package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type InitConfig struct {
	ProjectRoot string
}

type InitResponse struct {
	Created     []string `json:"created"`
	Overwritten []string `json:"overwritten"`
	Warnings    []string `json:"warnings"`
}

func InitHandler(cfg InitConfig) http.HandlerFunc {
	projectRoot := strings.TrimSpace(cfg.ProjectRoot)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if projectRoot == "" {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "project root not configured",
				Hint:    "This is a server configuration error (InitHandler requires ProjectRoot).",
			})
			return
		}

		resp := InitResponse{
			Created:     []string{},
			Overwritten: []string{},
			Warnings:    []string{},
		}

		mustMkdirAll := func(rel string) bool {
			abs := filepath.Join(projectRoot, filepath.FromSlash(rel))
			_, statErr := os.Stat(abs)
			alreadyExists := statErr == nil
			if err := os.MkdirAll(abs, 0o755); err != nil {
				WriteAPIError(w, http.StatusInternalServerError, APIError{
					Code:    "INIT_FAILED",
					Message: fmt.Sprintf("failed to create %s", rel),
					Hint:    err.Error(),
				})
				return false
			}
			if !alreadyExists {
				resp.Created = append(resp.Created, rel)
			}
			return true
		}

		ensureFile := func(srcRel, destRel string) bool {
			srcAbs := filepath.Join(projectRoot, filepath.FromSlash(srcRel))
			destAbs := filepath.Join(projectRoot, filepath.FromSlash(destRel))

			srcBytes, err := os.ReadFile(srcAbs)
			if err != nil {
				status := http.StatusInternalServerError
				code := "INIT_FAILED"
				hint := err.Error()
				if os.IsNotExist(err) {
					status = http.StatusNotFound
					code = "NOT_FOUND"
					hint = fmt.Sprintf("Missing file %q under the project root.", srcRel)
				}
				WriteAPIError(w, status, APIError{
					Code:    code,
					Message: fmt.Sprintf("failed to read %s", srcRel),
					Hint:    hint,
				})
				return false
			}

			// If dest exists and is identical, skip.
			if existing, err := os.ReadFile(destAbs); err == nil && bytes.Equal(existing, srcBytes) {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("already up-to-date: %s", destRel))
				return true
			}

			if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
				WriteAPIError(w, http.StatusInternalServerError, APIError{
					Code:    "INIT_FAILED",
					Message: fmt.Sprintf("failed to create parent dir for %s", destRel),
					Hint:    err.Error(),
				})
				return false
			}

			_, statErr := os.Stat(destAbs)
			destExists := statErr == nil

			if err := writeFileAtomic(destAbs, srcBytes, 0o644); err != nil {
				WriteAPIError(w, http.StatusInternalServerError, APIError{
					Code:    "INIT_FAILED",
					Message: fmt.Sprintf("failed to write %s", destRel),
					Hint:    err.Error(),
				})
				return false
			}

			if destExists {
				resp.Overwritten = append(resp.Overwritten, destRel)
			} else {
				resp.Created = append(resp.Created, destRel)
			}
			return true
		}

		// Mirror `./ralph-codex.sh init --tool codex`, but via Go FS operations.
		if !mustMkdirAll(".codex/skills") {
			return
		}
		if !mustMkdirAll("oh-my-agent-flow") {
			return
		}
		if !mustMkdirAll(".codex/skills/ralph-prd-generator") {
			return
		}
		if !mustMkdirAll(".codex/skills/ralph-prd-converter") {
			return
		}

		if !ensureFile("skills/ralph-prd-generator/SKILL-codex.md", ".codex/skills/ralph-prd-generator/SKILL.md") {
			return
		}
		if !ensureFile("skills/ralph-prd-converter/SKILL-codex.md", ".codex/skills/ralph-prd-converter/SKILL.md") {
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomicWithPrefix(path, data, perm, ".init-*")
}
