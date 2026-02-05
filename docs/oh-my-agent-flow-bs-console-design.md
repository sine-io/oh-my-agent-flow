# Oh-My-Agent-Flow 本地控制台（B/S）设计文档

> 版本：v0.5（MVP 设计 + 补充规格 + 风险强约束）
>
> 日期：2026-02-05
>
> 目标读者：项目维护者/实现者
>
> 说明：本文档固化当前已确认的约定与方案；在你发出“开始实现”指令前，不进入编码。

---

## 1. 背景与目标

当前项目主要通过 `sh` 脚本（如 `ralph-codex.sh`）驱动 Ralph agent loop。为提升可用性与可视化，我们计划提供一个“本地单二进制 + Web UI”的控制台：用户在自己的项目根目录执行二进制，即可打开一个本机页面完成初始化、PRD 生成、PRD 转换与启动执行（Fire）。

### 1.1 目标

- 用户在“自己的项目根目录”执行二进制后，打开本机 Web 页面完成全流程操作。
- 保持与现有脚本/技能的语义一致（尤其是 Fire 的执行逻辑）。
- 结构化日志：可按步骤与 iteration 分组，便于定位问题与复盘。
- 先做 macOS/Linux、本机单用户；后续再扩展多端/远程。

### 1.2 非目标（MVP）

- 不做远程多用户访问、不做权限系统。
- 不改变现有产物目录结构（先沿用根目录逻辑）。
- 不引入复杂数据库/ORM（可预留扩展点）。

---

## 2. 已确认的关键约定（Decision Log）

### 2.1 平台与运行形态

- 目标平台：macOS/Linux
- 交付形态：单个 Go 编译后的二进制；启动后“程序所有功能均可用”（服务端 + UI 静态资源一体）。
- 访问范围：仅本机（默认 `127.0.0.1`）。

### 2.1.1 启动参数与端口策略（v0.2 固化）

为确保 Origin 精确匹配与“自动打开浏览器”稳定可用，MVP 固化如下策略：

- 监听地址：固定 `127.0.0.1`
- 端口选择：
  - 默认使用随机可用端口（等价于 `--port 0`），避免端口冲突。
  - 可选参数：`--port <1..65535>`（显式指定端口；若被占用则启动失败并提示）。
- Base URL：
  - 服务启动后以实际监听端口构造 canonical Base URL：`http://127.0.0.1:<port>`
  - 控制台启动时必须在 stdout 打印该 URL（便于用户手动打开/复制）。
- 自动打开浏览器：
  - macOS：优先 `open <url>`
  - Linux：优先 `xdg-open <url>`
  - 若自动打开失败：仅记录 warning，不影响服务运行（用户可手动打开打印出的 URL）。
  - 自动打开的行为可通过 `--no-open` 禁用（MVP 建议提供；默认开启）。

### 2.2 目录与产物（先保持原逻辑）

产物先放在用户项目根目录（未来可迁移到统一子目录，但不在 MVP）：

- `tasks/`（PRD markdown 输出）
- `prd.json`（Ralph 执行所需输入）
- `progress.txt`、`archive/`、`.last-branch`（Ralph 脚本产物）
- `.codex/skills/.../SKILL.md`（本地技能安装）

### 2.3 技术栈

- 后端：Go
- 路由：Gin（确定使用）
- 日志：zerolog（结构化日志）
- 存储：MVP 不使用 ORM；如需持久化运行历史，优先 `jsonl` 落盘，后续可选 sqlite（尽量避免 cgo）
- 前端：构建静态资源并通过 `go:embed` 内嵌到二进制（“部署不分离”，工程上保持清晰边界）

### 2.4 PRD / Convert / Fire 的真实职责（以 repo 文档为准）

以下职责以：
- `skills/ralph-prd-generator/SKILL-codex.md`
- `skills/ralph-prd-converter/SKILL-codex.md`
- `ralph-codex.sh`
为准：

- PRD（generator）：输出 PRD Markdown 到 `tasks/prd-[feature-name].md`
- Convert（converter）：将 PRD Markdown 转换为 `prd.json`
- Fire（`ralph-codex.sh`）：循环执行 `codex exec ... < CODEX.md` 或 `claude ... < CLAUDE.md`，并检测 `<promise>COMPLETE</promise>`
- 重要说明：`ralph-codex.sh` 本身不包含“自动从 PRD 生成 prd.json”的逻辑；因此“Convert 可选”的体验需要在 UI/后端通过前置检查与引导实现（缺 `prd.json` 时阻止 Fire 并提示一键 Convert/先生成 PRD）。

---

## 3. 产品功能（页面按钮与行为）

页面包含 4 个核心功能按钮：

1) **Init**
- 行为：等价于 `./ralph-codex.sh init --tool codex` 的效果（安装本地 skills 到 `.codex/skills/.../SKILL.md` 等）。
- 安全策略：不直接执行 shell；由 Go 代码完成创建目录与复制文件（减少注入面）。

2) **PRD**
- 两种模式可选：
  - 问卷模式（推荐）：结构化表单，确定性生成符合模板的 PRD 文件。
  - 自由对话模式（高级）：聊天式交互，但必须“收敛”到同一模板字段，最终输出仍为可解析 PRD。
    - 自由对话模式采用“会话（session）+ 槽位状态（slot state）”模型：无论是否接入 LLM，后端都只产出结构化槽位状态与缺口列表，前端据此引导补齐。
    - `Finalize PRD` 时执行强校验：若缺必填字段，返回可定位的缺口清单，并支持一键切换到问卷模式补齐。
- 输出：`tasks/prd-<feature_slug>.md`

3) **Convert（可选）**
- 行为：将选择的 PRD 文件确定性转换为根目录 `prd.json`（严格依赖模板结构）。
- UI 提示：明确标注“可选”；但 Fire 需要 `prd.json`，缺失时会阻止并引导。

