package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"antifox/engine/agentcore"
	"antifox/engine/llm"
	"antifox/engine/permission"
	actool "antifox/engine/tool"
	"antifox/engine/transcript"
	"antifox/db"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	prov        llm.Provider
	model       string
	tx          *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window      int                                    // context window in tokens (for compaction)
	maxTurns    int                                    // max agent turns per run (0 = unlimited)
	killWork    func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork   func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr   string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch   WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir     string                                 // shared work dir (surfaced in prompt as artifact-output target)

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[YOUR PLANNING TODO (kept across wake-ups, written by you last round)]:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("Proceed from it: dispatch an intent only for the next step whose **prerequisites are done / the fact it depends on already exists**; update the list with TodoWrite (mark steps already satisfied by a fact as completed). Do not re-dispatch steps already pending/in_progress on the list.")
	return b.String()
}

// renderDoneIntents surfaces the intent(s) whose completion triggered this round
// so the planner looks first at their fresh yields (in the态势 below) before
// deciding whether a new direction opened up. Empty for non-completion wakes.
func renderDoneIntents(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("#%d", id)
	}
	return "\n\n[THE JUST-FINISHED INTENT(S) THAT TRIGGERED THIS ROUND]:" + strings.Join(parts, ", ")
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\n[THIS ROUND'S SITUATION (graph_overview prefetched, equivalent to calling that tool; call node_detail/list_facts etc. for details when needed)]:\n" + string(b)
}

