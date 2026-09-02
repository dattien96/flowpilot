package runner

import (
	"context"
	"strings"
	"testing"
)

// BUG-343 (live-probed grok 1.0.13, 2026-09-02): with the default permission
// mode, Grok resolves exec `run_terminal_command` permissions internally
// (`pending_interaction` self-resolve) and runs bash with ZERO permission
// round-trip — scan/plan read-only postures and the YOLO-off approval card
// could never gate bash (live: real `ls` output in scan posture, yolo on AND
// off). `--always-approve` on the launch args forced the same bypass even when
// GROK_DEFAULT_PERMISSION_MODE=ask was set (flag wins, live-probed).
//
// The fix launches every Grok process with GROK_DEFAULT_PERMISSION_MODE=ask
// and no --always-approve, so Grok emits a standard
// session/request_permission for exec and BLOCKS; grokAdapter.handleInbound
// already owns the decision layer (BUG-331 "always-ask + runner decides"):
// YOLO-on auto-approves, scan/plan deny writes+exec via the read-only policy,
// YOLO-off shows the approval card.
//
// Live probe matrix (real grok binary, scripts in CA-714):
//   - default mode, no flag:        exec self-resolves, runs            (bug)
//   - env ask + no flag:            session/request_permission, blocks  (fixed)
//   - env ask + reject-once reply:  command NOT run, stopReason=cancelled
//   - env ask + allow-once reply:   command runs, stopReason=end_turn
//   - env ask + --always-approve:   self-resolves again (flag wins)     (why the flag is gone)
//   - env ask + config always-approve: env wins, still asks             (config bypass dead)

// The launch env must always carry the always-ask mode, on every spawn, so no
// caller can forget it (BUG-331 choke-point discipline).
func TestGrokProcessEnvForcesAlwaysAskPermissionMode(t *testing.T) {
	env := grokProcessEnv(map[string]string{"GROK_HOME": "/tmp/grok-home"})
	found := false
	for _, e := range env {
		if e == "GROK_DEFAULT_PERMISSION_MODE=ask" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected GROK_DEFAULT_PERMISSION_MODE=ask in grok process env")
	}
}

// The ask gate must be the LAST occurrence of GROK_DEFAULT_PERMISSION_MODE:
// the child env is last-occurrence-wins, and provider_registry spreads
// account.ExtraEnv into the same map — a stale account env carrying the key
// must never ungate the spawn (BUG-331 overlay discipline for opencode,
// mirrored here). Red before the fix (the overlay was appended before the
// extraEnv loop, so extraEnv won).
func TestGrokProcessEnvExtraEnvCannotDefeatAskGate(t *testing.T) {
	env := grokProcessEnv(map[string]string{
		"GROK_DEFAULT_PERMISSION_MODE": "always-approve",
	})
	last := ""
	for _, e := range env {
		if strings.HasPrefix(e, "GROK_DEFAULT_PERMISSION_MODE=") {
			last = e
		}
	}
	if last != "GROK_DEFAULT_PERMISSION_MODE=ask" {
		t.Fatalf("last GROK_DEFAULT_PERMISSION_MODE = %q, want \"GROK_DEFAULT_PERMISSION_MODE=ask\" (account ExtraEnv must never defeat the ask gate)", last)
	}
}

// YOLO handles must reach the same always-ask gate: the runner decides, not
// the provider's own bypass flag. Red before the fix — args carried
// --always-approve, which live-probes showed overrides the ask env.
func TestGrokSpawnArgsNeverCarryAlwaysApprove(t *testing.T) {
	var argsYolo []string
	defer mockGrokInitProcessCapturingArgs(t, &argsYolo)()

	r, _ := New(".")
	h, err := r.ensureGrokProcess(context.Background(), "bug343-yolo", ".", nil, "", "", true)
	if err != nil {
		t.Fatalf("ensureGrokProcess (yolo): %v", err)
	}
	defer h.close()

	for _, a := range argsYolo {
		if strings.TrimSpace(a) == "--always-approve" {
			t.Fatalf("launch args %v still carry --always-approve — scan posture cannot gate exec", argsYolo)
		}
	}
	if !h.alwaysApprove {
		t.Fatal("handle.alwaysApprove must stay true for the yolo key dimension (flip-respawn contract)")
	}
}

// Non-YOLO handles keep the same bare launch (model/effort untouched) so the
// gate comes from the env, not from per-call args.
func TestGrokSpawnArgsNonYoloStillBare(t *testing.T) {
	var args []string
	defer mockGrokInitProcessCapturingArgs(t, &args)()

	r, _ := New(".")
	h, err := r.ensureGrokProcess(context.Background(), "bug343-noyolo", ".", nil, "grok-4.5", "low", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (no yolo): %v", err)
	}
	defer h.close()

	want := []string{"agent", "--model", "grok-4.5", "--reasoning-effort", "low", "stdio"}
	if len(args) != len(want) {
		t.Fatalf("launch args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("launch args = %v, want %v", args, want)
		}
	}
}

// grokEncodePermissionDecision must keep mapping grok's bash approval options
// to the right optionIds through the standard channel: the live probes show
// exec requests now arrive with these exact option kinds.
func TestGrokEncodePermissionDecisionCoversExecOptions(t *testing.T) {
	opts := []any{
		map[string]any{"optionId": "always-allow", "kind": "allow_always", "name": "Yes, and don't ask again for bash commands"},
		map[string]any{"optionId": "allow-once", "kind": "allow_once", "name": "Yes, proceed"},
		map[string]any{"optionId": "reject-once", "kind": "reject_once", "name": "No, and tell Grok what to do differently"},
		map[string]any{"optionId": "reject-always", "kind": "reject_always", "name": "No, and don't ask again for this command"},
	}
	if got := grokEncodePermissionDecision(opts, true); got != "allow-once" {
		t.Fatalf("approve decision = %q, want allow-once (narrowest allow wins)", got)
	}
	if got := grokEncodePermissionDecision(opts, false); got != "reject-once" {
		t.Fatalf("deny decision = %q, want reject-once (scan posture path)", got)
	}
}