4) **Fire**
- 输入：`tool`（`codex|claude`）、`max_iterations`（整数）
- 行为：等价执行 `./ralph-codex.sh [--tool ...] [max_iterations]`
- 运行控制：支持 Stop/Cancel（SIGINT，必要时 SIGKILL）
- 日志：实时结构化输出（按 step、iteration 分组）

---

## 4. PRD 模板规范（可解析语法：ohmyagentflow/prd@1）

Convert **只支持**如下模板（不符合则报错并提示修复）：

- 文件路径：`tasks/prd-<feature-slug>.md`
- 必须包含 YAML Front Matter：

```md
---
schema: ohmyagentflow/prd@1
project: "<project name>"          # 可为空；为空则用 repo 目录名
feature_slug: "<kebab-case>"       # 必填；用于 branchName
title: "<human title>"             # 必填
description: "<1-3 sentences>"     # 必填
---
```

正文结构（关键段落必须存在，且 stories 与 AC 有强规则）：

```md
# PRD: <title>

## Goals
- ...

## User Stories
### US-001: <story title>
**Description:** As a <user>, I want <feature> so that <benefit>.

**Acceptance Criteria:**
- [ ] <criterion 1>
- [ ] <criterion 2>
- [ ] Typecheck passes

### US-002: ...

## Functional Requirements
1. FR-1: ...
2. FR-2: ...

## Non-Goals
- ...

## Success Metrics
- ...

## Open Questions
- ...
```

强规则（否则 Convert 报错）：

- `schema` 必须为 `ohmyagentflow/prd@1`
- `feature_slug/title/description` 必填
- `## User Stories` 下必须为 `### US-XXX: ...` 小节（US 编号连续由 PRD 生成器保证）
- 每个 story 必须有 `**Description:**` 单行与 `**Acceptance Criteria:**` checkbox 列表
- AC 列表项必须 `- [ ] ` 开头
- Convert 会保证每个 story 的 AC 最终包含 `"Typecheck passes"`（即使没写也会补齐）

---

## 5. Convert 规则：PRD -> prd.json（确定性映射）

### 5.1 输出格式（Ralph JSON）

输出根目录 `prd.json`，结构遵循 `skills/ralph-prd-converter/SKILL-codex.md`：

```json
{
  "project": "TaskApp",
  "branchName": "ralph/task-status",
  "description": "Task Status Feature - Track task progress with status indicators",
  "userStories": [
    {
      "id": "US-001",
      "title": "Add status field to tasks table",
      "description": "As a developer, I need to store task status in the database.",
      "acceptanceCriteria": ["...", "Typecheck passes"],
      "priority": 1,
      "passes": false,
      "notes": ""
    }
  ]
}
```

### 5.2 映射规则

- `project`：front matter `project`；为空则使用项目根目录名
- `branchName`：`ralph/` + `feature_slug`
- `description`：front matter `description`
- `userStories[]`：
  - 按 `US-001`…顺序生成
  - `priority` 从 1 递增
  - `passes=false`、`notes=""`
  - `acceptanceCriteria` 从 PRD checkbox 提取，并确保包含 `"Typecheck passes"`

### 5.3 报错规范（错误码 + 定位 + 修复建议）

错误对象统一格式：

```json
{
  "code": "PRD_PARSE_MISSING_SECTION",
  "message": "Missing '## User Stories' section",
  "file": "tasks/prd-foo.md",
  "location": { "line": 42, "column": 1 },
  "hint": "Add a '## User Stories' heading and at least one '### US-001: ...' story."
}
```

核心错误码（MVP 最小集合）：

- `PRD_PARSE_INVALID_FRONTMATTER`
- `PRD_PARSE_UNSUPPORTED_SCHEMA`
- `PRD_PARSE_MISSING_SECTION`
- `PRD_PARSE_STORY_HEADER_INVALID`
- `PRD_PARSE_STORY_DESCRIPTION_MISSING`
- `PRD_PARSE_STORY_AC_MISSING`
- `PRD_PARSE_AC_ITEM_INVALID`
- `CONVERT_IO_ERROR`

---

## 6. SSE 事件协议（结构化日志）

### 6.1 端点

- `GET /api/stream?runId=<id>&sinceSeq=<n>`（runId 可选；为空则推送全局事件）
  - `runId`：为空时推送“全局事件”（不保证可 replay，见下文）。
  - `sinceSeq`：仅在指定 `runId` 时生效；用于断线重连后 replay（服务端先补发 `seq > sinceSeq` 的缓存事件，再进入实时推送）。

### 6.2 事件格式

每条事件为 JSON：

```json
{
  "ts": "2026-02-05T16:22:10.123Z",
  "seq": 12,
  "runId": "run_20260205_162210_abcd",
  "type": "process_stdout",
  "step": "fire",
  "level": "info",
  "data": { "text": "Starting Ralph - Tool: codex - Max iterations: 10\n" }
}
```

- `seq`：
  - 在同一 `runId` 内单调递增（从 1 开始），用于 UI 去重、保持顺序与重连 replay。
  - 不要求全局（跨 run）单调递增。
- `type`（MVP）：
  - `run_started` / `run_finished`
  - `step_started` / `step_finished`（step：`init|prd|convert|fire`）
  - `process_stdout` / `process_stderr`
  - `progress`（iteration、检测到 COMPLETE 等）
  - `error`（同 Convert 报错结构）

#### 6.2.1 `progress` 事件 `data` 结构（v0.1 固化）

`progress` 事件用于 UI “结构化”展示，不依赖 stdout 解析细节。最小字段如下：

```json
{
  "ts": "2026-02-05T16:22:10.123Z",
  "runId": "run_20260205_162210_abcd",
  "type": "progress",
  "step": "fire",
  "level": "info",
  "data": {
    "tool": "codex",
    "iteration": 3,
    "maxIterations": 10,
    "phase": "iteration_started",
    "completeDetected": false,
    "note": "optional free text"
  }
}
```

