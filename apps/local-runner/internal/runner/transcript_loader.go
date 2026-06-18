package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// loadClaudeTranscriptEvents reads a Claude JSONL session file and converts
// each line to ProviderEvents using the existing stream-json mapper.
// For user frames that carry a typed prompt (not tool_result), a turn_started{prompt}
// event is prepended so the desktop renders the prompt bubble on resume.
// Correlation fields (RunID, Seq, etc.) are stamped by the caller.
func loadClaudeTranscriptEvents(filePath string) []ProviderEvent {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []ProviderEvent
	var promptN int
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		t, _ := raw["type"].(string)
		if t == "user" {
			if text := claudeUserPromptText(raw); text != "" {
				promptN++
				out = append(out, ProviderEvent{
					Type:           EventTurnStarted,
					Prompt:         text,
					ProviderTurnID: fmt.Sprintf("replay-prompt-%d", promptN),
				})
			}
		}
		sub, _ := raw["subtype"].(string)
		out = append(out, mapClaudeLine(claudeLine{Type: t, Subtype: sub, Raw: raw})...)
	}
	return out
}

// loadCodexTranscriptEvents reads a Codex rollout JSONL session file and converts
// its conversation entries to ProviderEvents using the rollout mapper.
// mapCodexRolloutLine is stateless and cannot assign sequence numbers, so unique
// ProviderTurnIDs are stamped here on replayed user-prompt turn_started events.
// Correlation fields (RunID, Seq, etc.) are stamped by the caller.
func loadCodexTranscriptEvents(filePath string) []ProviderEvent {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []ProviderEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		out = append(out, mapCodexRolloutLine(raw)...)
	}
	// Stamp unique ProviderTurnIDs on replayed prompt turn_started events so the
	// desktop derives distinct bubble ids ("prompt-${e.providerTurnId}") per turn.
	var promptN int
	for i := range out {
		if out[i].Type == EventTurnStarted && out[i].Prompt != "" {
			promptN++
			out[i].ProviderTurnID = fmt.Sprintf("replay-prompt-%d", promptN)
		}
	}
	return out
}
