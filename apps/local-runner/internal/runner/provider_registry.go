package runner

import (
	"context"
	"fmt"
)

// UnsupportedProviderRuntimeError is returned when a provider runtime is requested
// but not implemented/enabled (Claude/Gemini in P2). Mirrors the 04 contract.
type UnsupportedProviderRuntimeError struct {
	ProviderKey ProviderKey
}

func (e *UnsupportedProviderRuntimeError) Error() string {
	return fmt.Sprintf("%s controlled runtime is not implemented yet", e.ProviderKey)
}

// TurnBridge is how an adapter emits normalized events and pauses for user
// interaction. The interactive service implements it; the adapter calls it on its
// own goroutine. RequestApproval/AskQuestion BLOCK until the user responds, the
// pending record expires, or the turn context is cancelled (interrupt).
type TurnBridge interface {
	Emit(ev ProviderEvent)
	RequestApproval(details ApprovalDetails) (decision string, err error)
	AskQuestion(prompt string, options []QuestionOption, multiSelect bool) (choice []string, err error)
}

// TurnRequest is the per-turn input handed to an adapter.
type TurnRequest struct {
	RunID             string
	StepID            string
	ProviderSessionID string
	ProviderTurnID    string
	Prompt            string
	SelectedSkills    []SkillSelection
	YoloMode          bool
	// Cwd is the run's active workspace directory (04-06). The adapter binds the
	// provider thread to this cwd; it takes precedence over any adapter default.
	Cwd string
	// Scenario is a P2 fake-adapter hint (mirrors the desktop scenario switcher);
	// the real Codex adapter (P3) ignores it.
	Scenario string
}

// ProviderRuntimeAdapter is the provider-neutral adapter contract (03/04). The
// runner core depends only on this interface + the registry.
type ProviderRuntimeAdapter interface {
	Key() ProviderKey
	Capabilities() ProviderCapabilities
	SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error
}

// ProviderStatus is the registry availability of a provider.
type ProviderStatus string

const (
	ProviderStatusAvailable   ProviderStatus = "available"
	ProviderStatusDisabled    ProviderStatus = "disabled"
	ProviderStatusPlaceholder ProviderStatus = "placeholder"
)

// ProviderRegistration describes a provider in the registry.
type ProviderRegistration struct {
	Key          ProviderKey          `json:"key"`
	DisplayName  string               `json:"displayName"`
	Status       ProviderStatus       `json:"status"`
	Capabilities ProviderCapabilities `json:"capabilities"`
	newAdapter   func() ProviderRuntimeAdapter
}

// ProviderRegistry holds provider registrations in a stable order.
type ProviderRegistry struct {
	order []ProviderKey
	regs  map[ProviderKey]ProviderRegistration
}

func newProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{regs: map[ProviderKey]ProviderRegistration{}}
}

func (r *ProviderRegistry) register(reg ProviderRegistration) {
	if _, ok := r.regs[reg.Key]; !ok {
		r.order = append(r.order, reg.Key)
	}
	r.regs[reg.Key] = reg
}

// List returns registrations in registration order.
func (r *ProviderRegistry) List() []ProviderRegistration {
	out := make([]ProviderRegistration, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.regs[k])
	}
	return out
}

// Get returns a registration by key.
func (r *ProviderRegistry) Get(key ProviderKey) (ProviderRegistration, bool) {
	reg, ok := r.regs[key]
	return reg, ok
}

// Adapter returns the adapter for a provider, or UnsupportedProviderRuntimeError if
// the provider is disabled/placeholder (no adapter factory).
func (r *ProviderRegistry) Adapter(key ProviderKey) (ProviderRuntimeAdapter, error) {
	reg, ok := r.regs[key]
	if !ok || reg.newAdapter == nil || reg.Status == ProviderStatusDisabled {
		return nil, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	return reg.newAdapter(), nil
}

// Selectable reports whether a provider may be chosen for a controlled run. A
// disabled/placeholder provider (no adapter) is NOT selectable and returns a typed
// UnsupportedProviderRuntimeError — this is the runner-side boundary (04-07): the
// UI gate is a convenience, the runner is the enforcement point.
func (r *ProviderRegistry) Selectable(key ProviderKey) (ProviderRegistration, error) {
	reg, ok := r.regs[key]
	if !ok {
		return ProviderRegistration{}, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	if reg.newAdapter == nil || reg.Status != ProviderStatusAvailable {
		return ProviderRegistration{}, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	return reg, nil
}

// DefaultProviderKey returns the first available (selectable) provider — the safe
// default for a run when the client does not specify one. A disabled/placeholder
// provider can never be the default. Returns false if none are available.
func (r *ProviderRegistry) DefaultProviderKey() (ProviderKey, bool) {
	for _, k := range r.order {
		if _, err := r.Selectable(k); err == nil {
			return k, true
		}
	}
	return "", false
}

// DefaultProviderRegistry builds the P2 registry: Codex backed by the fake adapter
// (the real app-server adapter lands in P3), Claude/Gemini as disabled placeholders
// that surface UnsupportedProviderRuntimeError.
func DefaultProviderRegistry() *ProviderRegistry {
	r := newProviderRegistry()
	r.register(ProviderRegistration{
		Key:         ProviderKeyCodex,
		DisplayName: "Codex",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
			SkillSelection: true, Mcp: true, Interrupt: true,
		},
		newAdapter: func() ProviderRuntimeAdapter { return newFakeProviderAdapter(ProviderKeyCodex) },
	})
	r.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusPlaceholder,
		Capabilities: ProviderCapabilities{},
	})
	r.register(ProviderRegistration{
		Key:          ProviderKeyGemini,
		DisplayName:  "Gemini",
		Status:       ProviderStatusPlaceholder,
		Capabilities: ProviderCapabilities{},
	})
	return r
}
