// Translations for built-in records that the backend stores in Chinese (seeded in db/db.go and db/rbac.go):
// built-in agents, system roles, permission descriptions, prompt-variable descriptions, built-in MCP server
// descriptions and the built-in admin's display name. Applied where the data enters the UI (lib/api.ts), keyed by
// stable ids (agent key, role id, permission key, MCP name), so every page shows them in the selected language.
//
// Only built-in values are translated. MCP descriptions and the admin's display name are user-editable: they are
// translated only while they still equal the seeded (zh-CN) text, so an edited value is always shown as written.
// Data loaded before a language switch keeps its language until the page fetches it again.
import i18n from "@/i18n/config";
import type { Agent, MCPServer, PromptVar, RBACRole, RBACUser } from "@/lib/types";

const has = (key: string) => i18n.exists(key);
const tr = (key: string, fallback: string) => (has(key) ? i18n.t(key) : fallback);
// the seeded text is the zh-CN translation of the key
const isSeeded = (key: string, value: string | undefined) =>
  !!value && has(key) && i18n.t(key, { lng: "zh-CN" }) === value;

export function trAgent<T extends Pick<Agent, "key" | "name" | "description" | "builtin">>(a: T): T {
  if (!a?.builtin) return a;
  const base = `builtinData.agent.${a.key}`;
  return {
    ...a,
    name: tr(`${base}.name`, a.name),
    description: a.description ? tr(`${base}.desc`, a.description) : a.description,
  };
}

export function trPromptVars(agentKey: string, vars: PromptVar[]): PromptVar[] {
  return vars.map((v) => ({ ...v, description: tr(`builtinData.var.${agentKey}.${v.name}`, v.description) }));
}

export function trRole<T extends Pick<RBACRole, "id" | "name" | "description" | "is_system">>(r: T): T {
  if (!r?.is_system) return r;
  const base = `builtinData.role.${r.id}`;
  return { ...r, name: tr(`${base}.name`, r.name), description: tr(`${base}.desc`, r.description) };
}

export function trPermissions(perms: Record<string, string>): Record<string, string> {
  return Object.fromEntries(Object.entries(perms).map(([k, v]) => [k, tr(`builtinData.perm.${k}`, v)]));
}

export function trMCP(m: MCPServer): MCPServer {
  const key = `builtinData.mcp.${m.name}`;
  return isSeeded(key, m.description) ? { ...m, description: i18n.t(key) } : m;
}

export function trUser<T extends Pick<RBACUser, "is_builtin" | "display_name">>(u: T): T {
  const key = "builtinData.adminDisplayName";
  return u?.is_builtin && isSeeded(key, u.display_name) ? { ...u, display_name: i18n.t(key) } : u;
}