- `phase` 枚举（MVP）：`iteration_started|iteration_finished|complete_detected|stopped|error`
- `completeDetected`：当检测到 `<promise>COMPLETE</promise>` 时为 `true`

#### 6.2.2 Run/Step 生命周期事件 `data`（v0.2 固化）

为便于 UI 与归档复盘，生命周期事件的 `data` 最小字段固化如下：

- `run_started.data`：
  - `op`: `init|prd|convert|fire`
  - `cwd`: `"<abs project root>"`（可选；用于诊断）
- `run_finished.data`：
  - `op`: `init|prd|convert|fire`
  - `reason`: `completed|stopped|error`
  - `durationMs`: number
  - `exitCode`: number（仅 `fire` 且进程已启动时；被 signal 终止可为 `null`）
  - `signal`: string（仅 `fire`；例如 `"SIGINT"`/`"SIGKILL"`；无则为 `null`）
- `step_started.data` / `step_finished.data`：
  - `step`: `init|prd|convert|fire`
  - `ok`: boolean（仅 `step_finished`；成功为 true）

### 6.3 输出治理

- stdout/stderr 单条最大长度（例如 8KB），超出截断并 `data.truncated=true`
- 每个 run 最大事件数/总输出量超限时，提示并停止继续推送（MVP 必须避免浏览器卡死）

#### 6.3.1 事件超限策略（v0.1 固化）

为避免长时间 Fire 造成内存/页面卡顿，事件保留与归档采用以下策略：

- 内存保留上限（按 run）：保留最近 `N` 条事件（建议 `N=5000`）。
- 超出上限后：后端继续接收事件，但只保留“最后 N 条”，并发出一次 `progress` 提示：
  - `data.phase="error"` 或 `data.phase="iteration_finished"` + `note="log truncated in UI; archived"`
- 归档（推荐实现）：将完整事件流写入项目根目录下的 `.ohmyagentflow/runs/<runId>.jsonl`
  - 每行一个事件 JSON（便于后续回放/诊断）
  - 运行中写入 `.jsonl.tmp`，run 结束后原子 rename 为 `.jsonl`（见 14.5）
  - 单个 run 归档文件大小上限（建议 50MB）；超过则停止写入并发出 `error` 事件提示

#### 6.3.3 SSE 断线重连与 replay（v0.2 固化）

- 指定 `runId` 的 SSE 连接支持 replay：
  - 客户端断线重连时带 `sinceSeq=<lastSeenSeq>`。
  - 服务端先发送缓存中 `seq > sinceSeq` 的事件（最多 `N` 条；若因截断导致缺失，应额外发送一次 `progress`：`data.phase="error"`，note 提示 “replay truncated; some events missing”）。
  - replay 结束后进入实时推送。
- `runId` 为空的“全局 SSE”不保证 replay（MVP 可只实时推送），前端应以“用于当前页面状态提示”为主，不依赖其完整性。

#### 6.3.2 归档文件轮转与清理（v0.4 固化）

为避免长期使用导致磁盘占用不可控，归档采用“按文件数 + 按大小”双阈值清理：

- 归档目录：`.ohmyagentflow/runs/`
- 保留数量上限：默认保留最近 `K=50` 个 run 的归档文件
- 目录总大小上限：默认 `1GB`
- 清理触发时机：
  - 新 run 归档开始前执行一次清理
  - 或每次写入时发现目录总大小超过上限（可降频，例如每 5s 检查一次）
- 清理策略：
  - 按最后修改时间从旧到新删除，直到同时满足 `K` 与 `1GB` 约束
  - 删除时向 SSE 发出一次 `progress`（`data.phase="iteration_finished"`，note 提示“old archives removed”）
  - 清理失败（权限等）发 `error` 事件，但不影响当前 run 继续执行

---

## 7. 安全与错误处理（MVP 必做）

### 7.1 监听范围

- 默认仅 `127.0.0.1`
- MVP 不提供对外网开放参数

### 7.2 写操作防护（Origin + Session Token）

对所有写操作（`POST /api/*`）：

- 校验 `Origin` 必须为 `http://127.0.0.1:<port>` 或 `http://localhost:<port>`（本服务自身）
- 使用启动时生成的随机 `X-Session-Token`（首次加载页面下发，后续每个写请求必须携带）
- 不开启宽松 CORS（不允许 `*`）

#### 7.2.1 Session Token 下发与校验细则（v0.2 固化）

- Token 生成：服务启动时生成 128-bit 随机值（base64url 或 hex）。
- Token 下发：
  - 服务对 `/`（或 `/index.html`）响应时，动态注入一个 meta 标签（或等价机制）：
    - `<meta name="ohmyagentflow-session-token" content="...">`
  - 前端从 meta 读取并在所有 `POST /api/*` 请求头附带 `X-Session-Token: <token>`。
- Base URL / Origin 规范化（避免实现与使用分歧）：
  - 服务启动后确定一个 canonical Base URL：`http://127.0.0.1:<port>`（MVP 固定使用 `127.0.0.1`，不以 `localhost` 作为 canonical）。
  - 若用户通过 `http://localhost:<port>` 访问首页：服务端应 `302` 重定向到 canonical Base URL，确保后续浏览器 `Origin` 稳定且可精确匹配。
- Origin 校验：
  - 若请求带 `Origin`：必须精确匹配 `http://127.0.0.1:<port>` 或 `http://localhost:<port>`。
  - 若请求不带 `Origin`（含 `Origin: null` 等不可判定场景）：默认拒绝（避免被非浏览器环境绕过）；如确需 CLI/脚本调用，后续再新增显式不安全开关（MVP 不提供）。

### 7.3 路径白名单与越界防护

