package console

import (
	"bytes"
	"html/template"
)

type IndexPageData struct {
	SessionToken string
	ProjectRoot  string
}

var indexPageTmpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="ohmyagentflow-session-token" content="{{.SessionToken}}" />
    <title>Oh My Agent Flow</title>
    <style>
      :root {
        --bg: #0b1020;
        --panel: #111832;
        --panel2: #0f1630;
        --text: #e9edf7;
        --muted: #a5b0cc;
        --border: rgba(255, 255, 255, 0.10);
        --shadow: 0 12px 30px rgba(0,0,0,0.45);
        --accent: #7aa2ff;
        --good: #2dd4bf;
        --warn: #fbbf24;
        --bad: #fb7185;
        --mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
        --sans: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, "Apple Color Emoji", "Segoe UI Emoji";
      }

      * { box-sizing: border-box; }
      html, body { height: 100%; }
      body {
        margin: 0;
        font-family: var(--sans);
        color: var(--text);
        background: radial-gradient(1200px 900px at 15% 10%, rgba(122,162,255,0.18), transparent 55%),
                    radial-gradient(900px 700px at 70% 0%, rgba(45,212,191,0.12), transparent 55%),
                    var(--bg);
      }

      a { color: inherit; }

      .app {
        display: grid;
        grid-template-columns: 280px 1fr;
        height: 100%;
      }

      .sidebar {
        border-right: 1px solid var(--border);
        background: linear-gradient(180deg, rgba(255,255,255,0.06), rgba(255,255,255,0.02));
        padding: 16px 14px;
      }

      .brand {
        display: flex;
        gap: 10px;
        align-items: center;
        padding: 12px 10px;
        border: 1px solid var(--border);
        border-radius: 12px;
        background: rgba(0,0,0,0.12);
        box-shadow: var(--shadow);
        margin-bottom: 14px;
      }
      .brand .dot {
        width: 10px;
        height: 10px;
        background: var(--accent);
        border-radius: 999px;
        box-shadow: 0 0 0 4px rgba(122,162,255,0.18);
      }
      .brand .title {
        font-weight: 700;
        letter-spacing: 0.2px;
      }
      .brand .subtitle {
        font-size: 12px;
        color: var(--muted);
        margin-top: 2px;
      }

      .stepper {
        display: grid;
        gap: 10px;
        margin-top: 10px;
      }
      .step {
        width: 100%;
        text-align: left;
        padding: 12px 12px;
        border-radius: 12px;
        border: 1px solid var(--border);
        background: rgba(0,0,0,0.10);
        color: var(--text);
        cursor: pointer;
      }
      .step:hover { border-color: rgba(122,162,255,0.55); }
      .step[aria-current="page"] {
        background: rgba(122,162,255,0.14);
        border-color: rgba(122,162,255,0.65);
      }
      .step .name { font-weight: 650; }
      .step .desc { color: var(--muted); font-size: 12px; margin-top: 2px; }
      .badge {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 5px 10px;
        border: 1px solid var(--border);
        border-radius: 999px;
        font-size: 12px;
        color: var(--muted);
        background: rgba(0,0,0,0.12);
      }
      .badge .pill { width: 8px; height: 8px; border-radius: 999px; background: var(--warn); }
      .badge.good .pill { background: var(--good); }
      .badge.bad .pill { background: var(--bad); }

      .content {
        display: grid;
        grid-template-rows: auto 1fr;
      }

      .topbar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 14px 18px;
        border-bottom: 1px solid var(--border);
        background: rgba(0,0,0,0.10);
        backdrop-filter: blur(10px);
      }
      .topbar .left { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
      .topbar .label { font-size: 12px; color: var(--muted); }
      .topbar .root {
        font-family: var(--mono);
        font-size: 12px;
        padding: 7px 10px;
        border-radius: 10px;
        border: 1px solid var(--border);
        background: rgba(0,0,0,0.18);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: min(960px, 68vw);
      }
      .topbar .right { display: flex; align-items: center; gap: 10px; }
      .btn {
        border: 1px solid var(--border);
        background: rgba(0,0,0,0.14);
        color: var(--text);
        border-radius: 12px;
        padding: 10px 12px;
        cursor: pointer;
      }
      .btn:hover { border-color: rgba(122,162,255,0.55); }
      .btn.primary {
        background: rgba(122,162,255,0.18);
        border-color: rgba(122,162,255,0.55);
      }
      .btn:disabled { opacity: 0.5; cursor: not-allowed; }

      .main {
        padding: 18px;
        overflow: auto;
      }
      .panel {
        border: 1px solid var(--border);
        background: rgba(0,0,0,0.12);
        border-radius: 16px;
        padding: 16px;
        box-shadow: var(--shadow);
      }
      .panel h2 { margin: 0 0 10px 0; font-size: 18px; }
      .panel p { margin: 8px 0; color: var(--muted); line-height: 1.5; }

      .grid2 {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 14px;
        margin-top: 14px;
      }
      @media (max-width: 980px) {
        .app { grid-template-columns: 1fr; }
        .sidebar { border-right: none; border-bottom: 1px solid var(--border); }
        .grid2 { grid-template-columns: 1fr; }
        .topbar .root { max-width: 92vw; }
      }

      .field {
        display: grid;
        gap: 6px;
        margin: 12px 0;
      }
      .field label { color: var(--muted); font-size: 12px; }
      .field input, .field select, .field textarea {
        padding: 10px 12px;
        border-radius: 12px;
        border: 1px solid var(--border);
        background: rgba(0,0,0,0.18);
        color: var(--text);
        outline: none;
        font-family: inherit;
      }
      .field textarea { min-height: 92px; resize: vertical; font-family: var(--mono); font-size: 12px; line-height: 1.4; }
      .field input:focus, .field select:focus, .field textarea:focus { border-color: rgba(122,162,255,0.65); }

      .story {
        border: 1px solid var(--border);
        border-radius: 14px;
        padding: 12px;
        background: rgba(0,0,0,0.10);
        margin-top: 12px;
      }
      .story .row {
        display: grid;
        grid-template-columns: 1fr auto;
        align-items: center;
        gap: 10px;
      }
      .story .id {
        font-family: var(--mono);
        font-size: 12px;
        color: var(--muted);
      }
      .btn.danger {
        border-color: rgba(251,113,133,0.55);
        background: rgba(251,113,133,0.12);
      }

      pre {
        margin: 0;
        padding: 12px;
        border-radius: 12px;
        border: 1px solid var(--border);
        background: rgba(0,0,0,0.20);
        overflow: auto;
        font-family: var(--mono);
        font-size: 12px;
        line-height: 1.5;
        white-space: pre;
      }

      .kv {
        display: grid;
        grid-template-columns: 120px 1fr;
        gap: 10px;
        align-items: center;
        margin-top: 10px;
      }
      .kv .k { color: var(--muted); font-size: 12px; }
      .kv .v { font-family: var(--mono); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .muted { color: var(--muted); }

      .tabs {
        display: inline-flex;
        gap: 8px;
        padding: 6px;
        border: 1px solid var(--border);
        border-radius: 14px;
        background: rgba(0,0,0,0.10);
      }
      .tab {
        border: 1px solid var(--border);
        background: transparent;
        color: var(--text);
        border-radius: 12px;
        padding: 10px 12px;
        cursor: pointer;
        font-size: 13px;
      }
      .tab:hover { border-color: rgba(122,162,255,0.55); }
      .tab[aria-current="page"] {
        background: rgba(122,162,255,0.14);
        border-color: rgba(122,162,255,0.65);
      }
    </style>
    <script>
      (function () {
        const meta = document.querySelector('meta[name="ohmyagentflow-session-token"]');
        const token = meta && meta.content ? meta.content : '';
        if (!token) return;

        const originalFetch = window.fetch.bind(window);
        window.fetch = function (input, init) {
          const requestInit = init || {};
          const method = (requestInit.method || (input instanceof Request ? input.method : 'GET') || 'GET').toUpperCase();
          const url = new URL(input instanceof Request ? input.url : String(input), window.location.href);
          if (method !== 'POST' || !url.pathname.startsWith('/api/')) {
            return originalFetch(input, requestInit);
          }

          if (input instanceof Request) {
            const headers = new Headers(input.headers);
            if (requestInit.headers) new Headers(requestInit.headers).forEach((v, k) => headers.set(k, v));
            headers.set('X-Session-Token', token);
            const nextRequest = new Request(input, Object.assign({}, requestInit, { headers }));
            return originalFetch(nextRequest);
          }

          const headers = new Headers(requestInit.headers || {});
          headers.set('X-Session-Token', token);
          return originalFetch(input, Object.assign({}, requestInit, { headers }));
        };
      })();
    </script>
  </head>
  <body>
    <div class="app">
      <aside class="sidebar">
        <div class="brand">
          <div class="dot"></div>
          <div>
            <div class="title">Oh My Agent Flow</div>
            <div class="subtitle">Local console (safe-by-default)</div>
          </div>
        </div>

        <div class="stepper" role="navigation" aria-label="Workflow">
          <button class="step" data-step="init" aria-current="page" type="button">
            <div class="name">Init</div>
            <div class="desc">Install repo-local skills</div>
          </button>
          <button class="step" data-step="prd" type="button">
            <div class="name">PRD</div>
            <div class="desc">Preview and edit requirements</div>
          </button>
          <button class="step" data-step="convert" type="button">
            <div class="name">Convert</div>
            <div class="desc">Produce root prd.json</div>
          </button>
          <button class="step" data-step="fire" type="button">
            <div class="name">Fire</div>
            <div class="desc">Run Ralph until COMPLETE</div>
          </button>
        </div>
      </aside>

      <section class="content">
        <header class="topbar">
          <div class="left">
            <div class="label">Project root</div>
            <div class="root" id="project-root" title="{{.ProjectRoot}}">{{.ProjectRoot}}</div>
          </div>
          <div class="right">
            <span class="badge" id="stream-badge" title="SSE connection status">
              <span class="pill" aria-hidden="true"></span>
              <span id="stream-text">Stream: connecting…</span>
            </span>
            <button class="btn" id="copy-root" type="button">Copy root</button>
          </div>
        </header>

        <main class="main">
          <section class="panel" data-panel="init">
            <h2>Init</h2>
            <p>Prepare <span class="muted">.codex/skills</span> so Ralph can run safely without shell injection.</p>
            <div class="grid2">
              <div class="panel">
                <h2>Action</h2>
                <p>Runs <span class="muted">POST /api/init</span> to install repo-local Codex skills into <span class="muted">.codex/skills</span>.</p>
                <button class="btn primary" id="init-run" type="button">Initialize skills</button>
              </div>
              <div class="panel">
                <h2>Result</h2>
                <pre id="init-result">Not run yet.</pre>
              </div>
            </div>
          </section>

          <section class="panel" data-panel="prd" style="display:none">
            <h2>PRD</h2>
            <p>Generate a Convert-compatible PRD markdown file via questionnaire or chat mode, with safe saving under <span class="muted">tasks/</span>.</p>
            <div style="display:flex; align-items:center; justify-content:space-between; gap: 10px; flex-wrap: wrap;">
              <div class="tabs" role="tablist" aria-label="PRD mode">
                <button class="tab" id="prd-mode-questionnaire-btn" type="button" aria-current="page">Questionnaire</button>
                <button class="tab" id="prd-mode-chat-btn" type="button">Chat (beta)</button>
              </div>
              <span class="badge" title="Chat mode uses sessions and slot-state; finalize writes the exact same markdown template.">
                <span class="pill" aria-hidden="true"></span>
                <span>Deterministic template</span>
              </span>
            </div>
            <div style="height: 14px"></div>

            <div id="prd-mode-questionnaire">
            <div class="grid2">
              <div class="panel">
                <h2>Questionnaire</h2>
                <p class="muted">All fields are validated and the output follows <span class="muted">ohmyagentflow/prd@1</span>.</p>
                <div class="field">
                  <label for="prd-gen-project">Project (optional)</label>
                  <input id="prd-gen-project" placeholder="TaskApp" autocomplete="off" />
                </div>
                <div class="field">
                  <label for="prd-gen-slug">Feature slug (kebab-case)</label>
                  <input id="prd-gen-slug" placeholder="task-status" autocomplete="off" />
                </div>
                <div class="field">
                  <label for="prd-gen-title">Title</label>
                  <input id="prd-gen-title" placeholder="Task Status Feature" autocomplete="off" />
                </div>
                <div class="field">
                  <label for="prd-gen-desc">Description (1 line)</label>
                  <input id="prd-gen-desc" placeholder="Add ability to mark tasks with different statuses." autocomplete="off" />
                </div>
                <div class="field">
                  <label for="prd-gen-goals">Goals (one per line)</label>
                  <textarea id="prd-gen-goals" placeholder="Track task progress\nImprove visibility"></textarea>
                </div>
                <div class="field">
                  <label for="prd-gen-fr">Functional Requirements (one per line)</label>
                  <textarea id="prd-gen-fr" placeholder="FR-1: ...\nFR-2: ..."></textarea>
                </div>
                <div class="field">
                  <label for="prd-gen-nongoals">Non-Goals (one per line)</label>
                  <textarea id="prd-gen-nongoals" placeholder="Mobile app support"></textarea>
                </div>
                <div class="field">
                  <label for="prd-gen-metrics">Success Metrics (one per line)</label>
                  <textarea id="prd-gen-metrics" placeholder="Reduced support tickets"></textarea>
                </div>
                <div class="field">
                  <label for="prd-gen-questions">Open Questions (one per line)</label>
                  <textarea id="prd-gen-questions" placeholder="Do we need audit logs?"></textarea>
                </div>
                <div class="kv">
                  <div class="k">Endpoint</div>
                  <div class="v">POST /api/prd/generate</div>
                </div>
                <div class="story">
                  <div class="row">
                    <div>
                      <div style="font-weight:650">User Stories</div>
                      <div class="id" id="prd-story-count">1 story</div>
                    </div>
                    <button class="btn" id="prd-gen-add-story" type="button">Add story</button>
                  </div>
                  <div id="prd-stories"></div>
                </div>
                <div style="height: 12px"></div>
                <div style="display:flex; gap: 10px; flex-wrap: wrap;">
                  <button class="btn" id="prd-gen-preview" type="button">Preview</button>
                  <button class="btn primary" id="prd-gen-save" type="button">Save to tasks/</button>
                </div>
                <div class="kv">
                  <div class="k">Saved path</div>
                  <div class="v" id="prd-gen-saved">(not saved)</div>
                </div>
              </div>
              <div class="panel">
                <h2>Preview</h2>
                <pre id="prd-gen-result">Fill the form to see a live preview…</pre>
              </div>
            </div>

            <div style="height: 14px"></div>

            <div class="grid2">
              <div class="panel">
                <h2>Load existing file</h2>
                <div class="field">
                  <label for="prd-path">Path (project-relative)</label>
                  <input id="prd-path" list="prd-suggestions" placeholder="tasks/prd-your-feature.md" autocomplete="off" />
                  <datalist id="prd-suggestions">
                    <option value="prd.json"></option>
                    <option value="progress.txt"></option>
                    <option value="tasks/prd-example.md"></option>
                  </datalist>
                </div>
                <button class="btn primary" id="prd-preview" type="button">Load preview</button>
                <div class="kv">
                  <div class="k">Endpoint</div>
                  <div class="v">GET /api/fs/read</div>
                </div>
              </div>
              <div class="panel">
                <h2>File content</h2>
                <pre id="prd-result">No file loaded.</pre>
              </div>
            </div>
            </div>

            <div id="prd-mode-chat" style="display:none">
              <div class="grid2">
                <div class="panel">
                  <h2>Chat PRD</h2>
                  <p class="muted">Chat messages update a structured slot-state. When ready, Finalize validates required fields and writes the same template as questionnaire mode.</p>
                  <div style="display:flex; gap: 10px; flex-wrap: wrap;">
                    <button class="btn primary" id="prd-chat-new" type="button">New session</button>
                    <button class="btn" id="prd-chat-refresh" type="button" disabled>Refresh state</button>
                    <button class="btn primary" id="prd-chat-finalize" type="button" disabled>Finalize to tasks/</button>
                  </div>
                  <div class="kv">
                    <div class="k">Session</div>
                    <div class="v" id="prd-chat-session">(none)</div>
                  </div>
                  <div class="kv">
                    <div class="k">Expires</div>
                    <div class="v" id="prd-chat-expires">(n/a)</div>
                  </div>
                  <div class="field">
                    <label for="prd-chat-tool">Tool</label>
                    <select id="prd-chat-tool">
                      <option value="codex">codex</option>
                      <option value="claude">claude</option>
                      <option value="">manual (no LLM)</option>
                    </select>
                  </div>
                  <div class="field">
                    <label for="prd-chat-message">Message</label>
                    <textarea id="prd-chat-message" placeholder="feature_slug: task-status\ntitle: Task Status Feature\ndescription: Add ability to mark tasks with different statuses.\n/story As a user, I can set a status\nstory_desc: Users can set status to todo/in-progress/done.\n/ac Typecheck passes"></textarea>
                  </div>
                  <div style="display:flex; gap: 10px; flex-wrap: wrap;">
                    <button class="btn" id="prd-chat-send" type="button" disabled>Send message</button>
                    <button class="btn" id="prd-chat-reset" type="button" disabled>Reset slot-state</button>
                  </div>
                  <div class="kv">
                    <div class="k">Endpoint</div>
                    <div class="v">POST /api/prd/chat/*</div>
                  </div>
                  <div class="kv">
                    <div class="k">Saved path</div>
                    <div class="v" id="prd-chat-saved">(not saved)</div>
                  </div>
                </div>
                <div class="panel">
                  <h2>Slot state</h2>
                  <pre id="prd-chat-slot">Create a session to begin…</pre>
                  <div style="height: 10px"></div>
                  <h2>Finalize result</h2>
                  <pre id="prd-chat-result">Finalize writes tasks/prd-&lt;feature_slug&gt;.md when required fields are present.</pre>
                </div>
              </div>
            </div>
          </section>

          <section class="panel" data-panel="convert" style="display:none">
            <h2>Convert</h2>
            <p>Convert a PRD markdown file into root <span class="muted">prd.json</span> with precise errors.</p>
            <div class="grid2">
              <div class="panel">
                <h2>Inputs</h2>
                <div class="field">
                  <label for="convert-path">PRD markdown</label>
                  <input id="convert-path" placeholder="tasks/prd-your-feature.md" autocomplete="off" />
                </div>
                <button class="btn primary" id="convert-run" type="button">Convert</button>
              </div>
              <div class="panel">
                <h2>Result</h2>
                <pre id="convert-result">Not run yet.</pre>
              </div>
            </div>
          </section>

          <section class="panel" data-panel="fire" style="display:none">
            <h2>Fire</h2>
            <p>Run Ralph until all stories pass. Live stream updates appear as SSE events.</p>
            <div class="grid2">
              <div class="panel">
                <h2>Run</h2>
                <div class="field">
                  <label for="fire-tool">Tool</label>
                  <select id="fire-tool">
                    <option value="codex">codex</option>
                    <option value="claude">claude</option>
                  </select>
                </div>
                <div class="field">
                  <label for="fire-iterations">Max iterations</label>
                  <input id="fire-iterations" type="number" min="1" max="200" value="10" />
                </div>
                <div style="display:flex; gap:10px; flex-wrap:wrap">
                  <button class="btn primary" id="fire-start" type="button">Start Fire</button>
                  <button class="btn danger" id="fire-stop" type="button">Stop</button>
                </div>
              </div>
              <div class="panel">
                <h2>Run log</h2>
                <pre id="fire-last">Waiting for events…</pre>
              </div>
            </div>
          </section>
        </main>
      </section>
    </div>

    <script>
      (function () {
        function setStreamBadge(state, text) {
          const badge = document.getElementById('stream-badge');
          const pill = badge.querySelector('.pill');
          const label = document.getElementById('stream-text');
          label.textContent = text;
          badge.classList.remove('good', 'bad');
          if (state === 'good') badge.classList.add('good');
          if (state === 'bad') badge.classList.add('bad');
          pill.style.background = state === 'good' ? 'var(--good)' : (state === 'bad' ? 'var(--bad)' : 'var(--warn)');
        }

        function setActivePanel(step) {
          const panels = document.querySelectorAll('[data-panel]');
          panels.forEach(p => { p.style.display = (p.getAttribute('data-panel') === step) ? '' : 'none'; });

          const steps = document.querySelectorAll('.step[data-step]');
          steps.forEach(s => {
            const isActive = s.getAttribute('data-step') === step;
            if (isActive) s.setAttribute('aria-current', 'page'); else s.removeAttribute('aria-current');
          });
        }

        function currentStepFromHash() {
          const raw = (window.location.hash || '').replace(/^#/, '').trim();
          return raw || 'init';
        }

        document.querySelectorAll('.step[data-step]').forEach(btn => {
          btn.addEventListener('click', () => {
            window.location.hash = btn.getAttribute('data-step');
          });
        });

        window.addEventListener('hashchange', () => setActivePanel(currentStepFromHash()));
        setActivePanel(currentStepFromHash());

        function setPRDMode(mode) {
          const questionnaire = document.getElementById('prd-mode-questionnaire');
          const chat = document.getElementById('prd-mode-chat');
          const btnQ = document.getElementById('prd-mode-questionnaire-btn');
          const btnC = document.getElementById('prd-mode-chat-btn');
          if (!questionnaire || !chat || !btnQ || !btnC) return;

          const next = (mode === 'chat') ? 'chat' : 'questionnaire';
          questionnaire.style.display = (next === 'questionnaire') ? '' : 'none';
          chat.style.display = (next === 'chat') ? '' : 'none';
          if (next === 'questionnaire') {
            btnQ.setAttribute('aria-current', 'page');
            btnC.removeAttribute('aria-current');
          } else {
            btnC.setAttribute('aria-current', 'page');
            btnQ.removeAttribute('aria-current');
          }
          try { window.localStorage.setItem('ohmyagentflow.prdMode', next); } catch (_) {}
        }

        document.getElementById('prd-mode-questionnaire-btn').addEventListener('click', () => setPRDMode('questionnaire'));
        document.getElementById('prd-mode-chat-btn').addEventListener('click', () => setPRDMode('chat'));
        try {
          const savedMode = window.localStorage.getItem('ohmyagentflow.prdMode');
          setPRDMode(savedMode || 'questionnaire');
        } catch (_) {
          setPRDMode('questionnaire');
        }

        document.getElementById('copy-root').addEventListener('click', async () => {
          const el = document.getElementById('project-root');
          const text = el.textContent || '';
          try {
            await navigator.clipboard.writeText(text);
          } catch (_) {
            const tmp = document.createElement('textarea');
            tmp.value = text;
            document.body.appendChild(tmp);
            tmp.select();
            document.execCommand('copy');
            tmp.remove();
          }
        });

        async function fetchJSON(url, init) {
          const resp = await fetch(url, init || {});
          const text = await resp.text();
          let data = null;
          try { data = text ? JSON.parse(text) : null; } catch (_) {}
          if (!resp.ok) {
            const msg = data && data.message ? data.message : ('HTTP ' + resp.status);
            const where = (data && data.file && data.location && data.location.line) ? ('\\nAt: ' + data.file + ':' + data.location.line + ':' + (data.location.column || 1)) : '';
            const hint = data && data.hint ? ('\\nHint: ' + data.hint) : '';
            throw new Error(msg + where + hint);
          }
          return data;
        }

        document.getElementById('init-run').addEventListener('click', async () => {
          const out = document.getElementById('init-result');
          out.textContent = 'Running…';
          try {
            const data = await fetchJSON('/api/init', { method: 'POST' });
            out.textContent = JSON.stringify(data, null, 2);
          } catch (e) {
            out.textContent = String(e && e.message ? e.message : e);
          }
        });

        document.getElementById('prd-preview').addEventListener('click', async () => {
          const path = (document.getElementById('prd-path').value || '').trim();
          const out = document.getElementById('prd-result');
          out.textContent = 'Loading…';
          try {
            const data = await fetchJSON('/api/fs/read?path=' + encodeURIComponent(path));
            out.textContent = data && typeof data.content === 'string' ? data.content : '(no content)';
          } catch (e) {
            out.textContent = String(e && e.message ? e.message : e);
          }
        });

        document.getElementById('convert-run').addEventListener('click', async () => {
          const path = (document.getElementById('convert-path').value || '').trim();
          const out = document.getElementById('convert-result');
          out.textContent = 'Converting…';
          try {
            const data = await fetchJSON('/api/convert', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ prdPath: path })
            });
            if (data && typeof data.json === 'string') {
              out.textContent = data.json;
            } else {
              out.textContent = JSON.stringify(data, null, 2);
            }
          } catch (e) {
            out.textContent = String(e && e.message ? e.message : e);
          }
        });

        function splitLines(text) {
          return String(text || '')
            .split(/\\r?\\n/)
            .map(s => s.trim())
            .filter(Boolean);
        }

        function storyId(i) {
          const n = i + 1;
          return 'US-' + String(n).padStart(3, '0');
        }

        function updateStoryCount() {
          const stories = document.querySelectorAll('[data-story]');
          const label = document.getElementById('prd-story-count');
          const n = stories.length;
          label.textContent = n + (n === 1 ? ' story' : ' stories');
          stories.forEach((el, idx) => {
            const idEl = el.querySelector('[data-story-id]');
            if (idEl) idEl.textContent = storyId(idx);
          });
        }

        function createStoryCard() {
          const wrap = document.createElement('div');
          wrap.className = 'story';
          wrap.setAttribute('data-story', '1');

          const header = document.createElement('div');
          header.className = 'row';

          const left = document.createElement('div');
          left.innerHTML = '<div style="font-weight:650">Story <span data-story-id></span></div><div class="id">IDs are auto-generated and must be sequential.</div>';

          const remove = document.createElement('button');
          remove.type = 'button';
          remove.className = 'btn danger';
          remove.textContent = 'Remove';
          remove.addEventListener('click', () => {
            wrap.remove();
            updateStoryCount();
            schedulePreview();
          });

          header.appendChild(left);
          header.appendChild(remove);

          const titleField = document.createElement('div');
          titleField.className = 'field';
          titleField.innerHTML = '<label>Title</label><input data-story-title placeholder="Add status field to tasks table" autocomplete="off" />';

          const descField = document.createElement('div');
          descField.className = 'field';
          descField.innerHTML = '<label>Description (single line)</label><input data-story-desc placeholder="As a user, I want ... so that ..." autocomplete="off" />';

          const acField = document.createElement('div');
          acField.className = 'field';
          acField.innerHTML = '<label>Acceptance Criteria (one per line)</label><textarea data-story-ac placeholder="...\\nTypecheck passes"></textarea>';

          [titleField, descField, acField].forEach(el => {
            el.addEventListener('input', schedulePreview);
            el.addEventListener('change', schedulePreview);
          });

          wrap.appendChild(header);
          wrap.appendChild(titleField);
          wrap.appendChild(descField);
          wrap.appendChild(acField);
          return wrap;
        }

        const storiesRoot = document.getElementById('prd-stories');
        if (storiesRoot && storiesRoot.children.length === 0) {
          storiesRoot.appendChild(createStoryCard());
          updateStoryCount();
        }

        document.getElementById('prd-gen-add-story').addEventListener('click', () => {
          storiesRoot.appendChild(createStoryCard());
          updateStoryCount();
          schedulePreview();
        });

        function buildPRDPayload() {
          const project = (document.getElementById('prd-gen-project').value || '').trim();
          const featureSlug = (document.getElementById('prd-gen-slug').value || '').trim();
          const title = (document.getElementById('prd-gen-title').value || '').trim();
          const description = (document.getElementById('prd-gen-desc').value || '').trim();

          const goals = splitLines(document.getElementById('prd-gen-goals').value);
          const functionalRequirements = splitLines(document.getElementById('prd-gen-fr').value);
          const nonGoals = splitLines(document.getElementById('prd-gen-nongoals').value);
          const successMetrics = splitLines(document.getElementById('prd-gen-metrics').value);
          const openQuestions = splitLines(document.getElementById('prd-gen-questions').value);

          const storyEls = Array.from(document.querySelectorAll('[data-story]'));
          const userStories = storyEls.map((el, idx) => {
            const stTitle = (el.querySelector('[data-story-title]').value || '').trim();
            const stDesc = (el.querySelector('[data-story-desc]').value || '').trim();
            const ac = splitLines(el.querySelector('[data-story-ac]').value);
            return { id: storyId(idx), title: stTitle, description: stDesc, acceptanceCriteria: ac };
          });

          return {
            mode: 'questionnaire',
            frontMatter: { project, featureSlug, title, description },
            goals,
            userStories,
            functionalRequirements,
            nonGoals,
            successMetrics,
            openQuestions
          };
        }

        let previewTimer = null;
        async function runPreview() {
          const out = document.getElementById('prd-gen-result');
          const payload = buildPRDPayload();
          if (!payload.frontMatter.featureSlug || !payload.frontMatter.title || !payload.frontMatter.description) {
            out.textContent = 'Fill feature slug, title, and description to preview…';
            return;
          }
          try {
            const data = await fetchJSON('/api/prd/generate?preview=1', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(payload)
            });
            out.textContent = data && typeof data.content === 'string' ? data.content : '(no content)';
          } catch (e) {
            out.textContent = String(e && e.message ? e.message : e);
          }
        }

        function schedulePreview() {
          if (previewTimer) window.clearTimeout(previewTimer);
          previewTimer = window.setTimeout(runPreview, 350);
        }

        ['prd-gen-project','prd-gen-slug','prd-gen-title','prd-gen-desc','prd-gen-goals','prd-gen-fr','prd-gen-nongoals','prd-gen-metrics','prd-gen-questions'].forEach(id => {
          const el = document.getElementById(id);
          if (!el) return;
          el.addEventListener('input', schedulePreview);
          el.addEventListener('change', schedulePreview);
        });

        document.getElementById('prd-gen-preview').addEventListener('click', async () => {
          const out = document.getElementById('prd-gen-result');
          out.textContent = 'Previewing…';
          await runPreview();
        });

        document.getElementById('prd-gen-save').addEventListener('click', async () => {
          const out = document.getElementById('prd-gen-result');
          out.textContent = 'Saving…';
          const payload = buildPRDPayload();
          try {
            const data = await fetchJSON('/api/prd/generate', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(payload)
            });
            out.textContent = data && typeof data.content === 'string' ? data.content : '(no content)';
            const saved = document.getElementById('prd-gen-saved');
            if (saved) saved.textContent = (data && data.path) ? data.path : '(unknown)';
            const pathInput = document.getElementById('prd-path');
            if (pathInput && data && data.path) pathInput.value = data.path;
          } catch (e) {
            out.textContent = String(e && e.message ? e.message : e);
          }
        });

        // Kick off an initial best-effort preview to populate placeholders.
        schedulePreview();

        // PRD chat mode (slot-state sessions)
        const chatNew = document.getElementById('prd-chat-new');
        const chatSend = document.getElementById('prd-chat-send');
        const chatReset = document.getElementById('prd-chat-reset');
        const chatRefresh = document.getElementById('prd-chat-refresh');
        const chatFinalize = document.getElementById('prd-chat-finalize');
        const chatMessage = document.getElementById('prd-chat-message');
        const chatTool = document.getElementById('prd-chat-tool');
        const chatSlot = document.getElementById('prd-chat-slot');
        const chatResult = document.getElementById('prd-chat-result');
        const chatSessionLabel = document.getElementById('prd-chat-session');
        const chatExpiresLabel = document.getElementById('prd-chat-expires');
        const chatSavedLabel = document.getElementById('prd-chat-saved');

        let chatSessionId = '';

        function setChatEnabled(enabled) {
          [chatSend, chatReset, chatRefresh, chatFinalize].forEach(btn => {
            if (!btn) return;
            btn.disabled = !enabled;
          });
        }

        function renderSlotState(slotState) {
          if (!chatSlot) return;
          chatSlot.textContent = JSON.stringify(slotState || null, null, 2);
        }

        function setChatSession(sessionId, expiresAt) {
          chatSessionId = sessionId || '';
          if (chatSessionLabel) chatSessionLabel.textContent = chatSessionId ? chatSessionId : '(none)';
          if (chatExpiresLabel) chatExpiresLabel.textContent = expiresAt ? expiresAt : '(n/a)';
          setChatEnabled(Boolean(chatSessionId));
        }

        if (chatNew) {
          chatNew.addEventListener('click', async () => {
            if (chatResult) chatResult.textContent = 'Creating session…';
            if (chatSavedLabel) chatSavedLabel.textContent = '(not saved)';
            try {
              const data = await fetchJSON('/api/prd/chat/session', { method: 'POST' });
              setChatSession(data && data.sessionId, data && data.expiresAt);
              renderSlotState(data && data.slotState);
              if (chatResult) chatResult.textContent = 'Session created. Send messages to fill the slot-state, then Finalize.';
            } catch (e) {
              setChatSession('', '');
              if (chatResult) chatResult.textContent = String(e && e.message ? e.message : e);
            }
          });
        }

        if (chatSend) {
          chatSend.addEventListener('click', async () => {
            if (!chatSessionId) return;
            if (chatResult) chatResult.textContent = 'Sending message…';
            const message = (chatMessage && chatMessage.value) ? chatMessage.value : '';
            const tool = (chatTool && chatTool.value !== undefined) ? String(chatTool.value) : '';
            try {
              const data = await fetchJSON('/api/prd/chat/message', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId: chatSessionId, message, tool })
              });
              renderSlotState(data && data.slotState);
              if (chatResult) chatResult.textContent = 'Message applied. Missing/warnings are in slotState.';
            } catch (e) {
              if (chatResult) chatResult.textContent = String(e && e.message ? e.message : e);
            }
          });
        }

        if (chatReset) {
          chatReset.addEventListener('click', async () => {
            if (!chatSessionId) return;
            if (chatResult) chatResult.textContent = 'Resetting slot-state…';
            try {
              const data = await fetchJSON('/api/prd/chat/message', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId: chatSessionId, message: '/reset' })
              });
              renderSlotState(data && data.slotState);
              if (chatSavedLabel) chatSavedLabel.textContent = '(not saved)';
              if (chatResult) chatResult.textContent = 'Slot-state reset.';
            } catch (e) {
              if (chatResult) chatResult.textContent = String(e && e.message ? e.message : e);
            }
          });
        }

        if (chatRefresh) {
          chatRefresh.addEventListener('click', async () => {
            if (!chatSessionId) return;
            if (chatResult) chatResult.textContent = 'Refreshing state…';
            try {
              const data = await fetchJSON('/api/prd/chat/state?sessionId=' + encodeURIComponent(chatSessionId));
              renderSlotState(data && data.slotState);
              if (chatResult) chatResult.textContent = 'State refreshed.';
            } catch (e) {
              if (chatResult) chatResult.textContent = String(e && e.message ? e.message : e);
            }
          });
        }

        if (chatFinalize) {
          chatFinalize.addEventListener('click', async () => {
            if (!chatSessionId) return;
            if (chatResult) chatResult.textContent = 'Finalizing…';
            if (chatSavedLabel) chatSavedLabel.textContent = '(not saved)';
            try {
              const data = await fetchJSON('/api/prd/chat/finalize', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ sessionId: chatSessionId })
              });
              if (data && data.slotState) renderSlotState(data.slotState);
              if (data && data.ok) {
                if (chatSavedLabel) chatSavedLabel.textContent = data.path || '(unknown)';
                if (chatResult) chatResult.textContent = data.content || '(no content)';
                const pathInput = document.getElementById('prd-path');
                if (pathInput && data.path) pathInput.value = data.path;
              } else {
                const missing = (data && data.missing) ? JSON.stringify(data.missing, null, 2) : '[]';
                const warnings = (data && data.warnings) ? JSON.stringify(data.warnings, null, 2) : '[]';
                if (chatResult) chatResult.textContent = 'Missing required fields:\n' + missing + '\n\nWarnings:\n' + warnings;
              }
            } catch (e) {
              if (chatResult) chatResult.textContent = String(e && e.message ? e.message : e);
            }
          });
        }

        setChatSession('', '');

        // Fire (run Ralph via bash and stream logs)
        const fireStart = document.getElementById('fire-start');
        const fireStop = document.getElementById('fire-stop');
        const fireTool = document.getElementById('fire-tool');
        const fireIterations = document.getElementById('fire-iterations');
        let fireRunId = '';
        let fireES = null;
        let fireState = null;
        let fireRenderPending = false;

        function setFireOutput(text) {
          const out = document.getElementById('fire-last');
          if (!out) return;
          out.textContent = String(text || '');
        }

        function resetFireState(runId) {
          fireState = {
            runId: runId || '',
            tool: '',
            maxIterations: 0,
            currentIteration: 0,
            phase: '',
            completeDetected: false,
            iterations: {},
            global: [],
            finished: null,
          };
        }

        function ensureFireState(runId) {
          if (!fireState) resetFireState(runId || '');
          if (runId && fireState.runId !== runId) resetFireState(runId);
          return fireState;
        }

        function parseIntSafe(v) {
          const n = parseInt(String(v || '0'), 10);
          return Number.isFinite(n) ? n : 0;
        }

        function iterationBucket(iteration) {
          const st = ensureFireState(fireRunId);
          const n = parseIntSafe(iteration);
          const key = String(n);
          if (!st.iterations[key]) st.iterations[key] = { lines: [], phase: '' };
          return st.iterations[key];
        }

        function pushBucketLine(iteration, line) {
          const bucket = iterationBucket(iteration);
          bucket.lines.push(String(line || ''));
          const maxLinesPerIteration = 400;
          if (bucket.lines.length > maxLinesPerIteration) {
            bucket.lines = bucket.lines.slice(bucket.lines.length - maxLinesPerIteration);
          }
        }

        function scheduleFireRender() {
          if (fireRenderPending) return;
          fireRenderPending = true;
          requestAnimationFrame(() => {
            fireRenderPending = false;
            renderFireLog();
          });
        }

        function renderFireLog() {
          const st = ensureFireState(fireRunId);
          const parts = [];
          parts.push('Run: ' + (st.runId || '(none)'));
          const tool = st.tool ? (' tool=' + st.tool) : '';
          const maxI = st.maxIterations ? (' maxIterations=' + st.maxIterations) : '';
          const phase = st.phase ? (' phase=' + st.phase) : '';
          const complete = st.completeDetected ? ' completeDetected=true' : '';
          parts.push('Status:' + tool + maxI + ' iteration=' + (st.currentIteration || 0) + phase + complete);

          const keys = Object.keys(st.iterations || {}).map(k => parseIntSafe(k)).sort((a, b) => a - b);
          keys.forEach((k) => {
            const bucket = st.iterations[String(k)];
            if (!bucket) return;
            parts.push('');
            parts.push('--- Iteration ' + k + (bucket.phase ? (' (' + bucket.phase + ')') : '') + ' ---');
            (bucket.lines || []).forEach(line => parts.push(line));
          });

          if (st.finished) {
            parts.push('');
            parts.push('--- Finished ---');
            parts.push(JSON.stringify(st.finished, null, 2));
          }

          setFireOutput(parts.join('\n'));
        }

        function handleFireEvent(ev) {
          if (!ev || typeof ev !== 'object') return;
          if (fireRunId && ev.runId && String(ev.runId) !== String(fireRunId)) return;

          const st = ensureFireState(fireRunId);
          const type = ev.type ? String(ev.type) : '';
          const step = ev.step ? String(ev.step) : '';
          const data = (ev.data && typeof ev.data === 'object') ? ev.data : {};

          if (type === 'run_started') {
            if (data.tool) st.tool = String(data.tool);
            if (data.maxIterations) st.maxIterations = parseIntSafe(data.maxIterations);
            st.phase = 'run_started';
            scheduleFireRender();
            return;
          }

          if (type === 'run_finished') {
            st.phase = 'run_finished';
            st.finished = ev;
            scheduleFireRender();
            return;
          }

          if (type === 'progress' && step === 'fire') {
            if (data.tool) st.tool = String(data.tool);
            if (data.maxIterations) st.maxIterations = parseIntSafe(data.maxIterations);
            if (data.iteration !== undefined) st.currentIteration = parseIntSafe(data.iteration);
            if (data.completeDetected !== undefined) st.completeDetected = !!data.completeDetected;
            if (data.phase) st.phase = String(data.phase);

            const iter = st.currentIteration || 0;
            const note = data.note ? String(data.note) : '';
            const phaseText = data.phase ? String(data.phase) : '';
            const line = '[progress] phase=' + phaseText + (note ? (' note=' + note) : '');
            pushBucketLine(iter, line);
            iterationBucket(iter).phase = phaseText || iterationBucket(iter).phase;
            scheduleFireRender();
            return;
          }

          if (type === 'process_stdout' || type === 'process_stderr') {
            const iter = (data.iteration !== undefined) ? parseIntSafe(data.iteration) : (st.currentIteration || 0);
            const txt = data.text ? String(data.text) : '';
            const prefix = (type === 'process_stderr') ? '[err] ' : '[out] ';
            if (txt) pushBucketLine(iter, prefix + txt);
            scheduleFireRender();
            return;
          }
        }

        function closeFireStream() {
          if (!fireES) return;
          try { fireES.close(); } catch (_) {}
          fireES = null;
        }

        function connectFireStream(runId) {
          closeFireStream();
          if (!runId) return;
          try {
            fireES = new EventSource('/api/stream?runId=' + encodeURIComponent(runId));
            fireES.onmessage = (e) => {
              const raw = (e && e.data) ? e.data : '';
              if (!raw) return;
              try {
                const ev = JSON.parse(raw);
                handleFireEvent(ev);
              } catch (_) {
                setFireOutput(raw);
              }
            };
            fireES.onerror = () => {
              // Keep last message; Stream badge still reports global connectivity.
            };
          } catch (_) {}
        }

        if (fireStart) {
          fireStart.addEventListener('click', async () => {
            const tool = (fireTool && fireTool.value !== undefined) ? String(fireTool.value) : '';
            const n = parseInt((fireIterations && fireIterations.value) ? String(fireIterations.value) : '0', 10);
            if (!n || n < 1) {
              setFireOutput('Pick maxIterations >= 1.');
              return;
            }
            setFireOutput('Starting Fire…');
            fireRunId = '';
            resetFireState('');
            closeFireStream();
            try {
              const data = await fetchJSON('/api/fire', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ tool, maxIterations: n })
              });
              fireRunId = (data && data.runId) ? String(data.runId) : '';
              if (!fireRunId) {
                setFireOutput('Started, but no runId was returned.');
                return;
              }
              resetFireState(fireRunId);
              setFireOutput('Started runId=' + fireRunId + '. Connecting stream…');
              connectFireStream(fireRunId);
            } catch (e) {
              setFireOutput(String(e && e.message ? e.message : e));
            }
          });
        }

        if (fireStop) {
          fireStop.addEventListener('click', async () => {
            if (!fireRunId) {
              setFireOutput('No active runId. Start Fire first.');
              return;
            }
            setFireOutput('Stopping runId=' + fireRunId + '…');
            try {
              const data = await fetchJSON('/api/fire/stop', { method: 'POST' });
              const stopping = !!(data && data.stopping);
              setFireOutput(stopping ? ('Stop requested for runId=' + fireRunId + '.') : ('Stopped runId=' + fireRunId + '.'));
            } catch (e) {
              setFireOutput(String(e && e.message ? e.message : e));
            }
          });
        }

        // Best-effort SSE connection for live status. Runs without requiring a runId.
        try {
          const es = new EventSource('/api/stream');
          setStreamBadge('warn', 'Stream: connecting…');
          es.onopen = () => setStreamBadge('good', 'Stream: connected');
          es.onerror = () => setStreamBadge('bad', 'Stream: disconnected');
          es.onmessage = (e) => {
            if (fireRunId) return;
            const target = document.getElementById('fire-last');
            if (!target) return;
            const raw = (e && e.data) ? e.data : '';
            if (!raw) return;
            try {
              const ev = JSON.parse(raw);
              target.textContent = JSON.stringify(ev, null, 2);
            } catch (_) {
              target.textContent = raw;
            }
          };
        } catch (_) {
          setStreamBadge('bad', 'Stream: unavailable');
        }
      })();
    </script>
  </body>
</html>
`))

func RenderIndexHTML(data IndexPageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := indexPageTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
