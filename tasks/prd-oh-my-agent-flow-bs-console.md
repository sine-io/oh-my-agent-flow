---
schema: ohmyagentflow/prd@1
project: "oh-my-agent-flow"
feature_slug: "oh-my-agent-flow-bs-console"
title: "Local B/S Console (Single Binary) for Ralph"
description: "Ship a local web console bundled in a single Go binary to init skills, generate/convert PRDs, and run/stop Ralph Fire with structured, replayable logs. The console must be safe-by-default (loopback only, strict Origin + session token, path/exec whitelists) and remain stable during long-running Fire sessions."
---

# PRD: Local B/S Console (Single Binary) for Ralph

## Introduction/Overview

Today Ralph is driven mainly by shell scripts (e.g. `ralph-codex.sh`) and manual file edits. This feature adds a **local, single-binary console**: users run one executable from their project root, it opens `http://127.0.0.1:<port>` and provides a guided flow for **Init**, **PRD**, **Convert**, and **Fire** (with Stop), plus **structured streaming logs** suitable for debugging and post-run replay.

This PRD is based on `docs/oh-my-agent-flow-bs-console-design.md` (v0.5). Choices confirmed:
- Scope: implement **v0.5** items (including chat mode, archives + cleanup, session limits, and SSE replay/truncation rules).
- Frontend: **React + Vite + TypeScript**.
- PRD feature: implement **full chat** experience (including LLM integration) while still guaranteeing deterministic conversion via slot-state + finalize.
- Storage: allow creating `.ohmyagentflow/` for run archives.
- Fire: **mixed strategy**: start with bash execution (`ralph-codex.sh`) with strict validation; keep a documented path to later migrate Fire logic into Go.

## Goals

- Provide a local web console runnable from any project root that covers the end-to-end workflow: Init → PRD → Convert → Fire (Stop).
- Keep behavior semantically consistent with existing scripts/skills (especially Fire loop semantics and COMPLETE detection).
- Ensure safe local operation: loopback-only server, strict Origin + session token checks for all writes, and strict path/exec whitelists.
- Provide structured log streaming via SSE with run-scoped replay, truncation protections, and JSONL archiving for postmortems.
- Stay responsive during long Fire runs: virtualized log UI, batched rendering, and server-side output governance.

## User Stories

### US-001: Launch local server with stable Base URL and optional auto-open
**Description:** As a user, I want to run one binary in my project root and get a stable local URL (and optional auto-open) so that I can access the console immediately.

**Acceptance Criteria:**
- [ ] Server listens only on `127.0.0.1` by default.
- [ ] Default port behavior is “random free port” (equivalent to `--port 0`).
- [ ] `--port <1..65535>` binds that port; if in use, startup fails with a clear error message.
- [ ] Startup prints the canonical Base URL to stdout: `http://127.0.0.1:<port>`.
- [ ] Auto-open attempts `open <url>` on macOS and `xdg-open <url>` on Linux; failures are warnings only.
- [ ] `--no-open` disables auto-open.
- [ ] Typecheck passes

### US-002: Canonicalize origin by redirecting localhost to 127.0.0.1
**Description:** As a developer, I want a canonical origin so that Origin checks can be exact and reliable.

**Acceptance Criteria:**
- [ ] If the user accesses the UI via `http://localhost:<port>`, server responds with `302` redirect to `http://127.0.0.1:<port>`.
- [ ] All UI assets continue to load correctly after redirect.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-003: Enforce Origin + session token for all write APIs
**Description:** As a maintainer, I want all `POST /api/*` operations protected so that a local web page from another origin cannot trigger actions.

**Acceptance Criteria:**
- [ ] Server generates a random 128-bit session token on startup.
- [ ] UI HTML response includes the token via a meta tag `ohmyagentflow-session-token`.
- [ ] Frontend reads the meta token and sends `X-Session-Token` on every `POST /api/*` request.
- [ ] For requests with `Origin`, server allows only `http://127.0.0.1:<port>` or `http://localhost:<port>`.
- [ ] If `Origin` is missing or `Origin: null`, server rejects the request (MVP behavior).
- [ ] Rejections return a JSON error with a stable `code` and a human hint.
- [ ] Typecheck passes

### US-004: Restrict filesystem reads/writes to project root and whitelist
**Description:** As a user, I want the console to be safe with my filesystem so that it cannot read or write outside my project root.