- 项目根目录固定为启动时 `cwd`
- 清理/规范化路径后必须仍在根目录内（拒绝 `..`、外部绝对路径、软链接逃逸）
- 文件读取白名单（MVP）：
  - `tasks/prd-*.md`
  - `prd.json`
  - `progress.txt`
- 文件读取大小上限（例如 1–2MB）

### 7.4 子进程执行白名单

- Init：不跑 shell（Go 直接复制文件）
- Fire：仅允许执行：
  - `bash <abs>/ralph-codex.sh --tool <codex|claude> <max_iterations>`
  - 参数严格校验（tool 枚举；iterations 例如 1..200）
- 并发限制：同一时间仅 1 个 run（其余拒绝并提示）

### 7.5 Stop 语义

- Stop 必须对“整个 Fire 进程树”生效（避免只杀掉父进程，子进程继续跑）。
- 语义（MVP 固化）：
  - Fire 启动时必须创建独立进程组（见 14.2），并记录 PGID。
  - Stop 流程：对 PGID 发送 `SIGINT` -> 等待最多 5s -> 若仍未退出则对 PGID 发送 `SIGKILL`。
  - 等待条件：子进程 `Wait()` 返回（或 context done 后仍未返回则进入 SIGKILL）。
  - Stop 幂等：重复 Stop（同 run）返回成功（`stopping=true` 或 `alreadyStopping=true`），不应返回错误打断 UI 流程。
- 事件流必须明确 run 终止原因（建议 `run_finished.data.reason`）：`completed|stopped|error`，并包含 `exitCode`/`signal`（如适用）。

---

## 8. UI 信息架构（单页控制台）

### 8.1 页面布局

- 顶部栏：项目根目录（只读）、运行状态、快捷操作（后续扩展）
- 左侧 Stepper：Init / PRD / Convert（可选）/ Fire
- 中间：当前步骤表单 + 结果预览
- 右侧：结构化日志（可筛选、折叠、按 step/iteration 分组）

### 8.2 各步骤输入输出要点

- Init：一键执行；输出创建/安装清单；错误提示缺文件/权限。
- PRD：
  - 问卷模式：结构化表单保证模板可解析；实时预览；保存到 `tasks/prd-*.md`
  - 自由对话模式：可不完整；点击 Finalize 必须补齐必填字段，否则引导转问卷补齐；最终仍落同模板
- Convert：选择 PRD 文件；生成 `prd.json`；失败时显示 `code + file:line:col + hint` 并在预览中高亮行。
- Fire：tool/max_iterations；缺 `prd.json` 则阻止并引导 Convert；运行中提供 Stop；日志按 iteration 分组显示。

### 8.3 前端日志性能强约束（v0.2 固化）

为保证长时间 Fire 运行时页面稳定，前端必须遵守以下约束：

- 日志列表必须使用虚拟列表（virtualized list），不得一次性渲染全部事件 DOM。
- UI 内存保留：
  - 每个 run 仅保留最近 `N=5000` 条事件（与后端保持一致），超出则丢弃更早的事件。
  - 以 `seq` 去重（同 `runId` 内，`seq` 小于等于已见最大值的事件应忽略；重连 replay 的重复事件不得导致重复渲染）。
- 渲染节流：
  - SSE 消息到达后先进入队列；UI 每 `100ms`（或 `requestAnimationFrame`）批量合并更新一次，避免每条事件触发一次 React render。
- 展示策略：
  - `process_stdout/stderr` 默认折叠或按 iteration 分组；展开时仍需虚拟化。
  - 单条事件文本超过 8KB时后端已截断；前端仍需对超长行做 CSS/布局保护（避免测量与换行开销过大）。

---

## 9. 后端实现蓝图（模块划分）

推荐结构（示例）：

- `cmd/ohmyagentflow/main.go`：入口、启动 gin、logger、embed 静态资源
- `internal/httpserver/`：路由与 handler（只做校验 + 调用 service）
- `internal/services/`：
  - `initservice`：安装技能（Go 文件复制）
  - `prdservice`：问卷 PRD 生成（确定性模板）；自由对话预留接口
  - `convertservice`：PRD parser + `prd.json` 生成
  - `fireservice`：启动/停止子进程、流式输出事件
- `internal/prd/`：PRD AST、parser、错误类型（行号定位）
- `internal/events/`：SSE event、hub、限流/截断
- `internal/security/`：路径校验、exec 白名单、参数约束
- `internal/storage/`：MVP 可用内存；可选 jsonl 落盘接口

---

## 10. API 草案（MVP）

- `POST /api/init`
- `POST /api/prd/generate`（问卷模式）
- `POST /api/prd/chat/session`（自由对话：创建会话，v0.3）
- `POST /api/prd/chat/message`（自由对话：发送消息，v0.3）
- `GET /api/prd/chat/state?sessionId=`（自由对话：读取槽位状态，v0.3）
- `POST /api/prd/chat/finalize`（自由对话：强校验并落盘 PRD，v0.3）
- `POST /api/convert`（传 `prdPath`）
- `POST /api/fire`（传 `tool/maxIterations`）
- `POST /api/fire/stop`
- `GET /api/stream`（SSE）
- `GET /api/fs/read?path=`（只读预览，白名单）

### 10.0 API 通用约定（v0.2 固化）

- Base URL：`http://127.0.0.1:<port>`
- 编码：JSON UTF-8（除 SSE）
- 鉴权（本机防护）：所有 `POST /api/*` 必须携带 `X-Session-Token`
- `runId` 语义（MVP 固化）：
  - 所有写操作（`POST /api/*`）都会创建一个 `runId`，用于将“本次操作的事件流与日志”关联到 SSE。
  - `init/prd/convert` 为短任务：仍会发 `run_started/run_finished` 与 `step_started/step_finished`，但一般不会持续推送 `process_stdout/stderr`。
  - `fire` 为长任务：除生命周期事件外，会持续推送 `process_stdout/stderr/progress`。
