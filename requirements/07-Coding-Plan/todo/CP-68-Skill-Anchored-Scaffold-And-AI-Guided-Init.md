# CP-68: Skill-Anchored Project Scaffolding & AI-Guided Init Engine

## Metadata

- Document ID: `CP-68`
- Title: `Skill-Anchored Project Scaffolding & AI-Guided Init Engine`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude Sonnet MAX, Operator`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-20: Definition Of Done Gate Contract](../../05-System-Specs/SS-20-Definition-Of-Done-Gate-Contract.md)
- Child Documents: [Task-383: Platform Scaffold Recipe Discovery & Skill Integrity Validator](../../08-Task/todo/Task-383-Platform-Scaffold-Recipe-Discovery-And-Skill-Integrity-Validator.md) (P-1), [Task-384: Runner Scaffold Dispatcher & AI Turn Orchestration](../../08-Task/todo/Task-384-Runner-Scaffold-Dispatcher-And-AI-Turn-Orchestration.md) (P-2), [Task-385: TUI Single-Command Init & Desktop UI Adaptive Scaffold Trigger](../../08-Task/todo/Task-385-TUI-Subcommand-And-Desktop-UI-Adaptive-Scaffold-Trigger.md) (P-3), [Task-386: Compiler Verification Gate & Self-Healing Loop](../../08-Task/todo/Task-386-Compiler-Verification-Gate-And-Self-Healing-Loop.md) (P-4)
- Related Documents: [CP-34: Init / Setup Tool](../done/CP-34-Init-tool.md), [CP-60: Vibe Working Mode](../done/CP-60-Vibe-Working-Mode.md), [CP-67: Contract-First Scaffold TDD & Signature Lock Gate](./CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Replaces: `None`
- Tags: `init, scaffold, skillpack, template, react-native, compiler-gate, local-runner, tui, desktop`
- Feature Keys: `skill-anchored-init, scaffold-engine, ai-guided-init`

---

## AI Quick View

### Summary

- Trước đây, lệnh `/init` (CP-34) chỉ thực hiện sao chép tĩnh các file skill từ embedded `flow-pack` sang thư mục `.agents/skills` của project mục tiêu, không có khả năng sinh mã nguồn hoặc dựng khung dự án (Step 0) thực tế.
- CP-68 nâng cấp toàn diện luồng `init` (chạy cả chủ động qua TUI `/init` và bị động khi tạo project mới trên Desktop) thành **Skill-Anchored AI Scaffold Flow**: Runner tự động nhận diện `platform`, cài đặt bộ kỹ năng chuyên dụng (`react-native-scaffold-bootstrap`, `react-native-mobile-plumbing`, `react-native-core-ui-tokens`, `react-native-screen-archetypes`), và tự động kích hoạt một lượt AI Turn chuyên trách nhiệm dựng khung.
- Runner tự động đính kèm (auto-attach) các skills nền tảng vào lượt gọi AI mà **người dùng không cần phải gõ `@mention` hay chỉ dẫn thủ công**.
- **Một lệnh `/init` duy nhất (Single Command):** Không thêm subcommand `scaffold` — `/init` (bare / `/init all`) chạy nguyên luồng init hiện có (cài skill, ledger, catalog) rồi tự kiểm tra manifest `flow-pack/<platform>/scaffold.yaml` + tính đầy đủ của `scaffold_skills` để quyết định trigger AI Scaffold Turn trên **cả TUI và Desktop**; thiếu manifest hoặc thiếu skill → bỏ qua an toàn nhánh scaffold.
- Thiết lập **Compiler Verification Gate** (`pnpm tsc --noEmit`, `go vet`, `./gradlew check`) làm chốt chặn Oracle bắt buộc: project sinh ra phải đạt trạng thái biên dịch sạch 100% (Zero Compile Error) mới được coi là `init` hoàn tất và sẵn sàng bước vào `vibe-sprint`.

### Current Ask

- Xây dựng 4 lát cắt kỹ thuật (P-1 đến P-4) để chuyển hóa lệnh `init`:
  - `P-1`: Bổ sung và đồng bộ các Blueprint Skills vào catalog `flow-pack` của từng platform (bắt đầu với React Native).
  - `P-2`: Xây dựng Runner Scaffold Orchestrator (`InitScaffoldDispatch`) chịu trách nhiệm chuẩn bị ngữ cảnh và khởi tạo AI Turn.
  - `P-3`: Tích hợp trải nghiệm người dùng trên TUI (một lệnh `/init` duy nhất, không thêm subcommand) và Desktop UI (khi bấm Create Project).
  - `P-4`: Thiết lập Compiler Verification Gate và vòng lặp tự sửa lỗi (Self-Healing Loop) cho Step 0.

### Key Decisions

- `P-1` Khế ước Platform Scaffold Recipe (`scaffold.yaml`): Mỗi nền tảng sở hữu một manifest `flow-pack/<platform>/scaffold.yaml` khai báo danh sách `scaffold_skills`, `verification_gate.command` và cờ `enabled`. Runner hoàn toàn KHÔNG hardcode danh sách skill hay ngôn ngữ trong code Go, cho phép mở rộng sang Python, Go, Android... hoàn toàn cắm-rút (plug-and-play).
- `P-2` Cơ chế Kiểm Định 3 Tầng & Graceful Ignore (The 3-Tier Verifiable Capability Gate):
  - *Tầng 1 (Recipe Discovery):* Runner kiểm tra sự tồn tại của `flow-pack/<platform>/scaffold.yaml`. Nếu thiếu hoặc `enabled: false`, Runner **Graceful Ignore** 100% bước gọi AI (chỉ chạy cài skill tĩnh CP-34, log thông báo `scaffold: skipped (no verified recipe)` và kết thúc an toàn, không báo lỗi).
  - *Tầng 2 (Skill Integrity):* Runner kiểm tra toàn bộ các skill được liệt kê trong `scaffold_skills` có thực sự hiện diện trong pack hay không trước khi gọi AI.
  - *Tầng 3 (Tooling Preflight):* Runner đối chiếu tooling máy khách (Node, pnpm, Go...) trước khi cho phép AI sinh mã.
- `P-3` Runner tự động sinh Prompt chuyên trách cho lượt Scaffold Turn, trích xuất hướng dẫn từ các skill đã cài đặt và đưa vào Context Profile, loại bỏ hoàn toàn sự phụ thuộc vào trí nhớ hay thao tác gõ prompt của người dùng.
- `P-4` Một lệnh init duy nhất (Single `/init` Command): TUI không bổ sung subcommand `scaffold`. Lệnh `/init` (bare, mặc định tương đương `/init all`) chạy trọn luồng init hiện có của CP-34/CP-63 (skillpack + ledger + catalog), sau đó Runner tự đánh giá năng lực scaffold:
  - Nếu `flow-pack/<platform>/scaffold.yaml` tồn tại (enabled) **và** toàn bộ `scaffold_skills` khai báo đều hiện diện trên đĩa (đủ skill) → tự động trigger AI Scaffold Turn ngay sau bước cài skill.
  - Nếu thiếu manifest, `enabled: false` hoặc thiếu skill → bỏ qua an toàn nhánh scaffold (chỉ log `scaffold: skipped (no verified recipe)`), giữ nguyên kết quả init tĩnh.
  - `/init skill` giữ nguyên như lối thoát chỉ cài skill tĩnh, không bao giờ gọi AI.
  - **Bị động (Passive/Desktop):** Khi tạo project mới qua Desktop UI, sau khi lưu project bindings, Desktop tự động trigger endpoint `POST /client/projects/{projectId}/scaffold` (chỉ khi platform capable).
- `P-5` Compiler Gate hoạt động theo nguyên tắc "Fail-Closed": AI chỉ được coi là hoàn thành Step 0 khi lệnh kiểm tra kiểu (`tsc`) hoặc build trả về Exit Code 0. Nếu fail, Runner tự động đẩy log lỗi compiler vào context để AI vá lỗi tối đa 3 vòng lặp (`cap: 3`).

### Constraints

- Tương thích ngược 100% với hành vi `/init skill` hiện tại của CP-34.
- Không đưa logic phụ thuộc đám mây; toàn bộ quá trình đọc template, gắn skill và kiểm tra compiler diễn ra hoàn toàn cục bộ trên máy khách ($0 Server Cost).
- Đảm bảo tính nhất quán trên cả 4 providers: Claude, Codex, Grok, Opencode.

### Open Questions

- `Q-1`: Đối với project mới hoàn toàn, khi nào Runner nên kích hoạt `pnpm install`? 
  $\rightarrow$ *Quyết định:* Runner sẽ chạy `pnpm install` tự động ngay sau khi AI sinh xong các file cấu hình `package.json` và `pnpm-workspace.yaml`, trước khi kích hoạt `pnpm tsc --noEmit`.
- `Q-2`: Nếu một platform mới chỉ có skill bình thường mà chưa có `scaffold.yaml` thì UI hiển thị ra sao?
  $\rightarrow$ *Quyết định:* Với TUI, thiết kế single-command khiến không tồn tại subcommand `scaffold` để hiển thị hay ẩn — picker `/init` giữ nguyên `skill` và `all`; platform không capable thì nhánh scaffold tự bị bỏ qua trong luồng `/init`. Trên Desktop UI, nếu platform không có `scaffold.yaml` (hoặc `enabled: false`) thì checkbox/tùy chọn "AI Scaffold" vẫn **ẨN HOÀN TOÀN**, loại bỏ hoàn toàn khả năng người dùng bấm nhầm vào tính năng chưa sẵn sàng.

### Source Refs

- [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- [CP-34: Init / Setup Tool — Desktop Engine Page & Project Auto-Init](../done/CP-34-Init-tool.md)
- `apps/local-runner/internal/skillpack/install.go`
- `apps/local-runner/internal/tui/app/init_engine.go`

---

## 1. Goal

Chuyển đổi lệnh `init` của FlowPilot từ một tác vụ sao chép file tĩnh đơn giản thành một **Động cơ khởi tạo dự án tự động (Autonomous Scaffolding Engine)**. Đảm bảo bất kỳ dự án mới nào (bắt đầu với React Native Expo Monorepo) khi được tạo ra đều có sẵn:
1. Đầy đủ bộ khung Monorepo và các cấu hình gốc (`package.json`, `turbo.json`, `tsconfig.json`).
2. 7 Packages nền tảng dùng chung (`core-ui`, `core-storage`, `core-billing`, `core-ads`, `core-pdf`, `core-security`, `core-common`).
3. Ứng dụng mẫu `apps/_template` tích hợp sẵn 9 trụ cột mobile plumbing.
4. Xác thực biên dịch thành công 100% qua TypeScript Compiler Gate trước khi bắt đầu code tính năng.

---

## 2. Input Documents

### 2.1 Governing Documents
- [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- [SS-20: Definition Of Done Gate Contract](../../05-System-Specs/SS-20-Definition-Of-Done-Gate-Contract.md)

### 2.2 Implemented Foundations
- [CP-34: Init / Setup Tool](../done/CP-34-Init-tool.md)
- [CP-56: Terminal TUI Chat And Flow Client](../done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- [CP-60: Vibe Working Mode](../done/CP-60-Vibe-Working-Mode.md)

---

## 3. Implementation Strategy

### 3.1 Sơ Đồ Kiến Trúc Luồng Init Mới

```text
[User / Desktop Trigger]
   │
   ├── TUI: /init (Chủ động — một lệnh duy nhất)
   └── Desktop: Create Project (Bị động)
   │
   ▼
