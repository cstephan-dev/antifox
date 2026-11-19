package report

// AI narrative layer: turns thin findings ({vulnclass, severity, summary, poc})
// into professional Chinese report prose (description / test process / risk /
// remediation + CVSS) via the platform's active LLM profile. Report-level and
// per-finding calls run concurrently. All AI output is JSON; parse failures fall
// back to the raw finding fields (assembly in build.go), so generation never
// hard-fails on a bad LLM response.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"antifox/agent"
	"antifox/db"
	"antifox/engine/agentcore"
	acperm "antifox/engine/permission"
	actool "antifox/engine/tool"
)

// complete runs one system+user → text completion on the active LLM profile.
// Returns an error when no profile is configured (caller falls back to skeleton).
func complete(ctx context.Context, pg *db.DB, system, user string, maxTokens int) (string, error) {
	// 报告/富化角色可绑定到指定 LLM(角色分工)——未绑定则用全局激活。让富化能走更强的模型
	// (如 Claude)而不动渗透 agent 的模型。
	p, err := pg.ReportRoleProfile()
	if err != nil || p == nil || strings.TrimSpace(p.APIKey) == "" {
		return "", fmt.Errorf("no active LLM profile")
	}
	cfg := agent.ConfigFrom(p.Format, p.Model, p.BaseURL, p.APIKey, p.Proxy)
	cfg.RatePerSecond, cfg.RatePerMinute = p.RatePerSecond, p.RatePerMinute
	cfg.ReasoningEffort = p.ReasoningEffort
	prov, err := cfg.NewProvider()
	if err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	out, err := agentcore.Run(cctx, agentcore.Options{
		Provider:       prov,
		SystemPrompt:   []string{system},
		Tools:          []actool.CoreTool{}, // non-nil empty → no default tools offered
		PermissionMode: acperm.ModeBypass,
		MaxTurns:       1,
		MaxTokens:      maxTokens,
	}, user)
	if err != nil {
		return "", err
	}
	return cleanJSON(out), nil
}

// ---------------------------------------------------------------------------
// Report-level narrative (background / service result / remediation / checklist)
// ---------------------------------------------------------------------------

type reportLevelOut struct {
	Background            []string `json:"background"`
	ServiceResult         string   `json:"service_result"`
	RemediationConclusion string   `json:"remediation_conclusion"`
	ChecklistHits         []string `json:"checklist_hits"`
}

const sysReportLevel = "You are a senior penetration test report writer for %s, skilled at producing professional, client-readable, technically rigorous penetration test reports. Output strictly JSON only — no markdown code fences."

const userReportLevel = `Write the report overview content based on the project and findings list below.

Project name: %s
Client name: %s
Findings list:
%s
Requirements (professional delivery tone; avoid wording such as "this test was performed today"):
- background: array of strings, 2 paragraphs. The first explains that the client engaged %s to perform a penetration test and states the test objectives; the second describes the test scope and the combined manual + tool-assisted approach. Each paragraph 40-90 words.
- service_result: 1 paragraph summarizing which systems were tested and the number and severity distribution of the findings.
- remediation_conclusion: 1 paragraph remediation conclusion (for a first-round penetration test, all findings are pending remediation by default; recommend prompt remediation followed by retesting).
- checklist_hits: array of strings — infer the standard test items hit, based on the vulnerability types (standard names such as "SQL Injection", "Unauthorized Access", "Authentication Bypass", "Sensitive Information Disclosure", "Command Injection", "Cross-Site Scripting (XSS)", "Vertical Privilege Escalation"). Use the exact standard test-item names in the output language specified below.

Output JSON only: {"background":["...","..."],"service_result":"...","remediation_conclusion":"...","checklist_hits":["...","..."]}`

// reportLevelPromptData feeds the editable report-level prompt template.
type reportLevelPromptData struct {
	Company, Project, Client, FindingsList, LangDirective string
}

// vulnPromptData feeds the editable per-vuln prompt template.
type vulnPromptData struct {
	Company, VulnClass, Severity, Summary, PoC, LangDirective string
}

