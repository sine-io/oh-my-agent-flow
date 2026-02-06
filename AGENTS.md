# Ralph Agent Instructions

## Overview

Ralph is an autonomous AI agent loop that runs AI coding tools (Amp or Claude Code) repeatedly until all PRD items are complete. Each iteration is a fresh instance with clean context.

## Commands

```bash
# Run the local console (Go)
go run ./cmd/ohmyagentflow --port 0

# Run the flowchart dev server
cd flowchart && npm run dev

# Build the flowchart
cd flowchart && npm run build

# Run Ralph with Amp (default)
./ralph.sh [max_iterations]

# Run Ralph with Claude Code
./ralph.sh --tool claude [max_iterations]

# Run Ralph with Codex
./ralph-codex.sh [max_iterations]

# Initialize Codex skills for this repo
./ralph-codex.sh init --tool codex
```

## Key Files

- `ralph.sh` - The bash loop that spawns fresh AI instances (supports `--tool amp` or `--tool claude`)
- `ralph-codex.sh` - The bash loop that spawns fresh AI instances (supports `--tool codex` or `--tool claude`), and `init` to install local Codex skills
- `prompt.md` - Instructions given to each AMP instance
-  `CLAUDE.md` - Instructions given to each Claude Code instance
-  `CODEX.md` - Instructions given to each Codex instance
- `prd.json.example` - Example PRD format
- `flowchart/` - Interactive React Flow diagram explaining how Ralph works

## Flowchart

The `flowchart/` directory contains an interactive visualization built with React Flow. It's designed for presentations - click through to reveal each step with animations.

To run locally:
```bash
cd flowchart
npm install
npm run dev
```

## Patterns

- Each iteration spawns a fresh AI instance (Amp or Claude Code) with clean context
- Memory persists via git history, `progress.txt`, and `prd.json`
- Stories should be small enough to complete in one context window
- For Codex, run `./ralph-codex.sh init --tool codex` once to install repo-local skills into `.codex/skills`
- The console also supports `POST /api/init` to install repo-local Codex skills without shelling out (copies `skills/<skill>/SKILL-codex.md` to `.codex/skills/<skill>/SKILL.md`).
- Keep `cmd/ohmyagentflow/main.go` minimal: render the console UI via `internal/console.RenderIndexHTML(...)` (uses `html/template`) and pass dynamic values via `IndexPageData`.
- The console UI in `internal/console/index_page.go` is embedded as a Go raw string: avoid using backticks in the HTML/JS content or the Go file won’t compile.
- For local-console write endpoints, guard `POST /api/*` with strict Origin allowlisting plus a per-run `X-Session-Token` (see `internal/console/RequireWriteAuth` and the `<meta name="ohmyagentflow-session-token">` convention).
- For local-console filesystem access, route all user-supplied paths through `internal/console/FSReader` to enforce project-root containment, symlink-escape prevention, whitelisting, and max read size.
- For PRD questionnaire mode, use `POST /api/prd/generate?preview=1` to render a Convert-compatible preview without writing; omit `preview` to save under `tasks/prd-<feature_slug>.md`.
- For PRD Convert parse/validation errors, return a structured `APIError` including `file` and `location:{line,column}` so the UI can display precise fix locations.
- When shelling out to LLM CLIs (`codex`/`claude`) from the console, require outputs to be wrapped in stable markers (e.g., `BEGIN_COMMANDS`/`END_COMMANDS`) and extract the last marker pair to avoid accidentally parsing echoed prompts; sanitize/redact provider output before returning it in API errors.
- For SSE logs, use `internal/console/StreamHub` + `StreamHandler` for run-scoped `seq` ordering, retention, and `sinceSeq` replay (and governance: truncate noisy `process_stdout/stderr` messages and emit a warning once per run).
- `StreamHub` also archives run streams to `.ohmyagentflow/runs/<runId>.jsonl` (writes `.tmp` during the run, renames on `run_finished`, size cap via `StreamHubConfig.MaxArchiveBytes`).
- Before opening a new `.ohmyagentflow/runs/*.jsonl.tmp`, `StreamHub` performs best-effort cleanup in the archive dir (retention by count + total size; deletes oldest-by-mtime).
- For Fire (`POST /api/fire`), enforce strict inputs (`tool ∈ {codex, claude}`, `maxIterations ∈ 1..200`), require `prd.json` and `ralph-codex.sh` to be regular non-symlink files under project root, execute only `bash <abs>/ralph-codex.sh --tool <tool> <maxIterations>`, and reject concurrent runs with `RESOURCE_CONFLICT`.
- For Fire Stop (`POST /api/fire/stop`), prefer process-group signaling: start Fire with a new PGID and stop via `SIGINT` → wait for the process group to be empty (`kill(-pgid, 0)`), then `SIGKILL` if still alive.
- For Fire iteration UX, emit `progress` events with `{tool,iteration,maxIterations,phase,completeDetected,note}` and drive the UI grouping from those events (avoid parsing stdout strings in the browser).
- Always update AGENTS.md with discovered patterns for future iterations
