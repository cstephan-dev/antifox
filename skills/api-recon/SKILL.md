---
name: api-recon
description: Invoke this skill when collecting a website's API endpoints.
---

# API Recon (front-end endpoint reconnaissance)

On an **authorized** basis, discover as completely as possible: the **back-end APIs** (paths, methods, parameters, response bodies), the **front-end routes**, and the **UI feature triggers** (tabs, dialogs, table actions, etc.).

---

## Scope and prohibitions (Agent must read · violating = out of scope)

This skill is **only for API / parameter-surface reconnaissance**; it is not the vulnerability-hunting or exploitation phase.

### Task boundary

| Scope | Allowed | Forbidden |
|---|---|---|
| **Goal** | Enumerate paths, methods, parameters, routes, UI triggers | SQLi/XSS/privilege/brute-force/fuzz vulnerabilities, tampered-packet attacks, destructive operations |
| **Auth** | Hook + stub/mock to bypass the **client-side** login gate | Asking the user for, or guessing, credentials; attempting a real login-form submission |
| **Runtime** | Hook endpoints without credentials, use mock responses to get the SPA into the post-login shell | Flows that require a real back-end session to continue |

### Credential-less dynamic analysis (Phase 3 default)

1. Use `preload.js` / `runtime_harvest.js` to **intercept and stub** bootstrap endpoints such as login, permissions and menus;
2. Return a mock body that is **structurally correct, with a success business code, and may have empty data** for business query endpoints;
3. Let the front-end still render post-login pages with no back-end or in a 401 environment, so that more XHR/fetch/WebSocket requests fire;
4. **Empty data, blank tables and placeholder UI are all expected** — do not switch to a real login or vulnerability testing because of them.

**In one sentence**: use mocks to prop open the front-end routes and component mounting, and **only record outbound requests**; what the back-end returns does not matter, what matters is **which further endpoints the front-end will call**.

### Hard process prohibitions

| Forbidden | Do instead |
|---|---|
| grep/curl/Read the main entry `index-*.js` to extract API paths before Phase 1 is complete | Run `OUTDIR/harvest_static.py` |
| Hand-write a script such as `extract_apis.py` to replace harvest | Edit `OUTDIR/harvest_static.py` and re-run |
| Repeating the same grep/command after it failed ≥2 times | Change strategy: read tool_logs, edit harvest, check reference |
| Skipping gates A/B and running the original `scripts/` directly | Copy to OUTDIR and adapt to the target |
| Real usernames/passwords, OTP, OAuth and other authentication | stub/mock (see above) |
| Skipping the stub to "get real data" and doing privilege/injection testing | Only record outbound; that is the recon boundary |
| Irreversible operations such as deleting, exporting sensitive data, bulk writes | Same applies to coverage clicks |
| Claiming all pages and endpoints are obtained without completing runtime + dynamic enumeration | See "Definition of done" or note the limitation |
| Claiming all parameters are known without completing the parameter trigger matrix + diff | Phase 3b matrix + Phase 5 diff |
| Inferring required/optional from a single runtime sample | Multi-sample diff, or reverse-engineer from validation rules/errors |

---

## Two-layer model + run modes

| Layer | Output | Ceiling |
|---|---|---|
| **Static** (JS bundle) | Full endpoint paths, route draft, request-assembly field candidates | No HTTP method; parameters need Phase 1b; misses runtime-assembled URLs |
| **Runtime** (live session) | Method + body + response + dynamic URLs + WS/SSE; multi-sample diff to fill in parameters | A page must actually render before requests fire; a single sample is not enough to pin required/optional |

| Run mode | Engine | Suited for |
|---|---|---|
| **depth** | `runtime_harvest.js` (Puppeteer) | API inventory, METHOD/params/response body, WS/SSE, reproducible batch runs |
| **coverage** | browser + `preload.js` | Clicking tabs/dialogs/tables, deeper feature-point coverage |
| **both** | depth first, then coverage | Most complete, longest to run |

**Parameter methodology** (no general-purpose script): paths come from harvest/regex; parameters come from **anchor-window expansion + UI binding chain + multi-sample diff + reverse-engineering from errors** (grep recipes in section J of [reference.md](reference.md)).

---

## Definition of done

Recon may be claimed complete only when all of the following hold:

