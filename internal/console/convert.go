package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ConvertConfig struct {
	ProjectRoot string
	FSReader    *FSReader
}

type ConvertRequest struct {
	PRDPath string `json:"prdPath"`
}

type ConvertSummary struct {
	Stories            int `json:"stories"`
	AcceptanceCriteria int `json:"acceptanceCriteria"`
}

type ConvertResponse struct {
	InputPath  string         `json:"inputPath"`
	OutputPath string         `json:"outputPath"`
	BackupPath string         `json:"backupPath,omitempty"`
	Summary    ConvertSummary `json:"summary"`
	PRD        any            `json:"prd"`
	JSON       string         `json:"json"`
}

type ConvertedPRD struct {
	Project     string               `json:"project"`
	BranchName  string               `json:"branchName"`
	Description string               `json:"description"`
	UserStories []ConvertedUserStory `json:"userStories"`
}

type ConvertedUserStory struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Priority           int      `json:"priority"`
	Passes             bool     `json:"passes"`
	Notes              string   `json:"notes"`
}

func ConvertHandler(cfg ConvertConfig) http.HandlerFunc {
	projectRoot := strings.TrimSpace(cfg.ProjectRoot)
	reader := cfg.FSReader

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if projectRoot == "" || reader == nil {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "convert not configured",
				Hint:    "This is a server configuration error (ConvertHandler requires ProjectRoot and FSReader).",
			})
			return
		}

		var req ConvertRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "VALIDATION_ERROR",
				Message: "invalid JSON body",
				Hint:    err.Error(),
			})
			return
		}
		req.PRDPath = strings.TrimSpace(req.PRDPath)
		if req.PRDPath == "" {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "VALIDATION_ERROR",
				Message: "prdPath is required",
				Hint:    "Provide a project-relative path, e.g. tasks/prd-foo.md",
			})
			return
		}

		fsResp, apiErr, status := reader.ReadWhitelistedText(req.PRDPath)
		if apiErr != nil {
			apiErr.File = req.PRDPath
			WriteAPIError(w, status, *apiErr)
			return
		}

		prd, apiErr, status := parseConvertPRDMarkdown(fsResp.Content, fsResp.Path, filepath.Base(projectRoot))
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}

		jsonBytes, err := json.MarshalIndent(prd, "", "  ")
		if err != nil {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "failed to encode prd.json",
			})
			return
		}
		jsonBytes = append(jsonBytes, '\n')

		destAbs := filepath.Join(projectRoot, "prd.json")
		backupRel, apiErr, status := backupPRDJSONIfExists(projectRoot, destAbs)
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}

		if err := writeFileAtomicWithPrefix(destAbs, jsonBytes, 0o644, ".convert-*"); err != nil {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "failed to write prd.json",
				Hint:    err.Error(),
			})
			return
		}

		totalAC := 0
		for _, s := range prd.UserStories {
			totalAC += len(s.AcceptanceCriteria)
		}

		resp := ConvertResponse{
			InputPath:  fsResp.Path,
			OutputPath: "prd.json",
			BackupPath: backupRel,
			Summary: ConvertSummary{
				Stories:            len(prd.UserStories),
				AcceptanceCriteria: totalAC,
			},
			PRD:  prd,
			JSON: string(jsonBytes),
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func backupPRDJSONIfExists(projectRoot, destAbs string) (string, *APIError, int) {
	info, err := os.Stat(destAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, http.StatusOK
		}
		return "", &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "failed to stat prd.json",
			Hint:    err.Error(),
		}, http.StatusInternalServerError
	}
	if !info.Mode().IsRegular() {
		return "", &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "prd.json exists but is not a regular file",
			Hint:    "Remove or rename the existing path and retry.",
		}, http.StatusInternalServerError
	}

	data, err := os.ReadFile(destAbs)
	if err != nil {
		return "", &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "failed to read existing prd.json for backup",
			Hint:    err.Error(),
		}, http.StatusInternalServerError
	}

	ts := time.Now().Format("20060102-150405")
	backupRel := fmt.Sprintf("prd.json.bak-%s.json", ts)
	backupAbs := filepath.Join(projectRoot, backupRel)
	if err := writeFileAtomicWithPrefix(backupAbs, data, 0o644, ".convert-bak-*"); err != nil {
		return "", &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "failed to write prd.json backup",
			Hint:    err.Error(),
		}, http.StatusInternalServerError
	}
	return backupRel, nil, http.StatusOK
}

