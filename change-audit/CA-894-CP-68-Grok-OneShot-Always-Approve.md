# CA-894 — Grok one-shot `-p` path honors AllowWrite/YoloMode via `--always-approve` (CP-68)

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: bugfix
summary: resolvePromptExecutionAdapter's grok branch now appends --always-approve when the caller requests AllowWrite or YoloMode, so one-shot scaffold turns can actually write files instead of ending with stopReason="cancelled"
# --->8---

## Problem

CP-68 live verification (proj-6, Grok-4.5, `/private/tmp/cp68-m1-rn`): the
scaffold dispatcher explicitly sends `AllowWrite: true, YoloMode: true`, but
`resolvePromptExecutionAdapter`'s grok branch silently dropped both flags —
unlike codex (`--sandbox workspace-write`), gemini (yolo flag), and opencode
(`--auto`). The one-shot `grok -p` spawn has NO `session/request_permission`
channel back to the runner, so every write tool call ended the turn with
`stopReason: "cancelled"` (observed: 3/3 turns cancelled ~17-21s in, only a
default `pnpm init` package.json produced, zero scaffold files written).
The compiler gate then failed `pnpm tsc --noEmit` (no typescript), the
self-healing loop retried 2 more times with identical results, and the
scaffold ended `status=error` after 3 attempts — the M-4 PASS path was
unreachable on Grok.

## Why `--always-approve` is correct HERE (and forbidden in the ACP path)

BUG-343 / CA-714 removed `--always-approve` from `grok agent stdio` because in
ACP mode the runner IS the permission peer: the flag makes Grok self-resolve
`pending_interaction`, so read-only postures and the YOLO-off approval card
become unreachable. In the `-p` one-shot path the runner is NOT an ACP peer —
no permission round-trip exists at all — so the flag cannot bypass a gate that
does not exist. Callers that do not opt in (summarizer, provider_driven_mcp)
leave both flags false and keep the default posture: read tools work, writes
cancel — unchanged behavior.

## What changed

- `runner.go` `resolvePromptExecutionAdapter` grok branch: append
  `--always-approve` when `request.AllowWrite || request.YoloMode` (same
  disjunction the gemini branch uses).
- `runner_test.go`: additive `TestResolvePromptExecutionAdapterGrokAlwaysApproveHonorsWriteFlags`
  covering allow_write / yolo_mode / both / neither — no legacy test touched.

## Live verification (post-fix)

- Rebuilt runner (:18999, binary `/tmp/fp-fixed-runner2`), created proj-3
  (`/private/tmp/cp68-m1-rn2`, platform=react-native, grok-4.5).
- Turn `prompt_20260920_150033_0`: command line contains `--always-approve`,
  `stopReason: "end_turn"` — Grok wrote the full Step 0 monorepo
  (turbo.json, pnpm-workspace.yaml, tsconfig*, 7 `packages/core-*`,
  `apps/_template`).
- Compiler gate `pnpm install && pnpm tsc --noEmit` → PASS on attempt 1.
- Log: `[scaffold] project=proj-3 platform=react-native status=done:
  scaffold: done — compiler gate PASS`; `scaffold-status.json`:
  `{"status":"done","attempts":1,"skillsAttached":[4 react-native-* skills]}`.

## R1 — old-suite regression evidence

- `go test ./internal/runner/ -run TestResolvePromptExecutionAdapter` — all
  PASS including existing codex/gemini/opencode/claude cases (unchanged for
  callers without write flags).
- No legacy test modified; pre-existing unrelated failures in the runner
  package (scaffold_lock_test signature pin, engine_setup) reproduce
  identically without this change.
