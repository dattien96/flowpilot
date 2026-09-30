# CA-1069 — Official kotlin-lsp: binary resolution + pull diagnostics

## What changed

`apps/local-runner/internal/lsp/platform_registry.go`:

- `PlatformLSPConfig` gains `AltBinaries` — additional executable names
  tried after `Binary` when resolving via PATH.
- New `Binaries()` (ordered candidates) and `ResolveBinary(lookPath)`
  (first hit wins, returns name + path) shared by every probe site.
- `android` entry: `Binary: "kotlin-lsp"` (official JetBrains server,
  github.com/Kotlin/kotlin-lsp — IntelliJ-based, experimental Android
  Gradle support), `AltBinaries: ["intellij-server", "kotlin-language-server"]`
  (newer bundle binary name; legacy fwcd community build). InstallHint now
  points at `brew install JetBrains/utils/kotlin-lsp` / GitHub releases
  instead of fwcd releases.
- `DetectAndResolveWith` resolves through the candidates and stores the
  resolved name back into the returned config.

`apps/local-runner/internal/lsp/runner_hook.go`:

- `getOrStart` resolves via `ResolveBinary` and spawns the resolved name —
  a fallback-only install (e.g. fwcd already on PATH) still starts. The
  once-per-session warning lists all tried candidates.
- `AfterFileWriteErrors` gains the pull-diagnostics path: when the server
  advertised `diagnosticProvider`, it issues `textDocument/diagnostic`
  instead of waiting for a publish that pull-only servers never send
  (kotlin-lsp would otherwise time out every check, 5s per file).
- Android comment updated: the gap justification is now "Kotlin LSP can
  miss Android generated-type errors" (holds for both servers; kotlin-lsp's
  AGP support is still experimental).

`apps/local-runner/internal/lsp/lsp_status.go`:

- `Status` resolves through candidates and reports the resolved binary
  name, so the sidebar/doctor reflect what would actually spawn.

`apps/local-runner/internal/cli/doctor.go`:

- `doctorCheck` probes `ResolveBinary`; a row is installed when any
  candidate resolves and reports the resolved name. Long text updated.

Pull-diagnostics support (the actual blocker for kotlin-lsp — it is
pull-only per upstream docs):

- `protocol.go` — `DocumentDiagnosticParams`, `DocumentDiagnosticReport`
  (full/unchanged kinds), and `ClientCapabilities` now advertises
  `textDocument.diagnostic`.
- `client.go` — `DocumentDiagnostic(ctx, uri)` request.
- `server_manager.go` — `WaitReady` stores the negotiated capabilities and
  advertises pull support in initialize; `PullDiagnostics()` accessor;
  caps reset on every respawn (new process = new handshake).
- Push servers are untouched: no `diagnosticProvider` → the existing
  publishDiagnostics wait path runs unchanged.

`apps/local-runner/internal/lsp/kotlin_config.go`,
`apps/local-runner/internal/lsp/gradle_fallback.go`:

- Comments no longer name fwcd's server as the only Kotlin option. The
  init options (storagePath, androidSdkPath, gradleProperties, compose
  hint) stay: fwcd consumes them, kotlin-lsp ignores unknown keys and
  imports the Gradle/Android project itself.

## Invariant

Detection prefers the official server but never regresses an existing
install: any candidate on PATH keeps diagnostics alive, and every probe
site (detect, spawn, status, doctor) resolves through the same ordered
list. Diagnostics work under both transport models — pushed publishes are
collected as before, pull-only servers are queried on demand; a server
that does neither still degrades to build/test validation, never a hang.

## Tests

`platform_registry_test.go` — kotlin-lsp wins over both fallbacks;
intellij-server and kotlin-language-server each resolve standalone
(PATH replaced wholesale for hermeticity).
`runner_hook_test.go` + `server_manager_test.go` — new `pulldiagnostics`
helper mode (advertises diagnosticProvider, answers
textDocument/diagnostic, never pushes); hook test proves diagnostics flow
through the pull path.
`kotlin_config_test.go` — android binary assertions now expect kotlin-lsp.
`doctor_test.go` — android row installed via the legacy alt binary alone.
`lsp_status_sidebar_test.go` — fixture updated to the official names.
