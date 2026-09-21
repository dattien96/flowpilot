package app

import (
	"strings"

	"flowpilot-runner/internal/tui/client"
)

// Task-319: opencode image support is per-MODEL (models.dev input.image from
// `opencode models --verbose`, carried on ProviderModel.InputImage via
// GET /providers). These helpers resolve the current provider+model selection
// against that catalog; every other provider stays on the provider-level
// SupportsImages set (codex/claude/grok).

// chatSupportsImages reports whether the current provider+model selection can
// accept image attachments. Devin (CP-70) follows the same per-model rule as
// opencode — the ACP catalog reports _meta.supportsImages per model, carried
// on ProviderModel.InputImage.
func (m *AppModel) chatSupportsImages() bool {
	if client.SupportsImages(m.provider) {
		return true
	}
	provider := strings.TrimSpace(m.provider)
	if !strings.EqualFold(provider, "opencode") && !strings.EqualFold(provider, "devin") {
		return false
	}
	modelID := strings.TrimSpace(m.model)
	if modelID == "" {
		return false
	}
	for _, p := range m.providers {
		if !strings.EqualFold(strings.TrimSpace(p.Key), provider) {
			continue
		}
		for _, mod := range p.Models {
			if strings.EqualFold(strings.TrimSpace(mod.ModelID()), modelID) {
				return mod.InputImage
			}
		}
	}
	return false
}

// imagesUnsupportedReason returns user-facing copy for a chatSupportsImages
// false result. For opencode it names the model gate so the operator knows the
// fix (pick a vision model), not just the provider block.
func (m *AppModel) imagesUnsupportedReason() string {
	if m.chatSupportsImages() {
		return ""
	}
	provider := strings.TrimSpace(m.provider)
	if strings.EqualFold(provider, "opencode") {
		return "selected opencode model does not support image input (models.dev input.image=false) — pick a vision model or switch provider"
	}
	if strings.EqualFold(provider, "devin") {
		return "selected devin model does not support image input (ACP _meta.supportsImages=false) — pick a vision model or switch provider"
	}
	return client.ImagesUnsupportedReason(m.provider)
}
