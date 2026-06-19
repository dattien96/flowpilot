package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Task-081 acceptance: the catalog is reachable over HTTP at GET /client/agents
// and returns the built-in definitions.
func TestListAgentsOverHTTP(t *testing.T) {
	_, srv := newTestServer(t)

	status, body := doJSON(t, "GET", srv.URL+"/client/agents", nil, nil)
	if status != 200 {
		t.Fatalf("GET /client/agents status = %d, body = %s", status, body)
	}
	var agents []AgentDefinition
	if err := json.Unmarshal(body, &agents); err != nil {
		t.Fatalf("decode agents: %v (body=%s)", err, body)
	}
	if len(indexAgentsByName(agents)) < 3 {
		t.Fatalf("expected at least the 3 built-in agents, got %v", agentNames(agents))
	}
	if _, ok := indexAgentsByName(agents)["reviewer"]; !ok {
		t.Errorf("expected built-in reviewer over HTTP, got %v", agentNames(agents))
	}
}

// Task-081 acceptance: the catalog returns built-ins when no on-disk
// definitions exist, so it is never empty on first run.
// providerHomeFn is stubbed to nil so developer provider-home agent files
// cannot interfere with this test.
func TestAgentCatalogReturnsBuiltinsWhenEmpty(t *testing.T) {
	catalog := &AgentCatalog{
		builtins:       builtinAgentDefinitions(),
		providerHomeFn: func() []AgentDefinition { return nil },
	}

	agents := catalog.listAgents("")

	byName := indexAgentsByName(agents)
	for _, want := range []string{"coder", "reviewer", "tester"} {
		def, ok := byName[want]
		if !ok {
			t.Fatalf("expected built-in agent %q in catalog, got %v", want, agentNames(agents))
		}
		if def.Source != "flowpilot" {
			t.Errorf("built-in %q: source = %q, want flowpilot", want, def.Source)
		}
	}
}

// Task-081 acceptance: project-local .claude/agents overrides a built-in of the
// same name (project > built-in precedence), and only one entry survives.
func TestAgentCatalogProjectLocalOverridesBuiltin(t *testing.T) {
	cwd := t.TempDir()
	agentsDir := filepath.Join(cwd, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" +
		"name: coder\n" +
		"description: Project-specific coder\n" +
		"provider: gemini\n" +
		"model: gemini-3-pro\n" +
		"tools: [Read, Edit, Bash]\n" +
		"---\n" +
		"You are the project coder.\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "coder.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := &AgentCatalog{
		builtins:       builtinAgentDefinitions(),
		providerHomeFn: func() []AgentDefinition { return nil },
	}
	agents := catalog.listAgents(cwd)

	var coders int
	var coder AgentDefinition
	for _, def := range agents {
		if def.Name == "coder" {
			coders++
			coder = def
		}
	}
	if coders != 1 {
		t.Fatalf("expected exactly one coder after dedupe, got %d", coders)
	}
	if coder.Source != "claude" {
		t.Errorf("coder.Source = %q, want claude (project-local wins)", coder.Source)
	}
	if coder.Provider != "gemini" || coder.Model != "gemini-3-pro" {
		t.Errorf("coder frontmatter not parsed: provider=%q model=%q", coder.Provider, coder.Model)
	}
	if len(coder.Tools) != 3 || coder.Tools[0] != "Read" || coder.Tools[2] != "Bash" {
		t.Errorf("coder.Tools = %v, want [Read Edit Bash]", coder.Tools)
	}
	if coder.SystemPrompt != "You are the project coder." {
		t.Errorf("coder.SystemPrompt = %q", coder.SystemPrompt)
	}
}

// Task-081 acceptance: project-local agent overrides a provider-home agent of
// the same name (project > provider-home precedence), and only one entry survives.
func TestAgentCatalogProjectLocalOverridesProviderHome(t *testing.T) {
	cwd := t.TempDir()
	agentsDir := filepath.Join(cwd, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: coder\ndescription: Project coder\nprovider: gemini\n---\nProject coder.\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "coder.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	providerHomeCoder := AgentDefinition{
		Name:     "coder",
		Provider: "claude",
		Source:   "provider",
	}
	catalog := &AgentCatalog{
		builtins:       builtinAgentDefinitions(),
		providerHomeFn: func() []AgentDefinition { return []AgentDefinition{providerHomeCoder} },
	}

	agents := catalog.listAgents(cwd)

	var coderCount int
	var coder AgentDefinition
	for _, def := range agents {
		if def.Name == "coder" {
			coderCount++
			coder = def
		}
	}
	if coderCount != 1 {
		t.Fatalf("expected exactly one coder, got %d", coderCount)
	}
	if coder.Source != "claude" {
		t.Errorf("project-local coder should win over provider-home: source = %q, want claude", coder.Source)
	}
	if coder.Provider != "gemini" {
		t.Errorf("project-local coder provider = %q, want gemini", coder.Provider)
	}
}

// Codex agents are discovered too, and a comma-separated tools list parses.
func TestAgentCatalogDiscoversCodexAgentsAndCommaTools(t *testing.T) {
	cwd := t.TempDir()
	agentsDir := filepath.Join(cwd, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" +
		"name: auditor\n" +
		"description: Security auditor\n" +
		"tools: Read, Grep, Glob\n" +
		"---\n" +
		"Audit for vulnerabilities.\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "auditor.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := &AgentCatalog{
		builtins:       builtinAgentDefinitions(),
		providerHomeFn: func() []AgentDefinition { return nil },
	}
	byName := indexAgentsByName(catalog.listAgents(cwd))
	def, ok := byName["auditor"]
	if !ok {
		t.Fatal("expected auditor agent discovered from .codex/agents")
	}
	if def.Source != "codex" {
		t.Errorf("auditor.Source = %q, want codex", def.Source)
	}
	if len(def.Tools) != 3 {
		t.Errorf("auditor.Tools = %v, want 3 tools", def.Tools)
	}
	if def.Role != "auditor" {
		t.Errorf("auditor.Role = %q, want auditor (defaulted from name)", def.Role)
	}
}

// YAML block-list tools parsing: tools: followed by "- Item" lines.
func TestParseAgentDefinitionBlockListTools(t *testing.T) {
	body := "---\n" +
		"name: scanner\n" +
		"description: Test scanner\n" +
		"tools:\n" +
		"  - Read\n" +
		"  - Grep\n" +
		"  - Glob\n" +
		"---\n" +
		"Scan the codebase.\n"

	def := parseAgentDefinition("/fake/scanner.md", body, "codex")

	if len(def.Tools) != 3 {
		t.Fatalf("block-list tools: got %v, want [Read Grep Glob]", def.Tools)
	}
	if def.Tools[0] != "Read" || def.Tools[1] != "Grep" || def.Tools[2] != "Glob" {
		t.Errorf("tools = %v", def.Tools)
	}
	if def.SystemPrompt != "Scan the codebase." {
		t.Errorf("systemPrompt = %q", def.SystemPrompt)
	}
}

func indexAgentsByName(agents []AgentDefinition) map[string]AgentDefinition {
	out := make(map[string]AgentDefinition, len(agents))
	for _, def := range agents {
		out[def.Name] = def
	}
	return out
}

func agentNames(agents []AgentDefinition) []string {
	out := make([]string, 0, len(agents))
	for _, def := range agents {
		out = append(out, def.Name)
	}
	return out
}
