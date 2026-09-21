package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	// defaultCompilerGateTimeout matches the recipe default (CP-68 §4): 300s is
	// enough for a cold `pnpm install && pnpm tsc --noEmit`.
	defaultCompilerGateTimeout = 300 * time.Second
	// maxCompilerErrorLines caps the structured feedback handed back to the AI so
	// a pathological build log cannot blow up the healing turn's context.
	maxCompilerErrorLines = 40
	// maxCompilerErrorLineLen caps one extracted error line (runes, not bytes).
	maxCompilerErrorLineLen = 400
	// compilerGateWaitDelay force-exits a hung grandchild that keeps the output
	// pipe open past the context deadline (same hardening as reverse_doc.go).
	compilerGateWaitDelay = 5 * time.Second
)

// compilerErrorLinePattern selects the lines worth quoting back to the AI: real
// compiler diagnostics (`error TS2307: ...`), generic error/failure markers, and
// the "cannot find" phrasing TypeScript/esbuild/Go emit for missing modules.
var compilerErrorLinePattern = regexp.MustCompile(`(?i)(\berror\b|✖|✗|\bfailed\b|cannot find|does not exist|is not assignable)`)

// CompilerGateResult is the objective verdict of one Compiler Verification Gate
// run (Task-386 T-1). Passed is the fail-closed bit: nothing else in the scaffold
// flow may treat a scaffold as complete unless this is true.
type CompilerGateResult struct {
	Passed        bool          `json:"passed"`
	ExitCode      int           `json:"exitCode"`
	RawOutput     string        `json:"rawOutput"`
	ParsedErrors  []string      `json:"parsedErrors,omitempty"`
	ExecutionTime time.Duration `json:"executionTime"`
	// TimedOut reports the gate was killed by its own timeout. Not healable:
	// the AI cannot fix a stalled network/install, so the caller must stop.
	TimedOut bool `json:"timedOut,omitempty"`
	// EnvError reports the command could not even start (binary missing, etc.).
	// Also not healable, mirroring validation-retry's environment semantics.
	EnvError string `json:"envError,omitempty"`
	// Command / WorkDir echo what actually ran, so the user-facing failure report
	// is actionable without re-deriving it.
	Command string `json:"command,omitempty"`
	WorkDir string `json:"workDir,omitempty"`
}

// CompilerGate runs one verification command inside a project workspace.
type CompilerGate struct {
	workingDir string
	command    string
	timeout    time.Duration
}

// NewCompilerGate builds a gate for one workspace. A non-positive timeout falls
// back to defaultCompilerGateTimeout.
func NewCompilerGate(workingDir, command string, timeout time.Duration) *CompilerGate {
	if timeout <= 0 {
		timeout = defaultCompilerGateTimeout
	}
	return &CompilerGate{
		workingDir: strings.TrimSpace(workingDir),
		command:    strings.TrimSpace(command),
		timeout:    timeout,
	}
}

// Command returns the shell command this gate runs.
func (g *CompilerGate) Command() string { return g.command }

// Timeout returns the effective per-attempt timeout.
func (g *CompilerGate) Timeout() time.Duration { return g.timeout }

// Run executes the gate command through a shell (recipe commands are shell
// fragments like `pnpm install && pnpm tsc --noEmit`), with the working directory
// pinned to the project and the parent process environment inherited.
//
// It returns a non-nil error only when the gate itself is misconfigured (empty
// command), never for a failing/timing-out compiler — those are reported inside
// the result so callers can distinguish "code is broken" (heal it) from
// "gate is broken" (report and stop).
func (g *CompilerGate) Run(ctx context.Context) (*CompilerGateResult, error) {
	if g.command == "" {
		return nil, errors.New("compiler gate: verification command is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cmd := compilerGateShellCommand(runCtx, g.command)
	if g.workingDir != "" {
		cmd.Dir = g.workingDir
	}
	// Env is nil ⇒ inherit os.Environ() (PATH, HOME, pnpm store, proxies, ...).
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	cmd.WaitDelay = compilerGateWaitDelay
	// The default CommandContext kill only terminates the shell; `cmd /c`
	// grandchildren keep the output pipes open and stall Wait until WaitDelay
	// (Windows). compilerGateCancel takes the whole tree down instead.
	cmd.Cancel = compilerGateCancel(cmd)

	started := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(started)

	result := &CompilerGateResult{
		ExitCode:      0,
		RawOutput:     combined.String(),
		ExecutionTime: elapsed,
		Command:       g.command,
		WorkDir:       g.workingDir,
	}

	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		result.ExitCode = -1
		result.TimedOut = true
	case runErr != nil:
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.EnvError = runErr.Error()
		}
	}

	result.Passed = runErr == nil && result.ExitCode == 0 && !result.TimedOut && result.EnvError == ""

	if !result.Passed {
		result.ParsedErrors = parseCompilerErrors(result.RawOutput)
		switch {
		case result.TimedOut:
			result.ParsedErrors = append([]string{
				fmt.Sprintf("compiler gate timed out after %s (command: %s)", g.timeout, g.command),
			}, result.ParsedErrors...)
		case result.EnvError != "":
			result.ParsedErrors = append([]string{
				fmt.Sprintf("compiler gate could not start: %s", result.EnvError),
			}, result.ParsedErrors...)
		case len(result.ParsedErrors) == 0:
			result.ParsedErrors = []string{
				fmt.Sprintf("compiler gate exited with code %d and produced no recognizable diagnostics", result.ExitCode),
			}
		}
	}

	return result, nil
}

// compilerGateShellCommand wraps the recipe command in the platform shell so
// `&&`, pipes, and quoted fragments behave the way the recipe author wrote them.
func compilerGateShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

// parseCompilerErrors extracts the actionable diagnostic lines (file:line plus
// message) from a raw combined stdout/stderr log, dropping noise lines and
// de-duplicating repeats.
func parseCompilerErrors(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	out := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" {
			continue
		}
		if !compilerErrorLinePattern.MatchString(trimmed) {
			continue
		}
		trimmed = truncateRunes(trimmed, maxCompilerErrorLineLen)
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
		if len(out) >= maxCompilerErrorLines {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// truncateRunes is shared with reverse_doc.go (identical rune-safe semantics).
