# CA-896 — CP-67 P-2/P-2b: signature extraction, lock rules, and the static stub-body whitelist

# ---8<--- flowpilot:change-ledger
feature_key: contract-first-tdd
source_doc_id: CP-67
change_type: feature
summary: Adds the deterministic detection layer for Contract-First Scaffold TDD — canonical signature extraction across Go/TSX/Kotlin/C-C++, the r-signature-lock and r-scaffold-red gate rules, the static stub-body whitelist (Task-383 B-11), and the frozen-contract fields (SignatureHash, LockedSignatures, read-only test paths) that pin the scaffold contract around the coder turn
# --->8---

## Why

Contract-first TDD is only meaningful if the runner can PROVE the scaffold
stayed a contract: the scaffold architect must ship compilable stubs whose
bodies are canonical not-implemented shapes plus a genuinely RED suite, and
the coder must then fill bodies without drifting a single pinned signature.
Three independent signals catch an agent that sneaks real logic into the
scaffold phase or edits signatures later: (1) an all-green suite, (2) a
compile failure, (3) a static body-shape whitelist that rejects if/for/
switch/multi-statement/call-bearing bodies — the last one works even when
the suite output is unavailable or misleading.

## Change

- `flowgate/ast_signatures.go` + `lang_adapter_{react,kotlin,cpp}.go` +
  `flowgate/scripts/` (new): canonical symbol extraction — go/parser for
  Go, the TypeScript Compiler API via a Node helper script for TSX,
  tree-sitter for C/C++ under an optional tag, LSP anchoring for Kotlin —
  with decl/def dedupe and CanonicalSignatureHash.
- `flowgate/signature_lock_rule.go` + `scaffold_red_rule.go` (new):
  r-signature-lock (before/after hash diff → drift list for the reprompt;
  the buffered renegotiation batch is the only legal bypass) and
  r-scaffold-red (compile PASS + runtime RED + stub whitelist).
- `flowgate/stub_bodies.go` + `stub_body_cache.go` (new): the Task-383 B-11
  per-language body-shape whitelist.
- `flowgate/evaluate.go`, `rules.go`: the two new rule kinds plus the
  TurnResult signal fields (SignatureHashBefore/After, SignatureDrift,
  CoderRenegotiating, ScaffoldExpected/CompileFailed/BodyNonStub,
  NonStubSymbols) — caller-computed, flowgate stays I/O-free.
- `changecontract/frozen_scope.go`, `preflight.go`: LockScaffoldArtifacts —
  one version bump that both read-only-locks the scaffold's test files and
  pins SignatureHash + LockedSignatures on the coder step's frozen contract
  (B-8.3); ReadOnlyLockedPaths / IsReadOnlyLockedPath helpers.
- `lsp/client.go`, `protocol.go`: documentSymbol plumbing used by the
  Kotlin/C++ anchoring path.
- Tests: ast_signatures/signature_lock_rule/scaffold_red_rule/stub_bodies
  in flowgate and scaffold_lock_test in changecontract — additive coverage
  for the extraction matrix, dedupe, lock diff, and stub whitelist.
