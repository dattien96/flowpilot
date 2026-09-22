package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Chat postures (Task-xxx / CA-xxx): OpenCode-style mode switching in Chat mode.
//
// Three named postures each carry a full provider profile (provider, model,
// reasoning effort, YOLO). Scan and Plan are read-only: reads are auto-approved,
// writes auto-denied, and the runner never asks. Code is today's normal gated
// chat (approvals / YOLO as configured).
//
// The config is a SINGLE shared file owned by the runner (SSOT). Both the TUI
// and the desktop read/write it ONLY through GET/PUT /client/chat-posture so the
// two surfaces can never drift.

const (
	ChatPostureScan = "scan"
	ChatPosturePlan = "plan"
	ChatPostureCode = "code"
	// ChatPostureNon (CA-685): "no mode" — the chat carries no posture pins at
	// all, so the session keeps the user's last provider/model choice across
	// restarts. Chat-only: flow mode never touches postures.
	ChatPostureNon = "non"
)

// chatPostureProfiles is the canonical posture key order.
var chatPostureProfiles = []string{ChatPostureScan, ChatPosturePlan, ChatPostureCode}

// ChatPostureProfile is one posture's provider configuration. Empty string
// fields mean "inherit the current session selection" (the composer keeps its
// own provider/model/reasoning/yolo until the user pins a value here).
type ChatPostureProfile struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Wire name is reasoningEffort (matches the turn body + Desktop contract);
	// the "reasoning" alias is also accepted on decode for older files.
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Yolo            *bool  `json:"yolo,omitempty"`
}

// UnmarshalJSON accepts both the canonical reasoningEffort wire name and the
// legacy "reasoning" alias so a file written before the rename still loads.
func (p *ChatPostureProfile) UnmarshalJSON(data []byte) error {
	var raw struct {
		Provider        string `json:"provider"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		Reasoning       string `json:"reasoning"`
		Yolo            *bool  `json:"yolo"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Provider = raw.Provider
	p.Model = raw.Model
	p.Yolo = raw.Yolo
	if raw.ReasoningEffort != "" {
		p.ReasoningEffort = raw.ReasoningEffort
	} else {
		p.ReasoningEffort = raw.Reasoning
	}
	return nil
}

// ChatPostureConfig is the persisted chat-posture document.
type ChatPostureConfig struct {
	// Active is the posture applied on new-chat / /new: one of scan/plan/code.
	Active string `json:"active"`
	// Profiles is keyed by posture name (scan/plan/code).
	Profiles map[string]ChatPostureProfile `json:"profiles"`
}

// IsReadOnlyChatPosture reports whether a posture forces the read-only policy.
func IsReadOnlyChatPosture(posture string) bool {
	return posture == ChatPostureScan || posture == ChatPosturePlan
}

// ValidChatPosture reports whether a posture name is one of scan/plan/code/non.
func ValidChatPosture(posture string) bool {
	switch posture {
	case ChatPostureScan, ChatPosturePlan, ChatPostureCode, ChatPostureNon:
		return true
	default:
		return false
	}
}

// chatPosturePaths returns candidate chat-posture file locations (FlowPilot +
// desktop-flowpilot userData, mirroring prefs.Paths). Tests override via
// FLOWPILOT_CHAT_POSTURE_FILE.
func chatPosturePaths() []string {
	if envPath := strings.TrimSpace(os.Getenv("FLOWPILOT_CHAT_POSTURE_FILE")); envPath != "" {
		return []string{envPath}
	}
	var out []string
	switch runtime.GOOS {
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			out = append(out,
				filepath.Join(appData, "FlowPilot", "chat-posture.json"),
				filepath.Join(appData, "desktop-flowpilot", "chat-posture.json"),
			)
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out,
				filepath.Join(home, "Library", "Application Support", "FlowPilot", "chat-posture.json"),
				filepath.Join(home, "Library", "Application Support", "desktop-flowpilot", "chat-posture.json"),
			)
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out,
				filepath.Join(home, ".config", "FlowPilot", "chat-posture.json"),
				filepath.Join(home, ".config", "desktop-flowpilot", "chat-posture.json"),
			)
		}
	}
	return out
}

// defaultChatPostureConfig returns the bootstrap posture document: active=non
// (CA-685 — the no-mode default, the session keeps the user's choices) with
// all profiles empty (inherit current selection). Existing files keep their
// persisted active; only fresh installs and invalid values fall to non.
func defaultChatPostureConfig() ChatPostureConfig {
	return ChatPostureConfig{
		Active:   ChatPostureNon,
		Profiles: map[string]ChatPostureProfile{},
	}
}