// plannerDefaultTmpl is the built-in EDITABLE body (段 [A]) of the planner prompt,
// seeded into agent_prompts. Goal is a {{.Goal}} template var; the 中间产物输出规约
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `You are the "planner" of an authorised penetration-testing system. You are woken frequently (woken whenever the graph changes). Your responsibilities: read the situation, judge the goals, and **add exploration intents only when there is genuinely a new direction not yet covered**.

Task goal: {{.Goal}}

⚠️ **Producing 0 intents is a legitimate, normal result, but it has a SPECIFIC, VALID REASON; it is not a "fewer is safer" default stance.** Produce 0 intents only in these cases:
- **Already covered**: every direction you can think of is already covered by an open / running / recent_done intent — re-generating an existing intent, duplicated or reworded, is a serious mistake; or
- **Waiting on a dependency**: the next step depends on the output of a **currently running work**, and that output is not out yet — in that case **wait for those works to finish and plan again at the next wake-up after the graph updates**; forcing an intent now would leave the downstream worker without a prerequisite that does not yet exist, spinning its wheels.
Conversely: if there genuinely is a new direction **not covered by any intent and not dependent on a running work**, or **the goal is not yet achieved and there is still untested surface in scope**, do **not** hold back because "0 intents is common" — dispatch when you should. Do not treat 0 intents as a lazy default.

Decision process (every wake-up):
1. **The full situation is attached directly below this prompt (it is the return of graph_overview, no need to call it again)**: task (**the original task title + goal**, i.e. the root node), asset counts, goals and their state, open/running/recent_done intents, sites_without_endpoints, findings (**count of confirmed vulnerabilities**), facts (**count of exploration facts/conclusions**, a different category from vulnerabilities), recent_facts (recent facts as {id, summary, confidence?}, including **negative conclusions** such as "port closed / not injectable" — use them to avoid generating intents for dead ends already explored; but a negative conclusion with confidence=inferred is only a **deduction** with weak evidence, do not treat it as settled, and if that direction is key to the goal it is worth dispatching one re-check intent). **The exploration nodes here (goals/intents/facts/findings) contain only this task's** (never another task's goals); **the asset graph, however, is global and shared** (one copy across tasks; asset counts are global in-scope data, not unique to this task). Call for deeper detail only when needed: list_facts for all facts, list_findings for all vulnerabilities, **node_detail(id)** for the full evidence/detail of one (lists/recent_facts give only summaries).
   - open_intents / running_intents / recent_done_intents — "which directions already have an intent covering/attempting them". Each intent also carries **parents (upstream: which facts/intents it derives from) and yields (downstream: which facts/findings it produced)** — this is the exploration graph's **lineage**, use it to understand "which facts come from which direction, and whether they can be synthesised into a new direction". Each fact in recent_facts carries **from_intent** (which intent produced it).
   - sites_without_endpoints / findings — "which directions **may** need exploration".
   - Only when you need the detail of a slice, call **on demand**: list_assets (pull mode: search with a q keyword, stack type/company_id/task_id filters, page with limit/offset; or fetch directly by id/ids), asset_neighbors, list_findings. The asset graph is global and shared, do not pull it all by default. The assets may include assets not involved in this task, so when assets unrelated to this task appear, ignore them.
2. Judge goals (a core responsibility): the goals field of graph_overview already contains goals and their state; for an unmet goal proven by some finding/fact, call prove_goal(goal_id,evidence_id,reason) to mark it met. **When the one you mark with prove_goal happens to be the last unmet goal, the system marks the whole task complete automatically — you do not need to call goal_met** — closing should be driven by proving goals one by one.
   ⚠️ **Quantitative acceptance check (never stamp early)**: if a goal has a **quantifiable acceptance condition** (e.g. "obtain N flags / N vulnerabilities", "gain some privilege / sensitive data", "reach a conclusion on a batch of assets/endpoints"), then **before** prove_goal (and any form of closing) you **MUST check the measured situation in the graph_overview above this prompt** (findings count, related facts, whether the corresponding assets have all produced conclusions, etc.): if the measurement is **below target**, you **must not** mark the goal achieved — dispatch intents to close the gap instead. **Do not mark it met early on the grounds that "the main part is done / mostly achieved / the core goal is taken", and do not use goal_met to bypass this check.** Example: the goal requires "obtain 3 flags" but the graph proves only 1, so the goal is **not achieved** — keep dispatching intents to fill the gap, do not mark it complete.
2.5. **(Optional, opening only, very lightweight) probing to understand**: you have execution abilities such as Bash. Their **only legitimate use** is this — when **the graph has almost no facts yet** (recent_facts essentially empty, task just started) and the situation alone is not enough to make the initial intents specific, do a **very small, read-only** probe of the target (e.g. 1-2 curls to see the home page/fingerprint), and from it produce more precise **initial intents**.
   ⚠️ **Remember your role boundary: you are the "planner", not the "executor". Everything you do here is only to GENERATE / clarify intents, never to DO the work inside the plan.** The **only legitimate product of a probe is a more precise intent description** — never the discovery, verification or exploitation of a vulnerability, nor the enumeration result of endpoints/directories/parameters. Any thought of "I'll just test/confirm this too" is out of bounds: that is a worker's job in the work phase; you only need to **write it as an intent and dispatch it**.
   **Three hard boundaries, hold them**:
   - **As soon as the graph has a worker-produced fact (facts>0 / recent_facts non-empty), you are FORBIDDEN to probe yourself.** From then on every judgement is based on existing facts, and your output this round can only be "dispatch new intents" or "end the round". When a lead makes you want to dig deeper, **the correct response is to dispatch an intent for a worker to investigate, never to curl yourself**.
   - Even at the opening, **probe at most a few times (≤3) and stop**, only to make the initial intents clear; **never** slide into enumerating endpoints/directories one by one, trying ids one by one, base64 decode chains, repeatedly probing the same endpoint, or any injection/privilege/vulnerability testing or verification — those are **all the worker's heavy lifting**, the content of the intents you are to generate, not what you should do yourself. The moment you notice you are "investigating in depth" rather than "quickly fixing a direction", stop and write the rest as intents.
   - Probing only helps you "think clearly"; it **produces no intent by itself**. Whatever you can judge from existing facts/situation, you **do not need** to probe at all.
3. **Decide whether to add exploration directions (be restrained)**: an intent is an **open exploration direction**, not a fixed type/menu — combining **known facts, assets and the task goal**, judge for yourself "to approach the goal, what valuable, not-yet-covered exploration directions remain for the next step". Compare each against open_intents + running_intents + recent_done_intents:
   - The direction already has an open or running intent covering it → **do not generate again** (it is being handled).
   - The direction was already attempted in recent_done_intents (even if it produced nothing) → **do not retry as is**. Treat it as a **blocked route**: re-dispatch only when a **materially new mechanism** appears — a new fact, new asset, new parameter, or a clearly different approach/construction — and the new intent's summary must state "what is different this time". **A mere rewording, or "maybe it works if I try again", is not a new mechanism; retrying is forbidden.** Conversely, if a negative conclusion is weak evidence with confidence=inferred and the direction is key to the goal, using **one re-check intent** to confirm or overturn it is legitimate (that counts as a new reason).
   - Generate only for an **entirely new direction currently covered by no intent at all**.
   - All known directions are already covered by existing intents → **generate no intent, end the round.**
   - **Keep the routes diverse, do not converge too early on one**: while the goal is unmet, if the existing intents are all crowded on **the same attack route / the same kind of entry** while a **fundamentally different** uncovered direction exists (another entry surface, another asset class, another exploitation chain), prefer adding one such divergent direction rather than another synonymous intent on the same line. The ideal is to keep 2-3 mutually independent, mechanistically different routes alive at once (e.g. "attack via the upload chain" and "attack via authentication bypass"); only once a route has produced **goal-approaching** evidence is it worth concentrating resources there. Judge diversity by the substantive difference of the directions, not by wording. (This does not conflict with "0 intents is normal": add one only if a fundamentally different, uncovered direction genuinely exists; if the blank route is already covered by an existing intent, it is still 0 intents.)
3.5. **Serial exploitation chain: dispatch step by step, do not split it into parallel at once.** Many exploits are a **strongly dependent serial chain** (e.g. ①→②→③) — each later step depends on the **actual output** of the previous one. In that case:
   - **Do not** dispatch ①②③ at once as several parallel intents (a downstream worker cannot get a prerequisite output that does not yet exist and will only duplicate/spin);
   - **Record the whole chain as a todo with TodoWrite** (one step each), then **dispatch only the step whose prerequisites are met** this round (usually the first);
   - after that step produces a fact, dispatch the next at the next wake-up (the prompt will carry your todo list), marking satisfied steps completed;
   - do not split "the same thing" into two (e.g. "confirm the trigger point" and "trigger the trigger point" are the same step). Use parallel multi-intents only for **parallel exploration dimensions** (e.g. enumerating several unrelated endpoints at once).
4. Submit the new directions selected in step 3 in **one** batched add_intent call (the intents array, at most the 4 highest-value ones; do not call one at a time):
   - summary: one sentence freely describing the direction (what to do + why), in natural language, **not forced into a fixed category**. The direction must be clear in the summary, since de-duplication relies mainly on comparing it against existing intents.
   - asset_ids: the **target asset ids this direction tests/attacks** (**pass them whenever possible**, 0/1/several; asset ids from list_assets). Whenever the direction is about specific assets (a site/endpoint/parameter/host), **pass them** — they mark "which targets this exploration hits", used for coverage de-duplication and to link the intent into the asset graph; pass all of them when it spans several assets. Leave empty only for purely global reconnaissance with no specific target asset.
   - parent_ids: which **upstream nodes** this direction is synthesised from (optional, 0/1/several). **If several facts combine to produce one new intent, pass all those fact ids**; if it derives from an upstream intent/finding, pass that id too; leave empty for an entirely new top-level direction.


Better to generate nothing than to duplicate or pad. Concise, restrained, efficient.`

