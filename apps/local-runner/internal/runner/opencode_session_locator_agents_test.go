package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CP-57 Task-303 gap closure: LocateSessionFile opencode branch (OC-12/E2E-23,
// Drive restore) and the .opencode/agents catalog discovery had wiring but zero
// opencode tests. Additive.

func TestLocateSessionFileOpencodeLayouts(t *testing.T) {
	home := t.TempDir()
	sid := "ses_locator1"
	cfgBase := filepath.Join(home, ".config", "opencode")

	// Layout 1: config-dir sessions/<id>.json
	if err := os.MkdirAll(filepath.Join(cfgBase, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	p1 := filepath.Join(cfgBase, "sessions", sid+".json")
	if err := os.WriteFile(p1, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := LocateSessionFile(ProviderKeyOpencode, home, sid, "/tmp/ws"); !ok || got != p1 {
		t.Fatalf("config-dir layout: got %q ok=%v, want %q", got, ok, p1)
	}

	// Layout 2: storage/session/<id>/store.json
	home2 := t.TempDir()
	base2 := filepath.Join(home2, ".config", "opencode")
	if err := os.MkdirAll(filepath.Join(base2, "storage", "session", sid), 0o755); err != nil {
		t.Fatal(err)
	}
	p2 := filepath.Join(base2, "storage", "session", sid, "store.json")
	if err := os.WriteFile(p2, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := LocateSessionFile(ProviderKeyOpencode, home2, sid, "/tmp/ws"); !ok || got != p2 {
		t.Fatalf("storage layout: got %q ok=%v, want %q", got, ok, p2)
	}

	// Layout 3: HOME variant .local/share/opencode/sessions/<id>.jsonl
	home3 := t.TempDir()
	p3 := filepath.Join(home3, ".local", "share", "opencode", "sessions", sid+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p3), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p3, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := LocateSessionFile(ProviderKeyOpencode, home3, sid, "/tmp/ws"); !ok || got != p3 {
		t.Fatalf("HOME data layout: got %q ok=%v, want %q", got, ok, p3)
	}

	// Guards: synthetic id never located; unknown id not found.
	if _, ok := LocateSessionFile(ProviderKeyOpencode, home, "thread-42", "/tmp/ws"); ok {
		t.Fatal("thread-* must not locate")
	}
	if _, ok := LocateSessionFile(ProviderKeyOpencode, home, "ses_missing", "/tmp/ws"); ok {
		t.Fatal("missing session must not locate")
	}
}

func TestAgentCatalogDiscoversOpencodeAgents(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".opencode", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: oc-fixer
description: Fixes opencode-side regressions
provider: opencode
model: opencode/muse-spark-1.2-contributor-free
model_reasoning_effort: high
role: coder
tools:
  - read
  - edit
---
Fix the bug and add an additive test.
`
	if err := os.WriteFile(filepath.Join(dir, "oc-fixer.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog := newAgentCatalog()
	defs := catalog.listAgents(cwd)
	var found *AgentDefinition
	for i := range defs {
		if strings.EqualFold(defs[i].Name, "oc-fixer") {
			found = &defs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("opencode agent not discovered, got %+v", defs)
	}
	if found.Provider != "opencode" {
		t.Fatalf("provider = %q, want opencode", found.Provider)
	}
	if found.Model != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("model = %q", found.Model)
	}
	if found.ModelReasoningEffort != "high" || found.Role != "coder" {
		t.Fatalf("effort=%q role=%q", found.ModelReasoningEffort, found.Role)
	}
	if len(found.Tools) != 2 || found.Tools[0] != "read" {
		t.Fatalf("tools = %v", found.Tools)
	}
	if !strings.Contains(found.SystemPrompt, "additive test") {
		t.Fatalf("system prompt = %q", found.SystemPrompt)
	}
}
