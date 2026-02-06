package console

import (
	"bytes"
	"encoding/json"
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
