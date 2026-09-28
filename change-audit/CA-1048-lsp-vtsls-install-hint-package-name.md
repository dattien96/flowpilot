# CA-1048 — LSP install hint for vtsls named a nonexistent npm package

## What changed

`apps/local-runner/internal/lsp/platform_registry.go`:

- `InstallHint` for `nextjs`, `reactjs`, `react-native`, `node` was
  `npm i -g vtsls` — there is no `vtsls` npm package, so the hint fails if
  a user runs it. The server binary is `vtsls` but the package is
  `@vtsls/language-server`; all four hints now read
  `npm i -g @vtsls/language-server`. `Binary` is unchanged (still `vtsls`,
  the executable the package ships).

## Surfaces fixed

- TUI sidebar `⚠ lsp: vtsls missing` card (`internal/tui/app/lsp_status.go`)
- `flowpilot doctor` MISSING rows (`internal/cli/doctor.go`)
- `/client/lsp-status` notice (`internal/lsp/lsp_status.go`)

All three render `cfg.InstallHint` verbatim — one registry fix covers them.

## Invariant

An install hint must name a package that actually exists on the registry;
binary name ≠ package name for scoped packages.

## Tests

`internal/lsp` suite green (no test asserted the wrong hint).
