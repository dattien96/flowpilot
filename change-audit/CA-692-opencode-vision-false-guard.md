# CA-692 — Opencode Vision=false guard locked with additive tests

## Problem

Operator audit confirmed the `Vision=false` state for Opencode is **intentional
and consistent** across all 9 verified locations (CP-57 `P-12`/row 28/line 473,
Task-303 `T-7`/`DOD-15`, CP-57-Test-Steps `M1`, `opencode_adapter.go:184`,
`ChatInput.tsx:92`). However, two guard gaps existed:

1. **Desktop had no test** locking `VISION_PROVIDERS` to exclude `opencode` — a
   future accidental add would silently enable image attach for a provider whose
   ACP image round-trip is unproven.
2. **Runner prompt-params test** asserted the text-only shape but did not
   explicitly assert "no image block is ever built" — the
   `opencode_acp.go:88-90` comment (live-verified `promptCapabilities.image=true`
   in the initialize handshake, but attachment round-trip unproven) was the only
   guard against a premature flip.

## Change

- **New `apps/desktop-flowpilot/src/components/visionProviders.ts`** — `VISION_PROVIDERS`
  extracted from `ChatInput.tsx` (inline, non-exported) into an exported `.ts`
  module so the test can import it. `tsconfig.phase1-tests.json` only includes
  `*.test.ts` and does not compile `.tsx`. **Visibility-only refactor** — set
  contents (`codex`, `claude`, `grok`) and all call sites unchanged.
- **Modified `apps/desktop-flowpilot/src/components/ChatInput.tsx`** — imports
  `VISION_PROVIDERS` from `./visionProviders`; removed the inline declaration.
  No behavior change.
- **New `apps/desktop-flowpilot/src/components/chatInputVisionProviders.test.ts`** —
  asserts `VISION_PROVIDERS` includes `codex`/`claude`/`grok` and **excludes**
  `opencode` (locks the `M1` "chặn trước khi gửi prompt" behavior at the source).
- **New `apps/local-runner/internal/runner/opencode_vision_guard_test.go`** —
  `TestOpencodePromptParamsNeverBuildsImageBlock` asserts `opencodeACPPromptParams`
  produces exactly one `{type:"text",text}` block and never an image block
  (defensive contract test).

## Tests (additive)

- `TestOpencodePromptParamsNeverBuildsImageBlock` (runner) — PASS.
- `TestOpencodeCapabilitiesMatchProvenSet` (existing, asserts `caps.Vision == false`) — PASS, untouched.
- `TestOpencodeACPPromptParamsBuildsCorrectJSON` (existing, asserts text-only prompt) — PASS, untouched.
- `chatInputVisionProviders.test.ts` (desktop, 2 tests) — PASS.
- Desktop typecheck: `tsc -p ../../tsconfig.phase1-tests.json` — no errors in the new files (pre-existing unrelated errors in other test files remain).

## Unlock condition (recorded, not met)

`Vision` may flip `true` for Opencode only when the ACP image attachment
round-trip is proven end-to-end — image → `.tmp/images` path fallback (Grok
`CA-483` pattern) or native ACP image block → model sees it → turn completes.
When proven, update: `opencode_adapter.go:184` (`Vision:true`),
`visionProviders.ts` (add `opencode`), `opencode_acp.go:88-90` (comment),
CP-57 `P-12`/row 28/line 473, Task-303 `T-7`/`DOD-15`, CP-57-Test-Steps `M1`.

# ---8<--- flowpilot:change-ledger

feature_key: ai-providers
source_doc_id: CP-57
change_type: feature
summary: Lock Opencode Vision=false with additive desktop + runner guard tests; extract VISION_PROVIDERS to visionProviders.ts (visibility-only refactor)

# --->8---
