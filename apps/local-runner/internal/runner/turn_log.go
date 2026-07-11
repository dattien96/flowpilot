package runner

import "context"

// turnLogKind distinguishes the two kinds of entry written to a run's turn log.
type turnLogKind string

const (
	// turnLogKindPrompt records the raw user input typed at startTurn time —
	// before FlowPilot appends the ask_user reinforcement or skill/MCP preamble.
	turnLogKindPrompt turnLogKind = "prompt"
	// turnLogKindCodexSession records the Codex rollout session id produced for
	// each turn.  Codex writes one rollout file per turn with a distinct session
	// id, so seedTranscriptFromDisk needs the full chain to replay every turn.
	turnLogKindCodexSession turnLogKind = "codex_session"
	// turnLogKindGrokSession records the real Grok ACP session id produced for
	// each turn (BUG-GrokReplay-Restart). FlowPilot never feeds the real id back
	// for session/load, so every Grok turn spins a fresh session dir under
	// ~/.grok/sessions/<enc-cwd>/<id>/ — the exact per-turn shape Codex has. This
	// records the chain so seedTranscriptFromDisk can replay each turn's
	// chat_history.jsonl precisely (instead of the cwd-wide mtime fallback).
	turnLogKindGrokSession turnLogKind = "grok_session"
	// turnLogKindAssistant records the full assistant response for providers
	// that do not expose a provider-owned transcript file for replay.
	turnLogKindAssistant turnLogKind = "assistant"
	// turnLogKindTranscriptTurn records one visible prompt/assistant pair.
	turnLogKindTranscriptTurn turnLogKind = "transcript_turn"
)

// turnLogLine is one NDJSON line in the per-run turn log.
type turnLogLine struct {
	Kind      turnLogKind `json:"kind"`
	TurnID    string      `json:"turn_id,omitempty"`
	Prompt    string      `json:"prompt,omitempty"`
	Assistant string      `json:"assistant,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
}

// TurnLogStore persists raw user prompts and per-turn Codex session IDs so
// seedTranscriptFromDisk can replay the user's typed text (not the composed
// CLI prompt) and load every Codex rollout file for a multi-turn chat.
// Implemented by localFileSessionStore; absent on the fakeWorkflowStore used
// in unit tests, so all callers use the ok-pattern and treat missing log as
// a no-op (backward-compatible with runs created before BUG-083 was fixed).
type TurnLogStore interface {
	AppendTurnLog(ctx context.Context, runID string, line turnLogLine) error
	ReadTurnLog(ctx context.Context, runID string) ([]turnLogLine, error)
	DeleteTurnLog(ctx context.Context, runID string) error
}
