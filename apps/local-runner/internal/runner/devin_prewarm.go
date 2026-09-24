package runner

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
)

// Task-439: Devin ACP prewarm. The first `devin acp` process pays a cold
// initialize+authenticate (PKCE browser) handshake — observed at tens of
// seconds on a cold machine and indistinguishable from a hang. Warming is
// triggered at runner boot when a connected account exists, when a devin
// account verifies (login), and when the active devin account switches, so
// the first chat turn reuses an already-authenticated process.
//
// The warm lands in the same scope+segment ("", chat) a turn resolves to, so
// ensureDevinProcess's same-segment reuse hands the turn the warm handle —
// session structure is untouched (the turn still mints its own session slug
// inside the shared process).

// devinPrewarmInFlight dedupes concurrent warm triggers per account scope —
// boot+login+switch can land close together; only the first pays a spawn.
// Keyed by scope (account.ID): two Runners warming the same account share the
// guard, which is correct — the credential scope is what is expensive.
var devinPrewarmInFlight sync.Map // map[string]bool

// WarmDevinProcessAsync warms the currently-active connected devin account's
// `devin acp` process in the background. Boot path: requires a connected
// account up front — warming an unauthenticated install would pop a browser
// unprompted, so the guard fails closed.
func (r *Runner) WarmDevinProcessAsync(reason string) {
	if !devinAgentEnabled() {
		return
	}
	// Same ambient-binary guard as warmDevinModelsCacheAsync: under `go test`
	// a test opts in via FLOWPILOT_DEVIN_BIN + a commandContextFn stub.
	if runningUnderGoTest() && os.Getenv("FLOWPILOT_DEVIN_BIN") == "" {
		return
	}
	account, err := r.ResolveProviderAccount(string(ProviderKeyDevin), "")
	if err != nil || strings.TrimSpace(account.ID) == "" {
		return
	}
	r.warmDevinAccountAsync(account, reason)
}

// warmDevinAccountAsync warms one specific account. Post-login the verified
// account may not be the active slot yet — callers pass it explicitly.
func (r *Runner) warmDevinAccountAsync(account ProviderAccount, reason string) {
	if !devinAgentEnabled() {
		return
	}
	if runningUnderGoTest() && os.Getenv("FLOWPILOT_DEVIN_BIN") == "" {
		return
	}
	if account.ProviderKey != string(ProviderKeyDevin) ||
		account.AuthStatus != "connected" ||
		strings.TrimSpace(account.ID) == "" {
		return
	}
	scopeKey := strings.TrimSpace(account.ID)
	if _, loaded := devinPrewarmInFlight.LoadOrStore(scopeKey, true); loaded {
		return
	}
	env := devinAccountEnv(account)
	go func() {
		defer devinPrewarmInFlight.Delete(scopeKey)
		log.Printf("[devin] prewarm start scope=%q reason=%q", scopeKey, reason)
		// context.Background(), not a timeout ctx: exec.CommandContext ties the
		// spawned process's lifetime to ctx — canceling after the handshake
		// would kill the just-warmed process (the registry's turn path already
		// does this). init/auth are bounded internally by devinInitTimeout /
		// devinAuthTimeout inside ensureDevinProcessSegmented.
		if _, err := r.ensureDevinProcess(context.Background(), scopeKey, r.workspace, env, "", ""); err != nil {
			log.Printf("[devin] prewarm failed scope=%q reason=%q err=%v", scopeKey, reason, err)
			return
		}
		log.Printf("[devin] prewarm ready scope=%q reason=%q", scopeKey, reason)
	}()
}

// devinChatProcessWarm reports whether a live chat-scope (segment "") `devin
// acp` handle already exists for the account a turn would resolve — the
// cold-start predictor startTurn uses to decide whether a provider_status
// event is owed.
func (r *Runner) devinChatProcessWarm() bool {
	scopeKey, _, err := r.devinLaunchEnv()
	if err != nil || scopeKey == "" {
		return false
	}
	r.devinProcessMu.Lock()
	defer r.devinProcessMu.Unlock()
	for _, h := range r.devinProcesses {
		if devinScopeBaseOf(h) == scopeKey && h.scopeSegment == "" && h.dispatcher != nil && !h.dispatcher.isClosed() {
			return true
		}
	}
	return false
}