var storyHeaderRe = regexp.MustCompile(`^US-\d{3}$`)

func parseConvertPRDMarkdown(md, file, defaultProject string) (ConvertedPRD, *APIError, int) {
	lines := splitNormalizeLines(md)
	if len(lines) == 0 {
		return ConvertedPRD{}, parseErr(file, 1, 1, "PRD_CONVERT_PARSE_ERROR", "empty PRD file", "Ensure the file contains YAML front matter and PRD sections."), http.StatusBadRequest
	}

	fm, fmEnd, apiErr := parseFrontMatter(lines, file)
	if apiErr != nil {
		return ConvertedPRD{}, apiErr, http.StatusBadRequest
	}

	required := []string{"## Goals", "## User Stories", "## Functional Requirements", "## Success Metrics", "## Open Questions"}
	for _, heading := range required {
		if findHeadingLine(lines, heading) == 0 {
			line := fmEnd + 2
			if line < 1 {
				line = 1
			}
			return ConvertedPRD{}, parseErr(file, line, 1, "PRD_CONVERT_MISSING_SECTION", "missing required section", "Add the "+heading+" section."), http.StatusBadRequest
		}
	}
	if findHeadingPrefixLine(lines, "## Non-Goals") == 0 {
		line := fmEnd + 2
		if line < 1 {
			line = 1
		}
		return ConvertedPRD{}, parseErr(file, line, 1, "PRD_CONVERT_MISSING_SECTION", "missing required section", "Add the ## Non-Goals section."), http.StatusBadRequest
	}
	if findHeadingPrefixLine(lines, "# PRD:") == 0 {
		line := fmEnd + 2
		if line < 1 {
			line = 1
		}
		return ConvertedPRD{}, parseErr(file, line, 1, "PRD_CONVERT_MISSING_SECTION", "missing required section", "Add the # PRD: <title> header after front matter."), http.StatusBadRequest
	}

	userStoriesLine := findHeadingLine(lines, "## User Stories")
	if userStoriesLine == 0 {
		return ConvertedPRD{}, parseErr(file, fmEnd+2, 1, "PRD_CONVERT_MISSING_SECTION", "missing required section", "Add the ## User Stories section."), http.StatusBadRequest
	}

	startIdx := userStoriesLine // 1-based line number
	stories, apiErr := parseUserStories(lines, startIdx, file)
	if apiErr != nil {
		return ConvertedPRD{}, apiErr, http.StatusBadRequest
	}

	project := strings.TrimSpace(fm.Project)
	if project == "" {
		project = strings.TrimSpace(defaultProject)
	}

	out := ConvertedPRD{
		Project:     project,
		BranchName:  "ralph/" + fm.FeatureSlug,
		Description: fm.Description,
		UserStories: stories,
	}
	return out, nil, http.StatusOK
}

type convertFrontMatter struct {
	Project     string
	FeatureSlug string
	Title       string
	Description string
}

