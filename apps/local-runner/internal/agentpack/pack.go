package agentpack

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed flow-pack
var embeddedPackFS embed.FS

const defaultPackRoot = "flow-pack"

// Pack is the parsed on-disk agent pack.
type Pack struct {
	Manifest Manifest
	Agents   []AgentSpec
	Flows    []FlowDefinition
	Tools    []ToolFace
	Contexts []ContextArtifact
	Prompts  []PromptTemplate
}

// Manifest describes the top-level pack index.
type Manifest struct {
	ID            string
	Version       string
	Description   string
	SchemaVersion int
	Agents        []string
	Flows         []ManifestFlow
	Tools         []string
	Contexts      []string
	Prompts       []string
	Compatibility ManifestCompatibility
	RawReferences []string
}

type ManifestFlow struct {
	Path         string
	Visibility   string
	Editable     bool
	ChatBaseline bool
	SelectableIn []string
	ChatSubModes []string
	Cloneable    bool
}

type ManifestCompatibility struct {
	MinRunnerVersion     string
	FallbackToGoBuiltins bool
}

// AgentSpec is a pack-native agent definition.
type AgentSpec struct {
	Name                 string
	Description          string
	Role                 string
	Provider             string
	Model                string
	ModelReasoningEffort string
	Tools                []string
	SystemPrompt         string
	Source               string
	Path                 string
}

// BuiltinMeta captures the read-only metadata around a built-in flow.
type BuiltinMeta struct {
	Editable     bool
	ChatBaseline bool
	SelectableIn []string
	ChatSubModes []string
	Cloneable    bool
	Mirror       MirrorRule
	ChatUI       ChatUIRule
}

type MirrorRule struct {
	Required bool
	Source   string
}

type ChatUIRule struct {
	Placement string
	ShowWhen  map[string]string
}

type FlowPolicy struct {
	Cap       int
	OnCap     string
	ExtendBy  int
	ExtendMax int
	// StallTimeoutSec is the Task-241 / T-11 member stall window in seconds.
	// Zero means "use runner default" (10 minutes). Additive — packs without
	// the field keep pre-Task-241 behavior via the default.
	StallTimeoutSec int
	// NegotiationCap is the CP-67 P-5 (B-10) phase-scoped budget for the
	// Contract-First TDD signature-renegotiation loop (coder batch → hub
	// mediation → scaffold revise → coder re-entry). Phase-scoped on purpose:
	// it never shares the review loop's cap. Zero means "runner default (5)";
	// declared values are validated to 1..20.
	NegotiationCap int
}

type FlowContextBinding struct {
	Ref string
	// Sources lists enabled ContextSource IDs for this binding (CP-44 P-4 /
	// Task-194). Empty means "use the runner's default built-in set" — a flow
	// that declares no sources keeps the pre-CP-44 behavior unchanged.
	// Validated against the registered context sources at flow-load time
	// (see runner.ValidateFlowContextSources); an unknown ID here fails flow
	// resolution rather than silently running without it.
	Sources []string
}

// ContextProfile is one CP-62 P-5 (Task-341) per-node context profile:
// the candidate source set + the token budget the Budget Packer enforces
// when packing that node's prompt. Declared under a flow's `contextProfiles:`
// map; nodes reference profiles by name (`contextProfile:`).
type ContextProfile struct {
	Name             string
	CandidateSources []string
	MaxTokens        int
}

type FlowNode struct {
	ID             string
	Run            string
	Lifecycle      string
	Behavior       string
	Agent          string
	Join           string
	Cohort         string
	DependsOn      []string
	PromptTemplate string
	// Model is this node's pack-declared model tier override (Task-320).
	// Only agent.delegate nodes consume it (resolveFlowNodeModel); empty
	// means "no pack default — resolve via step row / agent / inherit".
	Model string
	// Posture is the CP-62 P-4 (Task-340) execution posture for the node:
	// "read_only" (silent-deny writes; Bash classified per-command via the
	// BUG-344 invariant), "verdict_only" (reads + the verdict tool only — no
	// Bash), or "" / "standard" (unchanged behavior). Declared in flow YAML;
	// enforced at the shared approval bridge (turnBridge.RequestApproval) so
	// every provider is gated identically. Engine stays domain-free (SD-19
	// BR-1): no role strings, just the declared posture value.
	Posture string
	// ContextSources is this node's own enabled context-source ids (CP-44 P-7
	// / Task-196), the step-definition-level equivalent of
	// FlowContextBinding.Sources. Empty means "fall back to the flow-level
	// contexts.<name>.sources binding, then the runner's default built-in
	// set" — the same precedence pack-YAML flows and user-authored
	// (step_definitions-backed) flows both resolve through.
	ContextSources []string
	// ArtifactBindings are this node's typed artifact instance bindings
	// (CP-45/SD-23 D-1/D-4), resolved and denormalized at flow-definition
	// read time (see supabase_workflow_flow_store.go's recordFromWorkflowRow)
	// so the executor never needs a second round-trip to look up an
	// instance's config. Populated from `step_artifact_bindings` joined with
	// `artifact_instances`; empty for a node with no bindings (CP-44 fallback
	// path stays unaffected — SD-23 D-6).
	ArtifactBindings []FlowArtifactBinding
	// ContextProfile is the CP-62 P-5 (Task-341) profile name this node
	// resolves its context candidate set + token budget from. Empty means
	// "no profile — the pre-Task-341 source precedence applies unchanged".
	ContextProfile string
	// Config is the CP-65 P-3 (Task-370) free-form node config map (root YAML
	// key `config`): tournament nodes declare candidates/auto_pick/
	// max_attempts/serial here. Nil for every pre-CP-65 flow — additive only,
	// absent key parses to nil and changes nothing downstream.
	Config map[string]any
}

// FlowArtifactBinding is one resolved typed-artifact binding for a
// FlowNode's input or output slot (CP-45/SD-23 D-1/D-4/D-7). ArtifactTypeID
// and ConfigJSON are denormalized from the bound ArtifactInstance so
// resolvers (e.g. context_artifact's ContextSourceRegistry reuse, Task-201;
// file_artifact's path injection, Task-202) can act on a binding without a
// second DB lookup.
type FlowArtifactBinding struct {
	Direction          string // "input" | "output"
	SlotName           string
	ArtifactInstanceID string
	ArtifactTypeID     string
	ConfigJSON         map[string]any
	Required           bool
	Position           int
}

type FlowEdge struct {
	From string
	To   string
	When string
	Kind string
}