**Acceptance Criteria:**
- [ ] Server sets project root to process `cwd` at startup.
- [ ] Any requested path is cleaned/normalized and must remain within project root; `..` escapes and absolute external paths are rejected.
- [ ] Soft-link escape is prevented (reject or resolve and re-check within root).
- [ ] Read whitelist allows only: `tasks/prd-*.md`, `prd.json`, `progress.txt` (and only within root).
- [ ] Read size is limited (configurable; default ≤ 2MB); larger reads return `FS_READ_TOO_LARGE`.
- [ ] Typecheck passes

### US-005: Run-scoped SSE stream with replay support
**Description:** As a user, I want to see structured logs in real time and recover after disconnect so that I can follow long runs reliably.

**Acceptance Criteria:**
- [ ] `GET /api/stream?runId=<id>&sinceSeq=<n>` streams events as SSE `data: <json>\n\n`.
- [ ] Events within a run have `seq` starting at 1 and strictly increasing.
- [ ] When `runId` is provided, server replays cached events with `seq > sinceSeq` before live streaming.
- [ ] If replay is truncated due to server retention, server emits a `progress` event noting replay truncation.
- [ ] When `runId` is omitted, streaming is “best effort” and replay is not guaranteed.
- [ ] Typecheck passes

### US-006: Event governance to prevent browser and server overload
**Description:** As a maintainer, I want output limits and truncation so that a noisy process cannot crash the UI or server.

**Acceptance Criteria:**
- [ ] `process_stdout/stderr` event text is truncated per-message (default 8KB) and marks `data.truncated=true` when truncated.
- [ ] Server retains only the last `N=5000` events per run in memory.
- [ ] When run output exceeds governance limits, server emits a user-visible warning via `progress` or `error` event.
- [ ] Typecheck passes

### US-007: Archive complete event stream to JSONL per run
**Description:** As a user, I want a full run archive on disk so that I can inspect logs after closing the browser.

**Acceptance Criteria:**
- [ ] Server writes `.ohmyagentflow/runs/<runId>.jsonl` with one JSON event per line.
- [ ] During the run, server writes to `.jsonl.tmp` and atomically renames to `.jsonl` on completion.
- [ ] Archive file has a max size (default 50MB); exceeding it stops writing and emits an `error` event.
- [ ] Typecheck passes

### US-008: Cleanup archived runs by count and total size
**Description:** As a user, I want archive cleanup so that disk usage does not grow without bound.

**Acceptance Criteria:**
- [ ] Before starting a new archive, server performs cleanup in `.ohmyagentflow/runs/`.
- [ ] Retain at most `K=50` most-recent run archives and at most `1GB` total directory size.
- [ ] Cleanup deletes oldest-by-mtime files until both thresholds are satisfied.
- [ ] Cleanup failures emit an `error` event but do not block the current run.
- [ ] Typecheck passes

### US-009: Implement Stepper UI and project root display
**Description:** As a user, I want a single-page console with clear step navigation and status so that I can follow the intended workflow.

**Acceptance Criteria:**
- [ ] UI is a single page with a left Stepper: Init / PRD / Convert / Fire.
- [ ] Top bar shows project root (read-only) and current run status.
- [ ] Each step provides inputs, primary action button, and result/preview panel as applicable.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-010: Init installs local skills without executing shell
**Description:** As a user, I want Init to prepare `.codex/skills` safely so that my repo is ready to run Ralph without shell injection risk.

**Acceptance Criteria:**
- [ ] `POST /api/init` creates `.codex/skills/...` and copies required skill files using Go filesystem operations (no shell).
- [ ] Response includes `created`, `overwritten`, and `warnings` lists.
- [ ] If a source file is missing, response returns a clear `NOT_FOUND`-style error.
- [ ] Typecheck passes

### US-011: Questionnaire PRD generator produces convertable PRD template
**Description:** As a user, I want a structured form to generate a PRD that Convert can always parse so that I can proceed to Fire without manual formatting fixes.