func parseFrontMatter(lines []string, file string) (convertFrontMatter, int, *APIError) {
	if strings.TrimSpace(lines[0]) != "---" {
		return convertFrontMatter{}, 0, parseErr(file, 1, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "missing YAML front matter", "File must start with '---' YAML front matter.")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return convertFrontMatter{}, 0, parseErr(file, 1, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "unterminated YAML front matter", "Add a closing '---' line after the front matter fields.")
	}

	fm := convertFrontMatter{}
	var gotSchema string
	keyLine := map[string]int{}
	for i := 1; i < end; i++ {
		raw := lines[i]
		lineNo := i + 1
		trim := strings.TrimSpace(raw)
		if trim == "" {
			continue
		}
		parts := strings.SplitN(trim, ":", 2)
		if len(parts) != 2 {
			return convertFrontMatter{}, 0, parseErr(file, lineNo, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "invalid front matter line", "Expected 'key: value' format.")
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val, err := parseYAMLScalarString(val)
		if err != nil {
			return convertFrontMatter{}, 0, parseErr(file, lineNo, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "invalid front matter value", err.Error())
		}

		switch key {
		case "schema":
			gotSchema = val
			keyLine[key] = lineNo
		case "project":
			fm.Project = val
			keyLine[key] = lineNo
		case "feature_slug":
			fm.FeatureSlug = val
			keyLine[key] = lineNo
		case "title":
			fm.Title = val
			keyLine[key] = lineNo
		case "description":
			fm.Description = val
			keyLine[key] = lineNo
		default:
			// ignore unknown fields
		}
	}

	if strings.TrimSpace(gotSchema) != prdSchema {
		lineNo := keyLine["schema"]
		if lineNo == 0 {
			lineNo = 2
		}
		return convertFrontMatter{}, 0, parseErr(file, lineNo, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "schema is invalid", "Expected schema: "+prdSchema)
	}
	if err := validateFeatureSlug(strings.TrimSpace(fm.FeatureSlug)); err != nil {
		lineNo := keyLine["feature_slug"]
		if lineNo == 0 {
			lineNo = 2
		}
		return convertFrontMatter{}, 0, parseErr(file, lineNo, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "feature_slug is invalid", err.Error())
	}
	if strings.TrimSpace(fm.Title) == "" {
		lineNo := keyLine["title"]
		if lineNo == 0 {
			lineNo = 2
		}
		return convertFrontMatter{}, 0, parseErr(file, lineNo, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "title is required", "Set title: \"...\" in front matter.")
	}
	if strings.TrimSpace(fm.Description) == "" {
		lineNo := keyLine["description"]
		if lineNo == 0 {
			lineNo = 2
		}
		return convertFrontMatter{}, 0, parseErr(file, lineNo, 1, "PRD_CONVERT_INVALID_FRONT_MATTER", "description is required", "Set description: \"...\" in front matter.")
	}

	return fm, end + 1, nil
}

func parseYAMLScalarString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, "\"") {
		if !strings.HasSuffix(raw, "\"") || len(raw) < 2 {
			return "", fmt.Errorf("double-quoted string must end with '\"'")
		}
		s := strings.TrimSuffix(strings.TrimPrefix(raw, "\""), "\"")
		s = strings.ReplaceAll(s, "\\\"", "\"")
		s = strings.ReplaceAll(s, "\\\\", "\\")
		return s, nil
	}
	if strings.Contains(raw, "#") {
		// avoid silently dropping comment fragments; keep MVP strict
		return "", fmt.Errorf("unquoted values must not contain '#'; quote the string")
	}
	return raw, nil
}