func aiReportLevel(ctx context.Context, pg *db.DB, in *GenInput, sk *skillContent) reportLevelOut {
	var out reportLevelOut
	loc := normLocale(in.Locale)
	var sb strings.Builder
	// 把完整测试范围/目标系统作为上下文喂给叙述(经 FindingsList 通道，skill/兜底两条 prompt
	// 路径都会带上)，这样综述能点名具体系统——同时封面项目名保持短(ProjectName)不受影响。
	if sc := strings.TrimSpace(in.Scope); sc != "" {
		fmt.Fprintf(&sb, "[Test scope / target systems] %s\n\n[Findings list]\n", truncateRune(sc, 1500))
	}
	for _, f := range in.Findings {
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", severityName(loc, f.Severity), nz(f.VulnClass, "-"), truncateRune(f.Summary, 80))
	}
	dir := outputLangDirective(loc)
	data := reportLevelPromptData{Company: in.CompanyName, Project: in.ProjectName, Client: in.ClientName, FindingsList: sb.String(), LangDirective: dir}
	sys := renderPrompt(sk.reportLevelSys, data)
	if sys == "" {
		sys = fmt.Sprintf(sysReportLevel, in.CompanyName)
	}
	user := renderPrompt(sk.reportLevelUser, data)
	if user == "" {
		user = fmt.Sprintf(userReportLevel, in.ProjectName, in.ClientName, sb.String(), in.CompanyName) +
			"\n\nOutput language requirement: " + dir
	}
	raw, err := complete(ctx, pg, sys, user, 4000)
	if err != nil {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// ---------------------------------------------------------------------------
// Per-finding narrative (description / test process / risk / remediation / CVSS)
// ---------------------------------------------------------------------------

type Enrichment struct {
	Title        string            `json:"title"`  // concise professional vuln name
	Target       string            `json:"target"` // affected host / URL, extracted from evidence
	Description  []string          `json:"description"`
	Module       string            `json:"module"`
	TestProcess  []procBlock       `json:"test_process"`
	RiskAnalysis []string          `json:"risk_analysis"`
	Remediation  []string          `json:"remediation"`
	CVSSScore    float64           `json:"cvss_score"`
	CVSSVector   string            `json:"cvss_vector"`
	CVSSMetrics  map[string]string `json:"cvss_metrics"`
}

const sysVuln = "You are a senior penetration test report writer for %s. Rewrite the raw vulnerability record into **delivery-grade** report text — as if a senior consultant wrote it personally for the client: concise, professional, and strictly to the point. Output strictly JSON only — no markdown code fences, no pleasantries, no disclaimers, and no boilerplate. Original HTTP requests / commands / responses MUST be preserved verbatim and NEVER fabricated."

const userVuln = `Rewrite the vulnerability below as report body text. Core requirement: **concise, specific, restrained** — quality over quantity; do not pad the item count, do not add filler, do not state correct-but-empty truisms; every sentence must carry real information about this specific vulnerability on this specific target.

Vulnerability type / original name: %s
Severity: %s
Original summary:
%s
Original reproduction steps / evidence:
%s

Field requirements:
- title: a standardized, professional vulnerability name (roughly 4-14 words). If the original name is already standardized (e.g. contains a CVE), keep it or adjust it slightly; do not copy the summary verbatim.
- target: the affected host or URL, extracted from the summary/evidence; use an empty string if it cannot be determined.
- description: 1 paragraph of 2-3 sentences, stating only what the vulnerability is, where it occurs, and its root cause. Do not pile on OWASP categories, do not restate the remediation, no vague preamble.
- module: the affected functional module (short, e.g. "External account login / unauthenticated API").
- risk_analysis: 2-4 items, one sentence each; each MUST be a concrete consequence of this vulnerability on this target (who can do what, and which data or privileges they obtain) — no boilerplate.
- remediation: 2-4 items, one sentence each, concrete and actionable (which point to change, how to change it, what to validate) — no empty advice.
- CVSS 3.1: cvss_score (number), cvss_vector (e.g. "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"), cvss_metrics (the eight metrics with English values).

Do not output test_process — the platform generates the reproduction steps directly from the real evidence; just return "test_process":[].

Output JSON only: {"title":"...","target":"...","description":["..."],"module":"...","test_process":[],"risk_analysis":["..."],"remediation":["..."],"cvss_score":0,"cvss_vector":"...","cvss_metrics":{...}}`

func aiVulnNarrative(ctx context.Context, pg *db.DB, locale string, f FindingInput, companyName string, sk *skillContent) Enrichment {
	var out Enrichment
	loc := normLocale(locale)
	dir := outputLangDirective(loc)
	data := vulnPromptData{Company: companyName, VulnClass: nz(f.VulnClass, "-"), Severity: severityName(loc, f.Severity), Summary: nz(f.Summary, "-"), PoC: nz(f.PoC, "-"), LangDirective: dir}
	sys := renderPrompt(sk.vulnSys, data)
	if sys == "" {
		sys = fmt.Sprintf(sysVuln, companyName)
	}
	user := renderPrompt(sk.vulnUser, data)
	if user == "" {
		user = fmt.Sprintf(userVuln,
			nz(f.VulnClass, "-"), severityName(loc, f.Severity), nz(f.Summary, "-"), nz(f.PoC, "-")) +
			"\n\nOutput language requirement: " + dir
	}
	// Ground the PROSE (description / risk / remediation) in the ACTUAL work transcript
	// (real tool calls / requests / responses / analysis recorded when the finding was
	// made) so it reflects what actually happened, not an expansion of the one-liner.
	if tr := strings.TrimSpace(f.Transcript); tr != "" {
		user += "\n\n=== REAL WORK TRANSCRIPT recorded when this vulnerability was found (tool calls / requests / responses / analysis) ===\n" +
			"Base description/risk_analysis on what actually happened in the transcript; NEVER fabricate or speculate about anything not in the transcript.\n\n" + tr
	}
	// PROSE-ONLY, ALWAYS. We never ask the model to reconstruct a verbatim test_process:
	// that large generation is slow and unreliable (many models time out / return nothing
	// on it) AND risks fabricating a PoC (the user's key pain). Instead test_process is
	// filled DETERMINISTICALLY below from REAL evidence — the agent's in-situ 复现过程 if
	// present, else the raw PoC. The model only writes prose (title/desc/module/risk/
	// remediation/CVSS), which is small, fast, and reliable on any model.
	if len(f.Reproduction) > 2 { // ""/"{}" carry nothing
		user += "\n\n=== Reproduction steps already recorded in situ by the test engine ===\n" +
			"The entry point / preconditions / steps / source of this vulnerability were **recorded in situ** during testing and will be used directly as the report's \"Test Process\". You do **not need to and MUST NOT** reconstruct test_process; just return \"test_process\":[]."
	} else {
		user += "\n\n=== test_process is supplied directly from real evidence ===\n" +
			"The \"Test Process\" will be generated by the platform directly from the **real PoC / verbatim evidence**. You do **not need to and MUST NOT** reconstruct or fabricate test_process; just return \"test_process\":[]."
	}
	user += " Focus on making title / description / module / risk_analysis / remediation and CVSS solid and professional, based on what actually happened in the summary and transcript above; NEVER fabricate anything not in the transcript."
	raw, err := complete(ctx, pg, sys, user, 4000)
	if err != nil {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	// Fill test_process from REAL evidence, never the model's reconstruction: the agent's
	// in-situ 复现过程 wins; else the raw PoC (deterministic). This makes every consumer
	// (persisted enrichment → findings page UI, and the report) show real, non-fabricated
	// steps — and keeps the generation prose-only (small, reliable).
	if b := reproToBlocks(f.Reproduction, loc, nil); len(b) > 0 {
		out.TestProcess = b
	} else if len(out.TestProcess) == 0 && strings.TrimSpace(f.PoC) != "" {
		out.TestProcess = proofToBlocks(f.PoC)
	}
	return out
}

// EnrichFinding is the exported per-finding AI enrichment, shared by the report
// pipeline and the findings-management "AI 分析" endpoint. locale controls the
// output language (zh-CN | zh-TW | en-US); skillDir enables the editable
// report-generation prompt (falls back to the built-in prompt when unset/missing).
// Returns a zero Enrichment (not an error) when no LLM profile is configured —
// callers keep whatever raw fields the finding already has.
func EnrichFinding(ctx context.Context, pg *db.DB, skillDir, locale string, f FindingInput, companyName string) Enrichment {
	if strings.TrimSpace(companyName) == "" {
		companyName = pick(locale, "安全服务团队", "安全服務團隊", "Security Services Team")
	}
	sk := loadSkillContent(skillDir, locale)
	return aiVulnNarrative(ctx, pg, locale, f, companyName, sk)
}

// cleanJSON strips markdown fences and surrounding prose from an LLM response.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1] // strip leading AND trailing prose around the JSON object
		}
	}
	return s
}
