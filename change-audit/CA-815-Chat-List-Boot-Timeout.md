# CA-815 — chat list must not time out during boot scan

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Boot summary scan skips flow reconstruct; TUI history list timeout is 30s
# --->8---

## Why

TUI `Chat list failed: .../workflow-runs: context deadline exceeded`. Log:
startup silent fetch at 5s while ScanPersistedChatsForSummaries reconstructed
every vibe/flow session under s.mu. GET history blocked.

## Change

- Scan skips sessions with ActiveFlowNodes (summaries after /open).
- cmdFetchChats timeout 5s → 30s.

## Tests

ca815_boot_scan_skips_flow_test.go.

## Providers

Agnostic Case 1.

## Will not undo

BUG-060 persisted history list. Idle-timer summaries for opened chats.
