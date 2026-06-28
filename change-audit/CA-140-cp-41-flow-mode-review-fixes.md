# CP-41 Flow Mode review fixes

Adjusted the Flow Mode implementation to keep validation retries aligned with the testing step, load audit changed files from the exact coding turn artifact, and preserve legacy feature-history injection for Plan turns while keeping downstream flow steps on the bounded Flow Context Package path.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: CP-41
change_type: feature
summary: tighten Flow Mode retry, audit, and prompt-history boundaries
# --->8---
