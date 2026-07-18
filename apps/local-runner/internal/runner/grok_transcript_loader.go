package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Grok transcript replay (Task-212 T-6 / BUG-GrokReplay-Restart).
//
// Unlike Task-212's original assumption that Grok transcripts lived only in a
// ~/.grok/sessions SQLite (which was why it was deferred), Grok Build actually
// persists a Claude-shaped JSONL per session directory:
//
//	<grokHome>/sessions/<url-encoded-cwd>/<session-uuid>/chat_history.jsonl
//
// where <url-encoded-cwd> percent-encodes the workspace path (e.g.
// "D:\working\gate-sandbox" -> "D%3A%5Cworking%5Cgate-sandbox"). Each line is a
// message frame:
//
//	system      — the CLI system prompt (skipped)
//	user        — content:[{type:"text",text:"..."}]; the real prompt is inside
//	              a <user_query>...</user_query> tag; a leading <user_info> frame
//	              is the injected environment context (skipped, mirroring the
//	              Claude loader's own AGENTS.md/context filtering)
//	reasoning   — model thinking (encrypted_content, no display text; skipped)
//	assistant   — content:"<text>" plus tool_calls:[{id,name,arguments}]
//	tool_result — {tool_call_id, content}
//
// Because FlowPilot only ever stored a synthetic "thread-<n>" session id for a
// Grok run (the real ACP session id is never fed back for session/load — see
// grok_adapter.go ensureSession), every FlowPilot turn spins a fresh Grok
// session, i.e. one session dir per turn. seedTranscriptFromDisk therefore
// concatenates them in chronological order, exactly like the Codex per-turn
// rollout replay (BUG-083 F-3).

// loadGrokTranscriptEvents parses a single Grok chat_history.jsonl into
// ProviderEvents. Correlation fields (RunID, Seq, etc.) are stamped by the
// caller. Unique replay ProviderTurnIDs are stamped by the caller across the
// concatenated set so bubble ids stay distinct across per-turn session files.
func loadGrokTranscriptEvents(filePath string) []ProviderEvent {
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
		switch raw["type"] {
		case "user":
			if prompt := grokUserQueryText(raw); prompt != "" {
				out = append(out, ProviderEvent{Type: EventTurnStarted, Prompt: prompt, OccurredAt: transcriptOccurredAt(raw)})
			}
		case "assistant":
			if text, _ := raw["content"].(string); strings.TrimSpace(text) != "" {
				out = append(out, ProviderEvent{Type: EventMessageCompleted, Text: text, OccurredAt: transcriptOccurredAt(raw)})
			}
			for _, tc := range grokAssistantToolCalls(raw) {
				out = append(out, ProviderEvent{Type: EventToolStarted, ToolName: tc.name, Input: tc.input, OccurredAt: transcriptOccurredAt(raw)})
			}
		case "tool_result":
			name, _ := raw["tool_name"].(string) // usually absent; tool name comes from the call
			content := grokToolResultContent(raw)
			out = append(out, ProviderEvent{Type: EventToolCompleted, ToolName: name, Output: content, Status: "success", OccurredAt: transcriptOccurredAt(raw)})
		}
	}
	return out
}

