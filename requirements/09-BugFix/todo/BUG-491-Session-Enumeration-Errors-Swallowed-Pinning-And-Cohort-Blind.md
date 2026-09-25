# BUG-491 — session enumeration errors swallowed → provider-session pinning blind + resume misses persisted children

- Status: `todo`
- Severity: **medium-high** — the `ListAllProviderSessions`/`GetProviderSession`
  enumeration paths swallow errors in three places; pinning validation and
  resume cohort derivation then run on a silently-incomplete view.
- Found: 2026-09-25, deep audit pass 2.

## Root cause

**S1 — `foreignProviderSessionIDs`** (`interactive_service.go:~10420`)

```go
if listed, err := indexReader.ListAllProviderSessions(ctx); err == nil {
    sessions = listed
}
```

Store error → `out` contains only live in-memory related runs → persisted
siblings/parents/children invisible. Callers:
- `ownsProviderSession`-style check (~10401): a foreign sessionID absent
  from `foreign` → returns "not foreign" → session-pinning validation can
  **accept a session id owned by another run**.
- `filterOwnedProviderSessionIDs` (~10534): `len(foreign)==0 → return ids`
  unfiltered → foreign rotation ids kept as own.

**S2 — `interactive_resume.go:~107`** — childrenByParent/sessionsByRun
build: `ListAllProviderSessions` error → disk-persisted children missing →
resume's child-cohort map incomplete → cohort verdicts/lineage lost on
resume.

**S3 — `interactive_resume.go:~707`** — same enumeration feeding
`inferredFlowNodeByLegacyCohort` + per-session child loop → legacy cohort
inference blind on store fault.

## Blast radius

- Session pinning ("provider session is pinned per run/leg") only works if
  the foreign-session catalogue is complete; an unreadable index silently
  disables the check.
- Resume: a flow parent whose children exist only on disk (post-restart)
  can re-spawn children or lose their verdicts → duplicate provider turns
  or a synthesis gate waiting on verdicts that will never arrive.

## Fix contract

1. All three sites propagate the enumeration error instead of `err == nil`.
2. `foreignProviderSessionIDs` → return `(map, error)`; callers:
   - ownership check: error → answer "unverifiable" (fail-closed: treat as
     foreign/uncertain, never auto-accept).
   - filter: error → propagate so the caller keeps the conservative set.
3. Resume enumeration sites: propagate → resume returns typed error /
   defers reconstruction rather than building a partial cohort map.

## Required tests (RED first)

- `TestBUG491_ForeignSessionEnumerationErrorFailsClosed`: index reader
  error → ownership check does not auto-accept a session id.
- `TestBUG491_FilterOwnedEnumerationErrorConservative`: filter does not
  silently pass foreign ids through on store fault.
- `TestBUG491_ResumeEnumerationErrorPropagates`: resume path building
  childrenByParent gets error → returned to caller (pre-fix: silent partial
  map).
- Positive controls for healthy enumeration on each.

## Definition of Done

- No pinning or resume-cohort decision runs on a silently-partial session
  enumeration.
- Tests green, CA entry, commit.
