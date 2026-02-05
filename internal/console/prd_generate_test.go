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
)

func TestPRDGenerateHandler_PreviewDoesNotWrite(t *testing.T) {
	root := t.TempDir()

	body := PRDGenerateRequest{
		Mode: "questionnaire",
		FrontMatter: PRDGenerateFrontMatter{
			Project:     "Demo",
			FeatureSlug: "task-status",
			Title:       "Task Status Feature",
			Description: "Add task statuses.",
		},
		Goals: []string{"Track tasks"},
		UserStories: []PRDGenerateUserStory{
			{
				ID:          "US-001",
				Title:       "Add status to tasks",
				Description: "As a user, I want statuses so that I can track progress.",
				AcceptanceCriteria: []string{
					"Status is visible in the UI",
				},
			},
		},
		FunctionalRequirements: []string{"FR-1: Persist status"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/generate?preview=1", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	PRDGenerateHandler(PRDGenerateConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded PRDGenerateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !decoded.Preview {
		t.Fatalf("expected preview=true, got false")
	}
	if decoded.Path != "tasks/prd-task-status.md" {
		t.Fatalf("unexpected path: %q", decoded.Path)
	}
	if !strings.Contains(decoded.Content, "schema: "+prdSchema) {
		t.Fatalf("missing schema in content")
	}
	if !strings.Contains(decoded.Content, "## User Stories") {
		t.Fatalf("missing user stories section")
	}
	if !strings.Contains(decoded.Content, "Typecheck passes") {
		t.Fatalf("missing Typecheck passes in content")
	}

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(decoded.Path))); err == nil {
		t.Fatalf("expected file not to be written in preview mode")
	}
}

func TestPRDGenerateHandler_WritesFile(t *testing.T) {
	root := t.TempDir()

	body := PRDGenerateRequest{
		Mode: "questionnaire",
		FrontMatter: PRDGenerateFrontMatter{
			FeatureSlug: "my-feature",
			Title:       "My Feature",
			Description: "Do something useful.",
		},
		UserStories: []PRDGenerateUserStory{
			{
				ID:          "US-001",
				Title:       "First story",
				Description: "As a user, I want it so that it works.",
				AcceptanceCriteria: []string{
					"Works end-to-end",
					"Typecheck passes",
				},
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/generate", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	PRDGenerateHandler(PRDGenerateConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded PRDGenerateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Preview {
		t.Fatalf("expected preview=false")
	}

	destAbs := filepath.Join(root, filepath.FromSlash(decoded.Path))
	got, err := os.ReadFile(destAbs)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != decoded.Content {
		t.Fatalf("written content mismatch")
	}
}

func TestPRDGenerateHandler_ValidatesFeatureSlug(t *testing.T) {
	root := t.TempDir()

	body := PRDGenerateRequest{
		Mode: "questionnaire",
		FrontMatter: PRDGenerateFrontMatter{
			FeatureSlug: "Bad Slug",
			Title:       "Title",
			Description: "Desc",
		},
		UserStories: []PRDGenerateUserStory{
			{ID: "US-001", Title: "S", Description: "D", AcceptanceCriteria: []string{"Typecheck passes"}},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/generate", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	PRDGenerateHandler(PRDGenerateConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Code != "VALIDATION_ERROR" {
		t.Fatalf("unexpected code: %q", decoded.Code)
	}
}

func TestPRDGenerateHandler_RequiresSequentialStoryIDs(t *testing.T) {
	root := t.TempDir()

	body := PRDGenerateRequest{
		Mode: "questionnaire",
		FrontMatter: PRDGenerateFrontMatter{
			FeatureSlug: "ok-slug",
			Title:       "Title",
			Description: "Desc",
		},
		UserStories: []PRDGenerateUserStory{
			{ID: "US-002", Title: "S", Description: "D", AcceptanceCriteria: []string{"Typecheck passes"}},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/prd/generate", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	PRDGenerateHandler(PRDGenerateConfig{ProjectRoot: root}).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Code != "VALIDATION_ERROR" {
		t.Fatalf("unexpected code: %q", decoded.Code)
	}
}
