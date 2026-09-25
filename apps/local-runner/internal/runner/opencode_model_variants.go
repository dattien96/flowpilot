package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CA-689c: on-demand per-model reasoning variants. Chatting first to learn a
// model's real effort options is wrong UX — the picker must be correct the
// moment a model is selected. The variants live ONLY in the ACP session config
// (the `opencode models` CLI cannot report them), so fetching means: reuse the
// account-scoped `opencode acp` process, open a throwaway session, set the
// model, read the effort select from the response, close the session — seconds,
// no prompt, no turn.

// fetchDispatcher is a test seam: when set, FetchOpencodeModelVariants talks
// to this dispatcher instead of ensuring a real `opencode acp` process.
var fetchDispatcher *opencodeDispatcher

// BUG-331: opencode's default permission config is allow-all ("opencode allows
// all operations without approval"), so a YOLO-off turn could never show an
// approval card — opencode executed every write/bash silently and the runner's
// complete decision layer (handleInbound → yolo auto-approve / bridge
// RequestApproval / read-only posture deny) stayed dead code. `opencode acp`
// has no permission launch flag (verified 1.18.25 `--help`), but the config
// permission block does gate it: live probes proved OPENCODE_CONFIG_CONTENT
// with permission edit/bash=ask makes opencode emit real session/request_permission
// calls (allow → file created, reject → file NOT created, turn still end_turn),
// that the overlay WINS over the account config file on conflict and
// DEEP-MERGES without wiping its other keys (mcpServers/auth intact), and that
// opencode survives a client error reply to its fs/write_text_file probe.
// Always spawn ask-gated; the per-turn YOLO/posture decision is made runner-side
// (yoloModes + bridge), so no respawn on toggle (that would re-orphan sessions,
// BUG-329). webfetch is deliberately NOT pinned: Codex/Claude/Grok gate
// file/exec only.
const (
	opencodePermissionOverlayEnv  = "OPENCODE_CONFIG_CONTENT"
	opencodePermissionOverlayJSON = `{"permission":{"edit":"ask","bash":"ask"}}`
)

// opencodeLaunchEnv resolves the account-scoped scopeKey + launch env for an
// `opencode acp` process (shared by the turn adapter factory and the variant
// prober so both always agree). Task-447: the ForAccount variant resolves a
// routing pin ("" = active account, as before).
func (r *Runner) opencodeLaunchEnv() (string, map[string]string, error) {
	return r.opencodeLaunchEnvForAccount("")
}

func (r *Runner) opencodeLaunchEnvForAccount(accountID string) (string, map[string]string, error) {
	scopeKey := "default"
	env := map[string]string{}
	account, err := r.resolveAdapterAccount(string(ProviderKeyOpencode), accountID)
	if err == nil {
		scopeKey = account.ID
		for k, v := range account.ExtraEnv {
			env[k] = v
		}
		if account.HomePath != "" {
			env["OPENCODE_HOME"] = account.HomePath
			env["HOME"] = account.HomePath
			env["XDG_CONFIG_HOME"] = filepath.Join(account.HomePath, ".config")
			env["XDG_DATA_HOME"] = opencodeDataHomeForAccount(account.HomePath)
			env["OPENCODE_CONFIG"] = opencodeConfigFilePath(account.HomePath)
			if drive, path, ok := windowsHomeDriveAndPath(account.HomePath); ok {
				env["USERPROFILE"] = account.HomePath
				env["APPDATA"] = filepath.Join(account.HomePath, "AppData", "Roaming")
				env["LOCALAPPDATA"] = filepath.Join(account.HomePath, "AppData", "Local")
				env["HOMEDRIVE"] = drive
				env["HOMEPATH"] = path
			}
		}
		// BUG-331: the ask-gate overlay must win over account.ExtraEnv, so it is
		// assigned LAST here (a stale/custom account env carrying the key must
		// never ungate the process). Both the turn path and the variants prober
		// share this map, so same-scope reuse never hands a chat turn an
		// ungated process.
		env[opencodePermissionOverlayEnv] = opencodePermissionOverlayJSON
		return scopeKey, env, nil
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return "", nil, err
	}
	scopeKey = "env:" + home
	env["OPENCODE_HOME"] = home
	env["HOME"] = home
	env["XDG_CONFIG_HOME"] = filepath.Join(home, ".config")
	env["XDG_DATA_HOME"] = opencodeDataHomeForAccount(home)
	env["OPENCODE_CONFIG"] = opencodeConfigFilePath(home)
	env[opencodePermissionOverlayEnv] = opencodePermissionOverlayJSON
	return scopeKey, env, nil
}