func parseUserStories(lines []string, startLine int, file string) ([]ConvertedUserStory, *APIError) {
	end := len(lines) // line count

	stories := make([]ConvertedUserStory, 0, 8)
	expected := 1

	line := startLine + 1
	for line <= end {
		if line > end {
			break
		}
		trim := strings.TrimSpace(lines[line-1])
		if strings.HasPrefix(trim, "## ") {
			break
		}
		if strings.HasPrefix(trim, "### ") {
			headerLine := line
			h := strings.TrimSpace(strings.TrimPrefix(trim, "### "))
			colon := strings.Index(h, ":")
			if colon < 0 {
				return nil, parseErr(file, headerLine, 1, "PRD_CONVERT_INVALID_STORY_HEADER", "invalid story header", "Expected '### US-001: Story title'.")
			}
			id := strings.TrimSpace(h[:colon])
			title := strings.TrimSpace(h[colon+1:])
			if !storyHeaderRe.MatchString(id) {
				return nil, parseErr(file, headerLine, 1, "PRD_CONVERT_INVALID_STORY_HEADER", "invalid story id", "Expected story id like US-001.")
			}
			want := fmt.Sprintf("US-%03d", expected)
			if id != want {
				return nil, parseErr(file, headerLine, 1, "PRD_CONVERT_INVALID_STORY_HEADER", "story ids must be sequential", "Expected "+want+" at this position.")
			}
			if title == "" {
				return nil, parseErr(file, headerLine, 1, "PRD_CONVERT_INVALID_STORY_HEADER", "story title is required", "Add a title after the colon.")
			}

			line++
			for line <= end && strings.TrimSpace(lines[line-1]) == "" {
				line++
			}
			if line > end {
				return nil, parseErr(file, headerLine, 1, "PRD_CONVERT_INVALID_STORY", "story is incomplete", "Add a **Description:** line and **Acceptance Criteria:** list.")
			}

			descLineNo := line
			descLine := strings.TrimSpace(lines[line-1])
			if !strings.HasPrefix(descLine, "**Description:**") {
				return nil, parseErr(file, descLineNo, 1, "PRD_CONVERT_INVALID_DESCRIPTION", "missing story description", "Expected a line like '**Description:** As a user, ...'.")
			}
			desc := strings.TrimSpace(strings.TrimPrefix(descLine, "**Description:**"))
			if desc == "" {
				return nil, parseErr(file, descLineNo, 1, "PRD_CONVERT_INVALID_DESCRIPTION", "empty story description", "Provide a single-line description after **Description:**.")
			}
			line++

			for line <= end && strings.TrimSpace(lines[line-1]) == "" {
				line++
			}
			if line > end {
				return nil, parseErr(file, descLineNo, 1, "PRD_CONVERT_INVALID_ACCEPTANCE_CRITERIA", "missing acceptance criteria header", "Add '**Acceptance Criteria:**' after the description.")
			}
			acHeaderLineNo := line
			acHeader := strings.TrimSpace(lines[line-1])
			if acHeader != "**Acceptance Criteria:**" {
				return nil, parseErr(file, acHeaderLineNo, 1, "PRD_CONVERT_INVALID_ACCEPTANCE_CRITERIA", "missing acceptance criteria header", "Expected '**Acceptance Criteria:**' after the description.")
			}
			line++

			ac := make([]string, 0, 8)
			for line <= end {
				trim = strings.TrimSpace(lines[line-1])
				if strings.HasPrefix(trim, "### ") || strings.HasPrefix(trim, "## ") {
					break
				}
				if trim == "" {
					line++
					continue
				}
				item, ok, why := parseCheckboxItem(trim)
				if !ok {
					return nil, parseErr(file, line, 1, "PRD_CONVERT_INVALID_ACCEPTANCE_CRITERIA", "invalid acceptance criteria item", why)
				}
				ac = append(ac, item)
				line++
			}
			if len(ac) == 0 {
				return nil, parseErr(file, acHeaderLineNo, 1, "PRD_CONVERT_INVALID_ACCEPTANCE_CRITERIA", "acceptance criteria is empty", "Add at least one '- [ ] ...' item.")
			}

			stories = append(stories, ConvertedUserStory{
				ID:                 id,
				Title:              title,
				Description:        desc,
				AcceptanceCriteria: ac,
				Priority:           expected,
				Passes:             false,
				Notes:              "",
			})
			expected++
			continue
		}
		line++
	}

	if len(stories) == 0 {
		return nil, parseErr(file, startLine, 1, "PRD_CONVERT_INVALID_USER_STORIES", "no user stories found", "Add at least one story under '## User Stories'.")
	}
	return stories, nil
}

func parseCheckboxItem(line string) (string, bool, string) {
	const prefixUnchecked = "- [ ] "
	const prefixCheckedLower = "- [x] "
	const prefixCheckedUpper = "- [X] "

	var rest string
	switch {
	case strings.HasPrefix(line, prefixUnchecked):
		rest = strings.TrimSpace(strings.TrimPrefix(line, prefixUnchecked))
	case strings.HasPrefix(line, prefixCheckedLower):
		rest = strings.TrimSpace(strings.TrimPrefix(line, prefixCheckedLower))
	case strings.HasPrefix(line, prefixCheckedUpper):
		rest = strings.TrimSpace(strings.TrimPrefix(line, prefixCheckedUpper))
	case strings.HasPrefix(line, "- "):
		return "", false, "Expected checkbox list items like '- [ ] ...'."
	default:
		return "", false, "Acceptance criteria items must be markdown list items like '- [ ] ...'."
	}
	if rest == "" {
		return "", false, "Checkbox item text is required."
	}
	return rest, true, ""
}

func splitNormalizeLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func findHeadingLine(lines []string, exact string) int {
	want := strings.TrimSpace(exact)
	for i, raw := range lines {
		if strings.TrimSpace(raw) == want {
			return i + 1
		}
	}
	return 0
}

func findHeadingPrefixLine(lines []string, prefix string) int {
	p := strings.TrimSpace(prefix)
	for i, raw := range lines {
		trim := strings.TrimSpace(raw)
		if strings.HasPrefix(trim, p) {
			return i + 1
		}
	}
	return 0
}

func parseErr(file string, line, col int, code, message, hint string) *APIError {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	return &APIError{
		Code:     code,
		Message:  message,
		Hint:     hint,
		File:     file,
		Location: &SourceLocation{Line: line, Column: col},
	}
}