┌─────────────────────────────────────────────────────────────┐
│ Bước 1: Skillpack Installation (Deterministic)              │
│ - Gọi skillpack.Install(targetDir, platform)                │
│ - Cài đặt 9 skills vào .agents/skills/ & .claude/skills/    │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Bước 2: AI Scaffold Turn Dispatcher                         │
│ - Runner chuẩn bị Context Profile:                          │
│   + react-native-scaffold-bootstrap                         │
│   + react-native-mobile-plumbing                            │
│   + react-native-core-ui-tokens                             │
│ - Tạo Turn với posture: "scaffold_bootstrap"                │
│ - Prompt: Yêu cầu AI dựng Step 0 tuân thủ strict checklist  │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Bước 3: AI Code Generation                                  │
│ - AI sinh root configs (turbo, pnpm, tsconfig)              │
│ - AI sinh 7 packages/core-* và apps/_template               │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Bước 4: Compiler Verification Gate                          │
│ - Runner thực thi: pnpm install && pnpm tsc --noEmit        │
│   • Exit 0 ──► Hoàn tất Init, thông báo thành công          │
│   • Exit != 0 ──► Bơm lỗi vào AI để Self-Healing (cap: 3)    │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. Work Breakdown

### P-1: Hoàn Thiện Bộ Blueprint Skills Cho Nền Tảng (Core Skillpack)
- Nhúng 4 skill scaffold vào `apps/local-runner/internal/skillpack/flow-pack/react-native/` (đã có trên nhánh hiện tại):
  - `react-native-scaffold-bootstrap/SKILL.md`
  - `react-native-mobile-plumbing/SKILL.md`
  - `react-native-core-ui-tokens/SKILL.md`
  - `react-native-screen-archetypes/SKILL.md`
