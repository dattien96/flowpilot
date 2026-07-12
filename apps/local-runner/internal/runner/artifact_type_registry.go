package runner

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// Built-in artifact type ids (CP-45/SD-23 D-2). Code hardcodes this layer;
// the catalog rows themselves are seeded via migration
// (20260709090000_add_artifact_types_catalog.sql,
// 20260709093000_add_file_artifact_type.sql) and never user-created.
const (
	ArtifactTypeContext = "context_artifact.v1"
	ArtifactTypeFile    = "file_artifact.v1"
)

// resolveArtifactBoundContextSources implements SD-23 D-5/D-6's highest
// precedence tier for context.produce: if node has an OUTPUT binding to a
// context_artifact.v1 instance (SD-23 D-5: "context_artifact.v1 là output
// của Context step" — the context-producing node outputs the instance;
// Coding/Review/Synthesis bind that same instance as their own INPUT), its
// config_json.sources list wins over the node's own step-level
// ContextSources (Task-196), the flow-level contexts.<name>.sources binding
// (Task-194), and the runner default set. ok=false means no context_artifact
// instance is bound here, so the caller falls through to the pre-CP-45
// precedence chain unchanged (CP-44 fallback, SD-23 D-6/F-4) —
// resolveEnabledContextSourceIDs in context_sources_builtin.go is the sole
// caller.
//
// The instance's producer stays exactly BuildFlowContextPackageWithSources /
// ContextSourceRegistry.Collect (SD-23 D-5: compose over SD-22, don't
// reimplement it) — this function only changes WHICH source ids feed that
// existing pipeline.
func resolveArtifactBoundContextSources(node agentpack.FlowNode) ([]string, bool) {
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || b.ArtifactTypeID != ArtifactTypeContext {
			continue
		}
		raw, ok := b.ConfigJSON["sources"].([]any)
		if !ok {
			continue
		}
		ids := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
		if len(ids) > 0 {
			return ids, true
		}
	}
	return nil, false
}

// resolveArtifactBoundMCPDriverRef reads the legacy optional
// config_json.mcpDriverFileId off the same OUTPUT context_artifact.v1 binding
// resolveArtifactBoundContextSources reads config_json.sources from. New runs
// may ask for the file URL/id at runtime instead; this remains only as a
// backward-compatible default for existing saved instances. ok=false means no
// legacy default is configured.
func resolveArtifactBoundMCPDriverRef(node agentpack.FlowNode) (string, bool) {
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || b.ArtifactTypeID != ArtifactTypeContext {
			continue
		}
		ref, ok := b.ConfigJSON["mcpDriverFileId"].(string)
		ref = strings.TrimSpace(ref)
		if ok && ref != "" {
			return ref, true
		}
	}
	return "", false
}

// ArtifactResolveResult is what an ArtifactResolver produces for one bound
// instance: bounded, prompt-injectable content plus a source ref and any
// degrade warnings, mirroring ContextSource's FlowContextSection contract
// but generic across artifact categories (CP-45/SD-23 D-11).
type ArtifactResolveResult struct {
	SourceRef string
	Body      string
	Warnings  []string
}

// ArtifactResolver produces prompt-injectable content for one artifact type.
// context_artifact.v1 is deliberately NOT implemented through this
// interface — it keeps its own dedicated context.produce/context.render
// pipeline (FlowContextPackage/Sections), which ArtifactTypeRegistry would
// only duplicate. This interface exists for artifact types that have no
// existing pipeline to compose over, starting with file_artifact.v1
// (Task-202) — proving the framework is not context-specific (SD-23 D-8).
type ArtifactResolver interface {
	ArtifactTypeID() string
	Resolve(workspaceCwd string, binding agentpack.FlowArtifactBinding) (ArtifactResolveResult, error)
}

// ArtifactTypeRegistry resolves artifact-type ids to their resolver
// implementations (CP-45/SD-23 D-11), mirroring ContextSourceRegistry at the
// generic-artifact layer.
type ArtifactTypeRegistry struct {
	resolvers map[string]ArtifactResolver
}

// NewArtifactTypeRegistry returns an empty registry.
func NewArtifactTypeRegistry() *ArtifactTypeRegistry {
	return &ArtifactTypeRegistry{resolvers: make(map[string]ArtifactResolver)}
}

