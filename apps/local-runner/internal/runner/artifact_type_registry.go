package runner

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// Built-in artifact type ids (CP-45/SD-23 D-2). Code hardcodes this layer;
// the catalog rows themselves are seeded via migration
// (20260709090000_add_artifact_types_catalog.sql,
// 20260709093000_add_file_artifact_type.sql) and never user-created.
const (
	ArtifactTypeContext  = "context_artifact.v1"
	ArtifactTypeFile     = "file_artifact.v1"
	ArtifactTypeTelegram = "telegram.v1"
	// ArtifactTypeSprintHandoff is the CP-62 P-6 (Task-342) handoff artifact
	// written by the vibe-sprint audit hub from verified run state only.
	ArtifactTypeSprintHandoff = "sprint_handoff.v1"
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

// fileArtifactStructureFromConfig parses optional config_json.structure
// (Task-225). Only kind "markdown_sections" with a non-empty sections list
// is accepted. There is no separate format/preset field.
func fileArtifactStructureFromConfig(config map[string]any) []string {
	if config == nil {
		return nil
	}
	raw, ok := config["structure"]
	if !ok || raw == nil {
		return nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	kind, _ := obj["kind"].(string)
	if strings.TrimSpace(kind) != "markdown_sections" {
		return nil
	}
	secRaw, ok := obj["sections"].([]any)
	if !ok || len(secRaw) == 0 {
		return nil
	}
	var sections []string
	seen := make(map[string]bool)
	for _, item := range secRaw {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		sections = append(sections, s)
	}
	return sections
}

// requiredStructuredFileArtifactOutputs returns required OUTPUT file paths
// that declare structure.sections (Task-225). Paths without structure are
// omitted (existence-only). When multiple required bindings share a path,
// section lists are merged (stable first-seen order) so structure is not lost.
func requiredStructuredFileArtifactOutputs(node agentpack.FlowNode) []flowgate.StructuredFileArtifactOutput {
	// path -> ordered unique sections
	byPath := make(map[string][]string)
	var order []string
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || b.ArtifactTypeID != ArtifactTypeFile || !b.Required {
			continue
		}
		sections := fileArtifactStructureFromConfig(b.ConfigJSON)
		if len(sections) == 0 {
			continue
		}
		for _, p := range fileArtifactPathsFromConfig(b.ConfigJSON) {
			if _, ok := byPath[p]; !ok {
				order = append(order, p)
			}
			byPath[p] = mergeSectionTitles(byPath[p], sections)
		}
	}
	out := make([]flowgate.StructuredFileArtifactOutput, 0, len(order))
	for _, p := range order {
		out = append(out, flowgate.StructuredFileArtifactOutput{Path: p, Sections: byPath[p]})
	}
	return out
}

// mergeSectionTitles appends titles from add that are not already present
// (case-insensitive), preserving first-seen order.
func mergeSectionTitles(base, add []string) []string {
	seen := make(map[string]bool, len(base)+len(add))
	out := append([]string(nil), base...)
	for _, s := range base {
		seen[strings.ToLower(strings.TrimSpace(s))] = true
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
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
// section for required file_artifact OUTPUT paths. Task-225: section template
// is per-instance structure only — paths-only bindings get no What/Why block.
func appendRequiredOutputArtifactPrompt(prompt string, node agentpack.FlowNode) string {
	// Build path list + merged structure per path (same merge as gate).
	paths := requiredFileArtifactOutputPaths(node)
	if len(paths) == 0 {
		return prompt
	}
	structByPath := make(map[string][]string)
	for _, item := range requiredStructuredFileArtifactOutputs(node) {
		structByPath[item.Path] = item.Sections
	}
	type outItem struct {
		path     string
		sections []string
	}
	items := make([]outItem, 0, len(paths))
	for _, p := range paths {
		items = append(items, outItem{path: p, sections: structByPath[p]})
	}
	var b strings.Builder
	b.WriteString("\n\n## Required file outputs (write contract)\n")
	b.WriteString("Before you finish this turn you MUST create or update each of these workspace-relative paths:\n")
	for _, it := range items {
		b.WriteString("- `")
		b.WriteString(it.path)
		b.WriteString("`\n")
	}
	b.WriteString("Do not only describe the content in chat — write the file(s) with your tools. ")
	b.WriteString("The flow gate will reprompt if any required path is missing after your turn.\n")

	// Group paths that share the same section list for one template block.
	type structGroup struct {
		paths    []string
		sections []string
	}
	var groups []structGroup
	for _, it := range items {
		if len(it.sections) == 0 {
			continue
		}
		// Find matching group by section equality.
		matched := false
		for gi := range groups {
			if sameStringSliceFold(groups[gi].sections, it.sections) {
				groups[gi].paths = append(groups[gi].paths, it.path)
				matched = true
				break
			}
		}
		if !matched {
			groups = append(groups, structGroup{
				paths:    []string{it.path},
				sections: append([]string(nil), it.sections...),
			})
		}
	}
	for _, g := range groups {
		b.WriteString("\nFor ")
		for i, p := range g.paths {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("`")
			b.WriteString(p)
			b.WriteString("`")
		}
		b.WriteString(" write markdown including at least these sections (heading level and casing may vary):\n\n")
		hasWhy := false
		for _, sec := range g.sections {
			b.WriteString("## ")
			b.WriteString(sec)
			b.WriteString("\n")
			if strings.EqualFold(sec, "Why") {
				hasWhy = true
				b.WriteString("- Why this approach **now** (not rejected alternatives).\n")
				b.WriteString("- Past decisions already closed from Prior work / Prior discussion / the Flow Context Package — list them; do not silently reopen.\n")
				b.WriteString("- If you conflict with a closed decision, state the conflict explicitly.\n")
			} else if strings.EqualFold(sec, "What") {
				b.WriteString("- What you produced or changed (paths, scope).\n")
			} else if strings.EqualFold(sec, "Baseline") {
				b.WriteString("- feature_key:\n")
				b.WriteString("- source_doc_id / CA / Task / BUG / commit:\n")
			} else {
				b.WriteString("- …\n")
			}
			b.WriteString("\n")
		}
		_ = hasWhy
	}
	return prompt + b.String()
}

func sameStringSliceFold(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// telegramOutputTarget is one bound telegram.v1 OUTPUT instance's send
// target, extracted from its config_json (Task-233, CP-05-05 P-1/P-3).
type telegramOutputTarget struct {
	chatID          string
	messageTemplate string
}

// requiredTelegramOutputTargets mirrors requiredFileArtifactOutputPaths'
// shape for telegram.v1 OUTPUT bindings — this is the seam a Telegram
// notification actually cares about being real (composeFlowNodeAgentPrompt),
// NOT the ArtifactResolver/ArtifactTypeRegistry interface (Task-233 doc:
// that interface has zero production callers and no OUTPUT/side-effect
// method — dispatching there would be building on dead scaffolding).
func requiredTelegramOutputTargets(node agentpack.FlowNode) []telegramOutputTarget {
	var targets []telegramOutputTarget
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || b.ArtifactTypeID != ArtifactTypeTelegram || !b.Required {
			continue
		}
		chatID, _ := b.ConfigJSON["chatId"].(string)
		chatID = strings.TrimSpace(chatID)
		if chatID == "" {
			continue
		}
		template, _ := b.ConfigJSON["messageTemplate"].(string)
		targets = append(targets, telegramOutputTarget{chatID: chatID, messageTemplate: strings.TrimSpace(template)})
	}
	return targets
}

// appendTelegramOutputPrompt is Telegram's OUTPUT write-contract (Task-233,
// mirrors appendRequiredOutputArtifactPrompt's shape but the "artifact" is a
// sent message, not a file): the AI MUST call the `send_message` tool on the
// `telegram` MCP server (Task-232) before finishing, once per bound target.
// Verification (flowgate r-artifact-telegram-sent) checks for a real
// message_id in the tool-call response, not a file on disk — there is
// nothing to os.Stat here.
func appendTelegramOutputPrompt(prompt string, node agentpack.FlowNode) string {
	targets := requiredTelegramOutputTargets(node)
	if len(targets) == 0 {
		return prompt
	}
	// Deliberately plain, non-imperative phrasing (CA-follow-up to Task-233):
	// an earlier version of this section read "you MUST ... The flow gate
	// will reprompt if no successful send is detected", which a receiving
	// model can pattern-match as a coerced/urgent instruction embedded in
	// task content and refuse to act on as a suspected prompt injection —
	// even though this text is this run's own top-level task, authored by
	// FlowPilot on the user's behalf, not third-party observed content. This
	// section instead states the task as a plain part of the turn's work,
	// the same register as the rest of the composed prompt.
	var b strings.Builder
	b.WriteString("\n\n## This step's task includes sending a Telegram notification\n")
	b.WriteString(fmt.Sprintf("As part of finishing this turn, send a Telegram message using the `send_message` tool on the `%s` MCP server, for each target below:\n", telegramMcpServerName))
	for _, t := range targets {
		b.WriteString("- chat: `")
		b.WriteString(t.chatID)
		b.WriteString("`")
		if t.messageTemplate != "" {
			b.WriteString(" — message should follow this template:\n\n")
			b.WriteString(t.messageTemplate)
			b.WriteString("\n")
		} else {
			b.WriteString(" — summarize this run's final outcome in the message.\n")
		}
	}
	b.WriteString("\nCall the tool itself rather than only describing the notification in chat; a successful call returns a `message_id`, so only report the message as sent once you have one. ")
	b.WriteString(fmt.Sprintf("If `%s` is unavailable, end the response with `MCP_FAILURE_CODE: MCP_UNAVAILABLE` instead of a fabricated confirmation.\n", telegramMcpServerName))
	return prompt + b.String()
}

// composeFlowNodeAgentPrompt applies Task-223 INPUT read inject, OUTPUT
// write-contract inject, and Task-233's Telegram OUTPUT write-contract to a
// base agent prompt for a flow node.
// changeContractPromptMarker is a human-readable heading for Change Contract
// injection. Double-inject guard uses changeContractTrustedMarker(runID) so a
// user cannot suppress inject by typing the Vietnamese heading alone (V10R4).
const changeContractPromptMarker = "## Change Contract đã khai cho run này"

// changeContractTrustedMarker returns a run-scoped HTML comment only our
// injector writes. Skip inject only when this exact token is present.
// BUG-288 P1-20: parentRunID alone is visible to the user (desktop UI/API),
// so a bare "<!-- flowpilot-cc:<runID> -->" check let a user who knows/copies
// a run id suppress inject by typing it themselves. The trailing segment is
// an HMAC (runMarkerMAC, flow_context_handoff.go) over a server-only secret,
// so only this process can mint a marker that will match — mirrors
// flowContextTrustedMarker's fix for the same class of gap. This means
// appendChangeContractIfAny's existing strings.Contains(prompt, trusted)
// check is safe as-is: a user cannot construct the MAC suffix without
// runMarkerSecret, so a copied or guessed run id alone no longer suffices.
func changeContractTrustedMarker(parentRunID string) string {
	return changeContractTrustedMarkerWith(nil, parentRunID)
}

// changeContractTrustedMarkerWith mints using an explicit secret (BUG-288 R18-4).
func changeContractTrustedMarkerWith(secret []byte, parentRunID string) string {
	return "<!-- flowpilot-cc:" + parentRunID + ":" + runMarkerMACWith(secret, "cc", parentRunID) + " -->"
}

// templatedFileArtifactOutput is one required OUTPUT file_artifact binding
// that declares a pathTemplate (CP-58 Task-307) instead of a concrete paths
// list. The writer resolves the template at authoring time ({{idx}} = next
// free number in the target folder, {{slug}} = short kebab-case slug), so the
// contract is prompt-level: the flowgate exact-path check cannot verify a
// pattern and must NOT reprompt on it (requiredFileArtifactOutputPaths only
// ever sees concrete paths).
type templatedFileArtifactOutput struct {
	slotName     string
	pathTemplate string
}

func templatedFileArtifactOutputsForNode(node agentpack.FlowNode) []templatedFileArtifactOutput {
	var out []templatedFileArtifactOutput
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || !b.Required || b.ArtifactTypeID != ArtifactTypeFile {
			continue
		}
		// Concrete-paths bindings stay on the gated Task-223 contract.
		if len(fileArtifactPathsFromConfig(b.ConfigJSON)) > 0 {
			continue
		}
		raw, ok := b.ConfigJSON["pathTemplate"].(string)
		tmpl := strings.TrimSpace(raw)
		if !ok || tmpl == "" {
			continue
		}
		out = append(out, templatedFileArtifactOutput{slotName: b.SlotName, pathTemplate: tmpl})
	}
	return out
}

// appendTemplatedFileArtifactOutputsPrompt appends the CP-58 Task-307
// templated write contract: the writer must create one file per templated
// OUTPUT slot, resolving {{idx}}/{{slug}} placeholders itself, and report the
// concrete path it wrote in its final message.
func appendTemplatedFileArtifactOutputsPrompt(prompt string, node agentpack.FlowNode) string {
	outputs := templatedFileArtifactOutputsForNode(node)
	if len(outputs) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString("\n\n## Templated file outputs (write contract)\n")
	b.WriteString("Before you finish this turn you MUST create or update one real file for each template below — and ONLY these files. ")
	b.WriteString("Do NOT write source code, test files, configs, change-audit notes, or any other path, and do NOT run commands or tests ")
	b.WriteString("(a document-writer role has no shell). Resolve the placeholders yourself — `{{idx}}` is the next free number in the target folder, `{{slug}}` a short kebab-case slug of the title:\n")
	for _, o := range outputs {
		b.WriteString("- `")
		b.WriteString(o.pathTemplate)
		b.WriteString("`\n")
	}
	b.WriteString("Do not only describe the content in chat — write the file(s) with your tools, and name the exact path(s) you wrote in your final message.\n")
	return prompt + b.String()
}

// appendTemplatedInputArtifactMention tells a node (e.g. plan_reviewer or
// task_splitter) where its templated INPUT artifacts land. A template cannot
// be read up front like a concrete path (BUG-276 keeps INPUT mention-only
// anyway), so the node is told to find and read the newest matching file with
// its own tools before proceeding.
func appendTemplatedInputArtifactMention(prompt string, node agentpack.FlowNode) string {
	var templates []string
	seen := make(map[string]bool)
	for _, b := range node.ArtifactBindings {
		if b.Direction != "input" || b.ArtifactTypeID != ArtifactTypeFile {
			continue
		}
		raw, ok := b.ConfigJSON["pathTemplate"].(string)
		tmpl := strings.TrimSpace(raw)
		if !ok || tmpl == "" || seen[tmpl] {
			continue
		}
		seen[tmpl] = true
		templates = append(templates, tmpl)
	}
	if len(templates) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString("\n\n## Bound input artifacts (locate and read)\n")
	b.WriteString("The upstream node wrote its output under a naming template. Find the newest matching file in the workspace with your tools and read it before proceeding:\n")
	for _, t := range templates {
		b.WriteString("- `")
		b.WriteString(t)
		b.WriteString("`\n")
	}
	return prompt + b.String()
}

// composeFlowNodeAgentPrompt applies Task-223 INPUT read inject, OUTPUT
// write-contract inject, and Task-233's Telegram OUTPUT write-contract to a
// base agent prompt for a flow node. CP-58 Task-307 adds the templated
// (pathTemplate) file_artifact OUTPUT write contract and INPUT mention:
// harness plan outputs resolve their concrete file name at authoring time,
// so those contracts are prompt-level and deliberately NOT part of the
// flowgate exact-path check.
func composeFlowNodeAgentPrompt(workspaceCwd, prompt string, node agentpack.FlowNode) string {
	prompt = appendInputArtifactPrompt(workspaceCwd, prompt, node)
	prompt = appendTemplatedInputArtifactMention(prompt, node)
	prompt = appendRequiredOutputArtifactPrompt(prompt, node)
	prompt = appendTemplatedFileArtifactOutputsPrompt(prompt, node)
	prompt = appendTelegramOutputPrompt(prompt, node)
	// Task-293: a node's declared static promptTemplate (e.g. safe-fix-contract
	// instructions on the plan/review nodes, test-signatures/implement rules on
	// the writers) is appended to every composed node prompt. Render-only Go
	// templates are skipped — see appendStaticNodePrompt.
	prompt = appendStaticNodePrompt(prompt, node)
	// Task-247: inject latest Change Contract for the run when available.
	// parent/run id is not on node; callers pass it via workspace-side store lookup
	// using optional WorkflowRunID on a package-level helper when available.
	// Neo here only when prompt already carries a run marker or we can resolve store
	// by workspace alone is insufficient — inject when prompt does not yet contain
	// the contract marker and a package-global pending run is not required.
	// Actual run-scoped inject is applied by appendChangeContractIfAny from
	// flow_executor with the real parentRunID.
	return prompt
}

// appendStaticNodePrompt appends the node's declared static promptTemplate to
// a node prompt. Render-only Go templates (contents containing "{{") are
// skipped — they are composed by their own render paths (e.g.
// flow-context-handoff.md) and must never be injected raw into a provider
// prompt. Missing/undeclared templates are silently ignored so a node with a
// stale ref degrades to the base prompt instead of blocking the flow.
func appendStaticNodePrompt(prompt string, node agentpack.FlowNode) string {
	// CP-64 (Task-366 T-6): a reproduce node resolves its prompt through
	// resolveReproducePrompt so the flag-off path renders the legacy
	// empty-signature prompt without a second flow definition.
	rel := strings.TrimSpace(resolveReproducePrompt(ReproduceGateEnabled(), node))
	if rel == "" {
		return prompt
	}
	tmpl, ok, err := agentpack.LoadBuiltinPrompt(rel)
	if err != nil || !ok {
		return prompt
	}
	contents := strings.TrimSpace(tmpl.Contents)
	if contents == "" || strings.Contains(contents, "{{") {
		return prompt
	}
	return prompt + "\n\n" + contents
}

// appendChangeContractIfAny appends the run's latest Change Contract to a node
// prompt once (Task-247). Used from flow_executor after composeFlowNodeAgentPrompt.
//
// Skip inject only when the run-scoped trusted marker is present. The marker
// includes a non-guessable token derived from parentRunID + package body hash
// of the store path is not enough alone — user who knows runID could still type
// the HTML comment. Defense in depth: marker format is still checked, and
// package render embeds it only via our code path.
func appendChangeContractIfAny(workspaceCwd, parentRunID, prompt string) string {
	return appendChangeContractIfAnyWithSecret(workspaceCwd, parentRunID, prompt, nil)
}

// appendChangeContractIfAnyWithSecret is appendChangeContractIfAny using an
// explicit marker secret for mint/detect (BUG-288 R18-4 per-service).
func appendChangeContractIfAnyWithSecret(workspaceCwd, parentRunID, prompt string, secret []byte) string {
	if parentRunID == "" || workspaceCwd == "" {
		return prompt
	}
	// V10R4 P1: only trust our run-scoped inject marker. User-typed
	// "### change.contract" or the Vietnamese heading alone must NOT suppress inject.
	trusted := changeContractTrustedMarkerWith(secret, parentRunID)
	if strings.Contains(prompt, trusted) {
		return prompt
	}
	store, err := changecontract.OpenStoreReadOnly(workspaceCwd)
	if err != nil || store == nil {
		return prompt
	}
	c, ok := store.GetLatestForRun(parentRunID)
	if !ok {
		return prompt
	}
	body := changecontract.RenderContractBlock(c)
	if body == "" {
		return prompt
	}
	return prompt + "\n\n" + changeContractPromptMarker + "\n" + trusted + "\n" + body + "\n"
}