- Cùng manifest `scaffold.yaml` khai báo đúng 4 `scaffold_skills` trên và `verification_gate.command`.
- Đảm bảo bộ test `TestInstall_*` trong `internal/skillpack/skillpack_test.go` nhận diện đầy đủ 23 skills cho platform `react-native` (13 common + 10 react-native), gồm đủ 4 `scaffold_skills`.

### P-2: Xây Dựng Runner Scaffold Dispatcher (`internal/runner/scaffold.go`)
- Tạo hàm điều phối `DispatchScaffoldTurn(ctx, projectID, workspaceDir, platform)`:
  - Kiểm tra trạng thái cài đặt skillpack qua `skillpack.Status()`. Nếu chưa có, tự động gọi `skillpack.Install()`.
  - Đọc nội dung 3 skill scaffold và nạp vào prompt template của Scaffold Turn.
  - Tạo session turn gửi tới provider (Claude/Codex/Grok/Opencode) với chỉ dẫn rõ ràng về cấu trúc Monorepo và 7 core packages.

### P-3: Nâng Cấp Giao Diện Điều Khiển TUI & Desktop
- **Trong TUI (`apps/local-runner/internal/tui/app/`):**
  - Giữ nguyên mặt lệnh hiện có, KHÔNG thêm subcommand `scaffold`:
    - `/init` (bare, mặc định `all`) và `/init all`: chạy luồng engine init hiện có (skillpack + ledger + catalog) rồi tự kiểm tra `skillpack.HasScaffoldCapability(platform)`; nếu capable → trigger AI Scaffold Turn ngay sau khi cài skill; không capable → bỏ qua nhánh scaffold (log `scaffold: skipped (no verified recipe)`).
    - `/init skill`: hành vi cũ CP-34, chỉ cài skillpack tĩnh (không gọi AI).
  - Cập nhật suggestion picker (không thêm dòng `scaffold`) và xử lý sự kiện `EngineInitMsg` để hiển thị tiến trình Scaffold Turn + Compiler Gate.