// FlowDefinition is a parsed flow YAML document.
type FlowDefinition struct {
	ID          string
	Version     string
	Description string
	Mode        string
	Builtin     BuiltinMeta
	Policy      FlowPolicy
	Tools       []string
	Contexts    map[string]FlowContextBinding
	Nodes       []FlowNode
	Edges       []FlowEdge
	// AcceptanceNodes is the flow-defined acceptance boundary (CP-55 P-1,
	// root YAML key `acceptance_nodes`): the declared non-writer node ids
	// that must appear on every forward path from an agent.code writer to
	// the terminal "done" state. Optional and empty for every pre-CP-55
	// flow, since none declare an agent.code node yet; a flow that does
	// declare one must set this to at least one valid id (see
	// ValidateFlowSafetyTopology).
	AcceptanceNodes []string
	// ContextProfiles is the CP-62 P-5 (Task-341) per-node context profile
	// map (root YAML key `contextProfiles`). Nodes reference entries by name
	// via FlowNode.ContextProfile; validation (runner.ValidateFlowContextSources)
	// fails flow load on an unknown profile ref or unknown profile source.
	ContextProfiles map[string]ContextProfile
}

// ToolFace describes a declared tool face.
type ToolFace struct {
	ID          string
	Kind        string
	Version     string
	Description string
	ExposesTool bool
	MapsTo      string
	Input       map[string]any
	StatusMap   map[string]string
	PayloadMap  map[string]string
}

// ContextArtifact describes a typed context declaration.
type ContextArtifact struct {
	ID               string
	Version          string
	ArtifactType     string
	Producer         string
	Description      string
	Sources          []string
	Constraints      map[string]bool
	RenderedTemplate string
}

// PromptTemplate is a render-only markdown template.
type PromptTemplate struct {
	Path     string
	Contents string
}

var behaviorAliases = map[string]string{
	"agent.delegate":                        "agent.delegate",
	"coding":                                "agent.delegate",
	"implementation":                        "agent.delegate",
	"code":                                  "agent.delegate",
	"hub.inline":                            "hub.inline",
	"hub.synthesize":                        "hub.inline",
	"context.produce":                       "context.produce",
	"context.deterministic_feature_package": "context.produce",
	"context.deterministic_feature_context": "context.produce",
	"plan":                                  "context.produce",
	"planning":                              "context.produce",
	"design":                                "context.produce",
	"context.render":                        "context.render",
	"context.prompt_handoff":                "context.render",
	"command.validate":                      "command.validate",
	"validation.command":                    "command.validate",
	"validation.summarize":                  "validation.summarize",
	"artifact.audit_draft":                  "artifact.audit_draft",
	"audit.draft":                           "artifact.audit_draft",
	"telegram.notify":                       "telegram.notify",
	"notify.telegram":                       "telegram.notify",
	"hub.notify":                            "hub.notify",
	"notify.hub":                            "hub.notify",
	"flow.control":                          "flow.control",
	"flow.control_tool":                     "flow.control",
	"user.confirm":                          "user.confirm",
	// CP-55 P-1 (Task-263): explicit writer/freeze behavior ids. Self-mapping
	// only — no pre-existing alias is touched or retargeted, and nothing
	// else may alias onto these ids: agent.code writer status is a
	// graph-topology invariant (see ValidateFlowSafetyTopology), not an
	// interchangeable synonym like "coding"/"code".
	"agent.code":      "agent.code",
	"contract.freeze": "contract.freeze",
	// CP-64 P-2 (Task-365): the reproduce-first node behavior. Self-mapped plus
	// two documented aliases (behaviors/registry.yaml lists the same set).
	// Like agent.code, agent.reproduce is a graph-topology marker, not an
	// interchangeable synonym of agent.delegate: the runner's gate hook
	// activates the r-reproduce rule on this behavior, so an alias must never
	// resolve onto it from an unrelated id.
	"agent.reproduce":  "agent.reproduce",
	"reproduce":        "agent.reproduce",
	"reproducing_test": "agent.reproduce",
	// CP-67 P-3 (Task-380/B-3): the Contract-First Scaffold TDD node
	// behavior. Self-mapped plus two documented aliases (behaviors/
	// registry.yaml lists the same set). Like agent.code/agent.reproduce,
	// agent.scaffold is a graph-topology marker — the runner's gate hook
	// arms r-scaffold-red and snapshots signatures on this behavior, so an
	// unrelated id must never alias onto it.
	"agent.scaffold": "agent.scaffold",
	"scaffold":       "agent.scaffold",
	"scaffold_tdd":   "agent.scaffold",
	// CP-65 P-3 (Task-370): tournament inline behaviors. Self-mapped; the
	// runtime contract lives in runner/tournament_behavior.go and the
	// reference docs in behaviors/registry.yaml.
	"tournament.arbiter": "tournament.arbiter",
	"tournament.merge":   "tournament.merge",
}

// NormalizeBehaviorID maps known aliases onto canonical behavior IDs.
func NormalizeBehaviorID(id string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return "", false
	}
	canonical, ok := behaviorAliases[key]
	return canonical, ok
}

// ModelProviderKey maps a model name to its provider key string — the single
// source of truth for the model→provider prefix table (Task-320). The
// runner's providerKeyFromModel delegates to this function so pack-load
// validation and runtime resolution can never drift apart. Mirrors the prefix
// logic in resolvePromptExecutionAdapter (gpt-→codex, claude-→claude,
// gemini-/auto-gemini-→gemini, grok-→grok, opencode-→opencode).
// Returns ("", false) for an unrecognized model.
func ModelProviderKey(model string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-"):
		return "codex", true
	case strings.HasPrefix(m, "gemini-"), strings.HasPrefix(m, "auto-gemini-"):
		return "gemini", true
	case strings.HasPrefix(m, "claude-"):
		return "claude", true
	case strings.HasPrefix(m, "grok-"), m == "grok-build":
		// Appended last (CP-46 P-0): existing prefix cases above are unchanged.
		return "grok", true
	case strings.HasPrefix(m, "opencode/"), strings.HasPrefix(m, "opencode-go/"):
		// Appended last (CP-57 P-0): existing prefix cases above are unchanged.
		return "opencode", true
	}
	return "", false
}

// LoadBuiltinPack returns the embedded FlowPilot reference pack.
func LoadBuiltinPack() (Pack, error) {
	return LoadPackFS(embeddedPackFS, defaultPackRoot)
}

// LoadBuiltinAgents returns the embedded built-in agent specs.
func LoadBuiltinAgents() ([]AgentSpec, error) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		return nil, err
	}
	return append([]AgentSpec(nil), pack.Agents...), nil
}

// ReadBuiltinFlowRaw returns the raw YAML bytes for a built-in flow file,
// keyed by its manifest-relative path (e.g. "flows/review-loop.yaml"). Mirror
// sync hashes these bytes to detect pack content changes without depending on
// Go struct field order or map iteration order for hash stability.
func ReadBuiltinFlowRaw(relPath string) ([]byte, error) {
	return fs.ReadFile(embeddedPackFS, path.Join(defaultPackRoot, relPath))
}

// LoadBuiltinPrompt returns the embedded prompt template that matches relPath.
// The second return value reports whether the prompt was found.
func LoadBuiltinPrompt(relPath string) (PromptTemplate, bool, error) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		return PromptTemplate{}, false, err
	}
	want := path.Clean(strings.TrimSpace(relPath))
	for _, prompt := range pack.Prompts {
		if path.Clean(prompt.Path) == want {
			return prompt, true, nil
		}
	}
	return PromptTemplate{}, false, nil
}

