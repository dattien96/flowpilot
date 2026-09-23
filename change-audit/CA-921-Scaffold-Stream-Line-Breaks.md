# CA-921: Scaffold Stream Line Breaks (devin -p Narration Rendering)

## Summary

Operator report (TUI screenshot): the CA-916 scaffold progress feed renders the
AI turn's narration as one continuous line — "…skill files.All four skills
read. Now let me check the current workspace state.The workspace has…" — never
breaking between status updates.

**Root cause.** `devin -p` writes each status update to stdout back-to-back
with NO separator at all — verified byte-level on a real run's `stdout.txt`
(235 bytes, zero `\n`/`\r`/ANSI). `tailPromptStdout` emits raw byte-range
chunks, so by the time events reach the feed, TUI, or Desktop, there is nothing
to split on. Two further wrinkles:

- The TUI renders assistant messages through the markdown path, where a single
  `\n` is a CommonMark soft break — collapsed to a space. Injecting `\n` at the
  emit seam alone would still render glued.
- Desktop renders `feed.output` in `<pre white-space: pre-wrap>`, so literal
  `\n` is already correct there.

## Fix

**Runner** (`internal/runner/scaffold_progress.go` +
`scaffold_dispatcher.go`): `wrapScaffoldProgressSink` wraps `req.OnProgress`
once at the top of `Dispatch` and runs `output` event text through a
`scaffoldStreamJoiner` shared across all turns/heal attempts. A line boundary
is re-inserted (`\n` prepended to the chunk) when:

- the stream closed a sentence (`.`, `!`, `?`, `…`) and the next chunk opens
  with an uppercase letter or digit — observed Devin statuses are complete
  sentences, while mid-sentence streaming splits never trip it; or
- the provider went quiet ≥ `scaffoldStreamQuietGap` (1.5 s) before an
  uppercase/digit-led chunk — catches punctuation-less statuses (intra-status
  streaming sits at ~300 ms, inter-status gaps at ~4 s, both live-verified); or
- a phase event (`ai_turn` → `gate` → `heal`) marked a hard boundary — turns
  never glue onto each other.

Guards: whitespace-ending streams never double-break; lowercase/space-led
chunks always append raw; `nil` sinks stay `nil`.

**TUI** (`internal/tui/app/init_engine.go`): output events now append via
`appendScaffoldDelta`, which keeps the stream on an assistant message with
`FormatHint: "scaffold"` — the plain `wrapText` render path, so re-inserted
`\n`s render as literal line breaks (tight log lines, matching Desktop's
`<pre>`). It never glues onto a preceding plain assistant message and never
consumes a thinking placeholder.

**Desktop**: no change — `applyScaffoldSnapshot` concatenates event text and
`<pre pre-wrap>` renders the `\n`s as intended.

## Tests

- `TestScaffoldStreamJoiner_*` (6) — sentence boundary, space/lowercase
  continuations, quiet-gap boundary, quick-uppercase stays glued, phase
  boundary, no double break after provider newline.
- `TestScaffoldProgress_OutputDeltasBreakBetweenStatuses` — end-to-end HTTP:
  devin-shaped chunks through dispatch → feed → `\n`-separated output.
- `TestWrapScaffoldProgressSink_PhaseSeparatesOutputRuns` +
  `…_NilSinkStaysNil` — sink seam contract.
- TUI `TestScaffoldProgress_OutputStreamUsesLiteralNewlineRendering` — the
  rendered rows contain separate lines (markdown would collapse them) and the
  message carries `FormatHint: "scaffold"`.
- TUI `TestScaffoldProgress_OutputDoesNotAppendToPlainAssistantMessage` —
  stream opens its own message after a normal answer.
- Existing CA-916/917/918 suites all green (scaffold progress, init engine,
  busy state, post-lost recovery).

## Provider parity

Provider-agnostic presentation fix: `wrapScaffoldProgressSink` and
`appendScaffoldDelta` take no `providerKey` and never branch on one (grep:
`providerKey|ProviderKey` absent from both). Devin is the live-verified
producer of separator-less status streams; claude `--print`, codex `exec`, and
grok/opencode `--format json` stdout already carry real newlines — the
whitespace guard leaves them untouched, and JSON-lines streams additionally
gain a render win (plain text beats markdown-mangled JSON).

## Pre-existing failures (verified on clean HEAD, unrelated)

`TestIsFlowPlannerExcludedPathCoversSkillpackScaffold` (gitnexus env-dependent,
flagged in CA-916), `TestCreateProject_AutoTriggersScaffoldForCapablePlatform`
(executor-count/persistStatus race), and
`TestScaffoldDispatch_ClientDisconnectKeepsTurnAlive` (3 s start timeout) all
fail identically on stashed HEAD on this machine.

## GitNexus

MCP server unreachable this session (`list tools` failed twice) — impact
analysis done manually: `Dispatch` keeps its signature (only event `Text`
content changes); `appendScaffoldDelta` is additive; no caller updates needed.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: bugfix
summary: scaffold feed re-inserts stdout line boundaries (devin -p has none) and TUI renders the stream with literal newlines via a "scaffold" format hint
# --->8---
