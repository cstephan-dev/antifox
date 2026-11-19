package report

// Seeds the editable report-generation skill on startup by materializing the
// built-in Go defaults (defaults.go) into files under <skillDir>/report-generation/.
// Only writes files that are MISSING, so user edits in the Skills page are never
// overwritten. This makes the built-in skeleton + prompts editable without ever
// requiring a hand-maintained copy: the Go defaults remain the single source, and
// this projects them to disk once so they can be tuned and hot-reloaded.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var seedLocales = []string{"zh-CN", "zh-TW", "en-US"}

// SeedSkillFiles writes any missing report-generation skill files. Best-effort:
// returns the first write error but always attempts every file.
func SeedSkillFiles(skillDir string) error {
	if strings.TrimSpace(skillDir) == "" {
		return nil
	}
	base := filepath.Join(skillDir, reportSkillName)
	var firstErr error
	put := func(rel, content string) {
		if err := writeIfMissing(filepath.Join(base, rel), content); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	putJSON := func(rel string, v any) {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		put(rel, string(b)+"\n")
	}

	put("SKILL.md", skillMD)
	put(filepath.Join("prompts", "report-level.tmpl"), promptReportLevelTmpl)
	put(filepath.Join("prompts", "vuln-narrative.tmpl"), promptVulnTmpl)

	for _, loc := range seedLocales {
		d := filepath.Join("skeleton", loc)
		putJSON(filepath.Join(d, "methodology.json"), map[string]any{
			"intro": methodologyIntro(loc), "phases": methodology(loc),
		})
		putJSON(filepath.Join(d, "tools.json"), tools(loc))
		putJSON(filepath.Join(d, "standards.json"), standards(loc))
		putJSON(filepath.Join(d, "checklist.json"), checklist(loc))
		putJSON(filepath.Join(d, "personnel.json"), defaultPersonnel(loc))
	}
	return firstErr
}

// writeIfMissing creates parent dirs and writes content only when path is absent.
func writeIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // exists → keep user edits
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

const skillMD = `---
name: report-generation
description: Editable content for penetration test report generation — the AI rewriting prompts and the fixed skeleton (methodology / test checklist / tools / reference standards / testing personnel), split into files per language. The report engine reads them from disk on every generation, so edits take effect immediately.
---

# Report Generation (Editable Content)

This skill holds the **editable content of penetration test reports**, read by the generation pipeline of "Report Management". Changes **take effect on the next generation** — no restart or recompile needed; any missing or corrupted file automatically falls back to the built-in defaults.

## Structure

- ` + "`" + `prompts/report-level.tmpl` + "`" + ` — AI prompt for the report-level narrative (background / service results / remediation conclusion / checklist items hit).
- ` + "`" + `prompts/vuln-narrative.tmpl` + "`" + ` — AI prompt for rewriting a single vulnerability (description / reproduction steps / risk / remediation / CVSS).
- ` + "`" + `skeleton/<locale>/methodology.json` + "`" + ` — 2.2 Test Process: ` + "`" + `{intro, phases:[{name,desc}]}` + "`" + `.
- ` + "`" + `skeleton/<locale>/checklist.json` + "`" + ` — 2.3 Test Checklist: ` + "`" + `[{category,name,desc}]` + "`" + `. Items the AI judges as hit are rendered as "Fail".
- ` + "`" + `skeleton/<locale>/tools.json` + "`" + ` — 2.4 Test Tools: ` + "`" + `[{name,desc}]` + "`" + `.
- ` + "`" + `skeleton/<locale>/standards.json` + "`" + ` — 2.5 Reference Standards: ` + "`" + `["..."]` + "`" + `.
- ` + "`" + `skeleton/<locale>/personnel.json` + "`" + ` — 2.1 default testing personnel: ` + "`" + `[{name,role,contact}]` + "`" + ` (if the user fills them in the report settings, those take precedence).

Locale directories: ` + "`" + `zh-CN` + "`" + ` (Simplified Chinese) / ` + "`" + `zh-TW` + "`" + ` (Traditional Chinese) / ` + "`" + `en-US` + "`" + ` (English).

## Prompt template variables

The prompts are Go ` + "`" + `text/template` + "`" + `s, with ` + "`" + `===USER===` + "`" + ` separating the system and user parts:

- Report level: ` + "`" + `{{.Company}}` + "`" + ` ` + "`" + `{{.Project}}` + "`" + ` ` + "`" + `{{.Client}}` + "`" + ` ` + "`" + `{{.FindingsList}}` + "`" + ` ` + "`" + `{{.LangDirective}}` + "`" + `
- Vulnerability level: ` + "`" + `{{.Company}}` + "`" + ` ` + "`" + `{{.VulnClass}}` + "`" + ` ` + "`" + `{{.Severity}}` + "`" + ` ` + "`" + `{{.Summary}}` + "`" + ` ` + "`" + `{{.PoC}}` + "`" + ` ` + "`" + `{{.LangDirective}}` + "`" + `

` + "`" + `{{.LangDirective}}` + "`" + ` injects the target-language instruction (Simplified Chinese / Traditional Chinese / English), so the prompts themselves are language-independent and one copy is shared by all three languages.
`

const promptReportLevelTmpl = `You are a senior penetration test report writer for {{.Company}}, skilled at producing professional, client-readable, technically rigorous penetration test reports. Output strictly JSON only — no markdown code fences.
===USER===
Write the report overview content based on the project and findings list below.

Project name: {{.Project}}
Client name: {{.Client}}
Findings list:
{{.FindingsList}}
Requirements (professional delivery tone; avoid wording such as "this test was performed today"):
- background: array of strings, 2 paragraphs. The first explains that the client engaged {{.Company}} to perform a penetration test and states the test objectives; the second describes the test scope and the combined manual + tool-assisted approach. Each paragraph 40-90 words.
- service_result: 1 paragraph summarizing which systems were tested and the number and severity distribution of the findings.
- remediation_conclusion: 1 paragraph remediation conclusion (for a first-round penetration test, all findings are pending remediation by default; recommend prompt remediation followed by retesting).
- checklist_hits: array of strings — infer the standard test items hit, based on the vulnerability types (standard names such as "SQL Injection", "Unauthorized Access", "Authentication Bypass", "Sensitive Information Disclosure", "Command Injection", "Cross-Site Scripting (XSS)", "Vertical Privilege Escalation"). Use the exact standard test-item names in the output language specified below.

Output JSON only: {"background":["...","..."],"service_result":"...","remediation_conclusion":"...","checklist_hits":["...","..."]}

Output language requirement: {{.LangDirective}}
`

const promptVulnTmpl = `You are a senior penetration test report writer for {{.Company}}. Rewrite the raw vulnerability record into **delivery-grade** report text — as if a senior consultant wrote it personally for the client: concise, professional, and strictly to the point. Output strictly JSON only — no markdown code fences, no pleasantries, no disclaimers, and no generic boilerplate that would apply to anything. Original HTTP requests / commands / responses MUST be preserved verbatim and NEVER fabricated.
===USER===
Rewrite the vulnerability below as report body text. Core requirement: **concise, specific, restrained** — quality over quantity; do not pad the item count, do not add filler, do not state correct-but-empty truisms; every sentence must carry real information about this specific vulnerability on this specific target.

Vulnerability type / original name: {{.VulnClass}}
Severity: {{.Severity}}
Original summary:
{{.Summary}}
Original reproduction steps / evidence:
{{.PoC}}

Field requirements:
- title: a standardized, professional vulnerability name (roughly 4-14 words, e.g. "External Account Authentication Bypass Leading to Administrator Takeover"). If the original name is already standardized (e.g. contains a CVE), keep it or adjust it slightly; do not copy the summary verbatim.
- target: the affected host or URL, extracted from the summary/evidence; use an empty string if it cannot be determined.
- description: 1 paragraph, 2-3 sentences. State exactly three things — what the vulnerability is, where it occurs, and its root cause. Do not pile on OWASP categories, do not restate the remediation, no vague preamble.
- module: the affected functional module (short, e.g. "External account login / unauthenticated API").
- risk_analysis: 2-4 items, one sentence each; each MUST be a concrete consequence that **this vulnerability on this target** can cause (who can do what, and which data or privileges they obtain). If there are no concrete consequences, write fewer items — no boilerplate such as "may affect system security".
- remediation: 2-4 items, one sentence each, concrete and actionable (which point to change, how to change it, what to validate). No empty advice such as "raise security awareness" or "perform regular security audits".
- CVSS 3.1: cvss_score (number), cvss_vector (e.g. "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"), cvss_metrics (the eight metrics with English values, e.g. {"AV":"Network","AC":"Low","PR":"None","UI":"None","S":"Unchanged","C":"High","I":"High","A":"High"}).

Do not output test_process — the platform generates the reproduction steps directly from the real evidence; just return "test_process":[].

Output JSON only: {"title":"...","target":"...","description":["..."],"module":"...","test_process":[],"risk_analysis":["..."],"remediation":["..."],"cvss_score":0,"cvss_vector":"...","cvss_metrics":{...}}

Output language requirement: {{.LangDirective}}
`