func transcriptOccurredAt(raw map[string]any) string {
	for _, key := range []string{"timestamp", "createdAt", "created_at", "occurredAt", "occurred_at"} {
		if value, _ := raw[key].(string); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type grokToolCall struct {
	name  string
	input any
}

// grokAssistantToolCalls extracts the tool calls from an assistant frame,
// parsing each call's JSON-string arguments into a map when possible (falling
// back to the raw string) so the desktop can render the tool input.
func grokAssistantToolCalls(raw map[string]any) []grokToolCall {
	rawCalls, ok := raw["tool_calls"].([]any)
	if !ok {
		return nil
	}
	out := make([]grokToolCall, 0, len(rawCalls))
	for _, rc := range rawCalls {
		call, ok := rc.(map[string]any)
		if !ok {
			continue
		}
		name, _ := call["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		var input any
		if argsStr, ok := call["arguments"].(string); ok && argsStr != "" {
			var parsed any
			if json.Unmarshal([]byte(argsStr), &parsed) == nil {
				input = parsed
			} else {
				input = argsStr
			}
		} else if argsObj, ok := call["arguments"]; ok {
			input = argsObj
		}
		out = append(out, grokToolCall{name: name, input: input})
	}
	return out
}

// grokToolResultContent returns the tool_result content as a display value.
// Grok stores it as a plain string in the observed transcripts, but tolerate a
// structured value too.
func grokToolResultContent(raw map[string]any) any {
	if c, ok := raw["content"].(string); ok {
		return c
	}
	return raw["content"]
}

// grokUserQueryText returns the real user prompt from a Grok user frame — the
// text inside the <user_query>...</user_query> tag. Returns "" for the leading
// <user_info> environment-context frame (the injected preamble, not a typed
// prompt) so it is not rendered as a prompt bubble on replay.
func grokUserQueryText(raw map[string]any) string {
	parts, ok := raw["content"].([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if txt, _ := part["text"].(string); txt != "" {
			b.WriteString(txt)
		}
	}
	full := b.String()
	if inner, ok := extractTagged(full, "user_query"); ok {
		return strings.TrimSpace(inner)
	}
	return ""
}

// extractTagged returns the content between <tag>...</tag> (first occurrence).
func extractTagged(s, tag string) (string, bool) {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// grokSessionsCwdDirName encodes a workspace path the way Grok names its
// per-cwd session directory: percent-encode every byte that is not an RFC-3986
// unreserved character (A-Z a-z 0-9 - . _ ~), with uppercase hex. Verified
// against the real store: "D:\working\gate-sandbox" -> "D%3A%5Cworking%5Cgate-sandbox",
// "C:\Users\dat.nguyen" -> "C%3A%5CUsers%5Cdat.nguyen".
func grokSessionsCwdDirName(cwd string) string {
	var b strings.Builder
	for i := 0; i < len(cwd); i++ {
		c := cwd[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// grokSessionDirPath returns the Grok session directory for a real ACP session
// id under a workspace cwd: <grokHome>/sessions/<encoded-cwd>/<sessionID>/.
func grokSessionDirPath(grokHome, cwd, sessionID string) string {
	return filepath.Join(grokHome, "sessions", grokSessionsCwdDirName(cwd), sessionID)
}

// isGrokRealSessionID reports whether sessionID is a provider-owned Grok ACP
// session id safe to use as a single path segment under sessions/<cwd>/.
// Rejects FlowPilot synthetic thread-* handles and any id that could escape
// the session tree (path separators, . / .., absolute paths).
func isGrokRealSessionID(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" || strings.HasPrefix(id, "thread-") {
		return false
	}
	if id == "." || id == ".." {
		return false
	}
	if strings.ContainsAny(id, `/\`) {
		return false
	}
	// filepath.IsAbs catches Windows drive paths when present; Clean would
	// otherwise let ".." through before the ContainsAny check above.
	if filepath.IsAbs(id) {
		return false
	}
	if filepath.Base(id) != id {
		return false
	}
	return true
}

// grokChatHistoryPath returns the chat_history.jsonl path for a given Grok
// session id under a workspace cwd.
func grokChatHistoryPath(grokHome, cwd, sessionID string) string {
	return filepath.Join(grokSessionDirPath(grokHome, cwd, sessionID), "chat_history.jsonl")
}

// discoverGrokSessionDirs returns the session ids under <grokHome>/sessions/
// <enc-cwd>/ that have a chat_history.jsonl, ordered oldest-first by the file's
// modification time. This is the best-effort fallback for a run with no
// per-turn session ids recorded in its turn log (e.g. a run created before the
// turn-log capture landed): it replays every Grok session for that workspace in
// chronological order. UUIDv7 session ids are themselves time-ordered, so mtime
// and id order generally agree.
func discoverGrokSessionDirs(grokHome, cwd string) []string {
	base := filepath.Join(grokHome, "sessions", grokSessionsCwdDirName(cwd))
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	type dirTime struct {
		id      string
		modUnix int64
	}
	dirs := make([]dirTime, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		chatPath := filepath.Join(base, e.Name(), "chat_history.jsonl")
		info, statErr := os.Stat(chatPath)
		if statErr != nil {
			continue
		}
		dirs = append(dirs, dirTime{id: e.Name(), modUnix: info.ModTime().UnixNano()})
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		if dirs[i].modUnix == dirs[j].modUnix {
			return dirs[i].id < dirs[j].id
		}
		return dirs[i].modUnix < dirs[j].modUnix
	})
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, d.id)
	}
	return out
}