// Register adds a resolver. A duplicate or empty type id is a programming
// error (mirrors ContextSourceRegistry.Register / BehaviorRegistry.Register).
func (r *ArtifactTypeRegistry) Register(resolver ArtifactResolver) error {
	if resolver == nil {
		return fmt.Errorf("artifact type registry: nil resolver")
	}
	id := resolver.ArtifactTypeID()
	if id == "" {
		return fmt.Errorf("artifact type registry: resolver missing artifact type id")
	}
	if _, exists := r.resolvers[id]; exists {
		return fmt.Errorf("artifact type registry: id %q already registered", id)
	}
	r.resolvers[id] = resolver
	return nil
}

// Resolve returns the resolver registered for typeID. Unknown ids fail fast
// (SD-23 D-7 — a binding to an unresolvable type must never silently no-op).
func (r *ArtifactTypeRegistry) Resolve(typeID string) (ArtifactResolver, error) {
	resolver, ok := r.resolvers[typeID]
	if !ok {
		return nil, fmt.Errorf("artifact type registry: no resolver registered for %q", typeID)
	}
	return resolver, nil
}

var defaultArtifactTypeRegistry = func() *ArtifactTypeRegistry {
	r := NewArtifactTypeRegistry()
	if err := r.Register(&fileArtifactResolver{}); err != nil {
		panic(err)
	}
	return r
}()

// DefaultArtifactTypeRegistry returns the process-wide registry of non-context
// built-in artifact resolvers.
func DefaultArtifactTypeRegistry() *ArtifactTypeRegistry {
	return defaultArtifactTypeRegistry
}

// fileArtifactResolver implements file_artifact.v1 (CP-45/SD-23 D-8/Task-202):
// config_json.paths is a bounded, workspace-safe file path list. Reuses
// readSourceExcerpts (source.excerpt's own reader) so workspace-safety
// guarantees (outside_workspace, symlink escape, binary, size caps) are
// identical, not re-implemented.
type fileArtifactResolver struct{}

func (fileArtifactResolver) ArtifactTypeID() string { return ArtifactTypeFile }

func (fileArtifactResolver) Resolve(workspaceCwd string, binding agentpack.FlowArtifactBinding) (ArtifactResolveResult, error) {
	raw, _ := binding.ConfigJSON["paths"].([]any)
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			paths = append(paths, s)
		}
	}
	if len(paths) == 0 {
		return ArtifactResolveResult{}, fmt.Errorf("file_artifact.v1: instance %q has no paths configured", binding.ArtifactInstanceID)
	}

	excerpts, omitted := readSourceExcerpts(workspaceCwd, paths)
	var body strings.Builder
	for _, ex := range excerpts {
		body.WriteString(fmt.Sprintf("File: %s\n%s\n\n", ex.Path, ex.Excerpt))
	}
	return ArtifactResolveResult{
		SourceRef: "file:" + strings.Join(paths, ","),
		Body:      strings.TrimSpace(body.String()),
		Warnings:  omitted,
	}, nil
}

