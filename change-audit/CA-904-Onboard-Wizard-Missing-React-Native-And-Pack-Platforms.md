# CA-904: Onboard Wizard Missing React Native And Pack Platforms

## Summary

- Bug: the TUI "Onboard Project" wizard cycled only 8 hardcoded platforms
  (`nextjs golang android reactjs python node rust general`), so
  `react-native` — and 7 other platforms that own embedded `flow-pack`
  groups (`ios`, `kmm`, `flutter`, `vuejs`, `angularjs`, `java`, `nodejs`)
  — were unselectable. Picking `reactjs` for an RN/Expo project silently
  installed the web `reactjs` group and skipped the only `scaffold.yaml`
  recipe (react-native bootstrap + compiler gate).
- `wizardPlatformOptions` now lists every pack-backed platform; `node` was
  replaced by the canonical pack token `nodejs` (desktop Projects settings
  already stores `nodejs`).
- `DetectPlatform` now detects React Native via a `"react-native"` entry in
  package.json (checked before `"react"`, since RN manifests contain both),
  so auto-detect pre-selects the right platform instead of `reactjs`.
- `DefaultRegistry` gained a `react-native` → vtsls entry so RN workspaces
  keep TypeScript LSP instead of degrading to "no server registered".
- `platformGroups` aliases the LSP token `node` to the `nodejs` pack group —
  previously any stored/detected `node` platform installed common-only.

## Files

- `apps/local-runner/internal/tui/app/project_wizard.go` (wizardPlatformOptions)
- `apps/local-runner/internal/lsp/platform_detect.go` (react-native detection)
- `apps/local-runner/internal/lsp/platform_registry.go` (react-native vtsls entry)
- `apps/local-runner/internal/skillpack/install.go` (node → nodejs alias)
- New tests: `TestProjectWizardPlatformOptionsCoverSkillPackGroups`,
  `TestDetectPlatformReactNative`, `TestPlatformGroupsNodeAlias` (all red
  before the fix, green after).

## Out of Scope

- `nextjs` still installs common-only; mapping it to the `reactjs` group is
  a product decision left for a follow-up.
- `cpp` remains a detect token without a pack group (no cpp pack exists).
- Pre-existing failures on HEAD (verified via clean worktree at dd217c7):
  `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle`,
  `TestApprovalBarAndStopAreClickable` (tui/app chat chrome), and stale
  counts in `TestScaffoldYAMLIsNotTreatedAsSkill` /
  `TestInstall_CommonOnlyForNonePlatform` (common group grew 13→15 in
  CA-903). Not caused by — and not fixed by — this change.
- GitNexus MCP was unreachable during this change; impact was assessed
  manually (all edited symbols are leaf config/data; callers unchanged).

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: user-report
change_type: bugfix
summary: onboard wizard can select every pack platform (react-native et al); detect RN; node aliases nodejs pack
# --->8---
