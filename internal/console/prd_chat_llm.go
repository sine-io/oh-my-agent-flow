package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type PRDChatTool string

const (
	PRDChatToolCodex  PRDChatTool = "codex"
	PRDChatToolClaude PRDChatTool = "claude"
)

type PRDChatModelToolFunc func(ctx context.Context, tool PRDChatTool, prompt string) ([]byte, error)

const (
	prdChatCommandsBegin = "BEGIN_COMMANDS"
	prdChatCommandsEnd   = "END_COMMANDS"
)

func DefaultPRDChatModelToolFunc(ctx context.Context, tool PRDChatTool, prompt string) ([]byte, error) {
	var cmd *exec.Cmd
	switch tool {
	case PRDChatToolCodex:
		cmd = exec.CommandContext(ctx, "codex", "exec", "--dangerously-bypass-approvals-and-sandbox", "-")
	case PRDChatToolClaude:
		cmd = exec.CommandContext(ctx, "claude", "--dangerously-skip-permissions", "--print")
	default:
		return nil, fmt.Errorf("unsupported tool: %q", tool)
	}
	cmd.Stdin = strings.NewReader(prompt)
	return cmd.CombinedOutput()
}

func prdChatTranslateToCommands(
	ctx context.Context,
	modelFn PRDChatModelToolFunc,
	tool PRDChatTool,
	state PRDChatSlotState,
	activeStory int,
	userMessage string,
) (string, *APIError, int) {
	prompt := buildPRDChatTranslatePrompt(state, activeStory, userMessage)
	out, err := modelFn(ctx, tool, prompt)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return "", &APIError{
				Code:    "LLM_TIMEOUT",
				Message: "LLM provider timed out",
				Hint:    "Try again, shorten the message, or switch tools.",
			}, 504
		}
		if isExecNotFound(err) {
			return "", &APIError{
				Code:    "LLM_TOOL_NOT_FOUND",
				Message: fmt.Sprintf("LLM tool %q is not installed or not on PATH", tool),
				Hint:    "Install the CLI and ensure it is available in PATH, then retry. Or omit tool to use manual slot-state commands.",
			}, 400
		}
		hint := sanitizeProviderText(string(out))
		if hint == "" {
			hint = "Try again, or omit tool to use manual slot-state commands."
		} else {
			hint = "Provider output (sanitized):\n" + hint + "\n\nTry again, or omit tool to use manual slot-state commands."
		}
		return "", &APIError{
			Code:    "LLM_PROVIDER_FAILED",
			Message: "LLM provider call failed",
			Hint:    hint,
		}, 502
	}

	commands, extractErr := extractPRDChatCommands(string(out))
	if extractErr != nil {
		hint := sanitizeProviderText(string(out))
		if hint == "" {
			hint = "Try again, or omit tool to use manual slot-state commands."
		} else {
			hint = "Provider output (sanitized):\n" + hint + "\n\nTry again, or omit tool to use manual slot-state commands."
		}
		return "", &APIError{
			Code:    "LLM_BAD_OUTPUT",
			Message: "LLM provider returned an unexpected response format",
			Hint:    hint,
		}, 502
	}
	commands = filterPRDChatCommands(commands)
	if strings.TrimSpace(commands) == "" {
		return "", &APIError{
			Code:    "LLM_BAD_OUTPUT",
			Message: "LLM provider returned no slot-state updates",
			Hint:    "Try again, be more specific (feature_slug/title/description), or omit tool to use manual slot-state commands.",
		}, 502
	}
	return commands, nil, 200
}

func buildPRDChatTranslatePrompt(state PRDChatSlotState, activeStory int, userMessage string) string {
	stateJSON, _ := json.Marshal(state)
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		msg = "(empty)"
	}

	return strings.TrimSpace(fmt.Sprintf(`
You are a PRD assistant that updates a structured slot-state.

CURRENT_SLOT_STATE_JSON: %s
ACTIVE_STORY_INDEX: %d

TASK:
- Convert USER_MESSAGE into slot-state update commands for the server.
- Output ONLY commands between the markers.

ALLOWED COMMAND FORMS (one per line):
- /reset
- /story <title>
- /ac <acceptance criterion>
- key: value lines using these keys:
  - feature_slug
  - title
  - description
  - goal
  - fr
  - non_goal
  - success_metric
  - open_question
  - story_desc
  - story
  - ac

RULES:
- Do not include any text outside the markers.
- Do not use markdown or code fences.
- Prefer adding missing required fields (feature_slug, title, description, at least 1 story title+desc).

USER_MESSAGE:
%s

%s
<commands here>
%s
`, string(stateJSON), activeStory, msg, prdChatCommandsBegin, prdChatCommandsEnd))
}

func extractPRDChatCommands(output string) (string, error) {
	out := strings.ReplaceAll(output, "\r\n", "\n")
	beginIdx := strings.LastIndex(out, prdChatCommandsBegin)
	if beginIdx < 0 {
		return "", fmt.Errorf("missing %s marker", prdChatCommandsBegin)
	}
	rest := out[beginIdx+len(prdChatCommandsBegin):]
	endRel := strings.Index(rest, prdChatCommandsEnd)
	if endRel < 0 {
		return "", fmt.Errorf("missing %s marker", prdChatCommandsEnd)
	}
	return strings.TrimSpace(rest[:endRel]), nil
}

func filterPRDChatCommands(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if lower == "/reset" || strings.HasPrefix(lower, "/story ") || strings.HasPrefix(lower, "/ac ") {
			out = append(out, line)
			continue
		}
		if strings.Contains(line, ":") {
			out = append(out, line)
			continue
		}
	}
	return strings.Join(out, "\n")
}

func isExecNotFound(err error) bool {
	var ee *exec.Error
	if errors.As(err, &ee) && errors.Is(ee.Err, exec.ErrNotFound) {
		return true
	}
	return errors.Is(err, exec.ErrNotFound)
}

var (
	reOpenAISk = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)
	reAPIKeyKV = regexp.MustCompile(`(?i)\b(api[_-]?key|x-api-key|authorization)\b\s*[:=]\s*\S+`)
)

func sanitizeProviderText(s string) string {
	text := strings.TrimSpace(s)
	if text == "" {
		return ""
	}
	text = reOpenAISk.ReplaceAllString(text, "sk-REDACTED")
	text = reAPIKeyKV.ReplaceAllString(text, "$1: REDACTED")

	const max = 1200
	if len(text) > max {
		text = text[:max] + "\n…(truncated)"
	}
	return text
}
