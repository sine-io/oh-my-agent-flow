package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPRDChat_SessionMessageFinalize(t *testing.T) {
	root := t.TempDir()
	svc, err := NewPRDChatService(PRDChatConfig{ProjectRoot: root, SessionTTL: 10 * time.Minute})
	if err != nil {
		t.Fatalf("NewPRDChatService: %v", err)
	}

	// Create session.
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/session", nil)
		svc.SessionHandler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("session status: %d body=%s", rr.Code, rr.Body.String())
		}
		var resp PRDChatSessionResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.SessionID == "" {
			t.Fatalf("expected sessionId")
		}
		if len(resp.SlotState.Missing) == 0 {
			t.Fatalf("expected missing fields")
		}

		// Send a message to fill required fields.
		msg := strings.Join([]string{
			"feature_slug: task-status",
			"title: Task Status Feature",
			"description: Add ability to mark tasks with different statuses.",
			"story: As a user, I can set a status",
			"story_desc: Users can set status to todo/in-progress/done.",
			"ac: Typecheck passes",
		}, "\n")

		raw, _ := json.Marshal(PRDChatMessageRequest{SessionID: resp.SessionID, Message: msg})
		rr2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/message", bytes.NewReader(raw))
		req2.Header.Set("Content-Type", "application/json")
		svc.MessageHandler().ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusOK {
			t.Fatalf("message status: %d body=%s", rr2.Code, rr2.Body.String())
		}

		// Finalize should write to tasks/.
		finalRaw, _ := json.Marshal(PRDChatFinalizeRequest{SessionID: resp.SessionID})
		rr3 := httptest.NewRecorder()
		req3 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/finalize", bytes.NewReader(finalRaw))
		req3.Header.Set("Content-Type", "application/json")
		svc.FinalizeHandler().ServeHTTP(rr3, req3)
		if rr3.Code != http.StatusOK {
			t.Fatalf("finalize status: %d body=%s", rr3.Code, rr3.Body.String())
		}
		var fin PRDChatFinalizeResponse
		if err := json.Unmarshal(rr3.Body.Bytes(), &fin); err != nil {
			t.Fatalf("unmarshal finalize: %v", err)
		}
		if !fin.OK {
			t.Fatalf("expected ok=true, missing=%v warnings=%v", fin.Missing, fin.Warnings)
		}
		if fin.Path != "tasks/prd-task-status.md" {
			t.Fatalf("unexpected path: %q", fin.Path)
		}
		if !strings.Contains(fin.Content, "schema: "+prdSchema) {
			t.Fatalf("expected schema in content")
		}
		if !strings.Contains(fin.Content, "# PRD: Task Status Feature") {
			t.Fatalf("expected title header")
		}
		{
			expectedReq := chatStateToGenerateRequest(fin.SlotState)
			expected, apiErr, status := buildPRDMarkdown(expectedReq)
			if apiErr != nil {
				t.Fatalf("buildPRDMarkdown status=%d err=%s", status, apiErr.Message)
			}
			if expected != fin.Content {
				t.Fatalf("expected finalize content to match questionnaire template output")
			}
		}
		fileAbs := filepath.Join(root, filepath.FromSlash(fin.Path))
		b, err := os.ReadFile(fileAbs)
		if err != nil {
			t.Fatalf("read written file: %v", err)
		}
		if string(b) != fin.Content {
			t.Fatalf("written content mismatch")
		}
	}
}

func TestPRDChat_SessionTTLExpires(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	svc, err := NewPRDChatService(PRDChatConfig{
		ProjectRoot: root,
		SessionTTL:  time.Second,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewPRDChatService: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/session", nil)
	svc.SessionHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status: %d body=%s", rr.Code, rr.Body.String())
	}
	var resp PRDChatSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	now = now.Add(2 * time.Second)
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/prd/chat/state?sessionId="+resp.SessionID, nil)
	svc.StateHandler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after expiry, got %d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestPRDChat_MessageToolValidation(t *testing.T) {
	root := t.TempDir()
	svc, err := NewPRDChatService(PRDChatConfig{ProjectRoot: root})
	if err != nil {
		t.Fatalf("NewPRDChatService: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/session", nil)
	svc.SessionHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status: %d body=%s", rr.Code, rr.Body.String())
	}
	var sess PRDChatSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}

	raw, _ := json.Marshal(PRDChatMessageRequest{
		SessionID: sess.SessionID,
		Message:   "please do it",
		Tool:      "nope",
	})
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/message", bytes.NewReader(raw))
	req2.Header.Set("Content-Type", "application/json")
	svc.MessageHandler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr2.Code, rr2.Body.String())
	}
	var apiErr APIError
	if err := json.Unmarshal(rr2.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("unmarshal api error: %v", err)
	}
	if apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %q", apiErr.Code)
	}
}