- [ ] **Static**: Phase 1 harvest produced `api_static.txt`, `routes.txt`, `js/`
- [ ] **Runtime**: at least one of depth or coverage; coverage/both require **the hook in effect + the dynamic-enumeration loop**
- [ ] **In the shell**: visiting a business path does not land on `/login` (mind hash routing)
- [ ] **Parameters**: coverage/both completed the parameter trigger matrix + `param_samples.json`; Phase 5 merged `params_merged.json`
- [ ] **Depth** (if module pages are blank): Phase 4 permission tree reconstructed and re-run until **module-level APIs** appear (not just locale/bootstrap)
- [ ] **Delivery**: Phase 5 outputs complete (see the Phase 5 output table); `insert_assets` wrote the service and endpoint assets

---

## Scripts and gates

`scripts/` are reference templates only; **do not** run the originals directly and take them as the final result.

**Rule**: read first → adapt to the target → write into `OUTDIR` (e.g. `recon/`) → record `CHANGES.md`; if it does not match, rewrite per the methodology and borrow only the structure.

| Gate | When | Reference script → OUTDIR copy | Common edits needed |
|---|---|---|---|
| **A (static)** | After Phase 0, before running harvest/spider for the **first** time | `harvest_static.py` / `spider_mpa.py` | **The default regex runs as-is on most sites**; only when the manifest/dialect does not match, edit the endpoint regex, the webpack/Vite `publicPath`, the MPA exclude/cookie |
| **B (runtime)** | After Phase 2, before running depth/coverage | `runtime_harvest.js` / `preload.js` + `config.json` | Cookie/localStorage keys, neutralize success values, stubs, login regex, api prefix, hash/history |

**SPA mandatory order** (not interchangeable; the Phase numbering takes priority over "explore first, then script"):

| Step | Must | Forbidden |
|---|---|---|
| After Phase 0 is complete | Next Bash = `python3 OUTDIR/harvest_static.py <URL> OUTDIR` | curl/grep/Read the main entry `index-*.js` (usually >500KB) |
| Gate A | Copy the script → small edits as needed → **run immediately** | Manually extract APIs first, then decide whether to harvest |
| Before Phase 1 is complete | `wc -l` to validate the output; on 404, edit harvest and retry | Hand-write an extract script; grep an undownloaded URL repeatedly |
| From Phase 1b | grep only `OUTDIR/js/*.js` | Use the main bundle instead of harvest |

- ✅ Copy `harvest_static.py` → (optionally) edit the regex → **run immediately**
- ❌ curl the main bundle → grep many times → write a temporary extract → harvest only at the end
- **MPA**: after Phase 0, the next Bash = `python3 OUTDIR/spider_mpa.py ...`

---

## Tool and output constraints

| Constraint | Note |
|---|---|
| Large files | Do **not** Read/grep an `index-*.js` >100KB into context; batch-process it with an OUTDIR script |
| grep output | Always `\| head -20` or `-m 5`; keep only a path summary in the conversation, do not paste bundle fragments |
| Validation | Use `wc -l`, `ls \| wc -l`; do not Read a whole directory |
| regex first look | Optional, ≤1 time, only a ≤50KB small chunk or HTML; the formal static pass is harvest |
| reference | Recipes/templates/troubleshooting are in [reference.md](reference.md); do not repeat the full text inline |

---

## Execution roadmap

```
Phase 0 classify + OUTDIR
  → Gate A → Phase 1 harvest (★ run immediately ★)
  → Phase 1b parameter reversing
  → Phase 2 three auth gates → config.json
  → Gate B → Phase 3 runtime + parameter matrix
  → Phase 4 permission tree (when needed) → re-run Phase 3
  → Phase 5 merge report + insert_assets: bulk-insert every discovered service and endpoint API asset; never omit an already-discovered asset on insert
```

Check off in order; **you may not enter the next Phase until the previous item is complete**.

