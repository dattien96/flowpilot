package runner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

var recoverGeminiAgyLatestMessageFn = recoverGeminiAgyLatestMessage

func recoverGeminiAgyLatestMessage(cwd string, env map[string]string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	appDataDir := geminiAgyAppDataDir(env)
	if appDataDir == "" {
		return ""
	}
	conversationID := geminiAgyConversationIDForWorkspace(appDataDir, cwd)
	if conversationID == "" || !isSafeGeminiConversationID(conversationID) {
		return ""
	}
	dbPath := filepath.Join(appDataDir, "conversations", conversationID+".db")
	if _, err := os.Stat(dbPath); err != nil {
		return ""
	}
	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		log.Printf("[gemini-agy] recovery=conversation_db status=unavailable reason=sqlite3_not_found cwd=%q", cwd)
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	query := "select hex(step_payload) from steps where step_type=15 and length(step_payload)>0 order by idx desc limit 1;"
	out, err := exec.CommandContext(ctx, sqlitePath, "-batch", "-noheader", dbPath, query).Output()
	if err != nil {
		log.Printf("[gemini-agy] recovery=conversation_db status=query_failed cwd=%q db=%q err=%v", cwd, dbPath, err)
		return ""
	}
	hexPayload := strings.TrimSpace(string(out))
	if hexPayload == "" {
		return ""
	}
	payload, err := hex.DecodeString(hexPayload)
	if err != nil {
		log.Printf("[gemini-agy] recovery=conversation_db status=decode_failed cwd=%q db=%q err=%v", cwd, dbPath, err)
		return ""
	}
	return extractGeminiAgyOutputFromPayload(payload)
}

func geminiAgyAppDataDir(env map[string]string) string {
	candidates := []string{}
	if env != nil {
		if geminiHome := strings.TrimSpace(env["GEMINI_HOME"]); geminiHome != "" {
			candidates = append(candidates, filepath.Join(geminiHome, "antigravity-cli"))
			candidates = append(candidates, filepath.Join(geminiHome, ".gemini", "antigravity-cli"))
		}
		if home := strings.TrimSpace(env["HOME"]); home != "" {
			candidates = append(candidates, filepath.Join(home, ".gemini", "antigravity-cli"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".gemini", "antigravity-cli"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func geminiAgyConversationIDForWorkspace(appDataDir, cwd string) string {
	raw, err := os.ReadFile(filepath.Join(appDataDir, "cache", "last_conversations.json"))
	if err != nil {
		return ""
	}
	var conversations map[string]string
	if err := json.Unmarshal(raw, &conversations); err != nil {
		return ""
	}
	for workspace, conversationID := range conversations {
		if sameFilePath(workspace, cwd) {
			return strings.TrimSpace(conversationID)
		}
	}
	return ""
}

func isSafeGeminiConversationID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func extractGeminiAgyOutputFromPayload(payload []byte) string {
	var candidates []string
	collectGeminiProtoStrings(payload, 0, &candidates)
	best := ""
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if !isGeminiAgyMessageCandidate(candidate) {
			continue
		}
		if len(candidate) > len(best) {
			best = candidate
		}
	}
	return best
}

func collectGeminiProtoStrings(data []byte, depth int, out *[]string) {
	if depth > 4 || len(data) == 0 {
		return
	}
	for i := 0; i < len(data); {
		key, n := readGeminiProtoVarint(data[i:])
		if n <= 0 {
			i++
			continue
		}
		i += n
		wireType := key & 0x7
		switch wireType {
		case 0:
			_, n = readGeminiProtoVarint(data[i:])
			if n <= 0 {
				i++
			} else {
				i += n
			}
		case 1:
			i += min(8, len(data)-i)
		case 2:
			size, n := readGeminiProtoVarint(data[i:])
			if n <= 0 {
				i++
				continue
			}
			i += n
			if size > uint64(len(data)-i) {
				i = len(data)
				continue
			}
			field := data[i : i+int(size)]
			if utf8.Valid(field) {
				text := strings.TrimSpace(string(field))
				if text != "" {
					*out = append(*out, text)
				}
			}
			collectGeminiProtoStrings(field, depth+1, out)
			i += int(size)
		case 5:
			i += min(4, len(data)-i)
		default:
			i++
		}
	}
}

func readGeminiProtoVarint(data []byte) (uint64, int) {
	var value uint64
	for i, b := range data {
		if i == 10 {
			return 0, 0
		}
		value |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return value, i + 1
		}
	}
	return 0, 0
}

func isGeminiAgyMessageCandidate(text string) bool {
	if len(text) < 8 {
		return false
	}
	if !(strings.ContainsAny(text, " \n\t") || strings.Contains(text, "FP_")) {
		return false
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "file:///") || strings.HasPrefix(text, "bot-") {
		return false
	}
	if strings.Contains(text, "sessionID") || strings.Contains(text, "ResponseID") {
		return false
	}
	printable := 0
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' || r >= 0x20 {
			printable++
		}
	}
	return printable*100/len([]rune(text)) >= 90
}
