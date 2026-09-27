# CA-1044: RN Skillpack — AppStart 5-Layer Contract + Refreshed App Catalog

## Summary

- Aligned the `react-native` flow-pack skills with the Sep-27 plan set in
  `TitoCompose/docs/apps` (`shared-core-architecture-spec.md`,
  `towsafe-master-spec.md`, `cutcraft-master-spec.md`,
  `product-blueprint-micro-utilities.md`, `EXECUTE_PROMPT.md`).
- New skill `react-native-appstart-architecture` encodes the mandatory
  5-layer thin-client contract — `src/{api, domain, data, datasource,
  presentation}` — with the one-way dependency rule, zero-leakage boundary
  mapper chain (DbEntity ↔ Domain → UiModel), Zustand micro-store as
  ViewModel (persist → MMKV), an expo-router thin-route variant note, and a
  pre-flight gate checklist.
- `scaffold.yaml` now force-attaches `react-native-appstart-architecture`
  (5 scaffold skills); `apps/_template` in `react-native-scaffold-bootstrap`
  gained the `src/` 5-layer skeleton next to thin `app/` routes.
- `react-native-multi-module-monorepo`: added the missing `core-ads`
  package and replaced the stale app lineup
  (stepflow/pawvault/zipclip/focuszen) with the current 7-app catalog +
  build order `towsafe → cutcraft → proquote → sparkycalc → shiftsync →
  flipcalc → docuscan`; added an explicit mapping from the feature
  api/internal pattern onto the app-level 5-layer contract.
- `react-native-conventions`: ViewModel is now a Zustand micro-store (local
  hooks only for ephemeral UI state); routing line distinguishes thin-client
  `src/navigation/RootNavigator.tsx` from the `_template` expo-router
  skeleton.
- `react-native-clean-architecture`: added a pointer noting the 3-boundary
  model is realized as the 5-folder contract in the new skill.

## Files

- New: `apps/local-runner/internal/skillpack/flow-pack/react-native/react-native-appstart-architecture/SKILL.md`
- `apps/local-runner/internal/skillpack/flow-pack/react-native/scaffold.yaml`
- `apps/local-runner/internal/skillpack/flow-pack/react-native/react-native-scaffold-bootstrap/SKILL.md`
- `apps/local-runner/internal/skillpack/flow-pack/react-native/react-native-multi-module-monorepo/SKILL.md`
- `apps/local-runner/internal/skillpack/flow-pack/react-native/react-native-conventions/SKILL.md`
- `apps/local-runner/internal/skillpack/flow-pack/react-native/react-native-clean-architecture/SKILL.md`
- Contract-extension test updates (user-approved): `scaffold_recipe_test.go`
  (4→5 skill list, 25→26 skill count), `scaffold_handler_test.go` (3×),
  `scaffold_dispatcher_test.go` (1×).

## Out of Scope

- UI token drift (Emerald/Amber/Crimson + Safety Orange palette, semicircle
  SVG MetricGauge, StatusHeroCard/SegmentedSwitch/NumericInput/etc.) —
  `react-native-core-ui-tokens` update deferred.
- New plumbing pillars: EULA legal gatekeeper + local notifications +
  core-pdf template engine.
- TDD golden-dataset gate (TowingMathEngine / GuillotineBinPacker before UI).
- Stale `docs/` copies of shared-core-architecture-spec and
  product-blueprint (Sep-18 versions vs Sep-27 source) left untouched.

## Validation

- `go test -count=1 -run 'Scaffold|Skill' ./internal/runner/...
  ./internal/skillpack/... ./internal/tui/app/...` — all green.
