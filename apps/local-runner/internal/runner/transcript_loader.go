package runner

import (
	"encoding/json"
	"fmt"
	"os"
)

// loadClaudeTranscriptEvents reads a Claude JSONL session file and converts
// each line to ProviderEvents using the existing stream-json mapper.
// For user frames that carry a typed prompt (not tool_result), a turn_started{prompt}
// event is prepended so the desktop renders the prompt bubble on resume.
// Correlation fields (RunID, Seq, etc.) are stamped by the caller.
//
// BUG-487: the line reader is the uncapped readNDJSONLines and a mid-file
// read failure is returned alongside the parsed prefix — a truncated file is
// never indistinguishable from a short one. A missing/unopenable file stays
// (nil, nil): absence is not corruption.
func loadClaudeTranscriptEvents(filePath string) ([]ProviderEvent, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var out []ProviderEvent
	var promptN int
	scanErr := readNDJSONLines(f, func(line []byte) {
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			return
		}
		t, _ := raw["type"].(string)
		if t == "user" {
			if text := claudeUserPromptText(raw); text != "" {
				promptN++
				out = append(out, ProviderEvent{
					Type:           EventTurnStarted,
					Prompt:         text,
					ProviderTurnID: fmt.Sprintf("replay-prompt-%d", promptN),
					OccurredAt:     transcriptOccurredAt(raw),
				})
			}
		}
		sub, _ := raw["subtype"].(string)
		mapped := mapClaudeLine(claudeLine{Type: t, Subtype: sub, Raw: raw})
		for i := range mapped {
			mapped[i].OccurredAt = transcriptOccurredAt(raw)
		}
		out = append(out, mapped...)
	})
	return out, scanErr
}

// loadCodexTranscriptEvents reads a Codex rollout JSONL session file and converts
// its conversation entries to ProviderEvents using the rollout mapper.
// mapCodexRolloutLine is stateless and cannot assign sequence numbers, so unique
// ProviderTurnIDs are stamped here on replayed user-prompt turn_started events.
// Correlation fields (RunID, Seq, etc.) are stamped by the caller.
func loadCodexTranscriptEvents(filePath string) ([]ProviderEvent, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var out []ProviderEvent
	scanErr := readNDJSONLines(f, func(line []byte) {
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			return
		}
		mapped := mapCodexRolloutLine(raw)
		for i := range mapped {
			mapped[i].OccurredAt = transcriptOccurredAt(raw)
		}
		out = append(out, mapped...)
	})
	// Stamp unique ProviderTurnIDs on replayed prompt turn_started events so the
	// desktop derives distinct bubble ids ("prompt-${e.providerTurnId}") per turn.
	var promptN int
	for i := range out {
		if out[i].Type == EventTurnStarted && out[i].Prompt != "" {
			promptN++
			out[i].ProviderTurnID = fmt.Sprintf("replay-prompt-%d", promptN)
		}
	}
	return out, scanErr
}
