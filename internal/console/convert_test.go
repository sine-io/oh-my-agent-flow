package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertHandler_WritesPRDJSONAndBacksUpExisting(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll(tasks) error: %v", err)
	}

	md := `---
schema: ohmyagentflow/prd@1
project: "demo"
feature_slug: "demo-feature"
title: "Demo"
description: "Demo desc"
---

# PRD: Demo

## Goals
- Do thing

## User Stories
### US-001: First story
**Description:** As a user, I want one thing so that I can do it.

**Acceptance Criteria:**
- [ ] A
- [ ] Typecheck passes

## Functional Requirements
1. FR-1: TBD

## Non-Goals
- TBD

## Success Metrics
- TBD

## Open Questions
- TBD
`

	prdPath := "tasks/prd-demo-feature.md"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(prdPath)), []byte(md), 0o644); err != nil {
		t.Fatalf("WriteFile(prd) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "prd.json"), []byte("{\"old\":true}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(prd.json) error: %v", err)
	}

	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: DefaultMaxReadBytes})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	body, _ := json.Marshal(ConvertRequest{PRDPath: prdPath})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/convert", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ConvertHandler(ConvertConfig{ProjectRoot: root, FSReader: reader}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded ConvertResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.OutputPath != "prd.json" {
		t.Fatalf("unexpected outputPath: %q", decoded.OutputPath)
	}
	if decoded.BackupPath == "" {
		t.Fatalf("expected backupPath to be set")
	}
	if _, err := os.Stat(filepath.Join(root, decoded.BackupPath)); err != nil {
		t.Fatalf("expected backup file to exist: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "prd.json"))
	if err != nil {
		t.Fatalf("ReadFile(prd.json) error: %v", err)
	}
	var prd ConvertedPRD
	if err := json.Unmarshal(got, &prd); err != nil {
		t.Fatalf("prd.json invalid: %v", err)
	}
	if prd.Project != "demo" {
		t.Fatalf("unexpected project: %q", prd.Project)
	}
	if prd.BranchName != "ralph/demo-feature" {
		t.Fatalf("unexpected branchName: %q", prd.BranchName)
	}
	if len(prd.UserStories) != 1 || prd.UserStories[0].ID != "US-001" || prd.UserStories[0].Priority != 1 {
		t.Fatalf("unexpected stories: %+v", prd.UserStories)
	}
}

func TestConvertHandler_ReturnsParseErrorWithLocation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll(tasks) error: %v", err)
	}

	md := `---
schema: ohmyagentflow/prd@1
project: "demo"
feature_slug: "demo-feature"
title: "Demo"
description: "Demo desc"
---

# PRD: Demo

## Goals
- Do thing

## User Stories
### US-002: Wrong first story
**Description:** As a user, I want one thing so that I can do it.

**Acceptance Criteria:**
- [ ] A

## Functional Requirements
1. FR-1: TBD

## Non-Goals
- TBD

## Success Metrics
- TBD

## Open Questions
- TBD
`
	prdPath := "tasks/prd-demo-feature.md"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(prdPath)), []byte(md), 0o644); err != nil {
		t.Fatalf("WriteFile(prd) error: %v", err)
	}

	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: DefaultMaxReadBytes})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	body, _ := json.Marshal(ConvertRequest{PRDPath: prdPath})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/convert", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ConvertHandler(ConvertConfig{ProjectRoot: root, FSReader: reader}).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Code != "PRD_CONVERT_INVALID_STORY_HEADER" {
		t.Fatalf("unexpected code: %q", decoded.Code)
	}
	if decoded.File == "" || decoded.Location == nil || decoded.Location.Line < 1 {
		t.Fatalf("expected file+location in error, got %+v", decoded)
	}
}

func TestConvertHandler_RefusesNonWhitelistedPaths(t *testing.T) {
	root := t.TempDir()
	reader, err := NewFSReader(FSReadConfig{ProjectRoot: root, MaxBytes: DefaultMaxReadBytes})
	if err != nil {
		t.Fatalf("NewFSReader error: %v", err)
	}

	body, _ := json.Marshal(ConvertRequest{PRDPath: "README.md"})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/convert", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ConvertHandler(ConvertConfig{ProjectRoot: root, FSReader: reader}).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var decoded APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded.Code != "FS_READ_NOT_ALLOWED" {
		t.Fatalf("unexpected code: %q", decoded.Code)
	}
}