- **Trong Desktop UI:**
  - Khi hoàn thành wizard tạo project, kiểm tra platform:
    - Nếu platform có `scaffold.yaml`: Hiển thị checkbox `[x] Tự động dựng khung dự án (AI Scaffold Step 0)`.
    - Nếu platform không có: **ẨN hoàn toàn** checkbox này để giữ UI tối giản, không làm rối người dùng.

### P-4: Tích Hợp Compiler Verification Gate & Vòng Lặp Sửa Lỗi (Self-Healing)
- Viết module kiểm định `internal/runner/compiler_gate.go`:
  - Dựa vào `platform` để chọn lệnh kiểm tra thích hợp:
    - `react-native`: `pnpm install && pnpm tsc --noEmit`
    - `golang`: `go mod tidy && go vet ./...`
    - `android`: `./gradlew check -x test`
  - Nếu lệnh thất bại, bắt lấy stderr/stdout, định dạng thành `CompilerErrorFeedback` và gửi tiếp turn phản hồi cho AI để sửa chữa trực tiếp (tối đa 3 lần thử).

---

## 5. Touched Areas

- **`apps/local-runner/internal/skillpack/flow-pack/react-native/`**: Thêm 3 thư mục skill (`scaffold-bootstrap`, `mobile-plumbing`, `core-ui-tokens`).
- **`apps/local-runner/internal/runner/scaffold.go`**: Logic điều phối lượt Scaffold Turn.
- **`apps/local-runner/internal/runner/compiler_gate.go`**: Trình chốt chặn kiểm tra biên dịch.
- **`apps/local-runner/internal/tui/app/init_suggestions.go`**: Giữ nguyên picker `skill`/`all` — không thêm gợi ý `/init scaffold`.
- **`apps/local-runner/internal/tui/app/init_engine.go`**: Xử lý dispatch lệnh scaffold và hiển thị tiến trình.

