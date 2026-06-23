package runner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// AgentDefinition is a loadable sub-agent spec (CP-19 P-3 / Task-081). It is
// parsed from a markdown file with YAML-ish frontmatter
// (name/description/provider/model/role/tools) plus a system-prompt body, or
// provided as a FlowPilot built-in. Agents are provider-agnostic: an agent may
// declare a preferred provider, otherwise it inherits the spawning run's
// provider.
type AgentDefinition struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Role                 string   `json:"role"`
	Provider             string   `json:"provider,omitempty"`
	Model                string   `json:"model,omitempty"`
	ModelReasoningEffort string   `json:"modelReasoningEffort,omitempty"`
	Tools                []string `json:"tools,omitempty"`
	SystemPrompt         string   `json:"systemPrompt,omitempty"`
	// Source is where the definition came from: project-local "claude" /
	// "codex", a provider home "provider", or a FlowPilot "flowpilot" built-in.
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
}

// AgentCatalog discovers agent definitions from disk and falls back to
// built-ins so the catalog is never empty on first run. Discovery precedence
// (highest first, by case-insensitive name):
//
//	project-local .claude/agents + .codex/agents  >  provider-home agents  >  built-ins
//
// It mirrors the skill discovery in interactive_catalog.go but agents are
// cross-provider (both .claude/agents and .codex/agents are read regardless of
// the active provider).
type AgentCatalog struct {
	builtins       []AgentDefinition
	providerHomeFn func() []AgentDefinition // injectable for tests; nil uses discoverProviderHomeAgents
}

func newAgentCatalog() *AgentCatalog {
	return &AgentCatalog{
		builtins:       builtinAgentDefinitions(),
		providerHomeFn: discoverProviderHomeAgents,
	}
}

// listAgents merges disk-discovered agents with built-ins using name precedence
// (case-insensitive, first-seen wins). cwd is the active project workspace;
// an empty cwd skips project-local discovery and yields provider-home + built-ins.
func (c *AgentCatalog) listAgents(cwd string) []AgentDefinition {
	merged := make([]AgentDefinition, 0, 8)
	seen := make(map[string]struct{})
	add := func(defs []AgentDefinition) {
		for _, def := range defs {
			key := strings.ToLower(strings.TrimSpace(def.Name))
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, def)
		}
	}

	providerHomeFn := c.providerHomeFn
	if providerHomeFn == nil {
		providerHomeFn = discoverProviderHomeAgents
	}
	add(discoverProjectAgents(cwd)) // highest precedence
	add(providerHomeFn())           // middle
	add(c.builtins)                 // lowest; guarantees a non-empty catalog

	sort.Slice(merged, func(left, right int) bool {
		return strings.ToLower(merged[left].Name) < strings.ToLower(merged[right].Name)
	})
	return merged
}

func discoverProjectAgents(cwd string) []AgentDefinition {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil
	}
	defs := make([]AgentDefinition, 0, 8)
	defs = append(defs, agentsFromDir(filepath.Join(cwd, ".claude", "agents"), "claude")...)
	defs = append(defs, agentsFromDir(filepath.Join(cwd, ".codex", "agents"), "codex")...)
	return defs
}

func discoverProviderHomeAgents() []AgentDefinition {
	defs := make([]AgentDefinition, 0, 8)
	seenRoots := make(map[string]struct{})
	for _, provider := range []string{"claude", "codex"} {
		homePaths, err := DiscoverProviderAccountHomes(provider)
		if err != nil {
			continue
		}
		for _, homePath := range homePaths {
			for _, root := range providerHomeAgentDirs(provider, homePath) {
				cleanRoot := canonicalPathKey(root)
				if cleanRoot == "" {
					continue
				}
				if _, exists := seenRoots[cleanRoot]; exists {
					continue
				}
				seenRoots[cleanRoot] = struct{}{}
				defs = append(defs, agentsFromDir(root, "provider")...)
			}
		}
	}
	return defs
}

