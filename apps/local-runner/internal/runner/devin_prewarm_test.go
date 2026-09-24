package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Task-439: Devin ACP prewarm — the PKCE/initialize handshake must be paid by
// boot/login/account-switch triggers, not by the user's first chat turn.

// isolateDevinPrewarmEnv points every home/auth probe at temp dirs so the
// host machine's real devin credentials can never leak into a test (Windows
// resolution reads USERPROFILE/APPDATA first — all are redirected).
func isolateDevinPrewarmEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("APPDATA", filepath.Join(root, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "AppData", "Local"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, ".local", "share"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", filepath.Join(root, "provider-accounts.json"))
	// Opt past the go-test ambient-binary guard; the spawn itself is stubbed
	// through commandContextFn so no real `devin` binary ever runs.
	t.Setenv("FLOWPILOT_DEVIN_BIN", "devin")
	return root
}

// writeDevinAccountState writes the provider-accounts config directly —
// bypassing ConnectProviderAccount's interactive auth start.
func writeDevinAccountState(t *testing.T, accounts ...ProviderAccount) {
	t.Helper()
	payload, err := json.Marshal(providerAccountState{Accounts: accounts})
	if err != nil {
		t.Fatalf("marshal accounts: %v", err)
	}
	if err := os.WriteFile(providerAccountsConfigPath(), payload, 0o644); err != nil {
		t.Fatalf("write accounts: %v", err)
	}
}

// writeDevinCredentials drops a syntactically-valid credentials.toml under the
// account home so HasLocalAuthAtPath reports the account connected.
func writeDevinCredentials(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".local", "share", "devin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir creds: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.toml"), []byte("windsurf_api_key = \"test\"\n"), 0o644); err != nil {
		t.Fatalf("write creds: %v", err)
	}
}

func devinTestAccount(id, home string, active bool) ProviderAccount {
	return ProviderAccount{
		ID:          id,
		ProviderKey: "devin",
		DisplayName: id,
		HomePath:    home,
		SlotIndex:   1,
		IsActive:    active,
		AuthStatus:  "connected",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		ExtraEnv:    map[string]string{},
	}
}

// stubDevinACPSpawn replaces the spawn seam with a scripted ACP server:
// read initialize → reply, read authenticate → reply, then stay alive.
// Returns the spawn counter for assertions.
func stubDevinACPSpawn(t *testing.T) *atomic.Int32 {
	t.Helper()
	orig := commandContextFn
	var spawns atomic.Int32
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		spawns.Add(1)
		s := shellReadLine() +
			shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"browser"}]}}`) +
			shellReadLine() +
			shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{}}`) +
			"sleep 30\n"
		return testShellCommand(ctx, s)
	}
	t.Cleanup(func() { commandContextFn = orig })
	return &spawns
}

// waitForDevinScope polls until a live (non-closed) chat-scope handle exists
// for scopeBase or the deadline passes.
func waitForDevinScope(t *testing.T, r *Runner, scopeBase string) *devinProcessHandle {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.devinProcessMu.Lock()
		for _, h := range r.devinProcesses {
			if devinScopeBaseOf(h) == scopeBase && h.dispatcher != nil && !h.dispatcher.isClosed() {
				r.devinProcessMu.Unlock()
				return h
			}
		}
		r.devinProcessMu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	r.devinProcessMu.Lock()
	for k, h := range r.devinProcesses {
		t.Logf("map entry key=%q scopeBase=%q seg=%q closed=%v", k, devinScopeBaseOf(h), h.scopeSegment, h.dispatcher.isClosed())
	}
	r.devinProcessMu.Unlock()
	t.Fatalf("no live devin process for scope %q", scopeBase)
	return nil
}

func waitForDevinScopeClosed(t *testing.T, h *devinProcessHandle) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if h.dispatcher.isClosed() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("previous account's devin process was not reclaimed")
}

// Boot path: a connected+active devin account must warm WITHOUT the user
// picking devin first (the reported hang was exactly this cold start).
func TestWarmDevinProcessAsyncWarmsActiveConnectedAccount(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	home := filepath.Join(root, "devinhome1")
	writeDevinCredentials(t, home)
	writeDevinAccountState(t, devinTestAccount("acct-d1", home, true))
	spawns := stubDevinACPSpawn(t)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	r.WarmDevinProcessAsync("boot")

	h := waitForDevinScope(t, r, "acct-d1")
	if h.dispatcher.isClosed() {
		t.Fatal("prewarmed handle must be live")
	}
	if got := spawns.Load(); got != 1 {
		t.Fatalf("expected exactly 1 acp spawn, got %d", got)
	}
}

// No connected devin account → no spawn, no goroutine work. Warming an
// unauthenticated account would pop a browser unprompted — never allowed.
func TestWarmDevinProcessAsyncSkipsWithoutConnectedAccount(t *testing.T) {
	isolateDevinPrewarmEnv(t)
	spawns := stubDevinACPSpawn(t)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	r.WarmDevinProcessAsync("boot")

	time.Sleep(300 * time.Millisecond)
	if got := spawns.Load(); got != 0 {
		t.Fatalf("no connected account must never spawn, got %d", got)
	}
}