// LoadBuiltinToolFace returns the embedded pack's declared tool face by id
// (e.g. "submit_review_outcome"). The second return value reports whether a
// face with that id was found.
func LoadBuiltinToolFace(id string) (ToolFace, bool, error) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		return ToolFace{}, false, err
	}
	want := strings.TrimSpace(id)
	for _, tool := range pack.Tools {
		if tool.ID == want {
			return tool, true, nil
		}
	}
	return ToolFace{}, false, nil
}

// LoadPackFS loads and validates a pack rooted at root inside fsys.
func LoadPackFS(fsys fs.FS, root string) (Pack, error) {
	root = strings.Trim(strings.TrimSpace(root), string(filepath.Separator))
	if root == "" {
		root = "."
	}

	manifestPath := path.Join(root, "manifest.yaml")
	manifest, err := LoadManifestFS(fsys, manifestPath)
	if err != nil {
		return Pack{}, err
	}
	if err := ValidateManifestFS(fsys, root, manifest); err != nil {
		return Pack{}, err
	}

	pack := Pack{Manifest: manifest}
	for _, rel := range manifest.Agents {
		spec, err := LoadAgentSpecFS(fsys, path.Join(root, rel))
		if err != nil {
			return Pack{}, err
		}
		pack.Agents = append(pack.Agents, spec)
	}
	// BUG-NOTE-CP42 #5: ValidateManifestFS's seenFlowIDs is keyed by the
	// manifest entry's file path, which manifest.yaml already guarantees is
	// unique per entry — it never actually checked the flow file's own
	// content-level `id:` field. Two different files could both declare
	// `id: review-loop` and pass validation, leaving the resolver/picker
	// ambiguous about which one "review-loop" means. Check the parsed
	// FlowDefinition.ID for cross-file duplicates here, where every flow has
	// actually been loaded.
	seenFlowContentIDs := make(map[string]string, len(manifest.Flows)) // def.ID -> first-seen path
	for _, flow := range manifest.Flows {
		def, err := LoadFlowFS(fsys, path.Join(root, flow.Path))
		if err != nil {
			return Pack{}, err
		}
		if prevPath, exists := seenFlowContentIDs[def.ID]; exists {
			return Pack{}, fmt.Errorf("duplicate flow id %q declared in both %q and %q", def.ID, prevPath, flow.Path)
		}
		seenFlowContentIDs[def.ID] = flow.Path
		// BUG-NOTE-CP42 #22: the manifest and each flow file both declare
		// selectableIn/chatSubModes/cloneable/editable/chatBaseline
		// independently, but the picker/mirror path
		// (chat_builtin_orchestration.go) only ever reads def.Builtin.* (the
		// flow file's own copy) — nothing checks the two stay in sync. A
		// manifest-only edit to one of these would silently do nothing at
		// runtime, misleading anyone reading the manifest about the pack's
		// actual behavior. Fail fast on any disagreement instead.
		if err := validateManifestFlowMatchesDefinition(flow, def); err != nil {
			return Pack{}, err
		}
		pack.Flows = append(pack.Flows, def)
	}
	for _, toolPath := range manifest.Tools {
		tool, err := LoadToolFaceFS(fsys, path.Join(root, toolPath))
		if err != nil {
			return Pack{}, err
		}
		pack.Tools = append(pack.Tools, tool)
	}
	for _, ctxPath := range manifest.Contexts {
		ctx, err := LoadContextArtifactFS(fsys, path.Join(root, ctxPath))
		if err != nil {
			return Pack{}, err
		}
		pack.Contexts = append(pack.Contexts, ctx)
	}
	// BUG-NOTE-CP42 #6 (Task-173 T-8): prompt/context refs must resolve to a
	// manifest-declared entry, not just an existing file on disk — a context
	// or node referencing a real file that's simply missing from
	// manifest.Prompts would otherwise load fine today but silently drop off
	// the manifest's own bookkeeping of what the pack depends on.
	declaredPrompts := make(map[string]struct{}, len(manifest.Prompts))
	for _, p := range manifest.Prompts {
		declaredPrompts[p] = struct{}{}
	}
	for _, ctx := range pack.Contexts {
		if ctx.RenderedTemplate == "" {
			continue
		}
		if _, ok := declaredPrompts[ctx.RenderedTemplate]; !ok {
			return Pack{}, fmt.Errorf("context %q render.promptTemplate %q is not declared in manifest.prompts", ctx.ID, ctx.RenderedTemplate)
		}
	}
	for _, flow := range pack.Flows {
		for _, node := range flow.Nodes {
			if node.PromptTemplate == "" {
				continue
			}
			if _, ok := declaredPrompts[node.PromptTemplate]; !ok {
				return Pack{}, fmt.Errorf("flow %q node %q promptTemplate %q is not declared in manifest.prompts", flow.ID, node.ID, node.PromptTemplate)
			}
		}
	}
	for _, promptPath := range manifest.Prompts {
		b, err := fs.ReadFile(fsys, path.Join(root, promptPath))
		if err != nil {
			return Pack{}, fmt.Errorf("read prompt %q: %w", promptPath, err)
		}
		pack.Prompts = append(pack.Prompts, PromptTemplate{
			Path:     promptPath,
			Contents: string(b),
		})
	}
	return pack, nil
}

// LoadManifestFS parses manifest.yaml from fsys.
func LoadManifestFS(fsys fs.FS, manifestPath string) (Manifest, error) {
	raw, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}
	tree, err := parseYAMLDocument(string(raw))
	if err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
	}
	rootMap, ok := tree.(map[string]any)
	if !ok {
		return Manifest{}, fmt.Errorf("parse manifest %q: expected map document", manifestPath)
	}
	manifest, err := manifestFromMap(rootMap)
	if err != nil {
		return Manifest{}, fmt.Errorf("decode manifest %q: %w", manifestPath, err)
	}
	return manifest, nil
}

// ValidateManifestFS validates manifest references and pack invariants.
func ValidateManifestFS(fsys fs.FS, root string, manifest Manifest) error {
	seenFlowIDs := make(map[string]struct{}, len(manifest.Flows))
	for _, rel := range manifest.Agents {
		if err := ensureFileExists(fsys, path.Join(root, rel)); err != nil {
			return err
		}
	}
	for _, flow := range manifest.Flows {
		if strings.TrimSpace(flow.Path) == "" {
			return errors.New("manifest flow entry missing path")
		}
		if _, exists := seenFlowIDs[flow.Path]; exists {
			return fmt.Errorf("duplicate flow path %q in manifest", flow.Path)
		}
		seenFlowIDs[flow.Path] = struct{}{}
		if err := ensureFileExists(fsys, path.Join(root, flow.Path)); err != nil {
			return err
		}
		if len(flow.SelectableIn) > 0 {
			for _, mode := range flow.SelectableIn {
				switch strings.ToLower(strings.TrimSpace(mode)) {
				case "chat", "flow":
				default:
					return fmt.Errorf("flow %q has invalid selectableIn value %q", flow.Path, mode)
				}
			}
		}
		if flow.ChatBaseline {
			for _, mode := range flow.SelectableIn {
				if strings.EqualFold(mode, "chat") {
					return fmt.Errorf("flow %q cannot be chatBaseline and selectable in chat", flow.Path)
				}
			}
		}
	}
	for _, toolPath := range manifest.Tools {
		if err := ensureFileExists(fsys, path.Join(root, toolPath)); err != nil {
			return err
		}
	}
	for _, ctxPath := range manifest.Contexts {
		if err := ensureFileExists(fsys, path.Join(root, ctxPath)); err != nil {
			return err
		}
	}
	for _, promptPath := range manifest.Prompts {
		if err := ensureFileExists(fsys, path.Join(root, promptPath)); err != nil {
			return err
		}
	}
	return nil
}

