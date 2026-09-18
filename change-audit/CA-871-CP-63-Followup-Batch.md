# CA-871 — CP-63 follow-up batch (C/C++, Gradle call-site, doctor, desktop warning)

# ---8<--- flowpilot:change-ledger
feature_key: lsp-runtime
source_doc_id: CP-63
change_type: feature
summary: implement 4 deferred CP-63 follow-ups — C/C++ platform detection with clangd registry, Gradle-fallback call-site in the diagnostics pipeline, flowpilot doctor CLI, desktop right-sidebar LSP warning
# --->8---

## Why

Four items were explicitly deferred during CP-63 P-1→P-8 implementation (recorded in Task-356/361/362): C/C++ parity, the Gradle call-site decision, the R-1 doctor mitigation, and the desktop half of the missing-server warning.

## Change

- **C/C++ detection** (`internal/lsp/platform_detect.go`, `platform_registry.go`): `cpp` token on CMakeLists/Makefile markers (checked after Rust, before Python), clangd registry entry with c-family extensions, c/cpp LanguageIDs. TUI `wizardPlatformOptions` deliberately untouched (additive rule; LSP layer detects independently).
- **Gradle call-site** (`internal/lsp/runner_hook.go`): `CheckFiles` now collects structured per-file errors (new `AfterFileWriteErrors`; `AfterFileWrite` delegates) and runs `compileDebugKotlin` only when android LSP actually ran clean. Unavailable server counts as unknown, never "clean". Bounded by `gradleValidationTimeout` (120s) on top of the gate ctx.
- **flowpilot doctor** (`internal/cli/doctor.go` + 1-line registration): PATH probe per registry binary, aligned table with install hints, exit 1 when missing.
- **Desktop warning** (`LSPStatusNotice.tsx` pinned first in the right-sidebar stack, CSS ellipsis max 2 visual rows, `LSPStatus` type + optional `getLSPStatus` client method). No-logic component; all decisions server-side.

## Tests

- lsp: C/C++ detection/resolve/extensions/LanguageID (+precedence), 3 Gradle call-site tests (runs-when-clean / skipped-on-LSP-errors / skipped-non-android) with cross-platform fake gradlew.
- cli: 4 doctor tests (registration, table logic both ways, format) with stubbed PATH lookup.
- Desktop: `tsc --noEmit` shows zero errors in touched files (only pre-existing missing `@types/node` + `vite/client`, deps not installed); no DOM harness exists for component tests, so none added — contract surface is covered by Go handler/client tests.
- Pre-existing suites: lsp full green; cli green; tui/app untouched this round; no pre-existing test edited in this batch (wizard tests unaffected — fixtures carry no C markers; `wizardPlatformOptions` unchanged).

## Providers

Provider-agnostic. Evidence: detection is filesystem-marker based; Gradle parsing is compiler-output based; doctor probes PATH; sidebar renders endpoint data. No adapter, provider-selection, or per-provider code touched.

## Prior CA claims kept intact

- CA-869: gate allow-path contract unchanged (Gradle runs inside the existing allow-path hook, after LSP); `AfterFileWrite` signature and behavior preserved via delegation; `CheckFiles` ""-on-degradation contract preserved.
- CA-870: endpoint shape unchanged; warn-once semantics unchanged.
- Task-356/361/362 follow-up lines flipped to done with links here.
