# CA-807 — hold ready/send until first chat list settles

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-60
change_type: bugfix
summary: Cold start with a bound project holds ready/send until the first chat list settles; startup chat errors surface loudly instead of sticking /open on loading
# --->8---

## Why

Live: /open picker stuck on "loading chats…" with no error. Init passed
ready on SessionDefaultsMsg while the first chat fetch is lazy and silent,
so every failure mode (slow endpoint, runner hiccup) is invisible and the
picker never converges.

## Change

- SessionDefaultsMsg first load with a bound project and empty list fires
  a silent fetch, arms chatWaitPending, and holds statusMsg at
  "loading chats…" with non-slash send blocked. The session banner
  (sessionLoading) stays catalog-owned: 3 old tests pin defaults-clearing
  it, so the gate never touches it (footer spinner included).
- First ChatListMsg disarms: success shows ready; startup error with an
  empty list passes degraded but loud ("Chat list failed: …"); picker
  retries per keypress as before.
- First-open prefetch sets chatListInflight so keypresses stop piling
  fetch pairs onto a slow runner.
- 15s chat budget + 45s net force the same degraded pass; r-reg and
  approval gates untouched.

## Tests

New ca807_chat_gate_init_loading_test.go (6 pass). Full tui/app suite shows
only the 11 pre-existing environment failures also red on clean HEAD.

## Providers

Agnostic: TUI-only, no providerKey branching.

## Will not undo

CA-514 send-blocked-while-loading and banner contract, BUG-355 refresh
dedup/interval, 45s net.
