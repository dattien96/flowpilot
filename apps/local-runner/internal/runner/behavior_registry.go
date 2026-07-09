package runner

import (
	"context"
	"fmt"

	"flowpilot-runner/internal/agentpack"
)

// BehaviorID identifies a canonical node behavior. Pack/definition data may
// reference a behavior by an alias (see agentpack.NormalizeBehaviorID); the
// registry always resolves through that alias table before dispatch so a
// flow can never select an ID that has no Go-side guarantee (CP-42 P-1).
type BehaviorID string

const (
	BehaviorAgentDelegate       BehaviorID = "agent.delegate"
	BehaviorHubInline           BehaviorID = "hub.inline"
	BehaviorContextProduce      BehaviorID = "context.produce"
	BehaviorContextRender       BehaviorID = "context.render"
	BehaviorCommandValidate     BehaviorID = "command.validate"
	BehaviorValidationSummarize BehaviorID = "validation.summarize"
	BehaviorArtifactAuditDraft  BehaviorID = "artifact.audit_draft"
	BehaviorFlowControl         BehaviorID = "flow.control"
	BehaviorUserConfirm         BehaviorID = "user.confirm"
)

// BehaviorScope classifies how a behavior is allowed to act. Pack data never
// supplies the implementation, only which scope/ID a node uses (CP-42 T-2).
type BehaviorScope string

const (
	// BehaviorScopeInline runs deterministic, runner-owned logic with no
	// provider call (e.g. context production, validation summaries).
	BehaviorScopeInline BehaviorScope = "inline"
	// BehaviorScopeDelegate assembles prompt/context for a provider-backed
	// agent run; it does not itself talk to a provider.
	BehaviorScopeDelegate BehaviorScope = "delegate"
	// BehaviorScopeControl maps a declared tool's domain output onto the
	// generic FlowControlInput contract.
	BehaviorScopeControl BehaviorScope = "control"
)

// BehaviorInput is the generic payload passed to a behavior handler. Fields
// are deliberately untyped/optional so the same struct serves inline,
// delegate, and control behaviors without per-behavior input types.
type BehaviorInput struct {
	NodeID        string
	WorkflowRunID string
	StepRunID     string
	WorkspaceCwd  string
	Prompt        string
	RawArgs       map[string]any
	Payload       map[string]any
	// ContextSourceIDs is the enabled context-source set for a context.produce
	// dispatch (CP-44 P-4 / Task-194), resolved by the caller from the active
	// flow's declared `contexts.<name>.sources` binding. Empty means "use the
	// runner's default built-in set" — most callers (and every pre-Task-194
	// dispatch) leave this unset and get the pre-CP-44 behavior unchanged.
	ContextSourceIDs []string
}

// BehaviorOutput is the generic result a behavior handler returns. The
// executor converts this into existing provider/runtime events; behaviors
// never construct provider-specific payloads directly.
type BehaviorOutput struct {
	Status              string
	Summary             string
	Payload             map[string]any
	NextPromptFragments []string
	Events              []ProviderEvent
}

// BehaviorHandler executes one node behavior against a BehaviorInput. Handlers
// are plain Go functions; pack/definition data can select a handler by ID but
// can never supply or alter handler code (CP-42 P-1, Task-176 T-2).
type BehaviorHandler func(ctx context.Context, in BehaviorInput) (BehaviorOutput, error)

// BehaviorSpec pairs a behavior ID with its scope and handler.
type BehaviorSpec struct {
	ID      BehaviorID
	Scope   BehaviorScope
	Handler BehaviorHandler
}

// BehaviorRegistry resolves behavior IDs (and their pack aliases) to handlers.
type BehaviorRegistry struct {
	specs map[BehaviorID]BehaviorSpec
}

// NewBehaviorRegistry returns an empty registry with no behaviors registered.
func NewBehaviorRegistry() *BehaviorRegistry {
	return &BehaviorRegistry{specs: make(map[BehaviorID]BehaviorSpec)}
}

// Register adds a behavior spec. Registering the same ID twice is an error so
// a core behavior can never be silently shadowed by a later registration.
func (r *BehaviorRegistry) Register(spec BehaviorSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("behavior registry: spec missing id")
	}
	if spec.Handler == nil {
		return fmt.Errorf("behavior registry: spec %q missing handler", spec.ID)
	}
	if _, exists := r.specs[spec.ID]; exists {
		return fmt.Errorf("behavior registry: id %q already registered", spec.ID)
	}
	r.specs[spec.ID] = spec
	return nil
}

// Resolve normalizes id through the pack alias table and returns the matching
// spec. Unknown behavior IDs fail fast rather than degrading to a no-op, so a
// flow can never silently lose a step (Task-176 T-3).
func (r *BehaviorRegistry) Resolve(id string) (BehaviorSpec, error) {
	canonical, ok := agentpack.NormalizeBehaviorID(id)
	if !ok {
		return BehaviorSpec{}, fmt.Errorf("behavior registry: unknown behavior id %q", id)
	}
	spec, ok := r.specs[BehaviorID(canonical)]
	if !ok {
		return BehaviorSpec{}, fmt.Errorf("behavior registry: no handler registered for %q (canonical %q)", id, canonical)
	}
	return spec, nil
}

// Dispatch resolves id and executes its handler. Resolution errors surface
// before any handler runs, so config validation never silently no-ops.
func (r *BehaviorRegistry) Dispatch(ctx context.Context, id string, in BehaviorInput) (BehaviorOutput, error) {
	spec, err := r.Resolve(id)
	if err != nil {
		return BehaviorOutput{}, err
	}
	return spec.Handler(ctx, in)
}