1. [ ] **Phase 0**: first look at SPA/MPA; create `OUTDIR` → [Phase 0](#phase-0--classification)
2. [ ] **Gate A + Phase 1**: copy the script → harvest **immediately** → `wc -l` validate → [Phase 1](#phase-1--static)
3. [ ] **Phase 1b**: anchor-window expansion + binding layer → `param_candidates.json` → [Phase 1b](#phase-1b--parameter-reversing)
4. [ ] **Phase 2**: three auth gates → `config.json` → [Phase 2](#phase-2--three-auth-gates)
5. [ ] **Gate B**: adjust the runtime script → [Phase 3](#phase-3--runtime)
6. [ ] **Phase 3**: depth / coverage / both; confirm you are in the shell; parameter trigger matrix → `param_samples.json`
7. [ ] **Phase 4** (if needed): permission tree → patch stubs → re-run Phase 3 → [Phase 4](#phase-4--permission-tree-reconstruction)
8. [ ] **Phase 5**: merge outputs + report + `insert_assets` → [Phase 5](#phase-5--merge-and-report)

---

## Phase 0 — Classification

Fetch the entry HTML and **create `OUTDIR`** (do not edit the skill's own `scripts/`):

- **SPA**: empty shell + `<div id=app>` + chunks → Phase 1–5
- **MPA**: SSR + `<form>`, no endpoint bundle → after Gate A:

```bash
python3 recon/spider_mpa.py <BASE_URL> <OUTDIR> [--cookie "session=..."] [--max 300] [--depth 5] [--exclude "logout|delete|destroy"]
```

Produces `forms.txt`, `links.txt`, `api_inline.txt`. For an SPA, if forms ≈ 0 → switch to Phase 1.

---

## Phase 1 — Static

Follow [Scripts and gates](#scripts-and-gates) · [Tool and output constraints](#tool-and-output-constraints).

```bash
python3 recon/harvest_static.py <BASE_URL> <OUTDIR>
```

harvest: parse HTML scripts → webpack/Vite manifest → download all lazy chunks → produce `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt`.

```bash
wc -l OUTDIR/api_static.txt OUTDIR/routes.txt
ls OUTDIR/js | wc -l
```

- chunk count vs manifest: on 404, edit harvest and retry; do not manually curl chunks one by one
- `api_static.txt` too small → loosen the endpoint regex inside OUTDIR and re-run (see reference)

### Phase 1b — Parameter reversing

Paths come from Phase 1; parameter fields must be recon'd separately. grep rules in [Tool and output constraints](#tool-and-output-constraints).

**Completion criterion**: for important endpoints you can answer — field name, transport location, type inference, whether required, sample value, confidence.

#### 1b.0 — Transport form

| Form | Where the parameters are | Static priority |
|---|---|---|
| REST JSON | body + query | `(params\|data\|body)\s*:\s*\{` next to the path anchor |
| GraphQL | `variables` | gql templates, `$page: Int` |
| Traditional form | urlencoded | `<form>`, `FormData` |
| File upload | multipart | `FormData.append` |
| Path parameter | `/user/:id` | route table + `useParams` / `$route.params` |
| Encrypted/signed | wrapped in `sign`/`data` | Hook the crypto function's arguments (reference section D) |

Output: annotate each endpoint with `transport: query|json|form|graphql|encrypted`.

#### 1b.1 — Anchor-window expansion

Using a known path as the anchor, expand the window to find the request-assembly object:

```bash
grep -n '"/api/user/list"' OUTDIR/js/*.js | head -20
grep -rhoaE '.{0,120}("/api[^"]+").{0,200}' OUTDIR/js/*.js | head -20
grep -rhoaE '(params|data|body|payload)\s*:\s*\{' OUTDIR/js/*.js | head -20
```

| Wrapper layer | Parameter clue |
|---|---|
| axios instance | `data` / `params` |
| unified request | interceptor injects global fields |
| OpenAPI client | generated method signature |
| React Query / SWR | hook's second argument |
| Vue composable | composable's arguments |

Type residue: `yup`/`zod`/rules, `Form.Item name=`, embedded Swagger.

→ `param_candidates.json`: `{ path, fields[], source: "static-callsite", confidence }`

#### 1b.2 — Binding layer

```
Form field → onFinish/handleSubmit → transform → API payload
```

| Binding source | Technique |
|---|---|
| form submit | follow submit → transform → API |
| table search | `getFieldsValue()` → `params` |
| route | `:id` / `?tab=` |
| interceptor | global `tenantId`, paging, sign |
| enum select | `options` → API enum values |

From the DevTools call stack, trace upward from `fetch`/`XHR.send` to the assembly function.

#### 1b.3 — Three assembly questions (≠ the Phase 2 auth gates)

| Question | What to answer |
|---|---|
| **Assembly** | where the payload is built, transform traces |
| **Validation** | required, pattern, enum |
| **Transport** | path / query / body / multipart / header |

At the interceptor gate (Phase 2), also read the globally injected fields (Authorization, `X-Tenant-Id`, sign).

#### 1b.4 — Hand-off to Phase 3

Candidate fields come from the static/binding layer; **required/optional/conditional dependency** must come from the Phase 3 parameter matrix + diff + Phase 5 reverse-engineering from errors.

---

## Phase 2 — Three auth gates

grep in `OUTDIR/js/` (with `head`) and write into `config.json` (recipes in reference):

| Gate | Question | Keywords |
|---|---|---|
| **Render gate** | How is "logged in" decided? | `isLogin`, `getToken`, Cookie/localStorage |
| **Interceptor gate** | What triggers the jump to `/login`? | `response_code`, `errno`, axios interceptor |
| **Content gate** | Where do menus/permissions come from? | `menu`, `permission`, `role`, `acl`, `routes` |

Do not treat a localStorage key name as a credential — confirm it from the chunk/request chain.

**Exit = Gate B**: land the conclusions in `config.json` and edit `OUTDIR/runtime_harvest.js` / `preload.js`.

### Phase 2b — API observation (optional)

Use the `preload.js` in OUTDIR to confirm session key names, Authorization, and nested API URLs:

| Config | Output |
|---|---|
| `recordDetail: true` | `__API_RECON_DETAIL__` |
| `observe.xhrHeaders: true` | headers observation |
| `extractUrlsFromResponse: true` | sub-APIs inside responses |
| `observe.storageReads/cookieReads: true` | fill back into config |
| `neutralizeVueRouter: true` | `__API_RECON_ROUTES__` |

Each coverage round exports: `__API_RECON_LOG__`, `__API_RECON_DETAIL__`, `__API_RECON_ROUTES__`, `__API_RECON_OBSERVE__`.

---

## Phase 3 — Runtime

You must have passed Gate B; follow [Scope and prohibitions](#scope-and-prohibitions-agent-must-read) · the credential-less mock strategy.

In `config.json` set `"runtimeMode": "depth" | "coverage" | "both"` (templates in reference).

### Hook and stub (shared by depth + coverage)

| Layer | Scope | Purpose |
|---|---|---|
| L1 precise | stub auth/permission/bootstrap | pass the first-screen auth |
| L2 negative fix | all JSON responses | not-logged-in code → success |
| L3 fallback | `/api` etc. not caught by L1 | empty success body, props open the UI |

- **depth**: fake auth + `forward` to change the business code + `stubs`; traverse `routes` (hash/history); produce `runtime_api.json`
- **coverage**: inject `preload.js` at **document-start** (CDP `addScriptToEvaluateOnNewDocument` or Userscript)

Verify: `window.__API_RECON_PRELOAD__` exists; business paths do not return `/login`.

```bash
cd recon && npm install
node runtime_harvest.js config.json
```

### 3b — coverage dynamic enumeration (mandatory)

1. Main nav / sidebar — click each item, wait 1–3s for the network
2. Tabs — `role=tab`, `.ant-tabs-tab`
3. Tables — view/edit/detail on the first row
4. Toolbars — export, filter, create (**avoid irreversible deletes**)
5. Each module entered — merge APIs/routes
6. SPA — controlled `pushState` to paths not covered by `routes.txt` (forbidden for MPA)

**Parameter trigger matrix** (mandatory): record each module once per operation type, and **diff multiple samples**:

| Operation | Parameters that usually appear extra |
|---|---|
| list first screen | paging + default filters |
| click search | keyword, filter |
| advanced filter | more optional |
| create/edit | full entity |
| bulk/export/sort | `ids[]`, `exportType`, `sortField` |

**Under a stub, the outbound body/headers are still real** — go by the request. Record → `scan_raw.json`, `param_samples.json`, `api_detail.json`.

- **Vue**: `neutralizeVueRouter: true` + document-start preload
- **React**: `routes.txt` + sidebar clicks + `pushState`
- **both**: 3a depth first, then 3b coverage

---

## Phase 4 — Permission tree reconstruction

**Trigger**: module pages are blank / each route only has bootstrap (e.g. locale) → the content gate has not been passed.

| Symptom | Meaning |
|---|---|
| in the shell successfully | render gate + interceptor gate passed |
| sidebar items missing / clicks go blank | stub shape or permission codes incomplete |
| every route has the same, very few APIs | `v-if permission` did not pass |
| `routes.txt` far fewer than the bundle | must be completed from the auth module |

```bash
grep -rhoaE '"/api[^"]*(permission|perm|role|menu|acl)[^"]*"' OUTDIR/js/*.js | sort -u | head -30
grep -rhoaE 'userRouteAuth|getResultTree|routeMap|routeLink|menuList|authList' OUTDIR/js/*.js | head -20
```

Typical chain: `role_permissions` (flat codes) + `permissions/all` (tree) → `getResultTree` → `userRouteAuth[CODE].url`.

```bash
python3 recon/extract_route_map.py recon/js recon/
python3 recon/build_perm_tree.py recon/js recon/ --config recon/config.json
```

Intermediate outputs: `route_map.json`, `userRouteAuth.json`, `permissions_tree.json`, `*_stub.json`, `perm_codes_all.txt`.

stub check: the outer `response_code` matches the interceptor gate; flat codes align with the tree; `routes` covers every link in `route_map`.

After updating `config.json`, **re-run Phase 3**. For a large SPA you can tune `waitUntil`, `routeTimeout`, `perRouteMs` (see reference sections A3/I).

---

## Phase 5 — Merge and report

### Output table

| File | Stage | Content |
|---|---|---|
| `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt` | 1 | static bundle and paths |
| `param_candidates.json` | 1b | static parameter-field candidates |
| `config.json` | 2 | three gates + runtime config |
| `runtime_api.json` | 3a | depth detailed recording (incl. WS/SSE) |
| `param_samples.json`, `scan_raw.json`, `api_detail.json` | 3b | multi-sample, click log, detail |
| `route_map.json` etc. | 4 | permission-tree intermediates (if run) |
| `params_merged.json` | 5 | merged parameter fields + confidence |
| `api_merged.txt` | 5 | `METHOD /path [params] [static\|runtime\|both]` |
| `site_map.json` | 5 | routes, APIs, params, feature points, limitations |
| **insert_assets** | 5 | write all service and endpoint assets into the asset database |

### 5b — Parameter merge

diff from `param_samples.json`, **no general-purpose merge script**. Confidence rules in reference J7 (high/medium/low/to-trigger).

### 5c — Reverse-engineering from errors

Within the authorized scope you may send an incomplete request to read the 400 (**this is parameter recon, not vulnerability testing**): `field 'x' is required`, enum errors, etc. Mind the `data` wrapper, `variables`, and the pre-encryption `bizData`.

The report must note: runtimeMode, static/runtime API counts, parameter confidence, uncovered modules, and a `CHANGES.md` summary relative to the reference scripts.

Suggested `site_map.json` structure:

```json
{
  "site": "https://example.com",
  "runtimeMode": "both",
  "appType": "vue-spa",
  "routeGuardStrategy": ["nav-neutralize", "L1-auth", "L2-patch", "forward"],
  "apisFromStatic": [],
  "apisFromRuntime": [],
  "apis": [],
  "params": [{ "method": "POST", "path": "/api/user/list", "transport": "json", "fields": [] }],
  "frontendRoutes": [],
  "routesVerifiedByClick": [],
  "featuresTriggered": [],
  "limitations": ""
}
```

More fields and grep recipes in [reference.md](reference.md).

---

## General notes

- **Framework-agnostic**: webpack/Vite/Angular lazy load use the same method
- **Transport**: REST/JSON, GraphQL, WebSocket, SSE; gRPC-web is out of scope
- **SSR**: client-side fetch can be recorded; RSC/Server Actions are not fully enumerable
- **Blind spots**: JSVMP, WASM, strong HMAC/mTLS validation → static + note the limitation
- **Parameter blind spots**: conditional coupling, hidden params, WASM assembly → "to-trigger" / "unreachable"
- **Static is the safety net**: when runtime is blocked, static can still enumerate endpoints

---

## Additional resources

- Grep recipes, `config.json` templates, troubleshooting, hooks, parameter-reversing section J, site_map template: **[reference.md](reference.md)**
- Reference script paths are in the [Scripts and gates](#scripts-and-gates) table
