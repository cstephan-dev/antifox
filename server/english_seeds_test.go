package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateEnglishSeeds(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer m.Close()
	t.Setenv("ANTIFOX_DEFAULT_LOCALE", "en-US")
	pg := m.pg
	skillDir := t.TempDir()
	s := &Server{m: m, skillDir: skillDir}

	// an untouched upstream intercept rule and an edited one (same old name, other message)
	old := englishInterceptRules[0]
	for _, q := range []string{`DELETE FROM intercept_rules WHERE name=$1 OR name=$2`} {
		if _, err := pg.Exec(q, old[0], old[2]); err != nil {
			t.Fatal(err)
		}
	}
	ins := `INSERT INTO intercept_rules(name, enabled, priority, match_target, match_type, pattern, action, message, timeout_enabled, timeout_seconds, timeout_action)
		VALUES ($1, true, 100, 'tool_input', 'regex', 'x', 'deny', $2, false, 0, 'deny')`
	if _, err := pg.Exec(ins, old[0], old[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Exec(ins, old[0], "my own message"); err != nil {
		t.Fatal(err)
	}
	// MCP description back to the upstream Chinese seed
	mcp := englishMCPDescriptions[0]
	if _, err := pg.Exec(`UPDATE mcp_servers SET description=$2 WHERE name=$1`, mcp[0], mcp[1]); err != nil {
		t.Fatal(err)
	}
	// an orchestration tool whose stored description is still Chinese
	tool := s.orchestrationTools()[0]
	_ = pg.SeedTool(tool.Name(), "中文描述", nil, nil)
	if _, err := pg.Exec(`UPDATE tools SET description='中文描述' WHERE key=$1 AND system`, tool.Name()); err != nil {
		t.Fatal(err)
	}
	// the upstream report-generation SKILL.md (hash must match) vs an edited prompt file
	rg := filepath.Join(skillDir, "report-generation")
	_ = os.MkdirAll(filepath.Join(rg, "prompts"), 0o755)
	if err := os.WriteFile(filepath.Join(rg, "prompts", "vuln-narrative.tmpl"), []byte("edited by a user"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = pg.SetSetting(englishSeedsFlag, "")

	s.migrateEnglishSeeds()

	var n int
	_ = pg.QueryRow(`SELECT COUNT(*) FROM intercept_rules WHERE name=$1 AND message=$2`, old[2], old[3]).Scan(&n)
	if n != 1 {
		t.Errorf("untouched rule not migrated (found %d)", n)
	}
	_ = pg.QueryRow(`SELECT COUNT(*) FROM intercept_rules WHERE name=$1 AND message='my own message'`, old[0]).Scan(&n)
	if n != 1 {
		t.Errorf("edited rule was changed")
	}
	var desc, tdesc string
	_ = pg.QueryRow(`SELECT description FROM mcp_servers WHERE name=$1`, mcp[0]).Scan(&desc)
	if desc != mcp[2] {
		t.Errorf("MCP description: %q", desc)
	}
	if err := pg.QueryRow(`SELECT description FROM tools WHERE key=$1`, tool.Name()).Scan(&tdesc); err != nil || tdesc != tool.Description() {
		t.Errorf("tool description not refreshed: %q (%v)", tdesc, err)
	}
	if b, _ := os.ReadFile(filepath.Join(rg, "prompts", "vuln-narrative.tmpl")); string(b) != "edited by a user" {
		t.Errorf("edited skill file was replaced")
	}
	if v, _, _ := pg.GetSetting(englishSeedsFlag); v != "true" {
		t.Errorf("flag not set")
	}
	// clean up the rows this test added
	_, _ = pg.Exec(`DELETE FROM intercept_rules WHERE (name=$1 AND message=$2) OR (name=$3 AND message='my own message')`, old[2], old[3], old[0])
}
