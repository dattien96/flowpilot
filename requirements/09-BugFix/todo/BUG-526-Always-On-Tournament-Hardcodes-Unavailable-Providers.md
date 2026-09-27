# BUG-526 — Always-on tournament rescue hardcodes unavailable providers

## Severity

Important — automatic recovery is not provider-availability aware.

## Evidence

- `apps/local-runner/internal/runner/tournament_escalation.go:26-55`
- `apps/local-runner/internal/agentpack/flow-pack/flows/tournament-harness.yaml:48-75`
- Live `run-1383`: Claude had no connected local account and failed; the Codex path did not provide a genuine two-provider tournament.

Tournament escalation ignores its former rollout flag and is always enabled, while the built-in rescue flow fixes candidate A to Claude and candidate B to Codex. It does not select from connected accounts/providers. On a machine with Devin and Grok but no Claude, an automatic cap/stall rescue immediately launches a known-unavailable candidate.

## Impact

A mandatory recovery path can degrade to one candidate or fail entirely based on local provider installation rather than workload policy. This violates the provider-parity/typed-degradation contract and made the required live case impossible without rebuilding a temporarily modified pack.

## Missing regression

With only Devin and Grok connected, trigger automatic tournament escalation and assert that two eligible, real providers are selected or that the flow parks before dispatch with a typed route decision. Do not silently launch unavailable Claude/Codex candidates.

## Scope

Capture only. The temporary live pack modification was reverted; shipped YAML remains unchanged.
