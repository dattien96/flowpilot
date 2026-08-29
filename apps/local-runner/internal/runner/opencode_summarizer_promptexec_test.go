package runner

import (
	"reflect"
	"strings"
	"testing"
)

// CP-57 Task-303 gap closure: summarizer cheap-tier and the one-shot
// `opencode run --format json` prompt-execution adapter had complete code but
// zero test coverage. Additive — codex/claude/gemini/grok branches are asserted
// unchanged inline (safe-fix-contract R2 parity).

func TestOpencodeSummarizerModelForCheapTier(t *testing.T) {
	if got := summarizerModelFor(ProviderKeyOpencode); got != "opencode/gpt-5.4-nano" {
		t.Fatalf("opencode summarizer model = %q, want opencode/gpt-5.4-nano", got)
	}
	// Parity: other providers keep their CA-era values.
	want := map[ProviderKey]string{
		ProviderKeyClaude: "haiku",
		ProviderKeyGemini: "gemini-3.5-flash-medium",
		ProviderKeyCodex:  "",
		ProviderKeyGrok:   "",
	}
	for pk, model := range want {
		if got := summarizerModelFor(pk); got != model {
			t.Fatalf("summarizerModelFor(%s) = %q, want %q (parity drift)", pk, got, model)
		}
	}
}

func TestOpencodeResolvePromptExecutionAdapterOneShot(t *testing.T) {
	req := PromptExecutionRequest{
		ProviderKey:     "opencode",
		ModelName:       "opencode/gpt-5.4-nano",
		ReasoningEffort: "high",
		Prompt:          "summarize",
		YoloMode:        true,
	}
	bin, args, resolved, err := resolvePromptExecutionAdapter(req, "/tmp/out.txt", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolvePromptExecutionAdapter: %v", err)
	}
	if bin != opencodeBinaryName() {
		t.Fatalf("binary = %q, want %q", bin, opencodeBinaryName())
	}
	if resolved != "opencode" {
		t.Fatalf("resolved provider = %q, want opencode", resolved)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"run", "--format", "json", "--model opencode/gpt-5.4-nano", "--variant high", "--auto"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q must contain %q", joined, want)
		}
	}
}

func TestOpencodeResolvePromptExecutionAdapterMinimal(t *testing.T) {
	req := PromptExecutionRequest{ProviderKey: "opencode", ModelName: "opencode-go/deepseek-v4-flash", Prompt: "s"}
	_, args, resolved, err := resolvePromptExecutionAdapter(req, "/tmp/out.txt", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolvePromptExecutionAdapter: %v", err)
	}
	if resolved != "opencode" {
		t.Fatalf("opencode-go/ prefix must resolve opencode, got %q", resolved)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--variant") || strings.Contains(joined, "--auto") {
		t.Fatalf("no effort/yolo must omit --variant/--auto, got %q", joined)
	}
	if !strings.Contains(joined, "--model opencode-go/deepseek-v4-flash") {
		t.Fatalf("args %q must carry the model", joined)
	}
}

func TestOpencodeResolvePromptExecutionAdapterPrefixWinsAndParity(t *testing.T) {
	// BUG-171 model-wins routing: a caller-supplied provider is overridden by an
	// opencode/ model prefix.
	req := PromptExecutionRequest{ProviderKey: "codex", ModelName: "opencode/muse-spark-1.2-contributor-free", Prompt: "s"}
	_, _, resolved, err := resolvePromptExecutionAdapter(req, "/tmp/o", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != "opencode" {
		t.Fatalf("model prefix must win over supplied provider, got %q", resolved)
	}

	// Parity: codex/grok branches keep their prior shape (first args element).
	codexReq := PromptExecutionRequest{ProviderKey: "codex", ModelName: "gpt-5.4", Prompt: "s"}
	codexBin, codexArgs, codexResolved, err := resolvePromptExecutionAdapter(codexReq, "/tmp/o", "/tmp/ws")
	if err != nil || codexBin != "codex" || codexResolved != "codex" {
		t.Fatalf("codex branch drift: bin=%q resolved=%q err=%v", codexBin, codexResolved, err)
	}
	if !reflect.DeepEqual(codexArgs[:2], []string{"--sandbox", "read-only"}) {
		t.Fatalf("codex branch shape drift: %v", codexArgs)
	}
	grokReq := PromptExecutionRequest{ProviderKey: "grok", ModelName: "grok-4.5", Prompt: "s"}
	grokBin, _, grokResolved, err := resolvePromptExecutionAdapter(grokReq, "/tmp/o", "/tmp/ws")
	if err != nil || grokBin != "grok" || grokResolved != "grok" {
		t.Fatalf("grok branch drift: bin=%q resolved=%q err=%v", grokBin, grokResolved, err)
	}
}
