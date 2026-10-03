package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chatLangSvc(t *testing.T) *InteractiveService {
	t.Helper()
	return newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
}

func writeChatLang(t *testing.T, repoDir, body string) string {
	t.Helper()
	dot := filepath.Join(repoDir, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dot, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dot, "settings", "chat-language.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dot
}

// Default (no settings file, no env) must leave every prompt byte-identical —
// the feature is opt-in only.
func TestChatLanguage_DefaultsEnglishPromptUnchanged(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_LANGUAGE", "")
	svc := chatLangSvc(t)
	repo := t.TempDir()
	const prompt = "You are the reviewer sub-agent. Review the diff."
	if got := svc.promptWithChatLanguage(repo, prompt); got != prompt {
		t.Fatalf("default resolution must not alter the prompt, got %q", got)
	}
}

// Vietnamese setting → the clause lands after the original prompt and names
// the machine-format keep-list.
func TestChatLanguage_VietnameseClauseAppended(t *testing.T) {
	svc := chatLangSvc(t)
	repo := t.TempDir()
	writeChatLang(t, repo, `{"language":"vi"}`)
	const prompt = "You are the coder sub-agent. Implement the requested change."
	got := svc.promptWithChatLanguage(repo, prompt)
	if !strings.HasPrefix(got, prompt) {
		t.Fatal("clause must append after the original prompt, not replace it")
	}
	for _, want := range []string{"Vietnamese (Tiếng Việt)", "final messages", "tool calls", "exact required format"} {
		if !strings.Contains(got, want) {
			t.Fatalf("clause missing %q: %s", want, got)
		}
	}
}

// The clause must never disturb contract literals in the prompt — verdict
// enums and tool names stay byte-exact.
func TestChatLanguage_ClausePreservesContractLiterals(t *testing.T) {
	svc := chatLangSvc(t)
	repo := t.TempDir()
	writeChatLang(t, repo, `{"language":"vi"}`)
	const prompt = "Call submit_review_outcome with status=approved|changes_requested|blocked. AC-1 needs file:line evidence."
	got := svc.promptWithChatLanguage(repo, prompt)
	if !strings.Contains(got, "status=approved|changes_requested|blocked") {
		t.Fatal("verdict enum must remain verbatim")
	}
	if !strings.Contains(got, "submit_review_outcome") {
		t.Fatal("tool name must remain verbatim")
	}
}

// Corrupt settings fall back to English — chat language is UX, never a gate.
func TestChatLanguage_CorruptFileFallsBackEnglish(t *testing.T) {
	svc := chatLangSvc(t)
	repo := t.TempDir()
	writeChatLang(t, repo, `{not json`)
	const prompt = "hello"
	if got := svc.promptWithChatLanguage(repo, prompt); got != prompt {
		t.Fatalf("corrupt settings must fall back to English, got %q", got)
	}
}

// Runner env default applies when the project declares nothing.
func TestChatLanguage_EnvFallbackApplies(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_LANGUAGE", "vi")
	svc := chatLangSvc(t)
	repo := t.TempDir() // no settings file
	got := svc.promptWithChatLanguage(repo, "prompt")
	if !strings.Contains(got, "Vietnamese") {
		t.Fatalf("env default must resolve vi, got %q", got)
	}
}

// Project file wins over the env default — project is the more specific scope.
func TestChatLanguage_ProjectFileBeatsEnv(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_LANGUAGE", "fr")
	svc := chatLangSvc(t)
	repo := t.TempDir()
	writeChatLang(t, repo, `{"language":"vi"}`)
	got := svc.promptWithChatLanguage(repo, "prompt")
	if !strings.Contains(got, "Vietnamese") || strings.Contains(got, " in fr ") {
		t.Fatalf("project file must beat env default, got %q", got)
	}
}
