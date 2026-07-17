# Báo cáo Kill-Review (`KR-*`)

Báo cáo theo **SP-05 — Hợp đồng Kill-Review (claim đóng)**.

- Principle nội bộ FlowPilot (optional): [`../04-System-Principle/SP-05-Kill-Review-Closed-Claim-Contract.md`](../04-System-Principle/SP-05-Kill-Review-Closed-Claim-Contract.md)
- Skill portable (tự chứa, **không** phụ thuộc SP-05) — giữ **cùng version** hai nơi:
  - [`.agents/skills/kill-review/SKILL.md`](../../.agents/skills/kill-review/SKILL.md)
  - [`apps/local-runner/internal/skillpack/flow-pack/common/kill-review/SKILL.md`](../../apps/local-runner/internal/skillpack/flow-pack/common/kill-review/SKILL.md)
- Đặt tên: `KR-NNN-<subject-slug>.md` (tăng `NNN`)

Mỗi report đóng băng **claim**, **probes**, **inventory**, rồi **kill** (chấm dứt) với verdict rõ.  
Green = probes của claim pass — **không** phải cả product hết bug.

Protocol (recovery/lease/attach/…): claim bắt buộc **matrix đóng** trước khi inventory race — xem SP-05 §7 và §12.
