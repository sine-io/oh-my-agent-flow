package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const prdSchema = "ohmyagentflow/prd@1"

var featureSlugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type PRDGenerateConfig struct {
	ProjectRoot string
}

type PRDGenerateFrontMatter struct {
	Project     string `json:"project"`
	FeatureSlug string `json:"featureSlug"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type PRDGenerateUserStory struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
}

type PRDGenerateRequest struct {
	Mode                   string                 `json:"mode"`
	FrontMatter            PRDGenerateFrontMatter `json:"frontMatter"`
	Goals                  []string               `json:"goals"`
	UserStories            []PRDGenerateUserStory `json:"userStories"`
	FunctionalRequirements []string               `json:"functionalRequirements"`
	NonGoals               []string               `json:"nonGoals"`
	SuccessMetrics         []string               `json:"successMetrics"`
	OpenQuestions          []string               `json:"openQuestions"`
}

type PRDGenerateResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
	Preview bool   `json:"preview"`
}

func PRDGenerateHandler(cfg PRDGenerateConfig) http.HandlerFunc {
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
				Hint:    "This is a server configuration error (PRDGenerateHandler requires ProjectRoot).",
			})
			return
		}

		preview := isTruthy(r.URL.Query().Get("preview"))

		var req PRDGenerateRequest
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

		content, apiErr, status := buildPRDMarkdown(req)
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}

		relPath := fmt.Sprintf("tasks/prd-%s.md", req.FrontMatter.FeatureSlug)
		resp := PRDGenerateResponse{
			Path:    relPath,
			Content: content,
			Size:    int64(len([]byte(content))),
			Preview: preview,
		}

		if !preview {
			destAbs := filepath.Join(projectRoot, filepath.FromSlash(relPath))
			if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
				WriteAPIError(w, http.StatusInternalServerError, APIError{
					Code:    "INTERNAL_ERROR",
					Message: "failed to create tasks directory",
					Hint:    err.Error(),
				})
				return
			}
			if err := writeFileAtomicWithPrefix(destAbs, []byte(content), 0o644, ".prd-*"); err != nil {
				WriteAPIError(w, http.StatusInternalServerError, APIError{
					Code:    "INTERNAL_ERROR",
					Message: "failed to write PRD file",
					Hint:    err.Error(),
				})
				return
			}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func buildPRDMarkdown(req PRDGenerateRequest) (string, *APIError, int) {
	if strings.TrimSpace(req.Mode) != "questionnaire" {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "mode must be \"questionnaire\"",
			Hint:    "Set {\"mode\":\"questionnaire\"}.",
		}, http.StatusBadRequest
	}

	fm := req.FrontMatter
	fm.Project = strings.TrimSpace(fm.Project)
	fm.FeatureSlug = strings.TrimSpace(fm.FeatureSlug)
	fm.Title = strings.TrimSpace(fm.Title)
	fm.Description = strings.TrimSpace(fm.Description)

	if err := validateFeatureSlug(fm.FeatureSlug); err != nil {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "frontMatter.featureSlug is invalid",
			Hint:    err.Error(),
		}, http.StatusBadRequest
	}
	if err := validateSingleLine("frontMatter.title", fm.Title, 1, 120); err != nil {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "frontMatter.title is invalid",
			Hint:    err.Error(),
		}, http.StatusBadRequest
	}
	if err := validateSingleLine("frontMatter.description", fm.Description, 1, 200); err != nil {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "frontMatter.description is invalid",
			Hint:    err.Error(),
		}, http.StatusBadRequest
	}
	if err := validateSingleLine("frontMatter.project", fm.Project, 0, 120); err != nil {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "frontMatter.project is invalid",
			Hint:    err.Error(),
		}, http.StatusBadRequest
	}

	goals, apiErr, status := cleanList("goals", req.Goals, 50, true, "TBD")
	if apiErr != nil {
		return "", apiErr, status
	}
	frs, apiErr, status := cleanList("functionalRequirements", req.FunctionalRequirements, 50, true, "FR-1: TBD")
	if apiErr != nil {
		return "", apiErr, status
	}
	nonGoals, apiErr, status := cleanList("nonGoals", req.NonGoals, 50, true, "TBD")
	if apiErr != nil {
		return "", apiErr, status
	}
	successMetrics, apiErr, status := cleanList("successMetrics", req.SuccessMetrics, 50, true, "TBD")
	if apiErr != nil {
		return "", apiErr, status
	}
	openQuestions, apiErr, status := cleanList("openQuestions", req.OpenQuestions, 50, true, "TBD")
	if apiErr != nil {
		return "", apiErr, status
	}

	if len(req.UserStories) < 1 || len(req.UserStories) > 50 {
		return "", &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "userStories must contain 1..50 stories",
		}, http.StatusBadRequest
	}

	typecheckPasses := "Typecheck passes"
	stories := make([]PRDGenerateUserStory, 0, len(req.UserStories))
	for i, s := range req.UserStories {
		wantID := fmt.Sprintf("US-%03d", i+1)
		if strings.TrimSpace(s.ID) != wantID {
			return "", &APIError{
				Code:    "VALIDATION_ERROR",
				Message: "userStories[].id must be sequential starting at US-001",
				Hint:    fmt.Sprintf("Expected userStories[%d].id to be %q.", i, wantID),
			}, http.StatusBadRequest
		}
		s.Title = strings.TrimSpace(s.Title)
		s.Description = strings.TrimSpace(s.Description)
		if err := validateSingleLine(fmt.Sprintf("userStories[%d].title", i), s.Title, 1, 120); err != nil {
			return "", &APIError{
				Code:    "VALIDATION_ERROR",
				Message: "userStories[].title is invalid",
				Hint:    err.Error(),
			}, http.StatusBadRequest
		}
		if err := validateSingleLine(fmt.Sprintf("userStories[%d].description", i), s.Description, 1, 200); err != nil {
			return "", &APIError{
				Code:    "VALIDATION_ERROR",
				Message: "userStories[].description is invalid",
				Hint:    err.Error(),
			}, http.StatusBadRequest
		}

		ac, apiErr, status := cleanList(fmt.Sprintf("userStories[%d].acceptanceCriteria", i), s.AcceptanceCriteria, 30, false, "")
		if apiErr != nil {
			return "", apiErr, status
		}
		hasTypecheck := false
		for _, item := range ac {
			if item == typecheckPasses {
				hasTypecheck = true
				break
			}
		}
		if !hasTypecheck {
			if len(ac) >= 30 {
				return "", &APIError{
					Code:    "VALIDATION_ERROR",
					Message: "acceptanceCriteria has too many items to auto-add \"Typecheck passes\"",
					Hint:    "Remove one criterion or include \"Typecheck passes\" yourself within the 30-item limit.",
				}, http.StatusBadRequest
			}
			ac = append(ac, typecheckPasses)
		}
		if len(ac) == 0 {
			ac = []string{typecheckPasses}
		}
		s.AcceptanceCriteria = ac
		stories = append(stories, s)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema: " + prdSchema + "\n")
	b.WriteString("project: \"" + yamlEscapeDoubleQuoted(fm.Project) + "\"\n")
	b.WriteString("feature_slug: \"" + yamlEscapeDoubleQuoted(fm.FeatureSlug) + "\"\n")
	b.WriteString("title: \"" + yamlEscapeDoubleQuoted(fm.Title) + "\"\n")
	b.WriteString("description: \"" + yamlEscapeDoubleQuoted(fm.Description) + "\"\n")
	b.WriteString("---\n\n")

	b.WriteString("# PRD: " + fm.Title + "\n\n")

	writeBulletSection(&b, "Goals", goals)
	b.WriteString("\n")

	b.WriteString("## User Stories\n")
	for _, s := range stories {
		b.WriteString("### " + s.ID + ": " + s.Title + "\n")
		b.WriteString("**Description:** " + s.Description + "\n\n")
		b.WriteString("**Acceptance Criteria:**\n")
		for _, item := range s.AcceptanceCriteria {
			b.WriteString("- [ ] " + item + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Functional Requirements\n")
	for i, item := range frs {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	b.WriteString("\n")

	writeBulletSection(&b, "Non-Goals", nonGoals)
	b.WriteString("\n")

	writeBulletSection(&b, "Success Metrics", successMetrics)
	b.WriteString("\n")

	writeBulletSection(&b, "Open Questions", openQuestions)

	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}

	return b.String(), nil, http.StatusOK
}

func validateFeatureSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("required")
	}
	if len(slug) < 3 || len(slug) > 64 {
		return fmt.Errorf("must be 3..64 characters")
	}
	if !featureSlugRe.MatchString(slug) {
		return fmt.Errorf("must be kebab-case matching ^[a-z0-9]+(?:-[a-z0-9]+)*$")
	}
	return nil
}

func validateSingleLine(field, value string, minLen, maxLen int) error {
	if strings.Contains(value, "\n") || strings.Contains(value, "\r") {
		return fmt.Errorf("%s must be a single line", field)
	}
	if minLen > 0 && len(value) < minLen {
		return fmt.Errorf("%s must be at least %d characters", field, minLen)
	}
	if maxLen > 0 && len(value) > maxLen {
		return fmt.Errorf("%s must be at most %d characters", field, maxLen)
	}
	return nil
}

func cleanList(field string, in []string, maxItems int, requireNonEmpty bool, defaultItem string) ([]string, *APIError, int) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if strings.Contains(item, "\n") || strings.Contains(item, "\r") {
			return nil, &APIError{
				Code:    "VALIDATION_ERROR",
				Message: fmt.Sprintf("%s items must be single-line strings", field),
			}, http.StatusBadRequest
		}
		if len(item) > 200 {
			return nil, &APIError{
				Code:    "VALIDATION_ERROR",
				Message: fmt.Sprintf("%s items must be <= 200 characters", field),
			}, http.StatusBadRequest
		}
		out = append(out, item)
	}
	if len(out) > maxItems {
		return nil, &APIError{
			Code:    "VALIDATION_ERROR",
			Message: fmt.Sprintf("%s must contain at most %d items", field, maxItems),
		}, http.StatusBadRequest
	}
	if requireNonEmpty && len(out) == 0 {
		out = append(out, defaultItem)
	}
	return out, nil, http.StatusOK
}

func yamlEscapeDoubleQuoted(s string) string {
	// Keep it simple and predictable; front matter values are validated as single-line.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func writeBulletSection(b *strings.Builder, name string, items []string) {
	b.WriteString("## " + name + "\n")
	for _, item := range items {
		b.WriteString("- " + item + "\n")
	}
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