// LoadAgentSpecFS parses a markdown agent definition.
func LoadAgentSpecFS(fsys fs.FS, agentPath string) (AgentSpec, error) {
	raw, err := fs.ReadFile(fsys, agentPath)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("read agent %q: %w", agentPath, err)
	}
	spec, err := parseAgentSpec(agentPath, string(raw), "flowpilot")
	if err != nil {
		return AgentSpec{}, fmt.Errorf("parse agent %q: %w", agentPath, err)
	}
	return spec, nil
}

// LoadFlowFS parses a flow YAML file.
func LoadFlowFS(fsys fs.FS, flowPath string) (FlowDefinition, error) {
	raw, err := fs.ReadFile(fsys, flowPath)
	if err != nil {
		return FlowDefinition{}, fmt.Errorf("read flow %q: %w", flowPath, err)
	}
	tree, err := parseYAMLDocument(string(raw))
	if err != nil {
		return FlowDefinition{}, fmt.Errorf("parse flow %q: %w", flowPath, err)
	}
	rootMap, ok := tree.(map[string]any)
	if !ok {
		return FlowDefinition{}, fmt.Errorf("parse flow %q: expected map document", flowPath)
	}
	def, err := flowFromMap(rootMap)
	if err != nil {
		return FlowDefinition{}, fmt.Errorf("decode flow %q: %w", flowPath, err)
	}
	if err := ValidateFlowDefinition(def); err != nil {
		return FlowDefinition{}, fmt.Errorf("validate flow %q: %w", flowPath, err)
	}
	return def, nil
}

// LoadToolFaceFS parses a declared tool face YAML file.
func LoadToolFaceFS(fsys fs.FS, toolPath string) (ToolFace, error) {
	raw, err := fs.ReadFile(fsys, toolPath)
	if err != nil {
		return ToolFace{}, fmt.Errorf("read tool face %q: %w", toolPath, err)
	}
	tree, err := parseYAMLDocument(string(raw))
	if err != nil {
		return ToolFace{}, fmt.Errorf("parse tool face %q: %w", toolPath, err)
	}
	rootMap, ok := tree.(map[string]any)
	if !ok {
		return ToolFace{}, fmt.Errorf("parse tool face %q: expected map document", toolPath)
	}
	face, err := toolFaceFromMap(rootMap)
	if err != nil {
		return ToolFace{}, fmt.Errorf("decode tool face %q: %w", toolPath, err)
	}
	return face, nil
}

// LoadContextArtifactFS parses a typed context artifact declaration.
func LoadContextArtifactFS(fsys fs.FS, contextPath string) (ContextArtifact, error) {
	raw, err := fs.ReadFile(fsys, contextPath)
	if err != nil {
		return ContextArtifact{}, fmt.Errorf("read context artifact %q: %w", contextPath, err)
	}
	tree, err := parseYAMLDocument(string(raw))
	if err != nil {
		return ContextArtifact{}, fmt.Errorf("parse context artifact %q: %w", contextPath, err)
	}
	rootMap, ok := tree.(map[string]any)
	if !ok {
		return ContextArtifact{}, fmt.Errorf("parse context artifact %q: expected map document", contextPath)
	}
	ctx, err := contextArtifactFromMap(rootMap)
	if err != nil {
		return ContextArtifact{}, fmt.Errorf("decode context artifact %q: %w", contextPath, err)
	}
	return ctx, nil
}

// ensureFileExists validates that a referenced file exists in fsys.
func ensureFileExists(fsys fs.FS, relPath string) error {
	if _, err := fs.Stat(fsys, relPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("missing pack reference %q", relPath)
		}
		return fmt.Errorf("stat pack reference %q: %w", relPath, err)
	}
	return nil
}

func manifestFromMap(m map[string]any) (Manifest, error) {
	manifest := Manifest{
		ID:            stringField(m, "id"),
		Version:       stringField(m, "version"),
		Description:   stringField(m, "description"),
		SchemaVersion: intField(m, "schemaVersion"),
		Agents:        stringSliceField(m, "agents"),
		Tools:         stringSliceField(m, "tools"),
		Contexts:      stringSliceField(m, "contexts"),
		Prompts:       stringSliceField(m, "prompts"),
	}
	if comp, ok := mapField(m, "compatibility"); ok {
		manifest.Compatibility = ManifestCompatibility{
			MinRunnerVersion:     stringField(comp, "minRunnerVersion"),
			FallbackToGoBuiltins: boolField(comp, "fallbackToGoBuiltins"),
		}
	}
	flowsRaw, ok := sliceField(m, "flows")
	if !ok {
		return Manifest{}, errors.New("manifest missing flows")
	}
	manifest.Flows = make([]ManifestFlow, 0, len(flowsRaw))
	for _, item := range flowsRaw {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return Manifest{}, errors.New("manifest flow entries must be maps")
		}
		manifest.Flows = append(manifest.Flows, ManifestFlow{
			Path:         stringField(itemMap, "path"),
			Visibility:   stringField(itemMap, "visibility"),
			Editable:     boolField(itemMap, "editable"),
			ChatBaseline: boolField(itemMap, "chatBaseline"),
			SelectableIn: stringSliceField(itemMap, "selectableIn"),
			ChatSubModes: stringSliceField(itemMap, "chatSubModes"),
			Cloneable:    boolField(itemMap, "cloneable"),
		})
	}
	if manifest.ID == "" {
		return Manifest{}, errors.New("manifest missing id")
	}
	return manifest, nil
}

