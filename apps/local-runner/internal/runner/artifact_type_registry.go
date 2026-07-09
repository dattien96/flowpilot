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

// resolveInputArtifactPrompt renders every non-context input artifact
// binding on node (via DefaultArtifactTypeRegistry) into one prompt-appendable
// string (CP-45/SD-23 D-8/D-11, Task-202: "step B binds the same instance as
// input; the resolver injects the file path into step B's prompt"). A
// resolve error degrades to a warning line rather than failing the node —
// consistent with ContextSource.Collect's degrade-soft contract (SD-23 D-9,
// F-7) — since a stale/misconfigured non-required artifact must never block
// the step it's attached to.
func resolveInputArtifactPrompt(workspaceCwd string, node agentpack.FlowNode) string {
	var out strings.Builder
	for _, b := range node.ArtifactBindings {
		if b.Direction != "input" || b.ArtifactTypeID == ArtifactTypeContext {
			continue // context_artifact stays on its own context.produce/render pipeline
		}
		resolver, err := DefaultArtifactTypeRegistry().Resolve(b.ArtifactTypeID)
		if err != nil {
			out.WriteString(fmt.Sprintf("\n\n[artifact %s: %v]", b.ArtifactInstanceID, err))
			continue
		}
		result, err := resolver.Resolve(workspaceCwd, b)
		if err != nil {
			out.WriteString(fmt.Sprintf("\n\n[artifact %s: %v]", b.ArtifactInstanceID, err))
			continue
		}
		if result.Body == "" {
			continue
		}
		out.WriteString(fmt.Sprintf("\n\n### Bound artifact (%s, %s)\n%s", b.ArtifactTypeID, result.SourceRef, result.Body))
	}
	return out.String()
}
