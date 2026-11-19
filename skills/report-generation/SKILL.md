---
name: report-generation
description: Editable content for penetration test report generation — the AI rewriting prompts and the fixed skeleton (methodology / test checklist / tools / reference standards / testing personnel), split into files per language. The report engine reads them from disk on every generation, so edits take effect immediately.
---

# Report Generation (Editable Content)

This skill holds the **editable content of penetration test reports**, read by the generation pipeline of "Report Management". Changes **take effect on the next generation** — no restart or recompile needed; any missing or corrupted file automatically falls back to the built-in defaults.

## Structure

- `prompts/report-level.tmpl` — AI prompt for the report-level narrative (background / service results / remediation conclusion / checklist items hit).
- `prompts/vuln-narrative.tmpl` — AI prompt for rewriting a single vulnerability (description / reproduction steps / risk / remediation / CVSS).
- `skeleton/<locale>/methodology.json` — 2.2 Test Process: `{intro, phases:[{name,desc}]}`.
- `skeleton/<locale>/checklist.json` — 2.3 Test Checklist: `[{category,name,desc}]`. Items the AI judges as hit are rendered as "Fail".
- `skeleton/<locale>/tools.json` — 2.4 Test Tools: `[{name,desc}]`.
- `skeleton/<locale>/standards.json` — 2.5 Reference Standards: `["..."]`.
- `skeleton/<locale>/personnel.json` — 2.1 default testing personnel: `[{name,role,contact}]` (if the user fills them in the report settings, those take precedence).

Locale directories: `zh-CN` (Simplified Chinese) / `zh-TW` (Traditional Chinese) / `en-US` (English).

## Prompt template variables

The prompts are Go `text/template`s, with `===USER===` separating the system and user parts:

- Report level: `{{.Company}}` `{{.Project}}` `{{.Client}}` `{{.FindingsList}}` `{{.LangDirective}}`
- Vulnerability level: `{{.Company}}` `{{.VulnClass}}` `{{.Severity}}` `{{.Summary}}` `{{.PoC}}` `{{.LangDirective}}`

`{{.LangDirective}}` injects the target-language instruction (Simplified Chinese / Traditional Chinese / English), so the prompts themselves are language-independent and one copy is shared by all three languages.
