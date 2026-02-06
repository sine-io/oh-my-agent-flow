package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PRDChatConfig struct {
	ProjectRoot string
	SessionTTL  time.Duration
	Now         func() time.Time
}

type PRDChatService struct {
	projectRoot string
	ttl         time.Duration
	now         func() time.Time

	mu       sync.Mutex
	sessions map[string]*prdChatSession
}

type prdChatSession struct {
	id        string
	expiresAt time.Time

	activeStory int
	state       PRDChatSlotState
}

type PRDChatSlotState struct {
	FrontMatter PRDGenerateFrontMatter `json:"frontMatter"`

	Goals                  []string           `json:"goals"`
	UserStories            []PRDChatUserStory `json:"userStories"`
	FunctionalRequirements []string           `json:"functionalRequirements"`
	NonGoals               []string           `json:"nonGoals"`
	SuccessMetrics         []string           `json:"successMetrics"`
	OpenQuestions          []string           `json:"openQuestions"`

	Missing  []string `json:"missing"`
	Warnings []string `json:"warnings"`
}

type PRDChatUserStory struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
}

type PRDChatSessionResponse struct {
	SessionID  string           `json:"sessionId"`
	TTLSeconds int64            `json:"ttlSeconds"`
	ExpiresAt  string           `json:"expiresAt"`
	SlotState  PRDChatSlotState `json:"slotState"`
}

type PRDChatMessageRequest struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

type PRDChatMessageResponse struct {
	SessionID string           `json:"sessionId"`
	SlotState PRDChatSlotState `json:"slotState"`
}

type PRDChatStateResponse struct {
	SessionID string           `json:"sessionId"`
	SlotState PRDChatSlotState `json:"slotState"`
}

type PRDChatFinalizeRequest struct {
	SessionID string `json:"sessionId"`
}

