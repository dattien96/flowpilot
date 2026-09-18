# CA-887 — CP-66 verification: gitnexus cypher zero-row `[]` breaks knowledge distill bootstrap

# ---8<--- flowpilot:change-ledger
feature_key: living-knowledge-base
source_doc_id: CP-66
change_type: bugfix
summary: Live gate-sandbox run-717586 proved CP-66 M-1 bootstrap fails when gitnexus cypher returns bare [] for zero-row queries; one-line parseCypherTable fix + 3 additive tests green; full structure suite still has 2 pre-existing failures so fix not closed; M-1/M-2/M-3 live re-verification pending runner restart
# --->8---

## Why

During CP-66 Test-Steps verification (2026-09-17), a live Grok run on
`gate-sandbox` (run-717586, provider grok, model grok-4.5) triggered the
CP-66 knowledge bootstrap, which failed repeatedly (runner.log 22:37,
04:35, 04:37, 05:19, 05:20, 05:23):

```
[knowledge] bootstrap distill failed workspace="/Users/tiendat/Desktop/BE/gate-sandbox":
gitnexus cypher: cannot decode response: json: cannot unmarshal array into Go value of type
struct { Markdown string "json:\"markdown\""; Error string "json:\"error\"" }
```

Root cause chain: gate-sandbox GitNexus index is stale (2026-08-27) and has
0 Process rows (353 nodes). GitNexus CLI 1.4.8 answers a zero-row cypher
query with a bare `[]` array instead of the `{"markdown","error"}`
envelope; `parseCypherTable` only accepted the envelope. On the flowpilot
repo (300 processes) the envelope path works — which is why all 28
automated CP-66 fixture tests stayed green while live distill failed on
every workspace with an empty/young index.

## Change

- `apps/local-runner/internal/structure/processes.go` `parseCypherTable`
  (one line): treat a trimmed `[]` body as empty output (return nil, nil),
  same as empty string. Envelope + error handling untouched.
- Impact analysis (gitnexus impact parseCypherTable, refreshed index):
  LOW risk, single direct caller `runCypher` (d=1 only).
- New additive regression tests
  `internal/structure/processes_bare_array_test.go`:
  `TestParseCypherTableAcceptsBareEmptyArray` (RED before fix — reproduced
  the exact live error; GREEN after),
  `TestParseCypherTableEnvelopeStillParses`,
  `TestParseCypherTableEnvelopeErrorSurfaces`. Additive only; no old test
  edited.

## Not done / residual risks

- Runner process was not restarted after the fix; CP-66 M-1 live
  re-verification on gate-sandbox is pending (must re-bind workspace after
  runner restart; knowledgeBootstrapOnce is per-process).
- `go test ./internal/structure/` full suite is red on
  `TestRepoNameFromDirUsesBasename` (Windows-path expectation, reproduced
  at pristine HEAD 30c31a80 in a detached worktree) and
  `TestGitNexusDependentsSmokeScopeDiff` (expected affected-process names
  mismatch; pristine-HEAD run inconclusive — worktree repo not registered
  in gitnexus). Basename failure is confirmed on pristine HEAD; the smoke
  failure's baseline cause is not established. Neither test is edited here;
  both block the "old suite green" closure.
- Evidence files under /tmp are volatile.
