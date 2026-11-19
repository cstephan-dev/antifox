package server

// One-time migration of an existing database / data directory to the English seed texts of this fork
// (inmind). Upstream seeds several things once and never refreshes them (intercept rules, MCP descriptions,
// the built-in admin's display name, the demo report template, tool descriptions, the report-generation skill
// files in the skill directory). This moves them to English, but ONLY where the stored value is still exactly the
// original (Chinese) seed — anything a user edited is left alone. The old texts below are the upstream seeds
// (needed to recognise untouched values). Gated by the setting inmind_english_seeds_v1.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"antifox/agent"
	"antifox/report"
)

const englishSeedsFlag = "inmind_english_seeds_v1"

// built-in intercept rules: old name, old message → new name, new message
var englishInterceptRules = [][4]string{
	{"[内置] 递归强制删除 rm -rf", "禁止执行递归强制删除（rm -rf / rm --recursive），可能永久损坏系统或靶机环境", "[Built-in] Recursive forced delete rm -rf", "Recursive forced deletion (rm -rf / rm --recursive) is forbidden: it can permanently damage the system or the target environment"},
	{"[内置] 删除系统关键目录", "禁止删除系统关键路径", "[Built-in] Delete critical system directories", "Deleting critical system paths is forbidden"},
	{"[内置] 磁盘格式化 mkfs", "禁止格式化磁盘（mkfs）", "[Built-in] Disk format mkfs", "Formatting disks (mkfs) is forbidden"},
	{"[内置] 覆写磁盘设备 dd", "禁止使用 dd 覆写磁盘设备", "[Built-in] Overwrite disk device dd", "Overwriting disk devices with dd is forbidden"},
	{"[内置] Fork 炸弹", "禁止执行 Fork 炸弹", "[Built-in] Fork bomb", "Running a fork bomb is forbidden"},
	{"[内置] 关机 / 重启", "禁止执行关机或重启命令", "[Built-in] Shutdown / reboot", "Shutdown or reboot commands are forbidden"},
	{"[内置] 杀死全部进程", "禁止 kill -9 -1 或 killall -9（杀死所有进程）", "[Built-in] Kill all processes", "kill -9 -1 or killall -9 (killing all processes) is forbidden"},
	{"[内置] 磁盘擦除 shred / wipe", "禁止对磁盘设备执行 shred/wipe 擦除", "[Built-in] Disk wipe shred / wipe", "Wiping disk devices with shred/wipe is forbidden"},
	{"[内置] 清空防火墙规则", "禁止清空防火墙规则（iptables -F / nft flush）", "[Built-in] Flush firewall rules", "Flushing firewall rules (iptables -F / nft flush) is forbidden"},
	{"[内置] SQL DROP DATABASE / TABLE / SCHEMA", "禁止执行 DROP 操作，可能不可逆地销毁数据库对象", "[Built-in] SQL DROP DATABASE / TABLE / SCHEMA", "DROP operations are forbidden: they can irreversibly destroy database objects"},
	{"[内置] SQL TRUNCATE", "禁止执行 TRUNCATE，可能清空数据表所有数据", "[Built-in] SQL TRUNCATE", "TRUNCATE is forbidden: it can erase all data in a table"},
	{"[内置] MongoDB drop / dropDatabase", "禁止执行 MongoDB drop 操作", "[Built-in] MongoDB drop / dropDatabase", "MongoDB drop operations are forbidden"},
	{"[内置] Redis FLUSHALL / FLUSHDB", "禁止执行 Redis FLUSHALL / FLUSHDB，可能清空全部缓存数据", "[Built-in] Redis FLUSHALL / FLUSHDB", "Redis FLUSHALL / FLUSHDB is forbidden: it can erase all cached data"},
	{"[内置] curl / wget 发送 DELETE 请求", "禁止通过 curl/wget 发送 HTTP DELETE 请求，可能删除目标系统数据", "[Built-in] curl / wget sending DELETE requests", "Sending HTTP DELETE requests via curl/wget is forbidden: they can delete data on the target system"},
	{"[内置] Python HTTP 客户端 DELETE（requests/httpx/aiohttp）", "禁止使用 Python HTTP 客户端发送 DELETE 请求", "[Built-in] Python HTTP client DELETE (requests/httpx/aiohttp)", "Sending DELETE requests with a Python HTTP client is forbidden"},
	{"[内置] 脚本中声明 HTTP DELETE 方法（JS/通用）", "禁止在脚本中声明并发送 HTTP DELETE 请求", "[Built-in] HTTP DELETE method declared in scripts (JS/generic)", "Declaring and sending HTTP DELETE requests in scripts is forbidden"},
	{"[内置] 批量清空 / 清除接口路径", "禁止调用批量清空或销毁类接口（/clear /wipe /flush /purge 等）", "[Built-in] Bulk clear / wipe endpoint paths", "Calling bulk clear or destroy endpoints (/clear /wipe /flush /purge etc.) is forbidden"},
}

