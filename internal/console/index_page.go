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
            <p>Generate a Convert-compatible PRD markdown file (questionnaire mode), with live preview and safe saving under <span class="muted">tasks/</span>.</p>
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
                <button class="btn primary" type="button" disabled>Convert (coming soon)</button>
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
                    <option value="amp">amp</option>
                    <option value="claude">claude</option>
                    <option value="codex">codex</option>
                  </select>
                </div>
                <div class="field">
                  <label for="fire-iterations">Max iterations</label>
                  <input id="fire-iterations" type="number" min="1" max="50" value="10" />
                </div>
                <button class="btn primary" type="button" disabled>Start Fire (coming soon)</button>
              </div>
              <div class="panel">
                <h2>Last stream message</h2>
                <pre id="fire-last">Waiting for stream…</pre>
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
            const hint = data && data.hint ? ('\\nHint: ' + data.hint) : '';
            throw new Error(msg + hint);
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

        // Best-effort SSE connection for live status. Runs without requiring a runId.
        try {
          const es = new EventSource('/api/stream');
          setStreamBadge('warn', 'Stream: connecting…');
          es.onopen = () => setStreamBadge('good', 'Stream: connected');
          es.onerror = () => setStreamBadge('bad', 'Stream: disconnected');
          es.onmessage = (e) => {
            const target = document.getElementById('fire-last');
            if (!target) return;
            const raw = (e && e.data) ? e.data : '';
            target.textContent = raw;
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
