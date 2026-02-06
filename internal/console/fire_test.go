package console

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFireService_StartHandler_ValidatesAndStarts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte(`{"ok":true}`+"\n"), 0644); err != nil {
		t.Fatalf("write prd.json: %v", err)
	}
	script := "#!/usr/bin/env bash\n" +
		"echo \"hello stdout\"\n" +
		"echo \"hello stderr\" 1>&2\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(root, "ralph-codex.sh"), []byte(script), 0755); err != nil {
		t.Fatalf("write ralph-codex.sh: %v", err)
	}

	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 100, SubscriberBufSize: 16})
	svc, err := NewFireService(FireConfig{ProjectRoot: root, Hub: hub})
	if err != nil {
		t.Fatalf("NewFireService: %v", err)
	}

	body, _ := json.Marshal(FireStartRequest{Tool: "codex", MaxIterations: 1})
	req := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp FireStartResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK || !strings.HasPrefix(resp.RunID, "fire-") {
		t.Fatalf("unexpected response: %+v", resp)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.mu.Lock()
		state := hub.runs[resp.RunID]
		var haveStarted, haveFinished bool
		if state != nil {
			for _, ev := range state.events {
				if ev.Type == "run_started" {
					haveStarted = true
				}
				if ev.Type == "run_finished" {
					haveFinished = true
				}
			}
		}
		hub.mu.Unlock()

		if haveStarted && haveFinished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for stream events for runId=%s", resp.RunID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFireService_StartHandler_RejectsMissingPRDJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ralph-codex.sh"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write ralph-codex.sh: %v", err)
	}

	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 10, SubscriberBufSize: 4})
	svc, err := NewFireService(FireConfig{ProjectRoot: root, Hub: hub})
	if err != nil {
		t.Fatalf("NewFireService: %v", err)
	}

	body, _ := json.Marshal(FireStartRequest{Tool: "codex", MaxIterations: 1})
	req := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var apiErr APIError
	_ = json.Unmarshal(w.Body.Bytes(), &apiErr)
	if apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %+v", apiErr)
	}
	if !strings.Contains(strings.ToLower(apiErr.Hint), "convert") {
		t.Fatalf("expected hint to mention Convert, got %+v", apiErr)
	}
}

func TestFireService_StartHandler_RejectsBadTool(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte(`{}`+"\n"), 0644); err != nil {
		t.Fatalf("write prd.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ralph-codex.sh"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write ralph-codex.sh: %v", err)
	}

	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 10, SubscriberBufSize: 4})
	svc, err := NewFireService(FireConfig{ProjectRoot: root, Hub: hub})
	if err != nil {
		t.Fatalf("NewFireService: %v", err)
	}

	body, _ := json.Marshal(FireStartRequest{Tool: "amp", MaxIterations: 1})
	req := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var apiErr APIError
	_ = json.Unmarshal(w.Body.Bytes(), &apiErr)
	if apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %+v", apiErr)
	}
}

func TestFireService_StartHandler_RejectsConcurrentRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte(`{}`+"\n"), 0644); err != nil {
		t.Fatalf("write prd.json: %v", err)
	}
	script := "#!/usr/bin/env bash\n" +
		"sleep 0.2\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(root, "ralph-codex.sh"), []byte(script), 0755); err != nil {
		t.Fatalf("write ralph-codex.sh: %v", err)
	}

	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 50, SubscriberBufSize: 8})
	svc, err := NewFireService(FireConfig{ProjectRoot: root, Hub: hub})
	if err != nil {
		t.Fatalf("NewFireService: %v", err)
	}

	body, _ := json.Marshal(FireStartRequest{Tool: "codex", MaxIterations: 1})
	req1 := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w1 := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected first start 200, got %d: %s", w1.Code, w1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w2 := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected second start 409, got %d: %s", w2.Code, w2.Body.String())
	}
	var apiErr APIError
	_ = json.Unmarshal(w2.Body.Bytes(), &apiErr)
	if apiErr.Code != "RESOURCE_CONFLICT" {
		t.Fatalf("expected RESOURCE_CONFLICT, got %+v", apiErr)
	}
}

