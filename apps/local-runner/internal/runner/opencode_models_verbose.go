package runner

import (
	"encoding/json"
	"sort"
	"strings"
)

// opencodeVerboseModelMeta mirrors one metadata JSON blob emitted by
// `opencode models --verbose` (v1.18.25): an ID line (`opencode/big-pickle`)
// followed by a pretty-printed JSON object. Only capability fields FlowPilot
// consumes are decoded; everything else is ignored.
type opencodeVerboseModelMeta struct {
	Capabilities struct {
		Input struct {
			Image bool `json:"image"`
		} `json:"input"`
	} `json:"capabilities"`
}

// parseOpencodeVerboseModelsOutput parses the `--verbose` output shape:
// repeated `<providerID>/<modelID>` lines each followed by one JSON blob. It
// returns ok=false for any output that is not in this shape (older CLIs, or
// the plain `opencode models` line list) so the caller can fall back to
// parseOpencodePlainModelsOutput. Task-319.
func parseOpencodeVerboseModelsOutput(output []byte) ([]ProviderModel, bool) {
	s := string(output)
	models := make([]ProviderModel, 0, 32)
	pos := 0
	for pos < len(s) {
		nl := strings.IndexByte(s[pos:], '\n')
		var raw string
		if nl < 0 {
			raw = s[pos:]
			pos = len(s)
		} else {
			raw = s[pos : pos+nl]
			pos += nl + 1
		}
		id := strings.TrimSpace(raw)
		if !opencodeVerboseIDLine(id) {
			continue
		}
		// BUG-431: same non-chat-family filter as the plain parser.
		if !opencodeIsChatCapableModel(id) {
			// Still must consume the following JSON blob — skip it by letting
			// the decoder run so pos advances past the metadata.
			braceSkip := strings.IndexByte(s[pos:], '{')
			if braceSkip < 0 || opencodeVerboseIDLinesIn(s[pos:pos+braceSkip]) {
				return nil, false
			}
			skipDec := json.NewDecoder(strings.NewReader(s[pos+braceSkip:]))
			var skipMeta opencodeVerboseModelMeta
			if err := skipDec.Decode(&skipMeta); err != nil {
				return nil, false
			}
			pos += braceSkip + int(skipDec.InputOffset())
			continue
		}
		// The metadata blob must start at the next `{` with no other ID line
		// in between — otherwise this is not the verbose shape.
		brace := strings.IndexByte(s[pos:], '{')
		if brace < 0 || opencodeVerboseIDLinesIn(s[pos:pos+brace]) {
			return nil, false
		}
		dec := json.NewDecoder(strings.NewReader(s[pos+brace:]))
		var meta opencodeVerboseModelMeta
		if err := dec.Decode(&meta); err != nil {
			return nil, false
		}
		model := ProviderModel{
			ID:                        id,
			DisplayName:               opencodeModelDisplayName(id),
			Source:                    "opencode_models",
			Available:                 true,
			SupportedReasoningEfforts: []string{"minimal", "low", "medium", "high", "xhigh"},
			DefaultReasoningEffort:    "medium",
			InputImage:                meta.Capabilities.Input.Image,
		}
		models = append(models, model)
		pos += brace + int(dec.InputOffset())
	}
	if len(models) == 0 {
		return nil, false
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, true
}

// opencodeVerboseIDLine reports whether the line is a model ID header line in
// the `--verbose` output: `provider/segments` with no spaces. Stricter than
// the plain-format skip logic because JSON blob lines contain spaces and
// braces and must never be mistaken for ID lines.
func opencodeVerboseIDLine(line string) bool {
	if line == "" || strings.Contains(line, " ") {
		return false
	}
	if !strings.Contains(line, "/") {
		return false
	}
	// Skip the broken default model if ever listed (ProviderModelNotFoundError).
	if line == "opencode/deepseek-v4-flash-free" {
		return false
	}
	return true
}

// opencodeIsChatCapableModel reports whether a catalog id is a conversational
// model usable for chat turns (BUG-431). Multi-family accounts list Google's
// full model family via models.dev — including families that cannot serve a
// chat turn: Interactions-API-only deep-research (every turn fails "This model
// only supports Interactions API"), embeddings, image/video/music generation,
// TTS, live-API, and computer-use models. Filtering them here keeps pickers
// and default resolution on turn-capable ids.
func opencodeIsChatCapableModel(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return false
	}
	for _, bad := range []string{
		"deep-research", "deep_research",
		"embedding",
		"veo-", "lyria",
		"tts",
		"live-",
		"computer-use",
	} {
		if strings.Contains(id, bad) {
			return false
		}
	}
	return true
}

func opencodeVerboseIDLinesIn(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if opencodeVerboseIDLine(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

// ensureOpencodeVerboseFallback keeps the plain parser reachable for CLIs that
// do not support `--verbose` (pre-1.18.x): detectOpencodeModelsLive falls back
// to a plain `models` spawn when the verbose spawn fails, and the dispatcher
// falls back to the line parser when no JSON blobs are present.