func flowFromMap(m map[string]any) (FlowDefinition, error) {
	def := FlowDefinition{
		ID:              stringField(m, "id"),
		Version:         stringField(m, "version"),
		Description:     stringField(m, "description"),
		Mode:            stringField(m, "mode"),
		Tools:           stringSliceField(m, "tools"),
		Contexts:        make(map[string]FlowContextBinding),
		AcceptanceNodes: stringSliceField(m, "acceptance_nodes"),
	}
	if builtin, ok := mapField(m, "builtin"); ok {
		def.Builtin = BuiltinMeta{
			Editable:     boolField(builtin, "editable"),
			ChatBaseline: boolField(builtin, "chatBaseline"),
			SelectableIn: stringSliceField(builtin, "selectableIn"),
			ChatSubModes: stringSliceField(builtin, "chatSubModes"),
			Cloneable:    boolField(builtin, "cloneable"),
		}
		if mirror, ok := mapField(builtin, "mirror"); ok {
			def.Builtin.Mirror = MirrorRule{
				Required: boolField(mirror, "required"),
				Source:   stringField(mirror, "source"),
			}
		}
		if ui, ok := mapField(builtin, "chatUI"); ok {
			rule := ChatUIRule{Placement: stringField(ui, "placement")}
			if showWhen, ok := mapField(ui, "showWhen"); ok {
				rule.ShowWhen = make(map[string]string, len(showWhen))
				for k, v := range showWhen {
					rule.ShowWhen[k] = fmt.Sprint(v)
				}
			}
			def.Builtin.ChatUI = rule
		}
	}
	if policy, ok := mapField(m, "policy"); ok {
		def.Policy = FlowPolicy{
			Cap:             intField(policy, "cap"),
			OnCap:           stringField(policy, "onCap"),
			ExtendBy:        intField(policy, "extendBy"),
			ExtendMax:       intField(policy, "extendMax"),
			StallTimeoutSec: intField(policy, "stallTimeoutSec"),
			NegotiationCap:  intField(policy, "negotiationCap"),
		}
	}
	if contexts, ok := mapField(m, "contexts"); ok {
		for name, raw := range contexts {
			binding := FlowContextBinding{}
			if rawMap, ok := raw.(map[string]any); ok {
				binding.Ref = stringField(rawMap, "ref")
				binding.Sources = stringSliceField(rawMap, "sources")
			} else {
				binding.Ref = fmt.Sprint(raw)
			}
			def.Contexts[name] = binding
		}
	}
	// CP-62 P-5 (Task-341): per-node context profiles. Fail-closed on a
	// malformed entry — a profile that parses to nothing must not silently
	// disable a node's context.
	if profiles, ok := mapField(m, "contextProfiles"); ok {
		def.ContextProfiles = make(map[string]ContextProfile, len(profiles))
		for name, raw := range profiles {
			pm, isMap := raw.(map[string]any)
			if !isMap {
				return FlowDefinition{}, fmt.Errorf("flow %q contextProfiles.%q must be a map", def.ID, name)
			}
			def.ContextProfiles[name] = ContextProfile{
				Name:             name,
				CandidateSources: stringSliceField(pm, "candidateSources"),
				MaxTokens:        intField(pm, "maxTokens"),
			}
		}
	}
	nodesRaw, ok := sliceField(m, "nodes")
	if ok {
		def.Nodes = make([]FlowNode, 0, len(nodesRaw))
		for _, item := range nodesRaw {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return FlowDefinition{}, errors.New("flow nodes must be maps")
			}
			node, err := flowNodeFromMap(itemMap)
			if err != nil {
				return FlowDefinition{}, err
			}
			def.Nodes = append(def.Nodes, node)
		}
	}
	edgesRaw, ok := sliceField(m, "edges")
	if ok {
		def.Edges = make([]FlowEdge, 0, len(edgesRaw))
		for _, item := range edgesRaw {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return FlowDefinition{}, errors.New("flow edges must be maps")
			}
			def.Edges = append(def.Edges, FlowEdge{
				From: stringField(itemMap, "from"),
				To:   stringField(itemMap, "to"),
				When: stringField(itemMap, "when"),
				Kind: stringField(itemMap, "kind"),
			})
		}
	}
	if def.ID == "" {
		return FlowDefinition{}, errors.New("flow missing id")
	}
	return def, nil
}

func flowNodeFromMap(m map[string]any) (FlowNode, error) {
	node := FlowNode{
		ID:             stringField(m, "id"),
		Run:            stringField(m, "run"),
		Lifecycle:      stringField(m, "lifecycle"),
		Behavior:       stringField(m, "behavior"),
		Agent:          stringField(m, "agent"),
		Join:           stringField(m, "join"),
		Cohort:         stringField(m, "cohort"),
		PromptTemplate: stringField(m, "promptTemplate"),
		Model:          strings.TrimSpace(stringField(m, "model")),
		Posture:        strings.TrimSpace(stringField(m, "posture")),
		ContextProfile: strings.TrimSpace(stringField(m, "contextProfile")),
		DependsOn:      stringSliceField(m, "dependsOn"),
		ContextSources: stringSliceField(m, "contextSources"),
	}
	// CP-65 P-3 (Task-370): free-form node config for tournament nodes.
	// Absent key leaves Config nil (every pre-CP-65 flow unaffected).
	if config, ok := mapField(m, "config"); ok {
		node.Config = config
	}
	// CP-58 Task-307: pack-level typed artifact bindings. The FS loader never
	// parsed these before (only the Supabase mirror path populated them via
	// step_artifact_bindings), so a YAML-declared binding silently vanished —
	// harness plan outputs (plan_md/cp_md/task_md) are declared in pack YAML
	// and must survive LoadFlowFS. Malformed entries fail the flow load
	// instead of being skipped (fail-closed, SD-23 D-11).
	if rawBindings, ok := sliceField(m, "artifactBindings"); ok {
		node.ArtifactBindings = make([]FlowArtifactBinding, 0, len(rawBindings))
		for _, item := range rawBindings {
			itemMap, isMap := item.(map[string]any)
			if !isMap {
				return FlowNode{}, fmt.Errorf("flow node %q: artifactBindings entries must be maps", node.ID)
			}
			binding := FlowArtifactBinding{
				Direction:          stringField(itemMap, "direction"),
				SlotName:           stringField(itemMap, "slotName"),
				ArtifactInstanceID: stringField(itemMap, "artifactInstanceId"),
				ArtifactTypeID:     stringField(itemMap, "artifactTypeId"),
				Required:           boolField(itemMap, "required"),
				Position:           intField(itemMap, "position"),
			}
			if config, ok := mapField(itemMap, "config"); ok {
				binding.ConfigJSON = config
			}
			node.ArtifactBindings = append(node.ArtifactBindings, binding)
		}
	}
	return node, nil
}

// stringSetEqual reports whether a and b contain the same elements,
// regardless of order or duplicates.
func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

// artifactOutputRequirementsPrefix is the only workspace-relative directory a
// file_artifact OUTPUT binding may target (CP-58 Task-307): harness plan
// outputs are requirements documents, so a binding pointing outside
// requirements/ (or escaping the workspace entirely) is a load-time error,
// not a mid-run surprise.
const artifactOutputRequirementsPrefix = "requirements/"