- 标准响应 envelope：
  - 成功：HTTP 2xx
  - 失败：HTTP 4xx/5xx，返回统一错误对象

成功响应（示例）：

```json
{
  "ok": true,
  "runId": "run_20260205_162210_abcd",
  "data": {}
}
```

失败响应（示例）：

```json
{
  "ok": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "maxIterations must be between 1 and 200",
    "hint": "Choose a number in range 1..200"
  }
}
```

错误码约定（除 Convert/PRD 解析错误码外的通用错误码，MVP 最小集合）：

- `AUTH_MISSING_TOKEN` / `AUTH_INVALID_TOKEN`
- `AUTH_ORIGIN_NOT_ALLOWED`
- `VALIDATION_ERROR`
- `RESOURCE_CONFLICT`（例如 fire 已在运行）
- `NOT_FOUND`
- `INTERNAL_ERROR`

### 10.1 `fs/read` 返回格式与约束（v0.1 固化）

请求：

- Query：`path=<relative-path>`（相对项目根目录）

约束：

- 仅允许读取：
  - `tasks/prd-*.md`
  - `prd.json`
  - `progress.txt`
- 最大读取字节数（建议 1MB），超出截断并返回 `truncated=true`
- 仅支持 UTF-8 文本；无法解码时返回错误 `FS_READ_UNSUPPORTED_ENCODING`
- 截断必须在 UTF-8 字符边界上进行（不得返回半个 rune），以避免前端渲染/高亮失败。
- `size` 字段语义：原始文件字节数；返回内容实际字节数可由 `len(content)` 得到（MVP 不单独提供）。

响应：

```json
{
  "path": "tasks/prd-foo.md",
  "content": "# PRD: ...\n",
  "size": 12345,
  "truncated": false
}
```

错误（示例）：

```json
{
  "code": "FS_READ_NOT_ALLOWED",
  "message": "Path not allowed",
  "hint": "Only tasks/prd-*.md, prd.json, progress.txt are readable."
}
```

`fs/read` 相关错误码（MVP）：

- `FS_READ_NOT_ALLOWED`
- `FS_READ_NOT_FOUND`
- `FS_READ_TOO_LARGE`
- `FS_READ_UNSUPPORTED_ENCODING`

### 10.2 `POST /api/init`（v0.2 固化）

请求：

- Body：空（`{}` 或无 body）

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "created": [
      ".codex/skills",
      ".codex/skills/ralph-prd-generator/SKILL.md",
      ".codex/skills/ralph-prd-converter/SKILL.md"
    ],
    "overwritten": [],
    "warnings": []
  }
}
```

错误码（示例）：`NOT_FOUND`（缺源文件）、`INTERNAL_ERROR`（权限/IO）

### 10.3 `POST /api/prd/generate`（问卷模式，v0.2 固化）

请求：

```json
{
  "mode": "questionnaire",
  "frontMatter": {
    "project": "optional",
    "featureSlug": "task-status",
    "title": "Task Status Feature",
    "description": "Add ability to mark tasks with different statuses."
  },
  "goals": ["..."],
  "userStories": [
    {
      "id": "US-001",
      "title": "Add status field to tasks table",
      "description": "As a user, I want ... so that ...",
      "acceptanceCriteria": ["...", "Typecheck passes"]
    }
  ],
  "functionalRequirements": ["FR-1: ..."],
  "nonGoals": ["..."],
  "successMetrics": ["..."],
  "openQuestions": ["..."]
}
```

规则：

- `featureSlug/title/description` 必填
- `userStories[].id` 必须 `US-001` 顺序递增（由前端生成/校验）
- 后端会保证每个 story AC 含 `"Typecheck passes"`

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "path": "tasks/prd-task-status.md",
    "content": "# PRD: ...\n",
    "size": 1234
  }
}
```

错误码：`VALIDATION_ERROR`、`INTERNAL_ERROR`

### 10.4 `POST /api/convert`（PRD -> prd.json，v0.2 固化）

请求：

```json
{
  "prdPath": "tasks/prd-task-status.md"
}
```

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "outputPath": "prd.json",
    "backupPath": "prd.json.bak-20260205-162210",
    "summary": {
      "project": "TaskApp",
      "branchName": "ralph/task-status",
      "stories": 4
    },
    "content": "{\n  \"project\": ...\n}\n"
  }
}
```

错误码：

- PRD/Convert：`PRD_PARSE_*`、`CONVERT_IO_ERROR`
- 文件读取：`FS_READ_NOT_ALLOWED`、`FS_READ_NOT_FOUND`、`FS_READ_TOO_LARGE`、`FS_READ_UNSUPPORTED_ENCODING`

### 10.5 `POST /api/fire`（执行，v0.2 固化）

请求：

```json
{
  "tool": "codex",
  "maxIterations": 10
}
```

规则：

- `tool` ∈ `codex|claude`
- `maxIterations` ∈ `1..200`
- 若当前已有运行中的 fire：返回 `RESOURCE_CONFLICT`
- 若缺少 `prd.json`：返回 `VALIDATION_ERROR` 并给出 hint（引导先 Convert）

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "started": true
  }
}
```

### 10.6 `POST /api/fire/stop`（停止，v0.2 固化）

请求（两种形式）：