// FetchOpencodeModelVariants returns the real reasoning effort options for an
// opencode model. Cached observations short-circuit (fast path); otherwise a
// throwaway ACP session is probed and the result recorded for every later
// reader (TUI picker, Desktop detect, /providers).
func (r *Runner) FetchOpencodeModelVariants(ctx context.Context, modelID string) ([]string, string, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, "", fmt.Errorf("model id is required")
	}
	if entry, ok := cachedOpencodeModelVariants()[modelID]; ok && len(entry.Efforts) > 0 {
		return entry.Efforts, entry.Default, nil
	}
	if !opencodeAgentEnabled() {
		return nil, "", fmt.Errorf("opencode controlled runtime is disabled")
	}
	scopeKey, env, err := r.opencodeLaunchEnv()
	if err != nil {
		return nil, "", fmt.Errorf("resolve opencode account: %w", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var d *opencodeDispatcher
	if fetchDispatcher != nil {
		d = fetchDispatcher // test seam
	} else {
		// BUG-334: the probe runs on its OWN throwaway process (segment
		// "probe", closed below). The probe's session/new carries EMPTY
		// mcpServers, and opencode keys its MCP clients by server NAME per
		// process — probing on the SHARED chat process reset that connection
		// and would kill any in-flight parent MCP tool call (spawn_agent /
		// ask_user) with "Connection closed".
		h, err := r.ensureOpencodeProcessSegmented(probeCtx, scopeKey, "probe", r.workspace, env, modelID, "", false)
		if err != nil {
			return nil, "", err
		}
		d = h.dispatcher
		defer h.close()
	}
	newRes, err := d.call(probeCtx, "session/new", opencodeACPSessionNewParams(r.workspace, nil, nil, "", ""))
	if err != nil {
		return nil, "", fmt.Errorf("opencode variants probe session/new: %w", err)
	}
	sessionID := opencodeACPResponseSessionIDFromResult(newRes)
	if sessionID == "" {
		return nil, "", fmt.Errorf("opencode variants probe returned no sessionId")
	}
	defer func() {
		_ = d.notify("session/close", map[string]any{"sessionId": sessionID})
	}()
	cfgRes, err := d.call(probeCtx, "session/set_config_option", opencodeACPSessionSetConfigParams(sessionID, "model", modelID))
	if err != nil {
		cfgRes, err = d.call(probeCtx, "session/set_config", opencodeACPSessionSetConfigParamsAlt(sessionID, "model", modelID))
		if err != nil {
			return nil, "", fmt.Errorf("opencode variants probe set_config: %w", err)
		}
	}
	efforts, current := opencodeEffortOptionsFromConfig(cfgRes)
	if len(efforts) == 0 {
		return nil, "", fmt.Errorf("opencode variants probe: no effort options for %s", modelID)
	}
	recordOpencodeModelVariants(modelID, efforts, current)
	return efforts, current, nil
}

// handleGetOpencodeModelVariants serves GET /client/providers/opencode-variants
// ?model=<opencode model id> — the per-model effort options (live ACP probe on
// first ask, cached observations afterwards).
func (s *InteractiveService) handleGetOpencodeModelVariants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	modelID := strings.TrimSpace(r.URL.Query().Get("model"))
	if modelID == "" {
		http.Error(w, "model query parameter is required", http.StatusBadRequest)
		return
	}
	if s.runner == nil {
		http.Error(w, "opencode runtime not attached to this service", http.StatusServiceUnavailable)
		return
	}
	efforts, current, err := s.runner.FetchOpencodeModelVariants(r.Context(), modelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"modelId":                modelID,
		"supportedEfforts":       efforts,
		"defaultReasoningEffort": current,
	})
}