**Acceptance Criteria:**
- [ ] `POST /api/prd/generate` accepts the documented JSON payload and writes `tasks/prd-<feature_slug>.md`.
- [ ] Generated PRD includes YAML front matter with `schema: ohmyagentflow/prd@1` and required fields (`feature_slug`, `title`, `description`).
- [ ] Generated body includes required headings and story structure as specified in the design doc.
- [ ] Each story’s Acceptance Criteria includes `"Typecheck passes"` (auto-added if missing).
- [ ] UI shows a live PRD preview before saving.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-012: Chat PRD mode uses sessions and slot-state to converge on the same template
**Description:** As a user, I want a chat-based PRD experience that still converges to the deterministic PRD template so that I can iterate naturally without breaking Convert.

**Acceptance Criteria:**
- [ ] `POST /api/prd/chat/session` creates a session with TTL and returns `sessionId`.
- [ ] `POST /api/prd/chat/message` accepts a user message and returns updated `slotState` including `missing` and `warnings`.
- [ ] `GET /api/prd/chat/state?sessionId=...` returns current `slotState` (including `missing`).
- [ ] `POST /api/prd/chat/finalize` validates required fields; if missing, returns a structured gap list without writing a file.
- [ ] On successful finalize, server writes `tasks/prd-<feature_slug>.md` in the exact same template as questionnaire output.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-013: Integrate LLM provider for chat mode (Codex or Claude)
**Description:** As a user, I want the chat PRD mode to be powered by an LLM so that it can extract/organize requirements into slot-state efficiently.

**Acceptance Criteria:**
- [ ] Chat mode supports selecting provider/tool: `codex` or `claude`.
- [ ] Server never writes raw model output directly as PRD; it must update structured slot-state first.
- [ ] Provider errors return a structured error code and a retry hint without crashing the server.
- [ ] No secrets are ever streamed via SSE events (redact if needed).
- [ ] Typecheck passes

### US-014: Convert PRD markdown to root prd.json with precise errors
**Description:** As a user, I want Convert to deterministically generate `prd.json` and provide actionable parse errors so that I can fix format issues quickly.

**Acceptance Criteria:**
- [ ] `POST /api/convert` accepts `prdPath` and refuses non-whitelisted paths.
- [ ] Convert validates front matter, required sections, story headers, description line, and checkbox AC items.
- [ ] On parse errors, response returns `{code,message,file,location:{line,column},hint}` matching the documented error schema.
- [ ] On success, server writes `prd.json` and returns a summary plus the JSON content.
- [ ] If `prd.json` already exists, server writes a timestamped backup path and reports it in the response.
- [ ] Typecheck passes

### US-015: Fire runs Ralph via bash with strict validation and single-run concurrency
**Description:** As a user, I want to start Fire from the UI and have it run the same logic as `ralph-codex.sh` so that behavior matches the existing workflow.

**Acceptance Criteria:**
- [ ] `POST /api/fire` requires `tool ∈ {codex, claude}` and `maxIterations ∈ 1..200`.
- [ ] If another Fire run is already active, server returns `RESOURCE_CONFLICT`.
- [ ] If `prd.json` is missing, server returns `VALIDATION_ERROR` with a hint to run Convert.
- [ ] Server executes only the allowed command form: `bash <abs>/ralph-codex.sh --tool <tool> <maxIterations>`.
- [ ] All stdout/stderr from the process is streamed as structured SSE events.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-016: Stop terminates the entire Fire process tree reliably
**Description:** As a user, I want to stop Fire safely so that no orphan child processes continue running.

**Acceptance Criteria:**
- [ ] Fire starts in a new process group and server records the PGID.
- [ ] `POST /api/fire/stop` sends `SIGINT` to the process group, waits up to 5 seconds, then sends `SIGKILL` if needed.
- [ ] Stop is idempotent; repeated stop calls succeed with `stopping=true` or similar status.
- [ ] `run_finished` includes `reason=stopped` and includes `signal` and `exitCode` semantics as documented.
- [ ] Typecheck passes

### US-017: Detect COMPLETE during Fire and emit progress events
**Description:** As a user, I want the console to surface iteration progress and completion status so that I can understand what Ralph is doing without parsing raw logs.

**Acceptance Criteria:**
- [ ] Server emits `progress` events with fields: `tool`, `iteration`, `maxIterations`, `phase`, `completeDetected`, and optional `note`.
- [ ] Server detects `<promise>COMPLETE</promise>` and emits `phase=complete_detected` with `completeDetected=true`.
- [ ] UI groups and displays logs by step and iteration using `progress` events (not just stdout parsing).
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-018: Log viewer is virtualized, deduplicated by seq, and batched for performance
**Description:** As a user, I want the log viewer to stay smooth even with thousands of events so that I can use the console for long runs.