- 停止当前运行：`{}`
- 指定 run：`{"runId":"run_..."}`

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "stopping": true
  }
}
```

错误码：`NOT_FOUND`（无运行）、`INTERNAL_ERROR`

### 10.7 `GET /api/stream`（SSE，v0.2 固化）

- Query：`runId=<id>`（可选）
- Query：`sinceSeq=<n>`（可选；仅 runId 指定时生效，见 6.3.3）
- Response headers：
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
- 事件：使用 `data: <json>\n\n` 形式推送（JSON 即第 6 节 Event 格式）。

---

### 10.8 自由对话：槽位状态对象（v0.3 固化）

自由对话模式以“槽位状态”为核心协议，保证最终可收敛到第 4 节 PRD 模板，并保持 Convert 的确定性。

槽位状态（`slotState`）结构：

```json
{
  "frontMatter": {
    "project": "optional",
    "featureSlug": "task-status",
    "title": "Task Status Feature",
    "description": "Add ability to mark tasks with different statuses."
  },
  "goals": ["..."],
  "userStories": [
    {
      "id": "US-001",
      "title": "Add status field to tasks table",
      "description": "As a user, I want ... so that ...",
      "acceptanceCriteria": ["...", "Typecheck passes"]
    }
  ],
  "functionalRequirements": ["FR-1: ..."],
  "nonGoals": ["..."],
  "successMetrics": ["..."],
  "openQuestions": ["..."],
  "missing": [
    "frontMatter.featureSlug",
    "userStories[0].acceptanceCriteria"
  ],
  "warnings": [
    "US-002 is missing 'Typecheck passes' and will be auto-added on finalize."
  ]
}
```

规则：

- `missing` 由后端计算并返回，用于前端明确展示缺口（缺口不为空时禁止 Finalize 成功）。
- 后端在 `finalize` 时会补齐每条 story 的 `"Typecheck passes"`（与问卷模式一致）。
- 会话与槽位状态默认仅保存在内存，带 TTL（建议 2 小时）；MVP 不做持久化。

#### 10.8.1 会话资源上限与驱逐策略（v0.4 固化）

为避免内存不受控：

- 会话 TTL：默认 2 小时（无活动自动过期）
- 最大会话数：默认 `MAX_SESSIONS=50`
- 超过上限时的行为：拒绝创建新会话，返回错误：
  - `code=RESOURCE_CONFLICT`
  - `message="Too many active sessions"`
  - `hint="Finalize or delete an old session, or wait for TTL expiration."`
- 可选（实现时再定是否纳入）：提供 `DELETE /api/prd/chat/session?sessionId=` 用于主动释放（非 MVP 必需）

### 10.9 `POST /api/prd/chat/session`（v0.3 固化）

用途：创建自由对话会话，返回 `sessionId` 与初始 `slotState`。

请求：

```json
{
  "seed": {
    "project": "optional",
    "featureSlug": "optional",
    "title": "optional",
    "description": "optional"
  }
}
```

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "sessionId": "sess_20260205_162210_abcd",
    "slotState": { "missing": ["frontMatter.featureSlug", "frontMatter.title", "frontMatter.description"] }
  }
}
```

错误码：`VALIDATION_ERROR`、`INTERNAL_ERROR`

### 10.10 `POST /api/prd/chat/message`（v0.3 固化）

用途：发送一条用户消息。后端可选择接入 LLM 或使用规则引导，但必须返回结构化 `slotState` 以及可选的 `assistantReply`（用于 UI 展示）。

请求：

```json
{
  "sessionId": "sess_...",
  "message": {
    "role": "user",
    "text": "I want to add task statuses and filtering..."
  }
}
```

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "assistantReply": "Got it. What are the allowed statuses? A) pending/in_progress/done B) ...",
    "slotState": { "frontMatter": { "title": "..." }, "missing": ["userStories"] }
  }
}
```

错误码（MVP 最小集合）：

- `PRD_CHAT_SESSION_NOT_FOUND`
- `VALIDATION_ERROR`
- `INTERNAL_ERROR`

### 10.11 `GET /api/prd/chat/state?sessionId=`（v0.3 固化）

用途：读取当前 `slotState`（用于刷新/重连）。

响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "slotState": { "missing": ["..."] }
  }
}
```

错误码：`PRD_CHAT_SESSION_NOT_FOUND`

### 10.12 `POST /api/prd/chat/finalize`（v0.3 固化）

用途：对 `slotState` 进行强校验；缺口为空时生成并落盘 PRD 文件（第 4 节模板），返回路径与内容预览。

请求：

```json
{
  "sessionId": "sess_..."
}
```

成功响应：

```json
{
  "ok": true,
  "runId": "run_...",
  "data": {
    "path": "tasks/prd-task-status.md",
    "content": "# PRD: ...\n",
    "size": 1234,
    "slotState": { "missing": [] }
  }
}
```

失败响应（示例：缺必填字段）：

```json
{
  "ok": false,
  "error": {
    "code": "PRD_FINALIZE_MISSING_FIELDS",
    "message": "PRD is missing required fields",
    "hint": "Fill the missing fields or switch to questionnaire mode.",
    "missing": ["frontMatter.featureSlug", "userStories"]
  }
}
```

错误码：

- `PRD_CHAT_SESSION_NOT_FOUND`
- `PRD_FINALIZE_MISSING_FIELDS`
- `VALIDATION_ERROR`
- `INTERNAL_ERROR`

## 11. MVP 里程碑（按 PR/交付包拆分）

1) **PR-1 基础壳**
- Gin server + zerolog + embed 静态页 + SSE hub
- 启动自动打开浏览器；仅本机监听

2) **PR-2 Init**
- Go 实现 initservice（幂等复制 skills）

3) **PR-3 PRD（问卷模式）**
- 生成符合 `ohmyagentflow/prd@1` 的 `tasks/prd-*.md`（含预览与保存）

4) **PR-4 Convert（严格解析）**
- parser + convertservice；错误定位与 hint

5) **PR-5 Fire + Stop**
- 白名单执行 `ralph-codex.sh`；结构化日志；Stop 生效；并发拒绝

6) **PR-6 安全加固与自检**
- Origin/token 校验、路径白名单、输出限流、依赖自检提示

7) **PR-7 PRD（自由对话模式，可选）**
- 会话 + 槽位状态协议（第 10.8～10.12 节）
- Finalize 强校验与落盘 PRD

---