type PRDChatFinalizeResponse struct {
	OK        bool             `json:"ok"`
	Path      string           `json:"path,omitempty"`
	Content   string           `json:"content,omitempty"`
	Size      int64            `json:"size,omitempty"`
	Missing   []string         `json:"missing,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
	SlotState PRDChatSlotState `json:"slotState"`
}

func NewPRDChatService(cfg PRDChatConfig) (*PRDChatService, error) {
	projectRoot := strings.TrimSpace(cfg.ProjectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("ProjectRoot is required")
	}
	ttl := cfg.SessionTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &PRDChatService{
		projectRoot: projectRoot,
		ttl:         ttl,
		now:         now,
		sessions:    make(map[string]*prdChatSession),
	}, nil
}

func (s *PRDChatService) SessionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		sessionID, err := GenerateSessionToken()
		if err != nil {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "failed to generate session id",
				Hint:    err.Error(),
			})
			return
		}

		now := s.now()
		session := &prdChatSession{
			id:          sessionID,
			expiresAt:   now.Add(s.ttl),
			activeStory: 0,
			state: PRDChatSlotState{
				UserStories: []PRDChatUserStory{{}},
			},
		}
		session.state.Missing, session.state.Warnings = computeChatGaps(session.state)

		s.mu.Lock()
		s.cleanupLocked(now)
		s.sessions[sessionID] = session
		s.mu.Unlock()

		resp := PRDChatSessionResponse{
			SessionID:  sessionID,
			TTLSeconds: int64(s.ttl.Seconds()),
			ExpiresAt:  session.expiresAt.UTC().Format(time.RFC3339),
			SlotState:  session.state,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (s *PRDChatService) MessageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		var req PRDChatMessageRequest
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

		req.SessionID = strings.TrimSpace(req.SessionID)
		if req.SessionID == "" {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "VALIDATION_ERROR",
				Message: "sessionId is required",
			})
			return
		}

		now := s.now()
		s.mu.Lock()
		s.cleanupLocked(now)
		session := s.sessions[req.SessionID]
		if session == nil {
			s.mu.Unlock()
			WriteAPIError(w, http.StatusNotFound, APIError{
				Code:    "SESSION_NOT_FOUND",
				Message: "chat session not found (or expired)",
				Hint:    "Create a new session via POST /api/prd/chat/session.",
			})
			return
		}
		session.expiresAt = now.Add(s.ttl)
		applyChatMessage(session, req.Message)
		session.state.Missing, session.state.Warnings = computeChatGaps(session.state)
		state := session.state
		s.mu.Unlock()

		resp := PRDChatMessageResponse{SessionID: req.SessionID, SlotState: state}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (s *PRDChatService) StateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
		if sessionID == "" {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "VALIDATION_ERROR",
				Message: "sessionId is required",
			})
			return
		}

		now := s.now()
		s.mu.Lock()
		s.cleanupLocked(now)
		session := s.sessions[sessionID]
		if session == nil {
			s.mu.Unlock()
			WriteAPIError(w, http.StatusNotFound, APIError{
				Code:    "SESSION_NOT_FOUND",
				Message: "chat session not found (or expired)",
				Hint:    "Create a new session via POST /api/prd/chat/session.",
			})
			return
		}
		state := session.state
		s.mu.Unlock()

		resp := PRDChatStateResponse{SessionID: sessionID, SlotState: state}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (s *PRDChatService) FinalizeHandler() http.HandlerFunc {
	projectRoot := s.projectRoot
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.TrimSpace(projectRoot) == "" {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "INTERNAL_ERROR",
				Message: "project root not configured",
				Hint:    "This is a server configuration error (PRDChatService requires ProjectRoot).",
			})
			return
		}

		var req PRDChatFinalizeRequest
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
		req.SessionID = strings.TrimSpace(req.SessionID)
		if req.SessionID == "" {
			WriteAPIError(w, http.StatusBadRequest, APIError{
				Code:    "VALIDATION_ERROR",
				Message: "sessionId is required",
			})
			return
		}

		now := s.now()
		s.mu.Lock()
		s.cleanupLocked(now)
		session := s.sessions[req.SessionID]
		if session == nil {
			s.mu.Unlock()
			WriteAPIError(w, http.StatusNotFound, APIError{
				Code:    "SESSION_NOT_FOUND",
				Message: "chat session not found (or expired)",
				Hint:    "Create a new session via POST /api/prd/chat/session.",
			})
			return
		}

		state := session.state
		state.Missing, state.Warnings = computeChatGaps(state)
		session.state = state
		s.mu.Unlock()

		if len(state.Missing) > 0 {
			resp := PRDChatFinalizeResponse{
				OK:        false,
				Missing:   state.Missing,
				Warnings:  state.Warnings,
				SlotState: state,
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		genReq := chatStateToGenerateRequest(state)
		content, apiErr, status := buildPRDMarkdown(genReq)
		if apiErr != nil {
			WriteAPIError(w, status, *apiErr)
			return
		}

		relPath := fmt.Sprintf("tasks/prd-%s.md", strings.TrimSpace(state.FrontMatter.FeatureSlug))
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

		resp := PRDChatFinalizeResponse{
			OK:        true,
			Path:      relPath,
			Content:   content,
			Size:      int64(len([]byte(content))),
			Warnings:  state.Warnings,
			SlotState: state,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (s *PRDChatService) cleanupLocked(now time.Time) {
	for id, sess := range s.sessions {
		if now.After(sess.expiresAt) {
			delete(s.sessions, id)
		}
	}
}

func computeChatGaps(state PRDChatSlotState) ([]string, []string) {
	var missing []string
	var warnings []string

	slug := strings.TrimSpace(state.FrontMatter.FeatureSlug)
	title := strings.TrimSpace(state.FrontMatter.Title)
	desc := strings.TrimSpace(state.FrontMatter.Description)

	if slug == "" {
		missing = append(missing, "frontMatter.featureSlug")
	} else if err := validateFeatureSlug(slug); err != nil {
		warnings = append(warnings, "frontMatter.featureSlug invalid: "+err.Error())
	}
	if title == "" {
		missing = append(missing, "frontMatter.title")
	} else if err := validateSingleLine("frontMatter.title", title, 1, 120); err != nil {
		warnings = append(warnings, "frontMatter.title invalid: "+err.Error())
	}
	if desc == "" {
		missing = append(missing, "frontMatter.description")
	} else if err := validateSingleLine("frontMatter.description", desc, 1, 200); err != nil {
		warnings = append(warnings, "frontMatter.description invalid: "+err.Error())
	}

	// Need at least one complete story.
	completeStories := 0
	for i, st := range state.UserStories {
		if strings.TrimSpace(st.Title) == "" && strings.TrimSpace(st.Description) == "" && len(st.AcceptanceCriteria) == 0 {
			continue
		}
		if strings.TrimSpace(st.Title) == "" {
			missing = append(missing, fmt.Sprintf("userStories[%d].title", i))
			continue
		}
		if strings.TrimSpace(st.Description) == "" {
			missing = append(missing, fmt.Sprintf("userStories[%d].description", i))
			continue
		}
		completeStories++
	}
	if completeStories == 0 {
		missing = append(missing, "userStories[0].title", "userStories[0].description")
	}

	return dedupeStrings(missing), dedupeStrings(warnings)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func chatStateToGenerateRequest(state PRDChatSlotState) PRDGenerateRequest {
	stories := make([]PRDGenerateUserStory, 0, len(state.UserStories))
	for _, st := range state.UserStories {
		title := strings.TrimSpace(st.Title)
		desc := strings.TrimSpace(st.Description)
		if title == "" && desc == "" && len(st.AcceptanceCriteria) == 0 {
			continue
		}
		stories = append(stories, PRDGenerateUserStory{
			ID:                 fmt.Sprintf("US-%03d", len(stories)+1),
			Title:              title,
			Description:        strings.TrimSpace(st.Description),
			AcceptanceCriteria: st.AcceptanceCriteria,
		})
	}
	if len(stories) == 0 {
		stories = append(stories, PRDGenerateUserStory{ID: "US-001"})
	}

	return PRDGenerateRequest{
		Mode:                   "questionnaire",
		FrontMatter:            state.FrontMatter,
		Goals:                  state.Goals,
		UserStories:            stories,
		FunctionalRequirements: state.FunctionalRequirements,
		NonGoals:               state.NonGoals,
		SuccessMetrics:         state.SuccessMetrics,
		OpenQuestions:          state.OpenQuestions,
	}
}

func applyChatMessage(session *prdChatSession, raw string) {
	msg := strings.TrimSpace(raw)
	if msg == "" {
		return
	}

	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)

		if lower == "/reset" {
			session.state = PRDChatSlotState{UserStories: []PRDChatUserStory{{}}}
			session.activeStory = 0
			continue
		}
		if strings.HasPrefix(lower, "/story ") {
			title := strings.TrimSpace(line[len("/story "):])
			if title == "" {
				continue
			}
			session.state.UserStories = append(session.state.UserStories, PRDChatUserStory{Title: title})
			session.activeStory = len(session.state.UserStories) - 1
			continue
		}
		if strings.HasPrefix(lower, "/ac ") {
			item := strings.TrimSpace(line[len("/ac "):])
			if item == "" {
				continue
			}
			idx := ensureActiveStory(session)
			session.state.UserStories[idx].AcceptanceCriteria = append(session.state.UserStories[idx].AcceptanceCriteria, item)
			continue
		}

		// key: value lines
		if key, val, ok := splitKeyValue(line); ok {
			switch key {
			case "project":
				session.state.FrontMatter.Project = val
			case "feature_slug", "featureslug", "slug":
				session.state.FrontMatter.FeatureSlug = val
			case "title":
				session.state.FrontMatter.Title = val
			case "description", "desc":
				session.state.FrontMatter.Description = val
			case "goal":
				session.state.Goals = append(session.state.Goals, val)
			case "fr", "functional_requirement":
				session.state.FunctionalRequirements = append(session.state.FunctionalRequirements, val)
			case "non_goal", "nongoal":
				session.state.NonGoals = append(session.state.NonGoals, val)
			case "success_metric", "metric":
				session.state.SuccessMetrics = append(session.state.SuccessMetrics, val)
			case "open_question", "question":
				session.state.OpenQuestions = append(session.state.OpenQuestions, val)
			case "story":
				session.state.UserStories = append(session.state.UserStories, PRDChatUserStory{Title: val})
				session.activeStory = len(session.state.UserStories) - 1
			case "story_description", "story_desc":
				idx := ensureActiveStory(session)
				session.state.UserStories[idx].Description = val
			case "ac":
				idx := ensureActiveStory(session)
				session.state.UserStories[idx].AcceptanceCriteria = append(session.state.UserStories[idx].AcceptanceCriteria, val)
			default:
				// ignore unknown keys (chat mode evolves over time)
			}
			continue
		}
	}
}

func ensureActiveStory(session *prdChatSession) int {
	if session.activeStory < 0 || session.activeStory >= len(session.state.UserStories) {
		session.state.UserStories = append(session.state.UserStories, PRDChatUserStory{})
		session.activeStory = len(session.state.UserStories) - 1
	}
	return session.activeStory
}

func splitKeyValue(line string) (key string, val string, ok bool) {
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(line[:i]))
	val = strings.TrimSpace(line[i+1:])
	if key == "" || val == "" {
		return "", "", false
	}
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	return key, val, true
}
