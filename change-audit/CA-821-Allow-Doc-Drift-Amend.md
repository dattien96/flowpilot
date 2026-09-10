# CA-821 — Allow persists doc/audit frozen-scope drift (FEATURE-KEYS.md)

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-366
change_type: bugfix
summary: Allow on frozen-scope drift persists specific doc/audit files (change-audit/FEATURE-KEYS.md) on AllowedExtraPaths so the park resumes instead of 422 amend_failed
# --->8---

## Why

Live run-654339 (`gate-sandbox` snake, vibe-ingest): the coder wrote
`change-audit/FEATURE-KEYS.md` outside the frozen contract. The gate correctly
parked (BUG-278: only `CA-*.md` is auto-exempt). Operator clicked `[Allow]`
and `handleAmendFlow` called `AmendFrozenContract`, which requires
`IsConcreteCodeTarget`. `.md` files fail that predicate → `422 amend_failed`.
Retry cannot recover: the file is already in the diff against `BaseSHA`.

Task-309 advertised Allow as "continue with new scope (match code changed)"
for every frozen-scope drift path. CA-427 Finding 5's explicit rejection of
non-concrete amend paths made Allow a dead-end for the files the gate
itself reports.

## Change

- `FrozenContractRecord.AllowedExtraPaths` (new JSON field, omitempty):
  operator-Allowed specific doc/audit files. Retrieval-locus /
  GitNexus targeting still use `DeclaredPaths` only.
- `FrozenContractScopeDrift` treats `AllowedExtraPaths` as in-scope.
- `IsUserAllowableDriftPath`: concrete code OR specific doc/audit file
  with an extension. Globs, flags, Makefile stay rejected.
- `AmendFrozenContractForAllow` (new): Allow endpoint unions concrete
  paths into `DeclaredPaths` and doc/audit extras into
  `AllowedExtraPaths`, one version bump. Does not re-normalize already
  frozen declared paths (macOS temp-dir symlink false-escape).
- `handleAmendFlow` calls `AmendFrozenContractForAllow`.
- `AmendFrozenContract` still rejects non-concrete paths (CA-427 tests
  untouched) and copies extras forward on a later concrete widen.

## Tests

New files only:

- `changecontract/bug366_allow_doc_amend_test.go` — ForAllow FEATURE-KEYS.md,
  mixed `.go`+`.md`, Makefile/glob still error, no-op re-Allow, extras
  survive a later concrete amend, `IsUserAllowableDriftPath` table.
- `runner/bug366_amend_allow_feature_keys_test.go` — HTTP 200 + resume,
  mixed paths, first write still parks, second gate after Allow does not
  re-park, Makefile still 422, extras persist across store reopen.

Old tests run green and untouched: `TestAmendFrozenContractRejects*`,
`TestAmendFlow_RejectsNonConcretePath`, `TestBUG327_FeatureKeysWriteStillBlocks`.
Full `./internal/changecontract/` 159 passed.

## Providers

Agnostic Case 1: `handleAmendFlow` / `AmendFrozenContractForAllow` take no
`providerKey` and do not branch on one. TUI and desktop both POST the same
amend body.

## GitNexus impact

`gitnexus impact --repo flowpilot -d upstream` before edit:

| Symbol | Risk | d=1 |
|---|---|---|
| `AmendFrozenContract` | LOW | `handleAmendFlow` |
| `FrozenContractScopeDrift` | LOW | `runChildArtifactOutputGateAtEpoch` |
| `handleAmendFlow` | LOW | none (HTTP) |

No HIGH/CRITICAL. `DeclaredPaths` consumers (retrieval locus, GitNexus
targets) still filter via `IsConcreteCodeTarget`.

## Will not undo

- CA-427 Finding 5: library `AmendFrozenContract` still errors on
  Makefile / globs / flags instead of silently no-op.
- BUG-278 / BUG-327: `FEATURE-KEYS.md` still parks on first write.
- Task-309: Allow remains drift-only; Retry/Stop unchanged.