// Login path: VerifyProviderAccount flipping a devin account to connected must
// warm THAT account even when it isn't the active slot yet.
func TestVerifyProviderAccountLoginTriggersDevinPrewarm(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	home := filepath.Join(root, "devinhome2")
	writeDevinCredentials(t, home)
	acct := devinTestAccount("acct-login", home, false)
	acct.AuthStatus = "connecting"
	writeDevinAccountState(t, acct)
	stubDevinACPSpawn(t)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	got, verified, err := r.VerifyProviderAccount("acct-login")
	if err != nil || !verified {
		t.Fatalf("VerifyProviderAccount: verified=%v err=%v", verified, err)
	}
	if got.AuthStatus != "connected" {
		t.Fatalf("account should be connected, got %q", got.AuthStatus)
	}
	waitForDevinScope(t, r, "acct-login")
}

// Account switch: activating a second devin account warms the NEW account and
// the reclaim pass closes the OLD account's process (account isolation —
// never reuse a process belonging to the previous account).
func TestActivateProviderAccountSwitchPrewarmsAndReclaims(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	homeA := filepath.Join(root, "devinhomeA")
	homeB := filepath.Join(root, "devinhomeB")
	writeDevinCredentials(t, homeA)
	writeDevinCredentials(t, homeB)
	acctA := devinTestAccount("acct-a", homeA, true)
	acctB := devinTestAccount("acct-b", homeB, false)
	acctB.SlotIndex = 2
	writeDevinAccountState(t, acctA, acctB)
	stubDevinACPSpawn(t)

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	r.WarmDevinProcessAsync("boot")
	handleA := waitForDevinScope(t, r, "acct-a")

	if _, err := r.ActivateProviderAccount("acct-b"); err != nil {
		t.Fatalf("ActivateProviderAccount: %v", err)
	}
	waitForDevinScope(t, r, "acct-b")
	waitForDevinScopeClosed(t, handleA)
}

// devinChatProcessWarm is the cold-start predictor startTurn uses to decide
// whether a provider_status event is owed: live chat-scope handle for the
// account a turn would resolve → warm; dead/absent → cold.
func TestDevinChatProcessWarm(t *testing.T) {
	root := isolateDevinPrewarmEnv(t)
	home := filepath.Join(root, "devinhome3")
	writeDevinCredentials(t, home)
	writeDevinAccountState(t, devinTestAccount("acct-warm", home, true))

	r := &Runner{devinProcesses: map[string]*devinProcessHandle{}}
	t.Cleanup(r.closeAllDevinProcesses)
	if r.devinChatProcessWarm() {
		t.Fatal("no processes — must report cold")
	}

	key := devinProcessKey("acct-warm", "", "")
	r.devinProcessMu.Lock()
	h := &devinProcessHandle{scopeKey: key, scopeBase: "acct-warm", dispatcher: newDevinDispatcher(io.Discard, nil)}
	r.devinProcesses[key] = h
	r.devinProcessMu.Unlock()
	if !r.devinChatProcessWarm() {
		t.Fatal("live same-account chat handle must report warm")
	}

	h.dispatcher.fail(io.ErrClosedPipe)
	if r.devinChatProcessWarm() {
		t.Fatal("closed handle must report cold")
	}
}

// devinStatusProbeAdapter is the smallest SendTurn-capable devin adapter: it
// completes immediately so the test only exercises the admission path.
type devinStatusProbeAdapter struct{}

func (a *devinStatusProbeAdapter) Key() ProviderKey              { return ProviderKeyDevin }
func (a *devinStatusProbeAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *devinStatusProbeAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
	return nil
}

// The cold-start status event must reach subscribers WHILE the adapter
// factory is still blocked on the ACP handshake — that visibility during the
// silent spawn+PKCE window is the whole point (the reported "hang").
func TestStartTurnEmitsDevinColdStartProviderStatus(t *testing.T) {
	reg := newProviderRegistry()
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyDevin,
		DisplayName:  "Devin",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapterForTurn: func(_, _, _ string) ProviderRuntimeAdapter {
			<-release
			return &devinStatusProbeAdapter{}
		},
	})
	svc := NewInteractiveServiceWithRegistry(reg)

	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyDevin})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	_, ch, _, ok := svc.subscribe(run.RunID, 0)
	if !ok {
		t.Fatal("subscribe failed")
	}

	go func() {
		_, _ = svc.startTurn(run.RunID, TurnInput{StepID: "s1", Prompt: "hi"}, "", "")
	}()

	// The "connecting" stage must arrive while the factory is still blocked.
	select {
	case ev := <-ch:
		if ev.Type != EventProviderStatus || ev.Status != "connecting" {
			t.Fatalf("expected provider_status/connecting first, got %v/%v", ev.Type, ev.Status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no provider_status event during blocked adapter resolution")
	}

	unblock()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == EventProviderStatus && ev.Status == "ready" {
				return
			}
		case <-deadline:
			t.Fatal("no provider_status/ready after adapter resolution")
		}
	}
}