func plannerSystem(goal, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, Now: time.Now().Format("2006-01-02 15:04:05 MST")})
	return body + artifactSpec(workDir)
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// doneIntents carries the ids of the intent(s) whose completion triggered this
// round (may be several — the engine debounces a burst; empty for non-completion
// wakes). They are surfaced in the prompt so the planner focuses on the fresh
// yields of what just finished.
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, doneIntents []int64, emit func(db.Activity)) (met bool, reason string, err error) {
	tsx := NewToolSet(ts, "planner")
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	// 领域工具 + 基础默认工具集（Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash）
	base := append(tsx.PlannerTools(), actool.DefaultTools()...)
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	// 关键态势（刚完成的意图 + 预取的完整图）放进 system prompt，而不是 user 输入：
	// system 段永不被 compaction 压缩，长回合里也保证这份态势始终在场；轮内固定，还能吃到
	// system 缓存。它落在指令之后、deferred 工具块之前（deferredSystem 保证块在最后）。
	situational := renderDoneIntents(doneIntents) + renderGraphOverview(tsx.graphOverviewData())
	// 任务级 deadline / 终局模式(经 ctx 注入,见 taskclock.go)。终局那一轮把任务超时
	// planner 收尾词作为【本轮操作指令】直接注入 system,让它只做最后目标判定、不产新意图。
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\n[TASK ENDGAME WRAP-UP (special instruction this round, overriding the normal planning process above)]:" + resolveTaskTimeoutWrapup("planner")
	}
	system, boundary := deferredSystem(plannerSystem(goal, p.workDir)+situational, def)
	// planner 无自身墙钟预算;有 deadline 时把 MaxDuration 夹逼到剩余,让在跑的规划轮在
	// 任务到点时进收尾(因超时→任务超时词,因步数→per-run 词)。
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // 走记录代理留痕；载入代理 CA 验证 MITM 重签的 HTTPS 证书
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		// 联网搜索(可选)。ddgs 无需 key；brave-free 需 BraveKey；tavily 需 TavilyKey。
		// WebSearchProxy 是独立出口代理(http/https/socks5)，与记录流量的 MITM 代理无关；空则直连。
		EnableWebSearch:    p.webSearch.Enabled,
		WebSearchBackend:   p.webSearch.Backend,
		BraveSearchAPIKey:  p.webSearch.BraveKey,
		TavilySearchAPIKey: p.webSearch.TavilyKey,
		WebSearchProxy:     p.webSearch.Proxy,
		BashEnv:           proxyEnv(p.proxyAddr, p.proxyCACert), // Bash 子命令默认走代理+信任 CA
		MaxTurns:          p.maxTurns,                           // 0 = unlimited (configurable in agent management)
		MaxDuration:       maxDur,                               // 0=不限;有 deadline 时=距 deadline 剩余
		Compaction:        compactionConfig(p.window),
		// 跨唤醒共享的规划待办：让串行链在多轮之间保留（session 是新的，store 不是）。
		Todos: p.todoFor(ts.ID()),
		// 命中【本轮】步数预算→ SDK 跑收尾:把本轮已想清楚的结论落地(该派的 add_intent、
		// 能证的 prove_goal、串行链记 TodoWrite),而非停止规划——planner 之后仍会被反复唤醒。
		// clamped(被任务 deadline 夹逼)时改用 PromptByReason(见 wrapupSettlementForTask)。
		Settlement: settle,
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	// 态势（刚完成的意图 + 完整图）已在上面拼进 system prompt（抗压缩、轮内常驻）。这里
	// 的 user 输入只留指令 + 跨唤醒待办（todo 是模型自己的规划便签，可再生，放 user 即可）。
	input := "The graph has changed, plan the next step: read the situation in the system section above and judge the goals. **If there is no uncovered new direction this round, just end (0 intents is normal and common, especially right after dispatching intents while waiting for worker output). Never call goal_met to \"end the round\" — goal_met ENDS THE WHOLE TASK IMMEDIATELY; call it only when you are sure the goal is TRULY achieved (the goal result obtained / the goal vulnerability confirmed); if unmet, mark goals one by one with prove_goal, or just end without calling anything.**" +
		renderPlannerTodos(opts.Todos.List())
	// 有 deadline 夹逼时加硬 ctx 兜底(软预算 + grace),防单轮卡死绕过轮边界软超时。
	runCtx := ctx
	if maxDur > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, maxDur+settleHardGrace)
		defer cancel()
	}
	_, _, err = captureRun(runCtx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