**Acceptance Criteria:**
- [ ] UI uses a virtualized list for event rendering (no full DOM render of all events).
- [ ] UI retains only the last `N=5000` events per run, dropping older ones.
- [ ] UI deduplicates events by `seq` per `runId` (replay duplicates do not re-render).
- [ ] UI batches SSE updates (e.g., every ~100ms or `requestAnimationFrame`) to avoid per-event re-render.
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

## Functional Requirements

1. FR-1: The server must bind to `127.0.0.1` and choose a random free port by default, with an optional explicit `--port`.
2. FR-2: The server must print the canonical Base URL to stdout and optionally auto-open it (`open`/`xdg-open`) unless `--no-open`.
3. FR-3: The server must canonicalize origins by redirecting `localhost` UI access to `127.0.0.1`.
4. FR-4: All `POST /api/*` must enforce strict Origin matching and a session token (`X-Session-Token`).
5. FR-5: Filesystem access must be constrained to project root with path normalization and a strict whitelist and size limits.
6. FR-6: The console must implement Init (Go copy, no shell), PRD (questionnaire + chat), Convert, and Fire operations as step-based runs.
7. FR-7: The system must emit structured SSE events with run-scoped `seq` ordering and replay for `runId`.
8. FR-8: The system must apply output truncation and retention (`N=5000`) to protect server and UI performance.
9. FR-9: The system must archive run events to `.ohmyagentflow/runs/<runId>.jsonl` and cleanup by file count and total size thresholds.
10. FR-10: Fire must run via a strict exec whitelist and enforce single concurrent run.
11. FR-11: Fire Stop must terminate the entire process group reliably with SIGINT then SIGKILL escalation.
12. FR-12: The UI must provide a stepper workflow, PRD preview, Convert error highlighting, and a performant log viewer.

## Non-Goals (Out of Scope)

- No remote access, multi-user support, or auth/permissions system beyond local Origin + token.
- No exposing a “listen on 0.0.0.0” switch in MVP.
- No database/ORM requirement; persistence is via JSONL archives and existing root files.
- No change to `ralph-codex.sh` semantics (MVP should match behavior, not redefine it).
- No guarantee that global (no `runId`) SSE is replayable or complete.

## Design Considerations (Optional)

- UI layout: top bar (project root + status), left stepper, center form/preview, right structured logs.
- Convert errors should show `code + file:line:col + hint` and highlight the corresponding PRD preview lines.
- PRD generation must output the exact template required by Convert (including YAML front matter and required sections).

## Technical Considerations (Optional)

- Backend: Go + Gin, structured logs via zerolog, static assets embedded via `go:embed`.
- SSE: ensure correct headers and flushing; handle disconnects; replay from in-memory cache per run.
- Fire process management: create a new process group; capture stdout/stderr; enforce exec args; enforce one run at a time.
- Archives: write JSONL with atomic rename; cap file size; implement cleanup by count and total size.
- Frontend: React + Vite + TS; use a virtualized list component; batch incoming SSE updates; dedupe by `seq`.
- Chat LLM integration: expose provider selection (`codex|claude`); produce slot-state deterministically; never stream secrets.

## Success Metrics

- A new user can go from “download binary” to “Fire started” in ≤ 3 minutes on macOS/Linux.
- Fire runs for ≥ 30 minutes with ≥ 50,000 stdout lines without UI freezing (thanks to truncation + virtualization).
- Disconnect and refresh recovers the current run log via `sinceSeq` replay with no duplicate rendering.
- Convert failures provide actionable errors (code + exact location + hint) enabling format fixes in under 2 minutes.

## Open Questions

- Where do LLM credentials/config live for chat mode (env vars vs config file), and how are they surfaced in UI without leaking to logs?
- Should `project` defaulting to repo dir name be implemented in generator, converter, or both (current doc suggests converter)?
- Do we need a “dry-run” mode for Fire that validates `prd.json` and command availability without starting the process?
- Should Init support “overwrite existing skill files” policy choices (always/never/prompt) or always best-effort overwrite?