func discoverActiveProviderHomeAgents(r *Runner) []AgentDefinition {
	if r == nil {
		return discoverProviderHomeAgents()
	}
	defs := make([]AgentDefinition, 0, 8)
	seenRoots := make(map[string]struct{})
	for _, provider := range []string{"claude", "codex"} {
		account, err := r.ResolveProviderAccount(provider, "")
		if err != nil {
			continue
		}
		for _, root := range providerHomeAgentDirs(provider, account.HomePath) {
			cleanRoot := canonicalPathKey(root)
			if cleanRoot == "" {
				continue
			}
			if _, exists := seenRoots[cleanRoot]; exists {
				continue
			}
			seenRoots[cleanRoot] = struct{}{}
			defs = append(defs, agentsFromDir(root, "provider")...)
		}
	}
	return defs
}

func providerHomeAgentDirs(provider, homePath string) []string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}
	switch provider {
	case "claude":
		return []string{filepath.Join(homePath, ".claude", "agents"), filepath.Join(homePath, "agents")}
	case "codex":
		return []string{filepath.Join(homePath, ".codex", "agents"), filepath.Join(homePath, "agents")}
	default:
		return nil
	}
}

// agentsFromDir reads Claude markdown and Codex TOML agent files directly under
// baseDir. A missing or non-directory path yields nil.
func agentsFromDir(baseDir, source string) []AgentDefinition {
	info, err := os.Stat(baseDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}
	defs := make([]AgentDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".md" && extension != ".toml" {
			continue
		}
		path := filepath.Join(baseDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var def AgentDefinition
		switch extension {
		case ".toml":
			var ok bool
			def, ok = parseCodexAgentDefinition(path, raw, source)
			if !ok {
				continue
			}
		default:
			def = parseAgentDefinition(path, string(raw), source)
		}
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		defs = append(defs, def)
	}
	return defs
}

type codexAgentFile struct {
	Name                  string   `toml:"name"`
	Description           string   `toml:"description"`
	Role                  string   `toml:"role"`
	Provider              string   `toml:"provider"`
	Model                 string   `toml:"model"`
	ModelReasoningEffort  string   `toml:"model_reasoning_effort"`
	Tools                 []string `toml:"tools"`
	DeveloperInstructions string   `toml:"developer_instructions"`
}

func parseCodexAgentDefinition(path string, contents []byte, source string) (AgentDefinition, bool) {
	var file codexAgentFile
	if err := toml.Unmarshal(contents, &file); err != nil {
		return AgentDefinition{}, false
	}
	def := AgentDefinition{
		Name:                 strings.TrimSpace(file.Name),
		Description:          strings.TrimSpace(file.Description),
		Role:                 strings.ToLower(strings.TrimSpace(file.Role)),
		Provider:             strings.ToLower(strings.TrimSpace(file.Provider)),
		Model:                strings.TrimSpace(file.Model),
		ModelReasoningEffort: strings.ToLower(strings.TrimSpace(file.ModelReasoningEffort)),
		Tools:                file.Tools,
		SystemPrompt:         strings.TrimSpace(file.DeveloperInstructions),
		Source:               source,
		Path:                 path,
	}
	if def.Name == "" {
		def.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if def.Role == "" {
		def.Role = strings.ToLower(def.Name)
	}
	if def.Description == "" {
		def.Description = firstNonEmptyLine(def.SystemPrompt)
	}
	if len(def.Tools) == 0 {
		def.Tools = nil
	}
	return def, true
}

// parseAgentDefinition reads frontmatter fields and the system-prompt body from
// a markdown agent file. Frontmatter is the leading `---` block; everything
// after it is the system prompt. Unrecognized frontmatter keys are ignored.
func parseAgentDefinition(path, contents, source string) AgentDefinition {
	def := AgentDefinition{Source: source, Path: path}

	lines := strings.Split(contents, "\n")
	inFrontMatter := false
	frontMatterDone := false
	inToolsList := false // true while collecting YAML block-list tools
	body := make([]string, 0, len(lines))

	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if index == 0 && trimmed == "---" {
			inFrontMatter = true
			continue
		}
		if inFrontMatter && trimmed == "---" {
			inFrontMatter = false
			frontMatterDone = true
			inToolsList = false
			continue
		}
		if inFrontMatter {
			// Collect block-list items: "  - ToolName"
			if inToolsList {
				if strings.HasPrefix(trimmed, "- ") {
					tool := unquote(strings.TrimPrefix(trimmed, "- "))
					if tool != "" {
						def.Tools = append(def.Tools, tool)
					}
					continue
				}
				// Non-list line ends the block; fall through to key parsing.
				inToolsList = false
			}
			switch {
			case def.Name == "" && hasKey(trimmed, "name:"):
				def.Name = frontMatterValue(trimmed, "name:")
			case def.Description == "" && hasKey(trimmed, "description:"):
				def.Description = frontMatterValue(trimmed, "description:")
			case def.Provider == "" && hasKey(trimmed, "provider:"):
				def.Provider = strings.ToLower(frontMatterValue(trimmed, "provider:"))
			case def.Model == "" && hasKey(trimmed, "model:"):
				def.Model = frontMatterValue(trimmed, "model:")
			case def.ModelReasoningEffort == "" && hasKey(trimmed, "model_reasoning_effort:"):
				def.ModelReasoningEffort = strings.ToLower(frontMatterValue(trimmed, "model_reasoning_effort:"))
			case def.Role == "" && hasKey(trimmed, "role:"):
				def.Role = strings.ToLower(frontMatterValue(trimmed, "role:"))
			case def.Tools == nil && hasKey(trimmed, "tools:"):
				val := frontMatterValue(trimmed, "tools:")
				if val == "" {
					// YAML block list follows; mark non-nil to prevent re-entry.
					def.Tools = []string{}
					inToolsList = true
				} else {
					def.Tools = parseToolsList(val)
				}
			}
			continue
		}
		if frontMatterDone {
			body = append(body, line)
		}
	}

	// Normalize empty tools slice (e.g. block-list with zero items) to nil so
	// omitempty suppresses it in JSON output.
	if len(def.Tools) == 0 {
		def.Tools = nil
	}
	if def.Name == "" {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		def.Name = strings.ReplaceAll(base, "-", " ")
	}
	if def.Role == "" {
		def.Role = strings.ToLower(strings.TrimSpace(def.Name))
	}
	def.SystemPrompt = strings.TrimSpace(strings.Join(body, "\n"))
	if def.Description == "" {
		def.Description = firstNonEmptyLine(def.SystemPrompt)
	}
	return def
}

func hasKey(line, key string) bool {
	return strings.HasPrefix(line, key)
}

func frontMatterValue(line, key string) string {
	return unquote(strings.TrimSpace(strings.TrimPrefix(line, key)))
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// parseToolsList accepts either an inline YAML list (`[Read, Edit]`) or a
// comma-separated string (`Read, Edit, Bash`).
func parseToolsList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	parts := strings.Split(value, ",")
	tools := make([]string, 0, len(parts))
	for _, part := range parts {
		tool := unquote(strings.TrimSpace(part))
		if tool != "" {
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return nil
	}
	return tools
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return ""
}

// builtinAgentDefinitions are the FlowPilot defaults shipped in-repo so the
// catalog is non-empty even when no .claude/agents or .codex/agents files
// exist (CP-19 P-3 / Task-081 T-3). On-disk definitions of the same name
// override these.
func builtinAgentDefinitions() []AgentDefinition {
	return []AgentDefinition{
		{
			Name:        "coder",
			Role:        "coder",
			Description: "Implements a scoped change end-to-end, then signals ready-for-review.",
			Tools:       []string{"Read", "Edit", "Write", "Bash", "Grep", "Glob"},
			SystemPrompt: "You are the coder sub-agent. Implement the requested change end-to-end: " +
				"read the relevant code, make focused edits, keep tests green, and emit a ready-for-review " +
				"signal with a concise summary of what changed and why when you are done.",
			Source: "flowpilot",
		},
		{
			Name:        "reviewer",
			Role:        "reviewer",
			Description: "Adversarial code review; approves or requests changes to gate the loop.",
			Tools:       []string{"Read", "Grep", "Glob", "Bash"},
			SystemPrompt: "You are the reviewer sub-agent. Review the coder's diff adversarially for " +
				"correctness, regressions, and missed edge cases. Return either APPROVED or " +
				"CHANGES-REQUESTED with specific, actionable feedback.",
			Source: "flowpilot",
		},
		{
			Name:        "tester",
			Role:        "tester",
			Description: "Writes and runs tests, reports coverage gaps.",
			Tools:       []string{"Read", "Edit", "Write", "Bash", "Grep", "Glob"},
			SystemPrompt: "You are the tester sub-agent. Write and run tests for the change under review, " +
				"then report pass/fail results and any coverage gaps you could not close.",
			Source: "flowpilot",
		},
	}
}
