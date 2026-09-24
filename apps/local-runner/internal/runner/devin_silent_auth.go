package runner

import (
	"context"
	"os"
	"regexp"
	"strings"
)

// Silent Devin ACP auth (user request): an account that already stores a
// windsurf_api_key authenticates via `windsurf-api-key` + _meta.api_key —
// no browser, no PKCE listener. The PKCE `devin-browser` flow only runs
// when no key is stored or the silent call is rejected, so the first turn
// on an already-authenticated machine never pops a tab.

// devinAPIKeyPattern matches the flat TOML line Devin's own CLI writes:
// `windsurf_api_key = "..."` (double-quoted; single-quote literal accepted
// for robustness). No TOML dependency — the store is flat and
// machine-generated.
var devinAPIKeyPattern = regexp.MustCompile(`(?m)^\s*windsurf_api_key\s*=\s*["']([^"'\n]*)["']`)

// devinAPIKeyFromEnv resolves the windsurf_api_key the spawned process will
// see: an explicit WINDSURF_API_KEY in the launch env first (ambient
// occurrences are stripped by devinProcessEnv, so only account-provided
// ones count), then credentials.toml under the account home carried by
// extraEnv["HOME"], then the ambient homes the process inherits when
// extraEnv did not pin one. "" means no silent path — callers fall back to
// the browser flow.
func devinAPIKeyFromEnv(extraEnv map[string]string) string {
	if key := strings.TrimSpace(extraEnv["WINDSURF_API_KEY"]); key != "" {
		return key
	}
	for _, path := range devinCredentialSearchPaths(extraEnv) {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if key := parseDevinWindsurfAPIKey(string(b)); key != "" {
			return key
		}
	}
	return ""
}

// devinCredentialSearchPaths mirrors the credential locations the spawned
// `devin acp` resolves: the pinned account home when extraEnv carries HOME,
// plus the ambient HOME/USERPROFILE/AppData/XDG dirs the process inherits
// otherwise (the model probe spawns with devinProcessEnv(nil)).
func devinCredentialSearchPaths(extraEnv map[string]string) []string {
	home := strings.TrimSpace(extraEnv["HOME"])
	var paths []string
	if home != "" {
		paths = append(paths, devinCredentialFilePaths(home)...)
	}
	for _, envHome := range []string{os.Getenv("HOME"), os.Getenv("USERPROFILE")} {
		if envHome = strings.TrimSpace(envHome); envHome == "" || envHome == home {
			continue
		}
		paths = append(paths, devinCredentialFilePaths(envHome)...)
		for _, c := range devinAmbientAuthCandidates(envHome) {
			paths = append(paths, c.authPath)
		}
	}
	return dedupeFilePaths(paths)
}

// parseDevinWindsurfAPIKey extracts the windsurf_api_key value from a
// credentials.toml body. "" when absent or empty.
func parseDevinWindsurfAPIKey(contents string) string {
	if m := devinAPIKeyPattern.FindStringSubmatch(contents); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// devinAuthenticate runs the ACP authenticate handshake, preferring the
// silent windsurf-api-key method when a stored key exists. On a silent RPC
// error — expired/revoked key — it retries with devin-browser so the user
// still gets the web login instead of a dead process. ctx bounds both
// attempts together (devinAuthTimeout).
func devinAuthenticate(ctx context.Context, dispatcher *devinDispatcher, extraEnv map[string]string) error {
	if key := devinAPIKeyFromEnv(extraEnv); key != "" {
		if _, err := dispatcher.call(ctx, "authenticate", devinACPAPIKeyAuthParams(key)); err == nil {
			return nil
		}
		// Silent path rejected — fall through to the browser flow so the
		// user can sign in interactively.
	}
	_, err := dispatcher.call(ctx, "authenticate", devinACPAuthenticateParams())
	return err
}