func TestFireService_StopHandler_StopsProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group stop test is Unix-only")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte(`{}`+"\n"), 0644); err != nil {
		t.Fatalf("write prd.json: %v", err)
	}

	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"trap 'exit 0' INT\n" +
		"bash -c 'trap \"exit 0\" INT; while true; do sleep 1; done' &\n" +
		"child=$!\n" +
		"echo \"CHILD_PID=$child\"\n" +
		"while true; do sleep 1; done\n"
	if err := os.WriteFile(filepath.Join(root, "ralph-codex.sh"), []byte(script), 0755); err != nil {
		t.Fatalf("write ralph-codex.sh: %v", err)
	}

	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 200, SubscriberBufSize: 32})
	svc, err := NewFireService(FireConfig{ProjectRoot: root, Hub: hub})
	if err != nil {
		t.Fatalf("NewFireService: %v", err)
	}

	body, _ := json.Marshal(FireStartRequest{Tool: "codex", MaxIterations: 1})
	req := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var startResp FireStartResponse
	if err := json.Unmarshal(w.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.mu.Lock()
		state := hub.runs[startResp.RunID]
		if state != nil {
			for _, ev := range state.events {
				if ev.Type != "process_stdout" {
					continue
				}
				data, _ := ev.Data.(map[string]any)
				text, _ := data["text"].(string)
				if strings.HasPrefix(text, "CHILD_PID=") {
					n, err := strconv.Atoi(strings.TrimPrefix(text, "CHILD_PID="))
					if err == nil && n > 0 {
						childPID = n
						break
					}
				}
			}
		}
		hub.mu.Unlock()

		if childPID != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for child pid in logs")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/fire/stop", nil)
	stopW := httptest.NewRecorder()
	svc.StopHandler().ServeHTTP(stopW, stopReq)
	if stopW.Code != http.StatusOK {
		t.Fatalf("expected stop 200, got %d: %s", stopW.Code, stopW.Body.String())
	}

	deadline = time.Now().Add(4 * time.Second)
	var finished StreamEvent
	var haveFinished bool
	for {
		hub.mu.Lock()
		state := hub.runs[startResp.RunID]
		if state != nil {
			for _, ev := range state.events {
				if ev.Type != "run_finished" {
					continue
				}
				finished = ev
				haveFinished = true
			}
		}
		hub.mu.Unlock()

		if haveFinished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for run_finished")
		}
		time.Sleep(10 * time.Millisecond)
	}

	finishedData, _ := finished.Data.(map[string]any)
	if got, _ := finishedData["reason"].(string); got != "stopped" {
		t.Fatalf("expected reason=stopped, got %v", finishedData["reason"])
	}
	if got, _ := finishedData["signal"].(string); got != "SIGINT" && got != "SIGKILL" {
		t.Fatalf("expected signal SIGINT/SIGKILL, got %v", finishedData["signal"])
	}

	// Ensure the background child is gone too.
	deadline = time.Now().Add(1 * time.Second)
	for {
		err = syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected child pid %d to be gone, got err=%v", childPID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFireService_EmitsProgressEventsAndDetectsComplete(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte(`{}`+"\n"), 0644); err != nil {
		t.Fatalf("write prd.json: %v", err)
	}

	// Minimal script that mimics ralph-codex.sh output patterns.
	script := "#!/usr/bin/env bash\n" +
		"echo \"Starting Ralph - Tool: codex - Max iterations: 2\"\n" +
		"echo \"===============================================================\"\n" +
		"echo \"  Ralph Iteration 1 of 2 (codex)\"\n" +
		"echo \"===============================================================\"\n" +
		"echo \"some stdout line\"\n" +
		"echo \"assistant output: <promise>COMPLETE</promise>\" 1>&2\n" +
		"echo \"Iteration 1 complete. Continuing...\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(root, "ralph-codex.sh"), []byte(script), 0755); err != nil {
		t.Fatalf("write ralph-codex.sh: %v", err)
	}

	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 200, SubscriberBufSize: 64})
	svc, err := NewFireService(FireConfig{ProjectRoot: root, Hub: hub})
	if err != nil {
		t.Fatalf("NewFireService: %v", err)
	}

	body, _ := json.Marshal(FireStartRequest{Tool: "codex", MaxIterations: 2})
	req := httptest.NewRequest(http.MethodPost, "/api/fire", bytes.NewReader(body))
	w := httptest.NewRecorder()
	svc.StartHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp FireStartResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Wait for the run to finish so we can inspect the buffered events.
	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.mu.Lock()
		state := hub.runs[resp.RunID]
		haveFinished := false
		if state != nil {
			for _, ev := range state.events {
				if ev.Type == "run_finished" {
					haveFinished = true
				}
			}
		}
		hub.mu.Unlock()
		if haveFinished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for run_finished for runId=%s", resp.RunID)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var sawIterStart bool
	var sawComplete bool
	var completeCount int

	hub.mu.Lock()
	state := hub.runs[resp.RunID]
	events := append([]StreamEvent(nil), state.events...)
	hub.mu.Unlock()

	for _, ev := range events {
		if ev.Type != "progress" || ev.Step != "fire" {
			continue
		}
		data, ok := ev.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected progress data map, got %T", ev.Data)
		}
		if _, ok := data["tool"]; !ok {
			t.Fatalf("progress event missing tool: %+v", data)
		}
		if _, ok := data["iteration"]; !ok {
			t.Fatalf("progress event missing iteration: %+v", data)
		}
		if _, ok := data["maxIterations"]; !ok {
			t.Fatalf("progress event missing maxIterations: %+v", data)
		}
		phase, _ := data["phase"].(string)
		if phase == "iteration_started" {
			sawIterStart = true
		}
		if phase == "complete_detected" {
			completeCount++
			sawComplete = true
			if got, ok := data["completeDetected"].(bool); !ok || !got {
				t.Fatalf("expected completeDetected=true for complete_detected: %+v", data)
			}
		}
	}

	if !sawIterStart {
		t.Fatalf("expected to see iteration_started progress event; got %d total events", len(events))
	}
	if !sawComplete {
		t.Fatalf("expected to see complete_detected progress event; got %d total events", len(events))
	}
	if completeCount != 1 {
		t.Fatalf("expected exactly 1 complete_detected event, got %d", completeCount)
	}
}
