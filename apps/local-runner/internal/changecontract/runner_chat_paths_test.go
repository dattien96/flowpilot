package changecontract

import "testing"

func TestIsRunnerChatBookkeepingPath(t *testing.T) {
	for _, p := range []string{
		".flowpilot/chats/run-9968-turns.ndjson",
		".flowpilot/chats/proj-web/dispatch.ndjson",
		".flowpilot/chats/sessions.ndjson",
		"./.flowpilot/chats/x.ndjson",
	} {
		if !IsRunnerChatBookkeepingPath(p) {
			t.Errorf("%q must be exempt", p)
		}
	}
	// CA-427: gate inputs under .flowpilot must still drift.
	for _, p := range []string{
		".flowpilot/contracts/frozen_contracts.ndjson",
		".flowpilot/settings/flow-rules.json",
		".flowpilot/chatsx/evil.go",
		"flowpilot/chats/x.ndjson",
	} {
		if IsRunnerChatBookkeepingPath(p) {
			t.Errorf("%q must NOT be exempt", p)
		}
	}
}
