package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-385a: a test that touches the provider-account store without setting
// FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH must never resolve the real
// machine-global config path — a synthetic account (acct-grok, TempDir home)
// leaked into ~/Library/Application Support/FlowPilot/provider-accounts.json
// this way and was served to real consumers.
func TestBug385_ProviderAccountsPathIsolatedUnderGoTest(t *testing.T) {
	if !runningUnderGoTest() {
		t.Skip("guard only applies under go test")
	}
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", "")
	path := providerAccountsConfigPath()
	realDir, err := os.UserConfigDir()
	if err == nil {
		realPath := filepath.Join(realDir, "FlowPilot", "provider-accounts.json")
		if path == realPath {
			t.Fatalf("providerAccountsConfigPath resolved the real machine-global store under go test: %s", path)
		}
	}
	if home := os.Getenv("HOME"); home != "" && strings.HasPrefix(path, home+string(filepath.Separator)) {
		t.Fatalf("providerAccountsConfigPath resolved under the real HOME under go test: %s", path)
	}
}

// BUG-385b: an assistant-less turn (e.g. failed/quota turn) is NOT truncated —
// the handoff renderer must not append "[turn truncated due to handoff size
// limit]" while reporting truncated:false.
func TestBug385_HandoffNoTruncationMarkerWhenAssistantEmpty(t *testing.T) {
	rendered, truncated := renderConversationTurn(transcriptTurn{
		User: "Reply with exactly: ok",
	}, 4096, "[turn truncated due to handoff size limit]")
	if truncated {
		t.Fatal("empty-assistant turn reported truncated")
	}
	if strings.Contains(rendered, "truncated") {
		t.Fatalf("truncation marker rendered on an untruncated turn: %q", rendered)
	}
	if !strings.Contains(rendered, "Reply with exactly: ok") {
		t.Fatalf("user text lost: %q", rendered)
	}
}

// BUG-385b regression guard: a genuinely over-budget turn still reports
// truncated=true.
func TestBug385_HandoffTruncationMarkerWhenActuallyTruncated(t *testing.T) {
	_, truncated := renderConversationTurn(transcriptTurn{
		User:      strings.Repeat("u", 4000),
		Assistant: strings.Repeat("a", 4000),
	}, 512, "[turn truncated due to handoff size limit]")
	if !truncated {
		t.Fatal("oversized turn not marked truncated")
	}
}
