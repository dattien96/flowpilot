package runner

import "testing"

// Task-318 (CP-57 P-12 / Task-303 T-7 / DOD-15): Opencode Vision stays false
// until the ACP image attachment round-trip is proven. This guard locks the
// text-only prompt contract so a premature image block fails CI.
//
// additive-tests-only: new file only. Do not edit
// TestOpencodeCapabilitiesMatchProvenSet or TestOpencodeACPPromptParamsBuildsCorrectJSON.
func TestOpencodePromptParamsNeverBuildsImageBlock(t *testing.T) {
	params := opencodeACPPromptParams("ses_1", "hello")
	prompt, ok := params["prompt"].([]map[string]string)
	if !ok {
		t.Fatalf("expected prompt []map[string]string, got %T", params["prompt"])
	}
	if len(prompt) != 1 {
		t.Fatalf("expected exactly 1 prompt block, got %d", len(prompt))
	}
	if prompt[0]["type"] != "text" {
		t.Fatalf("expected type=text, got %q", prompt[0]["type"])
	}
	if prompt[0]["text"] != "hello" {
		t.Fatalf("expected text=hello, got %q", prompt[0]["text"])
	}
	// Defensive contract: the prompt builder must NEVER emit an image block.
	// opencode_acp.go:88-90 records promptCapabilities.image was live-verified
	// true in the initialize handshake, but the attachment round-trip (Task-301)
	// was never proven — so Vision stays false and no image block is built.
	for i, block := range prompt {
		if block["type"] == "image" {
			t.Fatalf("prompt block %d is an image block — Vision must stay false until round-trip proven", i)
		}
	}
}