// resolveInputArtifactPrompt renders non-context INPUT artifact bindings for
// a consumer node prompt.
//
// BUG-276 / owner product rule: file_artifact.v1 has durable workspace paths —
// the prompt only **mentions those paths** and tells the agent to open them
// with tools. Full file body is NOT pasted (contrast context_artifact / Flow
// Context Package, which must push content because it has no path handle).
//
// Task-202's excerpt resolver remains available for other call sites; INPUT
// prompt assembly intentionally does not use it.
func resolveInputArtifactPrompt(workspaceCwd string, node agentpack.FlowNode) string {
	_ = workspaceCwd // reserved for optional existence soft-checks
	var paths []string
	seen := make(map[string]bool)
	for _, b := range node.ArtifactBindings {
		if b.Direction != "input" || b.ArtifactTypeID == ArtifactTypeContext {
			continue // context_artifact stays on its own context.produce/render pipeline
		}
		if b.ArtifactTypeID != ArtifactTypeFile {
			// Unknown non-context types: path-only when config has paths; else skip body dump.
			for _, p := range fileArtifactPathsFromConfig(b.ConfigJSON) {
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
			continue
		}
		for _, p := range fileArtifactPathsFromConfig(b.ConfigJSON) {
			if seen[p] {
				continue
			}
			seen[p] = true
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	var out strings.Builder
	for _, p := range paths {
		out.WriteString("\n- `")
		out.WriteString(p)
		out.WriteString("`")
	}
	return out.String()
}

// fileArtifactPathsFromConfig extracts workspace-relative paths from a
// file_artifact.v1 binding's config_json.paths list.
func fileArtifactPathsFromConfig(config map[string]any) []string {
	if config == nil {
		return nil
	}
	raw, ok := config["paths"].([]any)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		paths = append(paths, s)
	}
	return paths
}

// requiredFileArtifactOutputPaths returns designated paths from required
// OUTPUT bindings of type file_artifact.v1 (Task-223 write contract).
// Optional (Required=false) outputs are skipped for hard enforcement.
func requiredFileArtifactOutputPaths(node agentpack.FlowNode) []string {
	var paths []string
	seen := make(map[string]bool)
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || b.ArtifactTypeID != ArtifactTypeFile || !b.Required {
			continue
		}
		for _, p := range fileArtifactPathsFromConfig(b.ConfigJSON) {
			if seen[p] {
				continue
			}
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// appendInputArtifactPrompt appends INPUT file_artifact **path mentions**
// (BUG-276) — not file bodies — plus a short instruction to read via tools.
func appendInputArtifactPrompt(workspaceCwd, prompt string, node agentpack.FlowNode) string {
	block := resolveInputArtifactPrompt(workspaceCwd, node)
	if block == "" {
		return prompt
	}
	header := "\n\n## Bound file artifacts (read with tools)\n" +
		"Open and read these workspace paths with your tools before proceeding. " +
		"File contents are not pasted into this prompt — use the paths as the source of truth.\n"
	return prompt + header + block
}

// nodeHasFileArtifactInput reports whether node has any file_artifact INPUT
// paths (Task-224: review handoff omits full coder final message when true).
func nodeHasFileArtifactInput(node agentpack.FlowNode) bool {
	for _, b := range node.ArtifactBindings {
		if b.Direction != "input" || b.ArtifactTypeID != ArtifactTypeFile {
			continue
		}
		if len(fileArtifactPathsFromConfig(b.ConfigJSON)) > 0 {
			return true
		}
	}
	return false
}

// appendRequiredOutputArtifactPrompt appends the Task-223 write-contract
// section for required file_artifact OUTPUT paths, plus Task-224 What/Why/
// Baseline template guidance so review can stay deliverable-centric.
func appendRequiredOutputArtifactPrompt(prompt string, node agentpack.FlowNode) string {
	paths := requiredFileArtifactOutputPaths(node)
	if len(paths) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString("\n\n## Required file outputs (write contract)\n")
	b.WriteString("Before you finish this turn you MUST create or update each of these workspace-relative paths:\n")
	for _, p := range paths {
		b.WriteString("- `")
		b.WriteString(p)
		b.WriteString("`\n")
	}
	b.WriteString("Do not only describe the content in chat — write the file(s) with your tools. ")
	b.WriteString("The flow gate will reprompt if any required path is missing after your turn.\n")
	b.WriteString("\nEach required file MUST be markdown including at least these sections:\n\n")
	b.WriteString("## What\n")
	b.WriteString("- What you produced or changed (paths, scope).\n\n")
	b.WriteString("## Why\n")
	b.WriteString("- Why this approach **now** (not rejected alternatives).\n")
	b.WriteString("- Past decisions already closed from Prior work / Prior discussion / the Flow Context Package — list them; do not silently reopen.\n")
	b.WriteString("- If you conflict with a closed decision, state the conflict explicitly.\n\n")
	b.WriteString("## Baseline\n")
	b.WriteString("- feature_key:\n")
	b.WriteString("- source_doc_id / CA / Task / BUG / commit:\n")
	return prompt + b.String()
}

// composeFlowNodeAgentPrompt applies Task-223 INPUT read inject then OUTPUT
// write-contract inject to a base agent prompt for a flow node.
func composeFlowNodeAgentPrompt(workspaceCwd, prompt string, node agentpack.FlowNode) string {
	prompt = appendInputArtifactPrompt(workspaceCwd, prompt, node)
	prompt = appendRequiredOutputArtifactPrompt(prompt, node)
	return prompt
}
