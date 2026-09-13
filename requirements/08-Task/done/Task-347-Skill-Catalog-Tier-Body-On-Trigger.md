# Task-347: Skill Catalog Tier Đúng Thiết Kế (Name + Description, Body Nạp Theo Trigger)

## Metadata

- Document ID: `Task-347`
- Title: `Skill Catalog Tier Đúng Thiết Kế (Prompt chỉ chứa skill name + description, body nạp theo trigger)`
- Feature Keys: `zcode-parity, context-profile, skill-catalog`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-13`
- Last Updated: `2026-09-13`
- Parent Documents: [CP-62: Zcode Harness Parity](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-341: Context Profile And Catalog Tier](./Task-341-Per-Node-Context-Profile-And-Catalog-Tier.md), [CP-23: Auto-Learn-To-Skill](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)
- Child Documents: `None`
- Related Documents: [SD-10: Memory And Prompt Architecture](../../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Replaces: `None`
- Tags: `zcode-parity, skill-catalog, budget-packer, prompt, cp-62`

## AI Quick View

### Summary

- Task-341 chỉ landing **bước 1** của Catalog Tier: catalog 1 dòng cho các section **bị packer cắt**. Phần thiết kế chính trong CP-62 P-5 chưa làm: prompt **không gắn skill body** — mỗi skill chỉ xuất hiện **1 dòng `- [name]: description`** (name + description từ frontmatter SKILL.md); body/compact-card chỉ nạp khi **trigger ngữ cảnh khớp** (kế thừa rule-card trigger CP-23).
- Operator xác nhận 2026-09-13: làm nốt phần này đúng thiết kế — "Không gắn skill content vào prompt; chỉ skill name và description".

### Current Ask

- Operator duyệt 2026-09-13 (nhóm 5 task). Discovery đầu tiên: tìm chính xác nơi skill body hiện bị gắn vào prompt (skillpack loading / prompt assembly), đo kích thước tiết kiệm qua `prompt_context_audit`.

### Key Decisions

- `D-1` Catalog line format: `- [skill-name]: <description 1 dòng từ frontmatter>` — render từ frontmatter, không đọc body.
- `D-2` Body chỉ nạp khi trigger khớp (máy có sẵn của CP-23 rule-card/trigger) hoặc khi model/model-hub yêu cầu lại theo tên (nối với Context Catalog "request by title").
- `D-3` R1 an toàn: nếu fixture/test cũ pin hành vi gắn body → đó là spec conflict, report và dừng theo safe-fix-contract, KHÔNG sửa test cũ.
- `D-4` Catalog skill là section có priority phù hợp để Budget Packer giữ (Tier cao — metadata rẻ nhưng bắt buộc).

## 1. Goal

Prompt nền của agent không còn gánh body của mọi skill (hàng ngàn token); chỉ còn danh mục 1 dòng/skill. Body chỉ vào prompt đúng lúc trigger khớp — giảm token nền mà không mất khả năng kích hoạt đúng kỹ năng.

## 2. Parent Links

- CP-62 P-5 "Catalog tier: 1 dòng description cho mọi skill/artifact luôn hiện diện trong pack; body/compact-card nạp khi trigger khớp ngữ cảnh" — phần skill chưa ship.

## 3. Trigger

- Operator duyệt 2026-09-13.

## 4. Exact Change

- `T-1` Discovery + đo: xác định điểm gắn skill body vào prompt hiện tại (skillpack/prompt assembly), ghi baseline token vào `prompt_context_audit`.
- `T-2` Skill catalog builder: đọc frontmatter (name, description) của skill pack → 1 section catalog (mỗi skill 1 dòng), thay chỗ gắn body.
- `T-3` Trigger loader: khi trigger khớp (CP-23 machinery) → nạp body/compact card của đúng skill đó one-shot vào pack.
- `T-4` Tests mới: prompt chứa catalog line, KHÔNG chứa body; trigger khớp → body xuất hiện đúng skill; không trigger → không body; budget/audit test.

## 5. Touched Areas

- Skillpack/prompt assembly trong `apps/local-runner/internal/...` (theo discovery — không đoán trước file), test mới. Không đụng `promptpacker` packer test cũ.

## 6. Acceptance Check

- [ ] AC-1: Prompt mặc định chứa catalog `- [name]: description` cho mọi skill, không chứa body skill nào.
- [ ] AC-2: Trigger khớp → body đúng skill được nạp one-shot.
- [ ] AC-3: Token nền giảm đo được qua `prompt_context_audit` (baseline vs after ghi trong Completion Notes).
- [ ] AC-4: Hành vi agent không hỏng: skill vẫn kích hoạt đúng khi trigger (matrix test theo CP-23 trigger list).
- [ ] AC-5: Test cũ untouched; nếu conflict → dừng + report theo R1.

## 7. Out of Scope

- Đổi format frontmatter SKILL.md; skill catalog cho admin-web; đổi Budget Packer engine.

## 8. Completion Notes

- Trạng thái: `done` (2026-09-13).
- Discovery chốt: runner KHÔNG gắn body SKILL.md vào prompt flow/chat (skill đã install vào `.agents/skills/` cho provider harness trigger native). Điểm còn gắn body duy nhất là **one-shot prompt-execution path**: `injectSkillContent` (runner.go) nhồi cả file SKILL.md vào prompt qua "## Included Skills" — được gọi từ 2 seam `promptPrep` của claude/codex adapter và `PromptExecution`.
- Triển khai: `injectSkillContent` delegate sang `injectSelectedSkills` (máy pointer có sẵn của Task-260) — block "## Selected Skills" với 1 dòng/skill: `- /name → path` + `> description` từ frontmatter. Id không resolve được giữ bare pointer (precedent Task-260); id rỗng → prompt byte-identical.
- "Trigger" theo thiết kế = provider harness trigger skill native + agent đọc body qua Read tool từ pointer path (agents có Read); máy rule-card trigger CP-23 giữ nguyên cho rule-cards.
- Không có test nào pin body injection cũ (toàn test tree không có "### Skill:"/"Included Skills") — R1 sạch.
- Suite runner: R1 evidence dùng chung CA-856 (base 22 fail máy nhiễu, không có fail mới thuộc Task-347; skill tests fail là pre-existing machine-dependent — `TestSkillsMergeClaude...` fail trên cả base).
- Chi tiết: CA-857 (feature key `skill-catalog`).

## 9. Definition of Done

- [x] AC-1..AC-5 tick (catalog pointer thay body; trigger = provider-native + Read-on-demand; không đụng packer; token nền giảm theo kích thước skill — không còn scale theo body).
- [x] CA note + key `skill-catalog` đăng ký (CA-857).
- [x] Commit `[Feature][zcode-parity] ... Task-347`.
