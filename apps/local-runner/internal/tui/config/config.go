// Package config defines the ChatConfig that aggregates resolved flags for
// the `flowpilot chat` command.  It has no dependency on the runner package.
//
// Skill: cli-tui (CP-56)
package config

// ChatConfig holds all resolved settings for a `flowpilot chat` session.
type ChatConfig struct {
	// RunnerURL is the explicit runner URL (--runner-url). When set, the TUI
	// skips auto-start and uses this URL directly.
	RunnerURL string
	// NoStartRunner disables the auto-start heuristic (--no-start-runner).
	NoStartRunner bool

	// RunnerWorkspace is the resolved runner workspace root.
	// Resolution order: --workspace flag → FLOWPILOT_WORKSPACE env → walk up.
	RunnerWorkspace string
	// RunnerHost is the host the runner listens on (from root --host).
	RunnerHost string
	// RunnerPort is the port the runner listens on (from root --port).
	RunnerPort int

	// ProjectPath is the selected project path (--project flag).
	// This is the StartRunInput.cwd — separate from RunnerWorkspace.
	ProjectPath string

	// Provider selects the AI provider (--provider, e.g. "codex", "claude", "grok").
	Provider string
	// Model selects the model within the provider (--model).
	Model string
	// ReasoningEffort is the reasoning level (--reasoning, e.g. "high", "medium", "low").
	ReasoningEffort string
	// Yolo enables YOLO mode (--yolo).
	Yolo bool

	// Print (headless) mode: output final message to stdout and exit (--print/-p).
	Print bool
	// Prompt is the initial prompt to send when Print mode is active.
	Prompt string

	// ResumeRunID is the run ID to resume (--resume).
	ResumeRunID string

	// Timeout is the session timeout (--timeout).
	Timeout string
}