// validateArtifactOutputPath checks one file_artifact OUTPUT path or
// pathTemplate: workspace-relative, no traversal, no Windows drive/separator,
// and always under requirements/. Task-307's deterministic fail — an
// OUTPUT path escaping requirements/ must fail validation, never the run.
func validateArtifactOutputPath(pathTemplate string) error {
	trimmed := strings.TrimSpace(pathTemplate)
	if trimmed == "" {
		return fmt.Errorf("file_artifact OUTPUT path is empty")
	}
	if strings.Contains(trimmed, "\\") || strings.Contains(trimmed, ":") {
		return fmt.Errorf("file_artifact OUTPUT path %q must be a workspace-relative path under requirements/", pathTemplate)
	}
	normalized := path.Clean(filepath.ToSlash(trimmed))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("file_artifact OUTPUT path %q must be a workspace-relative path under requirements/", pathTemplate)
	}
	if !strings.HasPrefix(normalized, artifactOutputRequirementsPrefix) {
		return fmt.Errorf("file_artifact OUTPUT path %q must be under requirements/", pathTemplate)
	}
	return nil
}

// artifactOutputPathCandidates lists the concrete paths and/or pathTemplate a
// binding's config declares (either form alone is acceptable; a template is
// resolved by the writer at authoring time, concrete paths are gate-checked).
func artifactOutputPathCandidates(config map[string]any) []string {
	var out []string
	if config != nil {
		if raw, ok := config["paths"].([]any); ok {
			for _, item := range raw {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
		}
		if s, ok := config["pathTemplate"].(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// validateArtifactOutputBindingPaths runs validateArtifactOutputPath over
// every path a file_artifact OUTPUT binding declares. A binding with neither
// form fails: an OUTPUT slot with no writable target is a malformed contract.
func validateArtifactOutputBindingPaths(b FlowArtifactBinding) error {
	candidates := artifactOutputPathCandidates(b.ConfigJSON)
	if len(candidates) == 0 {
		return fmt.Errorf("file_artifact OUTPUT binding %q declares neither a paths list nor a pathTemplate", b.SlotName)
	}
	for _, candidate := range candidates {
		if err := validateArtifactOutputPath(candidate); err != nil {
			return err
		}
	}
	return nil
}

// validateManifestFlowMatchesDefinition rejects a pack where a manifest flow
// entry's metadata disagrees with the same flow file's own builtin.*
// declaration (BUG-NOTE-CP42 #22) — the runtime picker/mirror path only ever
// reads the flow file's copy, so a manifest-only edit that falls out of sync
// would otherwise silently have no effect.
func validateManifestFlowMatchesDefinition(flow ManifestFlow, def FlowDefinition) error {
	if flow.Editable != def.Builtin.Editable {
		return fmt.Errorf("flow %q: manifest editable=%v disagrees with flow file's builtin.editable=%v", def.ID, flow.Editable, def.Builtin.Editable)
	}
	if flow.ChatBaseline != def.Builtin.ChatBaseline {
		return fmt.Errorf("flow %q: manifest chatBaseline=%v disagrees with flow file's builtin.chatBaseline=%v", def.ID, flow.ChatBaseline, def.Builtin.ChatBaseline)
	}
	if flow.Cloneable != def.Builtin.Cloneable {
		return fmt.Errorf("flow %q: manifest cloneable=%v disagrees with flow file's builtin.cloneable=%v", def.ID, flow.Cloneable, def.Builtin.Cloneable)
	}
	if !stringSetEqual(flow.SelectableIn, def.Builtin.SelectableIn) {
		return fmt.Errorf("flow %q: manifest selectableIn=%v disagrees with flow file's builtin.selectableIn=%v", def.ID, flow.SelectableIn, def.Builtin.SelectableIn)
	}
	if !stringSetEqual(flow.ChatSubModes, def.Builtin.ChatSubModes) {
		return fmt.Errorf("flow %q: manifest chatSubModes=%v disagrees with flow file's builtin.chatSubModes=%v", def.ID, flow.ChatSubModes, def.Builtin.ChatSubModes)
	}
	return nil
}

// ValidateFlowDefinition validates a parsed FlowDefinition's internal
// consistency (node ids, edge endpoints/kinds, dependsOn references, unique
// continue back-edges). Exported so callers outside this package — notably
// the Supabase-backed FlowDefinitionStore, whose rows never pass through
// LoadFlowFS at all (BUG-NOTE-CP42 #28) — can apply the same fail-fast
// checks the embedded pack gets for free at load time.
func ValidateFlowDefinition(def FlowDefinition) error {
	if def.Builtin.ChatBaseline {
		for _, mode := range def.Builtin.SelectableIn {
			if strings.EqualFold(mode, "chat") {
				return fmt.Errorf("flow %q cannot be chatBaseline and selectable in chat", def.ID)
			}
		}
	}
	nodeIDs := make(map[string]struct{}, len(def.Nodes))
	// CP-67 P-5 (B-10): the renegotiation loop budget is phase-scoped and
	// bounded — an unbounded or zero/negative declared cap would either
	// disable the guard or loop forever, so it fails closed at load.
	if def.Policy.NegotiationCap != 0 && (def.Policy.NegotiationCap < 1 || def.Policy.NegotiationCap > 20) {
		return fmt.Errorf("flow %q policy.negotiationCap = %d, want 1..20 (0 = runner default 5)", def.ID, def.Policy.NegotiationCap)
	}
	for _, node := range def.Nodes {
		if node.ID == "" {
			return fmt.Errorf("flow %q has a node without id", def.ID)
		}
		if _, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("flow %q has duplicate node id %q", def.ID, node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
		if node.Behavior != "" {
			if _, ok := NormalizeBehaviorID(node.Behavior); !ok {
				return fmt.Errorf("flow %q node %q uses unknown behavior %q", def.ID, node.ID, node.Behavior)
			}
		}
		if lifecycle := strings.ToLower(strings.TrimSpace(node.Lifecycle)); lifecycle != "" {
			switch lifecycle {
			case "once", "reinvoke", "spawn":
			default:
				return fmt.Errorf("flow %q node %q has invalid lifecycle %q", def.ID, node.ID, node.Lifecycle)
			}
		}
		// Task-320: pack-declared per-node model tier. Only agent.delegate
		// nodes consume a model at runtime (resolveFlowNodeModel) — a model on
		// any other behavior would be silently ignored, so it fails fast here.
		// A delegate model that maps to no known provider is a typo that would
		// otherwise inherit-and-confuse, so it fails fast too.
		// CP-67 P-3 (B-5): agent.scaffold nodes are scaffold-architect
		// delegate children (the runtime handler is behaviorAgentDelegate),
		// so their High-Reasoning `model:` tier is consumed the same way;
		// agent.code is accepted for parity so a pack author can pin the
		// coder tier without tripping the load check.
		if strings.TrimSpace(node.Model) != "" {
			canonical, ok := NormalizeBehaviorID(node.Behavior)
			if !ok || (canonical != "agent.delegate" && canonical != "agent.scaffold" && canonical != "agent.code") {
				return fmt.Errorf("flow %q node %q declares model %q but behavior %q never consumes a model (only agent.delegate does)", def.ID, node.ID, node.Model, node.Behavior)
			}
			if _, ok := ModelProviderKey(node.Model); !ok {
				return fmt.Errorf("flow %q node %q declares unknown model %q (no provider prefix match)", def.ID, node.ID, node.Model)
			}
		}
	}
	// BUG-NOTE-CP42 #33: dependsOn references another node's id within the same
	// flow (per the add_flow_engine_attrs_to_workflows migration's own comment),
	// but this was never actually checked — a typo'd dependsOn entry would
	// silently pass load/mirror instead of failing fast.
	for _, node := range def.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := nodeIDs[dep]; !ok {
				return fmt.Errorf("flow %q node %q dependsOn references missing node %q", def.ID, node.ID, dep)
			}
		}
	}
	// CP-58 Task-307: fail closed on malformed artifact bindings at flow-load
	// time — SD-23 D-11 requires deterministic binding validation, and a
	// file_artifact OUTPUT slot is the harness plan outputs' write contract, so
	// an unusable path template must break the load, not surface later as an
	// opaque provider or gate failure mid-run.
	for _, node := range def.Nodes {
		for _, b := range node.ArtifactBindings {
			direction := strings.ToLower(strings.TrimSpace(b.Direction))
			switch direction {
			case "input", "output":
			default:
				return fmt.Errorf("flow %q node %q artifact binding %q has invalid direction %q", def.ID, node.ID, b.SlotName, b.Direction)
			}
			if strings.TrimSpace(b.ArtifactInstanceID) == "" {
				return fmt.Errorf("flow %q node %q artifact binding %q is missing artifactInstanceId", def.ID, node.ID, b.SlotName)
			}
			if direction == "output" && strings.HasPrefix(strings.TrimSpace(b.ArtifactTypeID), "file_artifact") {
				if err := validateArtifactOutputBindingPaths(b); err != nil {
					return fmt.Errorf("flow %q node %q: %w", def.ID, node.ID, err)
				}
			}
		}
	}
	terminalIDs := map[string]struct{}{
		"done":     {},
		"ask_user": {},
	}
	// BUG-NOTE-CP42 #25 / CP-58 Task-304: the live back-edge resolution path
	// (resolveContinueBackEdgeTarget) is source-aware, so a flow may declare
	// ONE back-edge per (from, when) pair — task-harness needs two
	// when:continue kind:back edges (plan_synthesis -> plan_writer and
	// validate -> implement) to run a plan review loop and a code loop in the
	// same flow. A true duplicate — same From AND same when — would still be
	// ambiguous at resolve time, so it fails fast here instead of silently
	// routing "continue" to whichever target sorts first.
	backEdgeSources := make(map[string]string, len(def.Edges)) // "from\x00when" -> from
	for _, edge := range def.Edges {
		if edge.From == "" || edge.To == "" {
			return fmt.Errorf("flow %q has edge with empty endpoint", def.ID)
		}
		if _, ok := nodeIDs[edge.From]; !ok {
			return fmt.Errorf("flow %q edge from %q references missing node", def.ID, edge.From)
		}
		if _, ok := nodeIDs[edge.To]; !ok {
			if _, term := terminalIDs[edge.To]; !term {
				return fmt.Errorf("flow %q edge to %q references missing node", def.ID, edge.To)
			}
		}
		kind := strings.ToLower(strings.TrimSpace(edge.Kind))
		if edge.Kind != "" {
			switch kind {
			case "forward", "back":
			default:
				return fmt.Errorf("flow %q edge %q->%q has invalid kind %q", def.ID, edge.From, edge.To, edge.Kind)
			}
		}
		if kind == "back" {
			dedup := edge.From + "\x00" + edge.When
			if prev, exists := backEdgeSources[dedup]; exists {
				return fmt.Errorf("flow %q has duplicate back-edge for status %q from %q (already declared once)", def.ID, edge.When, prev)
			}
			backEdgeSources[dedup] = edge.From
		}
	}
	// CP-55 P-1: wired at the same definition-resolution boundary as the
	// checks above so it runs everywhere ValidateFlowDefinition already
	// does (LoadFlowFS and both FlowDefinitionResolver paths). A flow with
	// no agent.code node always passes, so this is additive for every
	// pre-CP-55 flow (see ValidateFlowSafetyTopology's own doc comment).
	if err := ValidateFlowSafetyTopology(def); err != nil {
		return err
	}
	return nil
}

func toolFaceFromMap(m map[string]any) (ToolFace, error) {
	face := ToolFace{
		ID:          stringField(m, "id"),
		Kind:        stringField(m, "kind"),
		Version:     stringField(m, "version"),
		Description: stringField(m, "description"),
		ExposesTool: boolField(m, "exposesTool"),
		MapsTo:      stringField(m, "mapsTo"),
	}
	if input, ok := mapField(m, "input"); ok {
		face.Input = input
	}
	if statusMap, ok := mapField(m, "statusMap"); ok {
		face.StatusMap = make(map[string]string, len(statusMap))
		for k, v := range statusMap {
			face.StatusMap[k] = fmt.Sprint(v)
		}
	}
	if payloadMap, ok := mapField(m, "payloadMap"); ok {
		face.PayloadMap = make(map[string]string, len(payloadMap))
		for k, v := range payloadMap {
			face.PayloadMap[k] = fmt.Sprint(v)
		}
	}
	return face, nil
}

func contextArtifactFromMap(m map[string]any) (ContextArtifact, error) {
	ctx := ContextArtifact{
		ID:           stringField(m, "id"),
		Version:      stringField(m, "version"),
		ArtifactType: stringField(m, "artifactType"),
		Producer:     stringField(m, "producer"),
		Description:  stringField(m, "description"),
		Sources:      stringSliceField(m, "sources"),
	}
	// BUG-NOTE-CP42 #6: the YAML shape is a nested `render.promptTemplate`
	// field (see flow-context-package.yaml), not a scalar `render` string.
	// The old `stringField(m, "render")` coerced the whole nested map via
	// fmt.Sprint, producing a Go stringified map ("map[promptTemplate:...]")
	// instead of the actual template path.
	if render, ok := mapField(m, "render"); ok {
		ctx.RenderedTemplate = stringField(render, "promptTemplate")
	}
	if constraints, ok := mapField(m, "constraints"); ok {
		ctx.Constraints = make(map[string]bool, len(constraints))
		for k, v := range constraints {
			ctx.Constraints[k] = boolFromAny(v)
		}
	}
	return ctx, nil
}

func parseAgentSpec(path, contents, source string) (AgentSpec, error) {
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	spec := AgentSpec{Source: source, Path: path}
	inFrontMatter := false
	frontMatterDone := false
	inToolsList := false
	body := make([]string, 0, len(lines))
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx == 0 && trimmed == "---" {
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
			if inToolsList {
				if strings.HasPrefix(trimmed, "- ") {
					tool := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
					if tool != "" {
						spec.Tools = append(spec.Tools, tool)
					}
					continue
				}
				inToolsList = false
			}
			switch {
			case strings.HasPrefix(trimmed, "name:"):
				spec.Name = unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "name:")))
			case strings.HasPrefix(trimmed, "description:"):
				spec.Description = unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "description:")))
			case strings.HasPrefix(trimmed, "role:"):
				spec.Role = strings.ToLower(strings.TrimSpace(unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "role:")))))
			case strings.HasPrefix(trimmed, "provider:"):
				spec.Provider = strings.ToLower(strings.TrimSpace(unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "provider:")))))
			case strings.HasPrefix(trimmed, "model_reasoning_effort:"):
				spec.ModelReasoningEffort = strings.ToLower(strings.TrimSpace(unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "model_reasoning_effort:")))))
			case strings.HasPrefix(trimmed, "model:"):
				spec.Model = strings.TrimSpace(unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "model:"))))
			case strings.HasPrefix(trimmed, "tools:"):
				remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, "tools:"))
				if remainder == "" {
					inToolsList = true
				} else {
					spec.Tools = append(spec.Tools, parseInlineTools(remainder)...)
				}
			}
			continue
		}
		if frontMatterDone {
			body = append(body, line)
		}
	}
	if spec.Name == "" {
		spec.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if spec.Role == "" {
		spec.Role = strings.ToLower(spec.Name)
	}
	if spec.Description == "" {
		spec.Description = firstMeaningfulLine(strings.Join(body, "\n"))
	}
	spec.SystemPrompt = strings.TrimSpace(strings.Join(body, "\n"))
	spec.Tools = dedupeStrings(spec.Tools)
	return spec, nil
}

