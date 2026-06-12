package runner

import "context"

// Phase 7 (04-07): Claude and Gemini as first-class-but-disabled providers. Each
// implements the same ProviderRuntimeAdapter contract and returns the typed
// UnsupportedProviderRuntimeError on use, so the abstraction stays honest: the core
// depends on the interface, not on Codex. They advertise disabled capabilities and
// are registered as placeholders (not selectable) until their real adapters land.
//
// Claude future: Claude Agent SDK (preferred) or `claude -p`, normalized into the
// same ProviderEvent schema — its own adapter + tests, not a Codex wrapper.
// Gemini future: `gemini -p --output-format stream-json`, after proving stream-json
// stability + resume + non-hanging permission/failure.
type placeholderAdapter struct {
	key  ProviderKey
	caps ProviderCapabilities
}

func newPlaceholderAdapter(key ProviderKey) *placeholderAdapter {
	// Disabled capabilities (all false) until the real adapter is built.
	return &placeholderAdapter{key: key}
}

func (a *placeholderAdapter) Key() ProviderKey                   { return a.key }
func (a *placeholderAdapter) Capabilities() ProviderCapabilities { return a.caps }

func (a *placeholderAdapter) SendTurn(_ context.Context, _ TurnRequest, _ TurnBridge) error {
	return &UnsupportedProviderRuntimeError{ProviderKey: a.key}
}