// built-in MCP servers: name, old description → new description
var englishMCPDescriptions = [][3]string{
	{"filesystem", "读写本地文件系统——在渗透测试中存取载荷、脚本输出、发现的配置文件等", "Read and write the local file system — store payloads, script output, discovered configuration files etc. during a pentest"},
	{"github", "GitHub 仓库侦察——搜索公开代码库中可能泄露的凭据、密钥、敏感配置，或分析目标组织开源项目代码（需在 MCP 页填入 GITHUB_PERSONAL_ACCESS_TOKEN 后启用）", "GitHub repository recon — search public code for leaked credentials, keys and sensitive configuration, or analyse the target organisation's open-source code (enable after entering GITHUB_PERSONAL_ACCESS_TOKEN on the MCP page)"},
}

// report-generation skill files (relative to <skillDir>/report-generation): sha256 of the upstream seeded content
var oldReportSkillFiles = map[string][]string{
	"SKILL.md":                    {"316942e6c3355fb2a134d21b8a6ad5c20463c7b1f2611fbb2dee737175430246", "cc2bbf32b334c2bff9f2ec041096648f9b28612a39ef16fba4ff5c229d5e3ada"},
	"prompts/report-level.tmpl":   {"a725d0bad58b59603361a9a5482a8993dc05a1a1489149a915787561a0cd38cb"},
	"prompts/vuln-narrative.tmpl": {"809bba75b06ebc9785e6ada4895255639187a1aaadece95a3e852742e9700638"},
}

var cjk = regexp.MustCompile(`\p{Han}`)

func (s *Server) migrateEnglishSeeds() {
	pg := s.m.PG()
	if v, _, _ := pg.GetSetting(englishSeedsFlag); v == "true" {
		return
	}
	n := 0
	count := func(res interface{ RowsAffected() (int64, error) }, err error) {
		if err != nil {
			log.Printf("[english-seeds] %v", err)
			return
		}
		if k, _ := res.RowsAffected(); k > 0 {
			n += int(k)
		}
	}
	for _, r := range englishInterceptRules {
		count(pg.Exec(`UPDATE intercept_rules SET name=$3, message=$4 WHERE name=$1 AND message=$2`, r[0], r[1], r[2], r[3]))
	}
	for _, m := range englishMCPDescriptions {
		count(pg.Exec(`UPDATE mcp_servers SET description=$3 WHERE name=$1 AND description=$2`, m[0], m[1], m[2]))
	}
	count(pg.Exec(`UPDATE rbac_users SET display_name='Administrator' WHERE id='admin' AND is_builtin AND display_name='管理员'`))
	if report.DefaultLocale() == "en-US" {
		pj, _ := json.Marshal([]map[string]string{
			{"name": "Alex Zhang", "role": "Project Lead / Security Engineer", "contact": ""},
			{"name": "Sam Li", "role": "Red Team Assessment", "contact": ""},
		})
		count(pg.Exec(`UPDATE report_templates SET name='Standard English delivery (example)', company_name='Example Security Team',
			personnel_json=$1::jsonb, locale='en-US' WHERE name='标准中文交付（示例）' AND company_name='示例安全服务团队' AND locale='zh-CN'`, string(pj)))
	}
	// tool descriptions: only rows still in Chinese whose code default is now English (an English or edited
	// non-Chinese description is kept)
	type seed struct {
		key, desc string
		schema    any
	}
	var seeds []seed
	for _, t := range append(s.orchestrationTools(), s.platformTools()...) {
		seeds = append(seeds, seed{t.Name(), t.Description(), t.InputSchema()})
	}
	for _, sd := range agent.BuiltinToolSeeds() {
		seeds = append(seeds, seed{sd.Key, sd.Desc, sd.Schema})
	}
	for _, sd := range seeds {
		if cjk.MatchString(sd.desc) {
			continue // still Chinese in code: nothing to migrate to
		}
		var cur string
		if err := pg.QueryRow(`SELECT description FROM tools WHERE key=$1 AND system`, sd.key).Scan(&cur); err != nil || !cjk.MatchString(cur) {
			continue
		}
		schema, _ := json.Marshal(sd.schema)
		if err := pg.RefreshToolDefaults(sd.key, sd.desc, schema); err == nil {
			n++
		}
	}
	// report-generation skill files: replace untouched upstream copies, then let SeedSkillFiles write the English ones
	replaced := 0
	for rel, hashes := range oldReportSkillFiles {
		p := filepath.Join(s.skillDir, "report-generation", rel)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		for _, h := range hashes {
			if hex.EncodeToString(sum[:]) == h {
				if os.Remove(p) == nil {
					replaced++
				}
				break
			}
		}
	}
	if replaced > 0 {
		if err := report.SeedSkillFiles(s.skillDir); err != nil {
			log.Printf("[english-seeds] report skill files: %v", err)
		}
		n += replaced
	}
	_ = pg.SetSetting(englishSeedsFlag, "true")
	log.Printf("[english-seeds] moved %d untouched seeded values to English (one-time)", n)
}