## 12. 验收标准（MVP-3 最小闭环）

- 单二进制启动后能打开页面并连上 SSE
- Init 能生成 `.codex/skills/*/SKILL.md`（幂等）
- PRD 问卷模式生成的 PRD 可被 Convert 解析通过
- Convert 可生成合法 `prd.json`，并确保每条 story 的 AC 含 `"Typecheck passes"`
- Fire 能执行 `ralph-codex.sh` 并实时显示 iteration 日志；Stop 能中断

---

## 13. v0.1 补充规格（为提升可实现性与稳定性）

本节固化评审中“扣分点”的落地选择，避免实现阶段返工。

### 13.1 Convert：多错误策略与覆盖写策略

- 多错误策略：MVP 采用**首错退出**（first-error wins），确保实现简单且错误定位明确；后续可升级为聚合多错。
- 覆盖写 `prd.json`：
  - 默认允许覆盖，但在写入前备份现有文件为：`prd.json.bak-<YYYYMMDD-HHMMSS>`
  - UI 提示“将覆盖并备份”，并展示备份文件名

### 13.2 PRD 模板边界（避免解析歧义）

为保证确定性解析，明确以下限制：

- front matter：
  - `description` 必须为单行字符串（不允许 YAML 多行 `|`），否则报 `PRD_PARSE_INVALID_FRONTMATTER`
- Story：
  - `**Description:**` 必须为单行（不允许换行续写）
  - `**Acceptance Criteria:**` 下只允许平铺 checkbox 列表；不允许嵌套列表/子项目
- Checkbox：
  - 必须严格以 `- [ ] ` 开头（不支持 `* [ ]`、不支持缩进 checkbox）

### 13.3 SSE/日志：前端展示与归档约定

- UI 默认仅展示“最近 N 条事件”（建议 `N=5000`），并可按 step/iteration 过滤。
- 前端必须按 8.3 的“虚拟列表 + 批量刷新 + seq 去重”约束实现，避免长跑卡顿。
- 完整日志（如启用归档）以 `.ohmyagentflow/runs/<runId>.jsonl` 保存，便于问题复现与后续多端回放。

### 13.4 Fire：stdout/stderr 读取策略（避免无换行卡住）

为避免子进程长输出但不换行导致 UI 不刷新：

- 读取采用“按块读取 + 按换行切分”，并设置“flush 计时器”（例如 200ms），即便没有换行也会把缓冲区内容作为一条 `process_stdout` 推送（可能被截断）。

### 13.5 最小测试计划（MVP 必做）

- `internal/prd`：
  - fixtures：`testdata/valid/*.md`、`testdata/invalid/*.md`
  - golden：解析 AST 与生成 `prd.json` 进行快照比对
- `internal/security`：
  - 路径越界（`..`、绝对路径、软链接）测试
  - exec 白名单参数校验测试
- `internal/events`：
  - hub 订阅/取消订阅、事件顺序、截断/保留 N 条策略测试
- `internal/services/fireservice`：
  - 用可注入的 Runner 接口做假进程（模拟 stdout/stderr/exit code），不依赖真实 `codex/claude`
- `internal/httpserver`：
  - handler 集成测试：Origin/token 拒绝、fs/read 白名单、convert 报错格式

自由对话模式（如纳入实现）额外测试：

- `internal/services/prdservice`：
  - 会话 TTL 过期行为
  - finalize 缺口计算与 `PRD_FINALIZE_MISSING_FIELDS` 返回
  - message 后 slotState 合法性（不要求 LLM，至少保证协议稳定）

### 13.7 PRD/输入约束上限（v0.2 固化）

为降低“意外超大输入/日志/文件”带来的稳定性风险，MVP 固化如下上限（超出返回 `VALIDATION_ERROR`）：

- `featureSlug`：必须为 kebab-case，正则：`^[a-z0-9]+(?:-[a-z0-9]+)*$`，长度 `3..64`
- `title`：长度 `1..120`
- `description`（front matter）：单行字符串，长度 `1..200`
- 列表项（Goals/FR/Non-Goals/Success Metrics/Open Questions）：单条长度 `1..200`，每类最多 `50` 条
- `userStories`：
  - 数量：`1..50`
  - `id`：必须为 `US-001` 起连续递增（由前端生成，后端强校验）
  - `story title`：长度 `1..120`
  - `**Description:**`：单行，长度 `1..200`
  - `acceptanceCriteria`：每条长度 `1..200`，每个 story 最多 `30` 条（后端确保包含 `"Typecheck passes"`）

---

## 14. v0.5 风险强约束（实现前置规避）

本节将剩余“实现质量风险”（安全边界条件、IO/子进程边界、资源治理）通过强约束与必过测试前置，降低踩坑概率。实现时必须遵守本节规定。

### 14.1 文件路径安全：`SafePath` 统一入口（强制）

风险：仅靠 `filepath.Clean` 与字符串前缀判断会被软链接逃逸（例如 `tasks/prd-x.md -> /etc/passwd`）绕过。

强约束：

- 任何文件读写（包含 `fs/read`、convert 读取 PRD、写入 `prd.json`、归档写入等）必须通过统一组件 `SafePath` 完成。
- 禁止在 handler/service 中直接调用 `os.Open`/`os.ReadFile`/`os.WriteFile` 对用户输入路径进行操作。

`SafePath` 校验规则（MVP 必做）：

1. 将项目根目录 `root` 做 `EvalSymlinks` 得到 `rootReal`
2. 目标路径 `p` 必须为相对路径；计算 `abs = Join(root, p)` 并 `Clean`
3. 对 `abs` 做 `EvalSymlinks` 得到 `absReal`
   - 若目标文件不存在：对其父目录做 `EvalSymlinks`，并在该父目录下创建/读取
4. 仅当 `absReal` 以 `rootReal + string(os.PathSeparator)` 为前缀时才允许
5. 读取类操作额外要求：目标必须是常规文件（`Mode().IsRegular()`）