func TestPRDChat_MessageToolAppliesModelCommands(t *testing.T) {
	root := t.TempDir()
	called := 0
	svc, err := NewPRDChatService(PRDChatConfig{
		ProjectRoot: root,
		ModelToolFunc: func(_ context.Context, tool PRDChatTool, prompt string) ([]byte, error) {
			called++
			if tool != PRDChatToolCodex {
				return nil, errors.New("unexpected tool")
			}
			if !strings.Contains(prompt, "USER_MESSAGE:") {
				return nil, errors.New("missing prompt header")
			}
			out := strings.Join([]string{
				"some header junk",
				prdChatCommandsBegin,
				"feature_slug: llm-feature",
				"title: LLM Feature",
				"description: Generated by an LLM provider.",
				"story: As a user, I can do a thing",
				"story_desc: So that I can test the integration.",
				"ac: Typecheck passes",
				prdChatCommandsEnd,
				"some footer junk",
			}, "\n")
			return []byte(out), nil
		},
	})
	if err != nil {
		t.Fatalf("NewPRDChatService: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/session", nil)
	svc.SessionHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status: %d body=%s", rr.Code, rr.Body.String())
	}
	var sess PRDChatSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}

	raw, _ := json.Marshal(PRDChatMessageRequest{
		SessionID: sess.SessionID,
		Message:   "make me a PRD for an LLM feature",
		Tool:      "codex",
	})
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/message", bytes.NewReader(raw))
	req2.Header.Set("Content-Type", "application/json")
	svc.MessageHandler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("message status: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if called != 1 {
		t.Fatalf("expected model to be called once, got %d", called)
	}

	var msgResp PRDChatMessageResponse
	if err := json.Unmarshal(rr2.Body.Bytes(), &msgResp); err != nil {
		t.Fatalf("unmarshal message resp: %v", err)
	}
	if got := strings.TrimSpace(msgResp.SlotState.FrontMatter.FeatureSlug); got != "llm-feature" {
		t.Fatalf("featureSlug mismatch: %q", got)
	}
	if got := strings.TrimSpace(msgResp.SlotState.FrontMatter.Title); got != "LLM Feature" {
		t.Fatalf("title mismatch: %q", got)
	}
	if got := strings.TrimSpace(msgResp.SlotState.FrontMatter.Description); got != "Generated by an LLM provider." {
		t.Fatalf("description mismatch: %q", got)
	}
	if len(msgResp.SlotState.Missing) != 0 {
		t.Fatalf("expected missing empty, got %v", msgResp.SlotState.Missing)
	}
}

func TestPRDChat_MessageToolProviderErrorRedactsSecrets(t *testing.T) {
	root := t.TempDir()
	secret := "sk-this-should-never-leak-1234567890"
	svc, err := NewPRDChatService(PRDChatConfig{
		ProjectRoot: root,
		ModelToolFunc: func(_ context.Context, _ PRDChatTool, _ string) ([]byte, error) {
			return []byte("provider failed: " + secret), errors.New("exit 1")
		},
	})
	if err != nil {
		t.Fatalf("NewPRDChatService: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/session", nil)
	svc.SessionHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status: %d body=%s", rr.Code, rr.Body.String())
	}
	var sess PRDChatSessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}

	raw, _ := json.Marshal(PRDChatMessageRequest{
		SessionID: sess.SessionID,
		Message:   "hello",
		Tool:      "codex",
	})
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/chat/message", bytes.NewReader(raw))
	req2.Header.Set("Content-Type", "application/json")
	svc.MessageHandler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rr2.Code, rr2.Body.String())
	}
	if strings.Contains(rr2.Body.String(), secret) {
		t.Fatalf("expected secret to be redacted, body=%s", rr2.Body.String())
	}
	var apiErr APIError
	if err := json.Unmarshal(rr2.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("unmarshal api error: %v", err)
	}
	if apiErr.Code != "LLM_PROVIDER_FAILED" {
		t.Fatalf("expected LLM_PROVIDER_FAILED, got %q", apiErr.Code)
	}
}
