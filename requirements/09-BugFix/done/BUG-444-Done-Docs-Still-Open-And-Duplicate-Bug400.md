---
id: BUG-444
title: Done bug reports still marked open and BUG-400 has duplicate records
status: done
version: 2
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [CP-Test-Progress-Tracking]
---

## AI Quick View
- **What**: Fourteen reports in done/ still state open; two done/ files share BUG-400.
- **Why**: Completion notes and file moves were not reconciled with canonical report metadata.
- **Key constraint**: Keep one canonical document per ID and preserve evidence during consolidation.

## 1. Metadata
- Document ID: `BUG-444`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `context-regression-engine`
- Parent Documents: [CP-Test-Progress-Tracking](../../07-Coding-Plan/done/CP-Test-Progress-Tracking.md)

## 2. Symptom and Impact
14 `requirements/09-BugFix/done/BUG-4*.md` reports still contain `Status: open`: BUG-415,416,417–423,425,427–430. E.g. BUG-425 has stale `Last Updated` and `Current Ask: awaiting prioritization` despite completion notes. Two done files represent BUG-400: `BUG-400-Turn-FlowRef-Bypasses-Working-Mode.md` is the full report; `BUG-400-TurnFlowRef-Bypasses-Working-Mode.md` is only a five-line completion note without metadata. Severity: **medium** for triage/document integrity.

## 3. Reproduction and Evidence
Search `^- Status: \`open\`` in `requirements/09-BugFix/done/BUG-4*.md` → 14 matches at review time. Read/list `done/BUG-400-*.md` → two files, same ID. Moving the second file out of todo/ did not merge it into canonical BUG-400.

## 4. Acceptance and Verification
Reconcile each report after verifying its fix; update status/date/Current Ask and merge BUG-400 notes into canonical report without losing evidence. Removing the duplicate file requires specific user confirmation under destructive-operation rules. Not changed here.

## 5. Resolution (2026-09-23, CA-935)

- 14 done reports reconciled: `Status: done`, `Last Updated: 2026-09-23`,
  Current Ask → completion notes.
- BUG-400: 5-line completion note merged into the canonical report's
  Completion Notes (with provenance remark). The duplicate file
  `BUG-400-TurnFlowRef-Bypasses-Working-Mode.md` was removed 2026-09-23 after
  operator confirmation — the canonical report is the single document for
  this ID.
