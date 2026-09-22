package runner

import (
	"path/filepath"
	"testing"
)

// CA-912: Windsurf exports ACP_BACKEND into shells it owns (integrated
// terminals, spawned tools). A runner launched from such a shell inherits it,
// and every spawned `devin -p` then ignores the on-disk credentials.toml
// entirely — scaffold turns die with "Not logged in" while the credential
// file is valid (live-verified: `env -u ACP_BACKEND devin -p` succeeds on the
// same machine/file). Strip the host-integration marker for spawned devin
// processes; other providers ignore the var but keep it to stay scoped like
// agyFilteredEnv.
func TestGetEnvForExecutionDevinStripsACPBackend(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("ACP_BACKEND", "windsurf")

	instance := &Runner{workspace: workspace}

	env := instance.getEnvForExecution("devin", "", nil, "")
	if containsEnvValue(env, "ACP_BACKEND=windsurf") {
		t.Fatalf("expected ACP_BACKEND stripped for devin one-shot spawn, got %v", env)
	}

	acctEnv := instance.getEnvForExecution("devin", filepath.Join(workspace, "acct"), nil, "")
	if containsEnvValue(acctEnv, "ACP_BACKEND=windsurf") {
		t.Fatalf("expected ACP_BACKEND stripped in account-scoped devin env, got %v", acctEnv)
	}

	codexEnv := instance.getEnvForExecution("codex", "", nil, "")
	if !containsEnvValue(codexEnv, "ACP_BACKEND=windsurf") {
		t.Fatalf("expected ACP_BACKEND preserved for non-devin spawn, got %v", codexEnv)
	}
}

// CA-912: same leak through the `devin acp` launch env — with ACP_BACKEND set
// the server adopts the "host is the sole source of credentials" policy, so
// authenticate never refreshes the on-disk store and `devin -p` starves of
// fresh tokens. devinProcessEnv already strips DEVIN_*/WINDSURF_API_KEY for
// the same reason; ACP_BACKEND belongs to that filter.
func TestDevinProcessEnvStripsACPBackend(t *testing.T) {
	t.Setenv("ACP_BACKEND", "windsurf")

	env := devinProcessEnv(nil)
	if containsEnvValue(env, "ACP_BACKEND=windsurf") {
		t.Fatalf("expected ACP_BACKEND stripped from devin acp env, got %v", env)
	}
}
