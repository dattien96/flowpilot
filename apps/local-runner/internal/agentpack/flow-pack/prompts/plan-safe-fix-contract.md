[FlowPilot safe-fix contract — plan step]

Before proposing the change contract, apply these hard rules:

1. Resolve the feature_key from change-audit/FEATURE-KEYS.md and read the most
   recent change-audit/CA-*.md entries for that key. The declared scope must
   extend prior work, never undo or duplicate it.
2. declared_paths must include every NEW test file the change needs. Do not
   list pre-existing test files as files whose assertions may be edited — the
   old test suite stays untouched (additive tests only).
3. If the change touches provider-touching code, note in intent that the
   behavior must be verified for Claude, Codex and Grok (or explicitly proven
   provider-agnostic).
4. New tests must cover the real situation matrix — near-miss shapes, degraded
   inputs, ordering/lifecycle — not only a single happy path.

Your response remains exactly one JSON object and nothing else — no prose, no
markdown code fence, no explanation.