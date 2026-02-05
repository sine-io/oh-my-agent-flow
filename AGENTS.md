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
- For local-console write endpoints, guard `POST /api/*` with strict Origin allowlisting plus a per-run `X-Session-Token` (see `internal/console/RequireWriteAuth` and the `<meta name="ohmyagentflow-session-token">` convention).
- For local-console filesystem access, route all user-supplied paths through `internal/console/FSReader` to enforce project-root containment, symlink-escape prevention, whitelisting, and max read size.
- For SSE logs, use `internal/console/StreamHub` + `StreamHandler` for run-scoped `seq` ordering, retention, and `sinceSeq` replay (and governance: truncate noisy `process_stdout/stderr` messages and emit a warning once per run).
- `StreamHub` also archives run streams to `.ohmyagentflow/runs/<runId>.jsonl` (writes `.tmp` during the run, renames on `run_finished`, size cap via `StreamHubConfig.MaxArchiveBytes`).
- Before opening a new `.ohmyagentflow/runs/*.jsonl.tmp`, `StreamHub` performs best-effort cleanup in the archive dir (retention by count + total size; deletes oldest-by-mtime).
- Always update AGENTS.md with discovered patterns for future iterations
