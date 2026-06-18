package runner

import (
	"bufio"
	"encoding/json"
	"os"
)

// loadClaudeTranscriptEvents reads a Claude JSONL session file and converts
// each line to ProviderEvents using the existing stream-json mapper.
// Correlation fields (RunID, Seq, etc.) are stamped by the caller.
func loadClaudeTranscriptEvents(filePath string) []ProviderEvent {
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
		t, _ := raw["type"].(string)
		sub, _ := raw["subtype"].(string)
		out = append(out, mapClaudeLine(claudeLine{Type: t, Subtype: sub, Raw: raw})...)
	}
	return out
}

// loadCodexTranscriptEvents reads a Codex rollout JSONL session file and converts
// its conversation entries to ProviderEvents using the rollout mapper.
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
	return out
}
