package console

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type FireTool string

const (
	FireToolCodex  FireTool = "codex"
	FireToolClaude FireTool = "claude"
)

type FireConfig struct {
	ProjectRoot string
	Hub         *StreamHub
}

type FireService struct {
	rootAbs string
	hub     *StreamHub

	mu     sync.Mutex
	active *fireRunState
}

type fireRunState struct {
	runID         string
	tool          FireTool
	maxIterations int

	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd

	done chan struct{}

	pgid int

	stopping     bool
	stopSignal   string
	stopIssuedAt time.Time
}

type FireStartRequest struct {
	Tool          string `json:"tool"`
	MaxIterations int    `json:"maxIterations"`
}

type FireStartResponse struct {
	OK    bool   `json:"ok"`
	RunID string `json:"runId"`
}

type FireStopResponse struct {
	OK       bool   `json:"ok"`
	RunID    string `json:"runId,omitempty"`
	Stopping bool   `json:"stopping"`
}

func NewFireService(cfg FireConfig) (*FireService, error) {
	if cfg.ProjectRoot == "" {
		return nil, errors.New("project root is required")
	}
	if cfg.Hub == nil {
		return nil, errors.New("stream hub is required")
	}
	rootAbs, err := filepath.Abs(cfg.ProjectRoot)
	if err != nil {
		return nil, err
	}
	return &FireService{rootAbs: rootAbs, hub: cfg.Hub}, nil
}

func (s *FireService) StartHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req FireStartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "BAD_JSON",
				Message: "Invalid JSON request body.",
				Hint:    "Send {\"tool\":\"codex\",\"maxIterations\":10}.",
			})
			return
		}

		tool, apiErr, status := parseFireTool(req.Tool)
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}
		if req.MaxIterations < 1 || req.MaxIterations > 200 {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "VALIDATION_ERROR",
				Message: "maxIterations must be between 1 and 200.",
				Hint:    "Pick a value like 10 (or 1 for a quick smoke run).",
			})
			return
		}

		if _, apiErr, status := requireRegularFileUnderRoot(s.rootAbs, "prd.json", "prd.json"); apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}
		scriptAbs, apiErr, status := requireRegularFileUnderRoot(s.rootAbs, "ralph-codex.sh", "ralph-codex.sh")
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}

		runToken, err := GenerateSessionToken()
		if err != nil {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to generate run id.",
				Hint:    "Retry the request.",
			})
			return
		}
		runID := "fire-" + runToken

		s.mu.Lock()
		if s.active != nil {
			s.mu.Unlock()
			WriteAPIError(w, http.StatusConflict, APIError{
				Code:    "RESOURCE_CONFLICT",
				Message: "A Fire run is already active.",
				Hint:    "Wait for it to finish (or use Stop once available).",
			})
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.active = &fireRunState{
			runID:         runID,
			tool:          tool,
			maxIterations: req.MaxIterations,
			ctx:           ctx,
			cancel:        cancel,
			done:          make(chan struct{}),
		}
		s.mu.Unlock()

		cmd := exec.Command("bash", scriptAbs, "--tool", string(tool), strconv.Itoa(req.MaxIterations))
		cmd.Dir = s.rootAbs
		setProcessGroup(cmd)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			s.clearActive(runID)
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "FIRE_START_FAILED",
				Message: "Failed to start Fire.",
				Hint:    "Retry the request.",
			})
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			s.clearActive(runID)
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "FIRE_START_FAILED",
				Message: "Failed to start Fire.",
				Hint:    "Retry the request.",
			})
			return
		}

		if err := cmd.Start(); err != nil {
			s.clearActive(runID)
			hint := "Ensure bash is installed and ralph-codex.sh is present under the project root."
			if isExecNotFound(err) {
				hint = "bash was not found on PATH. Install bash (or run on a Unix-like environment) and retry."
			}
			WriteAPIError(w, http.StatusBadGateway, APIError{
				Code:    "FIRE_START_FAILED",
				Message: "Failed to start Fire process.",
				Hint:    hint,
			})
			return
		}

		s.mu.Lock()
		if s.active != nil && s.active.runID == runID {
			s.active.cmd = cmd
			if runtime.GOOS != "windows" {
				s.active.pgid = cmd.Process.Pid
			}
		}
		s.mu.Unlock()

		s.hub.Publish(StreamEvent{
			RunID: runID,
			Type:  "run_started",
			Step:  "fire",
			Level: "info",
			Data: map[string]any{
				"op":            "fire",
				"cwd":           s.rootAbs,
				"tool":          tool,
				"maxIterations": req.MaxIterations,
				"pid":           cmd.Process.Pid,
				"cmd":           []string{"bash", scriptAbs, "--tool", string(tool), strconv.Itoa(req.MaxIterations)},
			},
		})

		go s.streamPipe(runID, "process_stdout", stdout)
		go s.streamPipe(runID, "process_stderr", stderr)
		go s.waitAndFinalize(runID, cmd)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(FireStartResponse{OK: true, RunID: runID})
	}
}

