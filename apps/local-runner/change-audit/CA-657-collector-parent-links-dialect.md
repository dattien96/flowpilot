# CA-657 — collector honors the Parent Links dialect (BUG-662)

## Scope
Two call sites, one shared regex. `collectVibeTaskPlanForCP` and
`collectVibeDoneTasksForCP` now accept `Parent Links:` exactly like
`Parent Documents:`, unioning CP ids across all matching lines.
`Related ...` lines still never parent (BUG-468 restriction intact).

## Evidence
Live PrivateVault run-640182 sprinted Task-0312 (CP-03) for a CP-12 run:
Task-121..124 declare `- Parent Links: CP-12` with no `Parent
Documents:` line, so the CP-12 scope resolved empty. Replicated on the
real repo: unscoped sorted-first = Task-0312, scoped CP-12 = [].

## Fix
- `vibe_cp.go`: `vibeTaskParentDocsLine` → `Parent (?:Documents|Links)`;
  new `parentDocCPRefs` helper (deduped union over `FindAllSubmatch`)
  shared by both collectors.
- Real-data re-check with the new semantics: CP-12 → exactly
  [121, 122, 123, 124] in order; CP-03 scope unchanged.

## Tests
- `bug662_parent_links_dialect_test.go` (new): RED (only the Documents
  file matched) → GREEN. Covers Links-only, both-dialects (no dup),
  split dialects (both scopes), Related-only exclusion (both spellings),
  done/ counterpart.
- All BUG-468 scope tests green (restriction not weakened).
- `detect-changes` gate before commit.
