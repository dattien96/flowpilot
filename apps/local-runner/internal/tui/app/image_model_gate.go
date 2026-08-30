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
// accept image attachments.
func (m *AppModel) chatSupportsImages() bool {
	if client.SupportsImages(m.provider) {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(m.provider), "opencode") {
		return false
	}
	modelID := strings.TrimSpace(m.model)
	if modelID == "" {
		return false
	}
	for _, p := range m.providers {
		if !strings.EqualFold(strings.TrimSpace(p.Key), "opencode") {
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
	if strings.EqualFold(strings.TrimSpace(m.provider), "opencode") {
		return "selected opencode model does not support image input (models.dev input.image=false) — pick a vision model or switch provider"
	}
	return client.ImagesUnsupportedReason(m.provider)
}