### 14.2 子进程执行：`ExecPolicy` 白名单与无拼接（强制）

强约束：

- 禁止使用 `sh -c` 或字符串拼接执行命令。
- 必须使用 `exec.CommandContext` 且以参数数组传递。
- Fire 仅允许：
  - 可执行：`bash`
  - 脚本：`<root>/ralph-codex.sh`（必须为常规文件且非 symlink）
  - 参数：`--tool <codex|claude> <maxIterations>`
- 工作目录：固定为服务启动时的项目根目录 `cwd`，不得被请求覆盖。
- 并发：同一时间最多 1 个 fire run。
- 进程组（Stop 必须可靠）：
  - 启动 Fire 时必须创建独立进程组（例如设置子进程 `Setpgid=true` 或等价机制），并将 PGID 记录在 run 状态中。
  - Stop 时必须对 PGID 发信号（见 7.5），确保子孙进程一并终止。
- 脚本文件校验（MVP 必做）：
  - 使用 `SafePath` 获取脚本绝对路径后，对其 `Lstat`：若为 symlink 或非 regular file 必须拒绝（返回 `VALIDATION_ERROR` 或 `NOT_FOUND`，并提示“ralph-codex.sh must be a regular file under project root”）。

### 14.3 子进程输出读取：按块读取 + flush（强制）

风险：无换行输出会导致 UI 长时间不更新；`bufio.Scanner` 默认 token 限制会截断。

强约束：

- 禁止使用默认 `bufio.Scanner` 直接读取 stdout/stderr。
- 必须实现“按块读取 + 换行切分 + flush 计时器（例如 200ms）”：
  - 即使没有换行，也要周期性将缓冲内容作为 `process_stdout/stderr` 事件推送（可能截断）。
- 单条事件 `data.text` 最大 8KB；超出拆分或截断并标记 `truncated=true`。

### 14.4 Origin/Token：浏览器调用默认安全（强制）

强约束：

- 所有 `POST /api/*` 必须同时满足：
  - `Origin` 存在且精确匹配 `http://127.0.0.1:<port>` 或 `http://localhost:<port>`
  - `X-Session-Token` 正确
- Token 生命周期：随进程生命周期；服务重启 token 立即失效。
- 不提供“无 Origin 放行”默认路径；若未来需要 CLI 调用，必须通过显式不安全开关启用（MVP 不提供）。

### 14.5 归档并发与清理：原子写入与跳过当前 run（强制）

强约束：

- 归档写入采用临时文件（`.tmp`）流式追加：运行中写入 `.ohmyagentflow/runs/<runId>.jsonl.tmp`，run 结束后执行一次原子 rename 为 `.jsonl`（或等价机制），确保“完成态归档文件”要么完整存在要么不存在。
- 清理逻辑必须：
  - 跳过当前 run 的归档文件（以及 `.tmp`）
  - 仅删除已完成 run 的归档
  - 对归档目录操作做互斥串行化（同一进程内，强制）
- 归档大小上限强制执行（建议 50MB）：
  - 超过上限：停止写入归档文件（但 SSE/运行不受影响），并发出一次 `error` 事件（建议 `code="ARCHIVE_TOO_LARGE"`，message 提示“archive stopped after reaching size limit”）。

#### 14.5.1 归档互斥与清理降频（v0.2 固化）

- 互斥范围：
  - 同一进程内对 `.ohmyagentflow/runs/` 的所有操作（创建目录、写入 `.tmp`、rename、扫描大小、删除旧文件）必须由同一把互斥锁串行化，避免竞态删除/大小统计不一致。
- 清理降频（避免每次写入都全量扫目录）：
  - `run_started`（开始归档前）强制执行一次清理检查。
  - 运行中最多每 `5s` 执行一次“目录大小/数量”检查；其余写入仅追加不检查。
  - 目录大小统计可实现为：
    - 低成本近似（优先）：维护当前 run 归档已写入字节数 + 定期（每 5s）扫描目录汇总一次；
    - 或直接扫描（实现简单但成本更高）：每 5s 扫描一次即可，禁止按每条事件扫描。
- 清理目标：
  - 先按数量上限 `K=50` 删除最旧 `.jsonl`（不删除 `.tmp` 与当前 run）。
  - 再按目录总大小上限 `1GB` 继续从旧到新删除，直到满足约束。

### 14.6 必过测试用例（作为验收的一部分）

以下用例必须在自动化测试中覆盖（未通过则不满足 MVP 质量门槛）：

1. `fs/read` 读取 `tasks/prd-x.md`（其为 symlink 指向根目录外）必须失败：`FS_READ_NOT_ALLOWED`
2. `convert` 指定 `prdPath=../x` 必须失败：`FS_READ_NOT_ALLOWED`
3. `fire` 在已有运行中的情况下再次触发返回：`RESOURCE_CONFLICT`
4. 子进程 stdout 连续 5 秒无换行输出，UI/SSE 在 1 秒内至少收到 1 条 `process_stdout`（flush 生效）
5. 单条输出超过 8KB 必须截断并标记 `truncated=true`
6. 归档目录超过 1GB 触发清理，且不删除当前 run 的归档文件
7. `POST /api/*` 缺 `Origin` 或 `X-Session-Token` 必须拒绝（`AUTH_ORIGIN_NOT_ALLOWED` / `AUTH_MISSING_TOKEN`）


### 13.6 自由对话模式：收敛接口契约（先固化接口，不强行上 LLM）

自由对话模式是否接入 LLM 可后置，但必须先固化“收敛到模板”的接口：

- 后端暴露一个“槽位状态”对象（Front Matter + stories 草稿），前端展示缺口
- `Finalize PRD` 时强校验模板必填项；缺口必须给出可执行提示，并提供“一键切换到问卷补齐”