---

## 6. Data or Migration Steps

- Không thay đổi schema cơ sở dữ liệu Supabase.
- Lưu trạng thái scaffold vào file nội bộ của project: `.flowpilot/scaffold-status.json` để tránh chạy lại không cần thiết khi project đã được khởi tạo thành công.

---

## 7. Validation Plan

### 7.1 Automated Unit Tests
- `TestSkillpack_ReactNativeIncludesScaffoldSkills`: Đảm bảo `skillsForPlatform("react-native")` trả về đầy đủ 23 skills (13 common + 10 react-native), bao gồm đủ 4 `scaffold_skills` khai báo trong `scaffold.yaml`.
- `TestScaffoldDispatcher_ContextProfileContainsSkills`: Kiểm tra Scaffold Turn được nhúng đúng nội dung 3 blueprint skills vào prompt context.
- `TestCompilerGate_CatchesTypeScriptErrors`: Mock lỗi typecheck và đảm bảo Compiler Gate trả về mã lỗi chính xác.
- `TestCompilerGate_PassesCleanProject`: Đảm bảo dự án chuẩn trả về `ok = true`.

### 7.2 Manual Verification Steps
1. Mở TUI trong một thư mục rỗng, bind project loại `react-native`.
2. Gõ `/init`.
3. Quan sát AI tự sinh các file cấu hình và packages.
4. Kiểm tra file `package.json`, `turbo.json`, `packages/core-*` và `apps/_template` xuất hiện đầy đủ.
5. Kiểm tra TUI hiển thị trạng thái `Compiler Gate: PASS` và terminal báo exit code 0.

---

## 8. Rollout and Fallback

- **Rollout:** Triển khai độc lập trong `apps/local-runner`. Người dùng dùng `/init` đầy đủ (auto scaffold khi capable) hoặc `/init skill` cũ (chỉ cài skill tĩnh).
- **Fallback:** Nếu lượt AI Scaffold Turn thất bại hoặc gặp sự cố mạng, trạng thái project vẫn giữ nguyên các file skill đã cài đặt; người dùng có thể kích hoạt lại lệnh hoặc tự code thủ công mà không bị hỏng cấu hình runner.

---

## 9. Risks

- `R-1`: **Quá tải Token (Context Window Overflow):** Nếu nhúng toàn bộ mã nguồn của cả 3 skill vào context của 1 turn, prompt có thể bị phình to.
  - *Biện pháp giảm thiểu:* Tách phần hướng dẫn cốt lõi vào System Prompt, các file mã nguồn mẫu chi tiết được nạp theo cơ chế lazy reference hoặc tóm tắt dạng interface/signatures.
- `R-2`: **Chậm trễ khi chạy `pnpm install`:** Trên máy có kết nối mạng yếu, lệnh install có thể mất vài phút.
  - *Biện pháp giảm thiểu:* Thiết lập timeout hợp lý (300s) và hiển thị live spinner trên TUI để người dùng nắm được tiến trình.

---

## 10. Definition of Done

- [ ] 4 Skill nền tảng (`scaffold-bootstrap`, `mobile-plumbing`, `core-ui-tokens`, `screen-archetypes`) được nhúng và đồng bộ trong `flow-pack/react-native/` (Đã hoàn thành ở bước trước).
- [ ] Runner có endpoint hoặc handler nội bộ tiếp nhận lệnh Scaffold Turn và gắn kèm skill tự động.
- [ ] Single `/init`: lệnh `/init` (bare / `/init all`) chạy luồng init hiện có rồi tự trigger Scaffold Turn khi platform capable; Tab completion giữ nguyên `skill`/`all`, KHÔNG có subcommand `scaffold` mới.
- [ ] Auto-run áp dụng cho cả TUI và Desktop; platform không capable (thiếu manifest hoặc thiếu skill) thì bỏ qua nhánh scaffold an toàn.
- [ ] Compiler Verification Gate xác thực thành công mã nguồn được sinh ra với `pnpm tsc --noEmit`.
- [ ] Toàn bộ unit test liên quan trong `local-runner` đều PASS.
