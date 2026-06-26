package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/promptblock"
)

const (
	summarizerMaxInputBytes  = 16 * 1024
	summarizerMaxOutputBytes = 4 * 1024
	summarizerTimeout        = 60 * time.Second
)

// summarizerModelFor picks the cheap summarizer model for the chat's own
// provider (Task-161 T-2 / Task-162 T-4). The summary follows the active
// provider so the connected account is always usable — a Codex-only user gets a
// Codex summary, not a failed Claude call. Model ids are registry/CLI-driven, so
// each default is overridable: a global FLOWPILOT_SUMMARIZER_MODEL wins for any
// provider, else FLOWPILOT_SUMMARIZER_MODEL_<PROVIDER>, else a conservative
// per-provider cheap tier. Returning "" lets the provider CLI use its own
// configured default model.
func summarizerModelFor(providerKey ProviderKey) string {
	if m := strings.TrimSpace(os.Getenv("FLOWPILOT_SUMMARIZER_MODEL")); m != "" {
		return m
	}
	if m := strings.TrimSpace(os.Getenv("FLOWPILOT_SUMMARIZER_MODEL_" + strings.ToUpper(string(providerKey)))); m != "" {
		return m
	}
	switch providerKey {
	case ProviderKeyClaude:
		return "haiku"
	case ProviderKeyGemini:
		return "gemini-2.5-flash"
	case ProviderKeyCodex:
		// No verified cheap-model id is hardcoded; empty lets `codex exec` use the
		// account's configured default. Set FLOWPILOT_SUMMARIZER_MODEL_CODEX (e.g.
		// a *-mini model) to force the cheap tier.
		return ""
	default:
		return ""
	}
}

const summarizerInstruction = `Summarize the DISCUSSION in the conversation below for handing off to another AI session.
Capture only: the original goal, decisions made, approaches tried and rejected, stated user preferences, and any open or blocked items.
Do NOT enumerate code changes, file edits, or commits — those are tracked separately.
Respond with at most 5 short bullet points, each on its own line starting with "- ". No preamble, no closing remarks.`

// SummarizeChatTranscript runs a one-shot, non-interactive cheap-model
// summarization of an ordered chat transcript using the chat's own provider. It
// is best-effort and non-fatal: it returns an error when the provider account is
// not connected, the CLI is unavailable, or the call fails, so callers fall back
// to a deterministic summary instead of blocking the turn or the handoff.
func (r *Runner) SummarizeChatTranscript(ctx context.Context, transcript string, cwd string, providerKey ProviderKey) (string, error) {
	if r == nil {
		return "", errors.New("nil runner")
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return "", errors.New("empty transcript")
	}
	if !supportsHandoffSource(providerKey) && providerKey != ProviderKeyGemini {
		// Only providers with a one-shot exec adapter can summarize.
		return "", fmt.Errorf("provider %q has no summarizer adapter", providerKey)
	}
	if len(transcript) > summarizerMaxInputBytes {
		transcript, _ = promptblock.TruncateUTF8(transcript, summarizerMaxInputBytes)
	}

	// Resolve the chat provider's connected account home. Claude additionally
	// accepts the ANTHROPIC_API_KEY fallback when no account is registered.
	homePath := ""
	if acct, err := r.ResolveProviderAccount(string(providerKey), ""); err == nil {
		homePath = strings.TrimSpace(acct.HomePath)
	} else if !(providerKey == ProviderKeyClaude && strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "") {
		return "", fmt.Errorf("no connected %s account for summarizer: %w", providerKey, err)
	}

	workspace := strings.TrimSpace(cwd)
	if workspace == "" {
		workspace = r.workspace
	}

	// Codex writes its final message to --output-last-message; give it a temp file.
	outDir, err := os.MkdirTemp("", "fp-summary-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(outDir)
	outPath := filepath.Join(outDir, "summary.txt")

	req := PromptExecutionRequest{ProviderKey: string(providerKey), ModelName: summarizerModelFor(providerKey)}
	binary, args, _, err := resolvePromptExecutionAdapter(req, outPath)
	if err != nil {
		return "", err
	}

	execCtx, cancel := context.WithTimeout(ctx, summarizerTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, binary, args...)
	cmd.Env = r.getEnvForExecution(string(providerKey), homePath, nil, "")
	cmd.Dir = workspace
	cmd.Stdin = strings.NewReader(summarizerInstruction + "\n\n<conversation>\n" + transcript + "\n</conversation>\n")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("summarizer call failed: %w", err)
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		// Codex emits the answer to the output file rather than stdout.
		if b, readErr := os.ReadFile(outPath); readErr == nil {
			out = strings.TrimSpace(string(b))
		}
	}
	if out == "" {
		return "", errors.New("summarizer returned empty output")
	}
	out, _ = promptblock.TruncateUTF8(out, summarizerMaxOutputBytes)
	return out, nil
}
