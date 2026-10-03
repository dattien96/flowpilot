package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// chatLanguageFile is the per-project settings file selecting the natural
// language for AI-authored chat output (Task-458).
const chatLanguageFile = "settings/chat-language.json"

// LoadChatLanguage reads .flowpilot/settings/chat-language.json and returns
// the configured language tag (e.g. "vi"). Missing or corrupt files return
// "" — callers fall back through the resolution chain. Chat language is UX,
// never a gate: fail-open, never an error.
func LoadChatLanguage(dotFP string) string {
	if dotFP == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dotFP, chatLanguageFile))
	if err != nil {
		return ""
	}
	var cfg struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Language)
}

// resolvedChatLanguage is the resolution chain for a run's chat language
// (Task-458 T-4): project settings file → runner env default → English.
func (s *InteractiveService) resolvedChatLanguage(workspaceCwd string) string {
	if lang := LoadChatLanguage(filepath.Join(workspaceCwd, ".flowpilot")); lang != "" {
		return lang
	}
	if lang := strings.TrimSpace(os.Getenv("FLOWPILOT_CHAT_LANGUAGE")); lang != "" {
		return lang
	}
	return "en"
}

// responseLanguageClause renders the directive appended to provider-bound
// prompts when the resolved chat language is not English. The clause
// instructs the model to answer in the target language for natural-language
// output while keeping every machine-format contract — tool calls, verdict
// enums, file paths, document ids, structured payloads — byte-exact.
func responseLanguageClause(lang string) string {
	const keepList = "Keep tool calls, status/verdict values, code, file paths, " +
		"document ids, and structured outputs in their exact required format."
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "", "en", "eng", "english":
		return ""
	case "vi", "vie", "vietnamese", "tieng-viet", "tieng viet", "tiếng việt":
		return "[FlowPilot language directive — respond to the user in Vietnamese (Tiếng Việt) " +
			"for all natural-language output: summaries, questions, explanations, and final messages. " + keepList + "]"
	default:
		return "[FlowPilot language directive — respond to the user in " + strings.TrimSpace(lang) +
			" for all natural-language output: summaries, questions, explanations, and final messages. " + keepList + "]"
	}
}

// promptWithChatLanguage appends the resolved response-language directive to
// a provider-bound prompt (Task-458). Prompts under English resolution are
// returned byte-identical — full backward compatibility.
func (s *InteractiveService) promptWithChatLanguage(workspaceCwd, prompt string) string {
	if clause := responseLanguageClause(s.resolvedChatLanguage(workspaceCwd)); clause != "" {
		return prompt + "\n\n" + clause
	}
	return prompt
}
