package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-319: the per-model image gate resolves the current provider+model
// selection against the GET /providers catalog (ProviderModel.InputImage).

func newImageGateModel(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{Provider: "opencode"}, "http://127.0.0.1:1")
	m.provider = "opencode"
	m.model = "opencode/mimo-v2.5-free"
	m.providers = []client.Provider{
		{
			Key: "opencode",
			Models: []client.ProviderModel{
				{ID: "opencode/big-pickle", InputImage: false},
				{ID: "opencode/mimo-v2.5-free", InputImage: true},
			},
		},
	}
	return m
}

func TestChatSupportsImages_opencodeVisionModelUnlocks(t *testing.T) {
	m := newImageGateModel(t)
	if !m.chatSupportsImages() {
		t.Fatal("opencode model with inputImage=true must unlock image support")
	}
}

func TestChatSupportsImages_opencodeNonVisionModelStaysGated(t *testing.T) {
	m := newImageGateModel(t)
	m.model = "opencode/big-pickle"
	if m.chatSupportsImages() {
		t.Fatal("opencode model with inputImage=false must stay gated")
	}
}

func TestChatSupportsImages_opencodeUnknownModelConservative(t *testing.T) {
	m := newImageGateModel(t)
	m.model = "opencode/not-in-catalog"
	if m.chatSupportsImages() {
		t.Fatal("unknown opencode model must stay gated (conservative)")
	}
	m.model = ""
	if m.chatSupportsImages() {
		t.Fatal("no selected model must stay gated (conservative)")
	}
}

func TestChatSupportsImages_providerLevelSetUnchanged(t *testing.T) {
	m := newImageGateModel(t)
	m.provider = "codex"
	m.model = ""
	if !m.chatSupportsImages() {
		t.Fatal("codex must keep provider-level image support")
	}
	m.provider = "gemini"
	if m.chatSupportsImages() {
		t.Fatal("gemini must stay gated")
	}
}

func TestImagesUnsupportedReason_namesTheModelGate(t *testing.T) {
	m := newImageGateModel(t)
	m.model = "opencode/big-pickle"
	reason := m.imagesUnsupportedReason()
	if !strings.Contains(reason, "input.image") {
		t.Fatalf("opencode reason must name the per-model gate, got %q", reason)
	}
	m.model = "opencode/mimo-v2.5-free"
	if reason := m.imagesUnsupportedReason(); reason != "" {
		t.Fatalf("vision model must have empty reason, got %q", reason)
	}
	m.provider = "gemini"
	if reason := m.imagesUnsupportedReason(); !strings.Contains(reason, "gemini") {
		t.Fatalf("gemini reason must name the provider, got %q", reason)
	}
}