func parseInlineTools(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	parts := strings.Split(value, ",")
	tools := make([]string, 0, len(parts))
	for _, part := range parts {
		tool := strings.TrimSpace(unquote(part))
		if tool != "" {
			tools = append(tools, tool)
		}
	}
	return tools
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		return boolFromAny(v)
	}
	return false
}

func boolFromAny(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	default:
		return false
	}
}

func intField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch x := v.(type) {
		case int:
			return x
		case int64:
			return int(x)
		case float64:
			return int(x)
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
				return n
			}
		}
	}
	return 0
}

func stringSliceField(m map[string]any, key string) []string {
	raw, ok := sliceField(m, key)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func mapField(m map[string]any, key string) (map[string]any, bool) {
	if m == nil {
		return nil, false
	}
	raw, ok := m[key]
	if !ok {
		return nil, false
	}
	cast, ok := raw.(map[string]any)
	return cast, ok
}

func sliceField(m map[string]any, key string) ([]any, bool) {
	if m == nil {
		return nil, false
	}
	raw, ok := m[key]
	if !ok {
		return nil, false
	}
	switch v := raw.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out, true
	default:
		return nil, false
	}
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	value = strings.TrimPrefix(value, "'")
	value = strings.TrimSuffix(value, "'")
	return value
}

