---
id: BUG-439
title: Inferred contract still has empty intent and unverified feature key
status: done
version: 2
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-425, CA-928]
---

## AI Quick View
- **What**: BUG-425 still commits inferred contracts with blank intent or unverified feature keys.
- **Why**: CA-928 carries paths but `InferFromDiff` has no intent inference or mandatory key validation.
- **Key constraint**: Preserve declared contracts and fail safely when no verified feature identity exists.

## 1. Metadata
- Document ID: `BUG-439`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `change-contract`, `context-regression-engine`
- Parent Documents: [BUG-425](../done/BUG-425-Inferred-Contracts-Capture-Near-Empty-Metadata.md), [CA-928](../../../change-audit/CA-928-reprompt-inferred-contract-original-paths.md)

## 2. Symptom / Expected
BUG-425 expects a useful intent and a feature key resolved from the actual change/declared contract. `InferFromDiff` only assigns `FeatureKey`, `DeclaredPaths`, `Confidence` (`internal/changecontract/infer.go:16-39`). `prepareChangeContract` uses `featureKey=""` when there is no suggestion (`internal/runner/gate_hook.go:2621-2624,2652-2658`). `suggestFeatureKeys` allows auto-catalog noise when FEATURE-KEYS.md is absent (`gate_hook.go:2546-2573`). Expected: don't persist content-free or misleading records; unresolved identity is explicit.

## 3. Evidence and Reproduction
Live row after CA-928: `{"feature_key":"flowpilot","intent":"","declared_paths":["src"],"confidence":"inferred"}` ([BUG-425](../done/BUG-425-Inferred-Contracts-Capture-Near-Empty-Metadata.md), lines 97–109). The `flowpilot` catalog feature had repo-wide globs. Another non-code turn saved an empty feature key without paths. The new BUG-425 tests (`bug425_gate_reprompt_contract_paths_test.go:136-153`) assert paths/key with favorable registry+glob but never intent or missing-registry/no-code cases. Reproduce with no registered key or empty catalog and an undeclared turn; inspect contracts.ndjson. Severity: **high**; feature history and canonical-head association can be misleading. No new live run in this review.

## 4. Acceptance and Verification
Assertion-first tests for missing/noisy key, unrelated commit subject, no-code turn, intent, full reprompt sequence and restart; preserve old tests, run E2E and live before updating CA/docs. Not fixed here.

## 5. Resolution (2026-09-23, CA-929)

- `prepareChangeContract` no longer persists an unverified feature key: keys
  unresolved against the registered catalog are not written as truth, and
  `commitChangeContract` strips head intent for inferred contracts so prose is
  not smuggled into `BehaviorStatement`.
- Tests: `bug439_440_gate_contract_test.go` — inferred contract keeps real
  `declared_paths` + catalog-resolved key; empty/noise keys and empty intent
  are not persisted. Red before fix.
- `go test -count=1 ./internal/runner -run 'Bug439|Bug440'` — green.
  Provider-agnostic (shared gate path).