// normalizeChatPostureConfig validates/normalizes a config: active must be a
// valid posture (falls back to non per CA-685), profile keys outside
// scan/plan/code are dropped, and field values are trimmed.
func normalizeChatPostureConfig(cfg *ChatPostureConfig) {
	if cfg == nil {
		return
	}
	if !ValidChatPosture(cfg.Active) {
		cfg.Active = ChatPostureNon
	}
	cleaned := make(map[string]ChatPostureProfile, len(chatPostureProfiles))
	for _, key := range chatPostureProfiles {
		if p, ok := cfg.Profiles[key]; ok {
			p.Provider = strings.TrimSpace(p.Provider)
			p.Model = strings.TrimSpace(p.Model)
			p.ReasoningEffort = strings.ToLower(strings.TrimSpace(p.ReasoningEffort))
			cleaned[key] = p
		}
	}
	cfg.Profiles = cleaned
}

// loadChatPosture reads the first existing chat-posture file. Missing files
// return the default config (Active=code) with the path of the first candidate.
func loadChatPosture() (ChatPostureConfig, string) {
	paths := chatPosturePaths()
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg ChatPostureConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			continue
		}
		normalizeChatPostureConfig(&cfg)
		return cfg, path
	}
	cfg := defaultChatPostureConfig()
	if len(paths) > 0 {
		return cfg, paths[0]
	}
	return cfg, ""
}

// saveChatPosture persists the config to every candidate path (best-effort),
// returning the first path written successfully.
func saveChatPosture(cfg ChatPostureConfig) (string, error) {
	normalizeChatPostureConfig(&cfg)
	paths := chatPosturePaths()
	if len(paths) == 0 {
		return "", os.ErrNotExist
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	var (
		firstOK string
		lastErr error
	)
	for _, dest := range paths {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			lastErr = err
			continue
		}
		if err := os.WriteFile(dest, raw, 0o600); err != nil {
			lastErr = err
			continue
		}
		if firstOK == "" {
			firstOK = dest
		}
	}
	if firstOK == "" {
		if lastErr == nil {
			lastErr = os.ErrNotExist
		}
		return "", lastErr
	}
	return firstOK, nil
}

// handleGetChatPosture handles GET /client/chat-posture. Returns the full
// persisted document; both TUI and Desktop read through here (runner SSOT).
func (s *InteractiveService) handleGetChatPosture(w http.ResponseWriter, _ *http.Request) {
	cfg, _ := loadChatPosture()
	writeInteractiveJSON(w, http.StatusOK, cfg)
}

// setChatPostureRequest is the PUT body: a full config document. Both TUI and
// Desktop write through here; the runner validates/normalizes before persisting.
type setChatPostureRequest struct {
	Active   string                        `json:"active"`
	Profiles map[string]ChatPostureProfile `json:"profiles"`
}

// handleSetChatPosture handles PUT /client/chat-posture.
func (s *InteractiveService) handleSetChatPosture(w http.ResponseWriter, r *http.Request) {
	var req setChatPostureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	cfg := ChatPostureConfig{Active: req.Active, Profiles: req.Profiles}
	normalizeChatPostureConfig(&cfg)
	if _, err := saveChatPosture(cfg); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "write_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, cfg)
}

// resolveTurnChatPosture is the per-turn posture: explicit TurnInput.ChatPosture
// overrides the run default, otherwise the run's current posture is kept. Flow /
// workflow runs are never read-only postures (they use their own gating); an
// invalid/unknown value falls back to the run default.
func resolveTurnChatPosture(rs *interactiveRun, in TurnInput) string {
	if rs == nil {
		return ""
	}
	posture := rs.chatPosture
	if in.ChatPosture != "" {
		if ValidChatPosture(in.ChatPosture) {
			posture = in.ChatPosture
		}
	} else if shouldForceFlowYolo(rs.runKind, rs.workflowID, rs.flowEngineDriven) {
		// Flow-engine / workflow runs never run a read-only posture.
		posture = ""
	}
	return posture
}

// Task-260: auto-inject the safe-fix-contract umbrella skill on Chat Plan/Code.

const safeFixContractSkillName = "safe-fix-contract"

func shouldAutoInjectSafeFixContract(posture, runKind string, flowEngineDriven bool) bool {
	if flowEngineDriven {
		return false
	}
	if runKind != "chat" {
		return false
	}
	return posture == ChatPosturePlan || posture == ChatPostureCode
}

func mergeChatSafeFixContractSkills(posture, runKind string, flowEngineDriven bool, selected []SkillSelection) []SkillSelection {
	if !shouldAutoInjectSafeFixContract(posture, runKind, flowEngineDriven) {
		return selected
	}
	for _, s := range selected {
		if strings.EqualFold(strings.TrimSpace(s.Name), safeFixContractSkillName) {
			return selected
		}
	}
	out := make([]SkillSelection, 0, len(selected)+1)
	out = append(out, selected...)
	out = append(out, SkillSelection{Name: safeFixContractSkillName, Source: "builtin"})
	return out
}