type yamlLine struct {
	indent int
	text   string
}

func parseYAMLDocument(src string) (any, error) {
	lines := normalizeYAMLLines(src)
	idx := 0
	return parseYAMLBlock(lines, &idx, 0)
}

func normalizeYAMLLines(src string) []yamlLine {
	rawLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	lines := make([]yamlLine, 0, len(rawLines))
	for _, raw := range rawLines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		trimmed := strings.TrimLeft(raw, " ")
		if strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(trimmed)
		lines = append(lines, yamlLine{indent: indent, text: trimmed})
	}
	return lines
}

func parseYAMLBlock(lines []yamlLine, idx *int, indent int) (any, error) {
	if *idx >= len(lines) {
		return map[string]any{}, nil
	}
	if lines[*idx].indent < indent {
		return map[string]any{}, nil
	}
	if strings.HasPrefix(strings.TrimSpace(lines[*idx].text), "- ") {
		return parseYAMLSeq(lines, idx, indent)
	}
	return parseYAMLMap(lines, idx, indent)
}

func parseYAMLMap(lines []yamlLine, idx *int, indent int) (map[string]any, error) {
	out := map[string]any{}
	for *idx < len(lines) {
		line := lines[*idx]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, fmt.Errorf("unexpected indent %d, expected %d near %q", line.indent, indent, line.text)
		}
		if strings.HasPrefix(line.text, "- ") {
			break
		}
		key, value, ok := splitYAMLKeyValue(line.text)
		if !ok {
			return nil, fmt.Errorf("invalid mapping line %q", line.text)
		}
		*idx = *idx + 1
		if value == "" {
			if *idx >= len(lines) || lines[*idx].indent <= indent {
				out[key] = ""
				continue
			}
			child, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
			if err != nil {
				return nil, err
			}
			out[key] = child
			continue
		}
		out[key] = parseScalar(value)
		if *idx < len(lines) && lines[*idx].indent > indent {
			child, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
			if err != nil {
				return nil, err
			}
			if childMap, ok := child.(map[string]any); ok {
				if existing, ok := out[key].(map[string]any); ok {
					mergeMaps(existing, childMap)
					out[key] = existing
				} else {
					out[key] = childMap
				}
			}
		}
	}
	return out, nil
}

func parseYAMLSeq(lines []yamlLine, idx *int, indent int) ([]any, error) {
	out := make([]any, 0, 4)
	for *idx < len(lines) {
		line := lines[*idx]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, fmt.Errorf("unexpected sequence indent %d, expected %d near %q", line.indent, indent, line.text)
		}
		if !strings.HasPrefix(line.text, "- ") {
			break
		}
		content := strings.TrimSpace(strings.TrimPrefix(line.text, "- "))
		*idx = *idx + 1
		if content == "" {
			if *idx < len(lines) && lines[*idx].indent > indent {
				child, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, child)
			} else {
				out = append(out, "")
			}
			continue
		}
		if key, value, ok := splitYAMLKeyValue(content); ok {
			item := map[string]any{key: parseScalar(value)}
			if *idx < len(lines) && lines[*idx].indent > indent {
				child, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
				if err != nil {
					return nil, err
				}
				if childMap, ok := child.(map[string]any); ok {
					mergeMaps(item, childMap)
				}
			}
			out = append(out, item)
			continue
		}
		item := parseScalar(content)
		if *idx < len(lines) && lines[*idx].indent > indent {
			child, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
			if err != nil {
				return nil, err
			}
			if childMap, ok := child.(map[string]any); ok {
				if scalarStr, ok := item.(string); ok && scalarStr != "" {
					item = map[string]any{"value": scalarStr}
				}
				if itemMap, ok := item.(map[string]any); ok {
					mergeMaps(itemMap, childMap)
					item = itemMap
				} else {
					item = childMap
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:colon])
	value := strings.TrimSpace(line[colon+1:])
	return key, value, true
}

func parseScalar(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
		if inner == "" {
			return []any{}
		}
		parts := splitCommaList(inner)
		out := make([]any, 0, len(parts))
		for _, part := range parts {
			out = append(out, parseScalar(part))
		}
		return out
	}
	if strings.EqualFold(raw, "true") {
		return true
	}
	if strings.EqualFold(raw, "false") {
		return false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return unquote(raw)
}

func splitCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// SortedAgentNames returns agent names in stable order for tests.
func SortedAgentNames(specs []AgentSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	sort.Strings(names)
	return names
}