func (s *FireService) StopHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		active := s.active
		if active == nil || active.cmd == nil || active.cmd.Process == nil {
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(FireStopResponse{OK: true, Stopping: false})
			return
		}
		runID := active.runID
		if active.stopping {
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(FireStopResponse{OK: true, RunID: runID, Stopping: true})
			return
		}
		active.stopping = true
		active.stopSignal = "SIGINT"
		active.stopIssuedAt = time.Now()
		pgid := active.pgid
		pid := active.cmd.Process.Pid
		s.mu.Unlock()

		if pgid == 0 {
			pgid = pid
		}

		// Best effort: stop via process group (Unix) or the process itself (Windows).
		_ = sendInterruptToProcessGroup(pgid, pid)

		s.hub.Publish(StreamEvent{
			RunID: runID,
			Type:  "progress",
			Step:  "fire",
			Level: "info",
			Data: map[string]any{
				"phase": "stopped",
				"note":  "Stop requested; sending SIGINT.",
			},
		})

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !processGroupExists(pgid) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_ = json.NewEncoder(w).Encode(FireStopResponse{OK: true, RunID: runID, Stopping: false})
				return
			}
			time.Sleep(50 * time.Millisecond)
		}

		s.mu.Lock()
		active = s.active
		if active != nil && active.runID == runID {
			active.stopSignal = "SIGKILL"
		}
		s.mu.Unlock()

		_ = sendKillToProcessGroup(pgid, pid)

		s.hub.Publish(StreamEvent{
			RunID: runID,
			Type:  "progress",
			Step:  "fire",
			Level: "warn",
			Data: map[string]any{
				"phase": "stopped",
				"note":  "Process did not exit after SIGINT; sent SIGKILL.",
			},
		})

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(FireStopResponse{OK: true, RunID: runID, Stopping: true})
	}
}

func (s *FireService) waitAndFinalize(runID string, cmd *exec.Cmd) {
	startedAt := time.Now()
	err := cmd.Wait()

	var exitCodePtr *int
	var signalPtr *string
	level := "info"
	ok := true
	reason := "completed"

	if runtime.GOOS != "windows" {
		if exitCode, sig, okParse := parseUnixExitStatus(err); okParse {
			if sig != "" {
				signalPtr = &sig
				exitCodePtr = nil
			} else {
				exitCodePtr = &exitCode
			}
		}
	}

	if exitCodePtr == nil && signalPtr == nil {
		exitCode := 0
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				exitCode = ee.ExitCode()
			} else {
				exitCode = -1
			}
		}
		exitCodePtr = &exitCode
	}

	if err != nil {
		ok = false
		level = "error"
		reason = "error"
	} else {
		ok = true
		level = "info"
		reason = "completed"
	}

	s.mu.Lock()
	active := s.active
	stopSignal := ""
	stopRequested := false
	if active != nil && active.runID == runID && active.stopping {
		stopRequested = true
		stopSignal = active.stopSignal
	}
	done := (active != nil && active.runID == runID && active.done != nil)
	if done {
		close(active.done)
	}
	s.mu.Unlock()

	if stopRequested {
		reason = "stopped"
		ok = false
		level = "info"
		if stopSignal != "" {
			signalPtr = &stopSignal
			exitCodePtr = nil
		}
	}

	exitCodeVal := any(nil)
	if exitCodePtr != nil {
		exitCodeVal = *exitCodePtr
	}
	signalVal := any(nil)
	if signalPtr != nil {
		signalVal = *signalPtr
	}

	s.hub.Publish(StreamEvent{
		RunID: runID,
		Type:  "run_finished",
		Step:  "fire",
		Level: level,
		Data: map[string]any{
			"op":     "fire",
			"ok":     ok,
			"reason": reason,
			"durationMs": func() int64 {
				return time.Since(startedAt).Milliseconds()
			}(),
			"exitCode": exitCodeVal,
			"signal":   signalVal,
		},
	})

	s.clearActive(runID)
}

