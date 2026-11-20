package agent

import (
	"context"
	"fmt"

	"antifox/engine/agentcore"
	"antifox/engine/llm"
	"antifox/engine/permission"
	actool "antifox/engine/tool"
	"antifox/engine/transcript"
	"antifox/db"
)

// MainAgent is the thin human-interface orchestrator (docs §4.2 / §7). The human
// chats with it; it observes (read tools), and steers by injecting hints
// (→planner) or direct high-priority intents (→frontier). It does NOT run the
// autonomous intent-generation loop (that is the planner's job).
type MainAgent struct {
	prov        llm.Provider
	model       string
	tx          *transcript.Store // raw LLM conversation persistence (nil = off)
	window      int               // context window in tokens (for compaction)
	maxTurns    int               // max agent turns per run (0 = unlimited)
	proxyAddr   string            // recording proxy for WebFetch (empty = direct)
	proxyCACert string            // recording proxy's CA cert path (HTTPS verify)
	webSearch   WebSearchOpts     // web_search tool backend selection (off by default)
	workDir     string            // shared work dir (surfaced in prompt as artifact-output target)
}

func NewMainAgent(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *MainAgent {
	return &MainAgent{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns}
}

// SetProxy points the main agent's WebFetch at the recording proxy plus the CA
// cert it trusts to verify HTTPS through it (empty addr = direct).
func (m *MainAgent) SetProxy(addr, caCert string) { m.proxyAddr, m.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the main agent (off by default).
func (m *MainAgent) SetWebSearch(o WebSearchOpts) { m.webSearch = o }

// mainAgentDefaultTmpl is the built-in EDITABLE body (段 [A]) of the main agent
// prompt, seeded into agent_prompts. Goal is a {{.Goal}} template var; the 中间
// 产物输出规约 tail is code-owned (artifactSpec), appended after rendering.
const mainAgentDefaultTmpl = `You are the "main agent" of an authorised penetration-testing system, the interface to the human operator. You do not explore yourself, nor do you generate intents continuously on your own (that is the planner's job). Your responsibilities:

1. Observe: use graph_overview / list_findings / list_facts / list_assets / get_worker_output to answer the human's questions about current progress.
2. Record vulnerabilities (important, do not miss any): for every vulnerability confirmed by a worker or re-checked and confirmed by you, **call report_finding immediately, one at a time** — one call per vulnerability, filling vulnclass (vulnerability name/type), severity (critical|high|medium|low; critical = can directly lead to full compromise of the system/data, such as RCE, unauthenticated arbitrary file read yielding keys/credentials, authentication bypass allowing account takeover, etc.), summary (including the affected target), evidence (reproducible requests/commands/responses, the real PoC verbatim), and asset_ids when the affected assets have ids; **and fill in the structured reproduction whenever possible** (entry_point / prerequisites including how to register a test account / steps with the real request + expected + proof for each / provenance) — requests taken verbatim from the worker transcript / actual execution, not rewritten, so the report can be reproduced from scratch by a human. **Confirm and record — do not ask permission, do not merely list vulnerabilities in your reply** — only vulnerabilities recorded with report_finding enter the findings page / exploration graph / report; one written in a reply but not recorded is wasted. Check what is already recorded with list_findings first to avoid duplicates. Never report as a confirmed vulnerability something that is mere speculation based on version/CVE matching or "looks exploitable". **Exposure / information-disclosure issues are findings too**: an unauthenticated-reachable sensitive surface confirmed by a worker (admin/debug endpoints, Swagger/API docs, public configuration or backup/source code, sensitive-data-leaking endpoints, etc., with actual response evidence) is likewise reported one at a time with report_finding (severity by sensitivity: ordinary information disclosure = low, unauthenticated-reachable sensitive/admin endpoints = medium, direct takeover or credential retrieval = high); do not treat them as mere assets/facts. **When the user asks you to "review the current findings", or at task wrap-up/pause**: store every qualifying vulnerability and confirmed exposure already written back by workers into the graph, one at a time (check what exists with list_findings/list_facts first, de-duplicate) — even on a hard target where exploitation did not break through, hand over these confirmed exposures/risks instead of just listing them in a reply.
3. Steer (turn the human's intent into system actions):
   - The human wants to "change direction / emphasise a vulnerability class / focus on an area" → write a hint with add_hint (the planner reads it next time).
   - The human wants to "test a specific target right now" → inject a high-priority intent directly with add_intent (priority 8-10).
4. Reply in plain language, concisely, stating what you did and which vulnerabilities you recorded.

Current task goal: {{.Goal}}

Do not fabricate findings; record and answer only from the real data returned by tools and from verified evidence.`

func mainAgentSystem(goal, workDir string) string {
	body := renderSystem("mainagent", mainAgentDefaultTmpl, MainVars{Goal: goal})
	return body + artifactSpec(workDir)
}

// Chat handles one human message and returns the assistant reply. emit, if
// non-nil, receives each execution step (thinking / tool_use / tool_result /
// text / result) so the main-agent session shows its work — exactly like the
// worker/planner sessions — not just the final answer.
func (m *MainAgent) Chat(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal, message string, emit func(db.Activity), notify func()) (string, error) {
	tsx := NewToolSet(ts, "human")
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetNotify(notify) // add_hint wakes this task's planner (debounced)
	// 领域工具 + 基础默认工具集（Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash）
	base := append(tsx.MainAgentTools(), actool.DefaultTools()...)
	tools, def, cleanup := AugmentTools(ctx, "mainagent", base)
	defer cleanup()
	system, boundary := deferredSystem(mainAgentSystem(goal, m.workDir), def)
	opts := agentcore.Options{
		Provider:        m.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // 走记录代理留痕；载入代理 CA 验证 MITM 重签的 HTTPS 证书
		WebFetchProxy:   m.proxyAddr,
		WebFetchCACert:  m.proxyCACert,
		// 联网搜索(可选)。ddgs 无需 key；brave-free 需 BraveKey；tavily 需 TavilyKey。
		// WebSearchProxy 是独立出口代理(http/https/socks5)，与记录流量的 MITM 代理无关；空则直连。
		EnableWebSearch:    m.webSearch.Enabled,
		WebSearchBackend:   m.webSearch.Backend,
		BraveSearchAPIKey:  m.webSearch.BraveKey,
		TavilySearchAPIKey: m.webSearch.TavilyKey,
		WebSearchProxy:     m.webSearch.Proxy,
		BashEnv:           proxyEnv(m.proxyAddr, m.proxyCACert), // Bash 子命令默认走代理+信任 CA
		MaxTurns:          m.maxTurns,                           // 0 = unlimited (configurable in agent management)
		Compaction:        compactionConfig(m.window),           // long chats stay within the window
		Todos:             actool.NewTodoStore(),                // 会话级临时待办（TodoWrite），纯规划用，退出即丢
		// 命中预算(步数)→ SDK 跑收尾:向用户输出一句进展总结。Prompt 与收尾轮数可后台编辑(默认 5 轮)。
		Settlement: wrapupSettlement("mainagent", nil),
	}
	if m.tx != nil { // persist raw human↔AI conversation; one accumulating file per task
		opts.Transcript = m.tx
		opts.SessionID = fmt.Sprintf("exp%d-main", ts.ID())
	}
	s := agentcore.NewSession(opts)
	defer s.Close()
	// reload the prior conversation from the transcript so the agent has context
	// across turns (each Chat is a fresh session; without this it can't see earlier
	// messages). First turn: no file yet → Resume loads nothing and proceeds.
	if m.tx != nil {
		_ = s.Resume(opts.SessionID)
	}
	// C2: this session is fresh each turn; re-unlock skill-gated MCPs from prior
	// Skill() calls in the reloaded history so revealed tools stay callable.
	seedUnlockFromHistory(s.Messages(), def.UnlockSkill)
	text, _, err := captureRunSession(ctx, s, message, func(r db.Activity) {
		if emit != nil {
			r.Worker = "mainagent"
			emit(r)
		}
	})
	return text, err
}