func (s *FireService) clearActive(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && s.active.runID == runID {
		if s.active.cancel != nil {
			s.active.cancel()
		}
		if s.active.done != nil {
			select {
			case <-s.active.done:
			default:
				close(s.active.done)
			}
		}
		s.active = nil
	}
}

func (s *FireService) streamPipe(runID string, eventType string, r io.Reader) {
	sc := bufio.NewScanner(r)
	// Allow long lines; StreamHub will truncate payloads for safety.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		text := sc.Text()
		if text == "" {
			continue
		}
		s.hub.Publish(StreamEvent{
			RunID: runID,
			Type:  eventType,
			Step:  "fire",
			Level: "info",
			Data:  map[string]any{"text": text},
		})
	}
	if err := sc.Err(); err != nil {
		s.hub.Publish(StreamEvent{
			RunID: runID,
			Type:  "error",
			Step:  "fire",
			Level: "error",
			Data: map[string]any{
				"message": fmt.Sprintf("failed to read process output: %v", err),
			},
		})
	}
}

func parseFireTool(raw string) (FireTool, *APIError, int) {
	tool := strings.ToLower(strings.TrimSpace(raw))
	switch FireTool(tool) {
	case FireToolCodex, FireToolClaude:
		return FireTool(tool), nil, http.StatusOK
	default:
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "tool must be one of: codex, claude.",
			Hint:    "Use tool=codex for Codex CLI or tool=claude for Claude CLI.",
		}, http.StatusBadRequest
	}
}

func requireRegularFileUnderRoot(rootAbs string, relPath string, displayName string) (string, *APIError, int) {
	relPath = filepath.Clean(relPath)
	if filepath.IsAbs(relPath) || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || strings.Contains(relPath, string(filepath.Separator)) {
		return "", &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "unsafe server path configuration.",
		}, http.StatusInternalServerError
	}

	abs := filepath.Join(rootAbs, relPath)
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			msg := fmt.Sprintf("%s is required but was not found.", displayName)
			hint := ""
			code := "VALIDATION_ERROR"
			status := http.StatusBadRequest
			if displayName == "prd.json" {
				hint = "Generate or Convert a PRD first so prd.json exists, then retry Fire."
			} else if displayName == "ralph-codex.sh" {
				hint = "Ensure ralph-codex.sh exists under the project root and is not a symlink."
			}
			return "", &APIError{Code: code, Message: msg, Hint: hint}, status
		}
		return "", &APIError{
			Code:    "INTERNAL_ERROR",
			Message: fmt.Sprintf("Failed to stat %s.", displayName),
		}, http.StatusInternalServerError
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: fmt.Sprintf("%s must be a regular file (symlinks are not allowed).", displayName),
			Hint:    "Replace the symlink with a real file under the project root and retry.",
		}, http.StatusBadRequest
	}
	if !info.Mode().IsRegular() {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: fmt.Sprintf("%s must be a regular file.", displayName),
		}, http.StatusBadRequest
	}
	return abs, nil, http.StatusOK
}
