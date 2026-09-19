# CP-67 Bản Ghi Chú Kiến Trúc: Contract-First Scaffold TDD & Signature Lock Protocol

> **Dành cho Tech Lead / Human Operator:** Tài liệu này ghi lại chi tiết toàn bộ quyết định thiết kế cuối cùng (Final Design) của tính năng **Contract-First Scaffold TDD & Signature Lock Gate** sau chuỗi thảo luận chuyên sâu về các luồng `task-harness`, `vibe-sprint` và bài học từ `CP-64`.

---

## 1. Bối Cảnh & Vấn Đề Cốt Lõi (The Dilemma)

### 1.1 Thành công của CP-64 ở BugFix
Gần đây, chúng ta đã triển khai thành công [CP-64](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md) học tập phương pháp **Reproduce-First** từ DeepSeek / SWE-bench:
- Trước khi Coder được sửa code production, Tester bắt buộc phải viết 1 test và test đó **phải chạy ĐỎ (Assertion Failure)**.
- Sau khi test ĐỎ được xác nhận, file test bị **khóa cứng Read-Only**.
- Coder vào sửa code production sao cho test từ **ĐỎ $\rightarrow$ XANH**. Coder không thể sửa test để né lỗi.

### 1.2 "Nghịch lý con gà & quả trứng" khi áp dụng vào Task mới và Vibe
Chúng ta từng nói: *Chỉ áp dụng Reproduce-First cho BugFix vì Bug thì code cũ đã có sẵn để test.*

Tuy nhiên, trong `task-harness` và `vibe-sprint`, việc giữ quy trình cũ dẫn tới một **tử huyệt kiến trúc**:
1. **Tại bước TDD (`test_signatures`)**: Vì code tính năng mới chưa tồn tại, nếu Tester viết assertion gọi hàm mới thì compiler sẽ báo lỗi cú pháp **Compile Error** (`undefined: NewService`). Mà hệ thống cấm Compile Error, nên TDD chỉ dám viết các khung hàm test rỗng (`func TestFoo(t *testing.T) {}`).
2. **Tại bước Coder (`implement`)**: Coder được giao vừa viết code production vừa tự điền nội dung test.
   - **Nếu Coder viết test trước**: Vẫn gặp đúng vấn đề là chưa có code cụ thể thì viết thân test kiểu gì. Nếu viết được thì tại sao không viết luôn ở TDD?
   - **Nếu Coder viết code trước rồi mới viết test**: Đây chính là **Confirmation Bias / Tautological Testing ("Test tự khen mình")**. Coder viết code xong, sau đó nó viết assertion khớp đúng với những gì nó vừa viết (dù có thể hiểu sai spec hoặc sót edge case). Kết quả: Test pass 100% (Xanh) nhưng test vô dụng, không bảo vệ được nghiệp vụ!
   - **Nếu thêm một agent điền test sau Coder**: Nếu agent này nhìn vào code của Coder, nó cũng sẽ bị "nhiễm" bias và sinh test theo implementation hiện tại.

---

## 2. Quyết Định Thiết Kế Cuối Cùng: "Contract-First Scaffold TDD"

Sau khi cân nhắc, chúng ta đã thống nhất triển khai **Phương án 1 (Contract-First Scaffold TDD)** với đầy đủ các tầng kiểm duyệt vật lý và giao thức điều phối chặt chẽ:

```text
                  ┌──────────────────────────────────────────────┐
                  │                MAIN AGENT                    │
                  │         (Trọng tài & Điều phối)              │
                  │   - Nắm Master Plan & Specs                  │
                  │   - Duyệt Batch Signature Changes            │
                  │   - Ngân sách vòng lặp: cap: 5               │
                  └───────┬──────────────────────────────▲───────┘
                          │                              │
             (1) Lệnh tạo │                  (4) Báo cáo │
                 Scaffold │                      kết quả │ (Báo hoàn thành
                 + Test Đỏ│                              │  hoặc nộp BATCH)
                          ▼                              │
              ┌───────────────────────┐                  │
              │       STEP: TDD       │                  │
              │   (Model Xịn / Tier 1)│                  │
              │ - Tạo Stub Signatures │                  │
              │ - Viết Red Test Suite │                  │
              └───────────┬───────────┘                  │
                          │                              │
                          │ (Compile OK & Test ĐỎ)       │
                          │ Khóa Test + Snapshot Hash    │
                          ▼                              │
              ┌───────────────────────┐                  │
              │      STEP: CODER      │                  │
              │ - Chỉ Fill Body Code  │──────────────────┘
              │ - Cấm sửa Signature   │
              │ - Gom BATCH cuối turn │
              └───────────────────────┘
```

---

## 3. Bảy Trụ Cột Kỹ Thuật Chi Tiết

### Trụ cột 1: Step TDD dùng Model Xịn ("Scaffold & Red Test") — model set qua flow YAML `model:`

- Không dùng model rẻ/nhẹ cho TDD. TDD lúc này đóng vai trò là **API Contract Architect** (model mặc định là model high-reasoning, ví dụ `claude-sonnet-4-5` — set qua field `model:` trên node trong flow YAML: `task-harness.yaml`/`vibe-sprint.yaml`; admin vẫn có thể override qua `step_definitions` rows — `agentpack/pack.go:1020-1035`).
- **Nhiệm vụ của TDD**:
  1. **Sinh bộ khung Production Stubs**: Tạo/bổ sung các file production với đầy đủ Struct, Interface, Function signatures (tên, tham số, kiểu trả về). Phần thân hàm bắt buộc là Stub rỗng:
     - *Kotlin / Android*: `fun doAction(param: String): Result = TODO("not implemented")`
     - *Go*: `func DoAction(param string) (*Result, error) { return nil, errors.New("not implemented") }`
     - *TypeScript*: `export function doAction(param: string): Result { throw new Error("not implemented"); }`
  2. **Sinh Test Suite Hoàn Chỉnh (Executable Red Tests)**: Viết các test cases với assertions đầy đủ gọi vào các hàm stub vừa tạo.
- **Kết quả khi chạy test**:
  - **Compile Check**: PASS 100% (do các symbol đã tồn tại trên đĩa, không bao giờ bị lỗi `undefined symbol`).
  - **Runtime Test**: Phải CHẠY ĐỎ (RED) 100% — enforced vật lý bởi rule `r-scaffold-red` (sau này), không chỉ prompt.
  - **Detect AI viết body logic thực tế**: Nếu AI tự tiện viết logic nghiệp vụ trong stub (thay vì trả về `nil`/TODO), test sẽ chạy **XANH (GREEN)** → `r-scaffold-red` sẽ fire reprompt yêu cầu AI quay lại viết stub rỗng. Nhờ cơ chế này, TDD step vật lý có thể detect và force AI chỉ giữ lại signature, không được implement body.

---

### Trụ cột 2: Cổng Khóa Chữ Ký (`r-signature-lock`) bằng Canonical AST Hash — Signature Hash (chỉ signature, không bao gồm body) (post-review B-8.1)

Làm sao ngăn chặn Coder tự tiện đổi signature, thêm/xóa function?

- **Khái niệm (đổi mới post-review B-8.1)**: Hash **CHỈ SIGNATURE** (function name, params, return types — không bao gồm body). Mọi thay đổi — sửa signature cũ, thêm function mới, xóa function — đều làm hash lệch và phải đi qua batch renegotiation. Không có khái niệm "additive change hợp lệ".
- **Cơ chế**:
  1. Runner dùng AST Parser (Go: `go/parser`; Kotlin/TS: LSP `DocumentSymbols` từ CP-63 — `lsp_client.go` thêm method `DocumentSymbols(filePath)`, fallback regex khi LSP off) bóc tách toàn bộ khai báo: Function name, Receiver, Param names & types, Return types, Struct/Interface fields.
  2. Loại bỏ hoàn toàn khối ruột `{ ... }`.
  3. Sắp xếp alphabet và tính `SHA256(canonical_signatures)` $\rightarrow$ lưu vào `FrozenContractRecord.SignatureHash`.
- **Kiểm duyệt**: Sau turn của Coder, nếu `Hash_Coder != Hash_TDD` mà Coder không nộp kèm yêu cầu đàm phán hợp lệ (`CoderRenegotiating=true`, từ payload `submit_coder_outcome.status == renegotiate_signatures` mà runner buffer được) $\rightarrow$ Gate `r-signature-lock` chặn đứng ngay lập tức! Nếu có batch → bypass rule, Main Agent mediate.

---

### Trụ cột 3: Nhiệm vụ Coder — "Fill Body Only"
- Coder bước vào một môi trường đã có sẵn:
  - Khung hàm đã định nghĩa rõ ràng.
  - Test suite đã viết sẵn và đang chạy ĐỎ.
  - File test đã bị khóa `ReadOnlyPaths` (không thể sửa hay weaken).
  - Chữ ký hàm đã bị khóa bởi `r-signature-lock`.
- Nhiệm vụ duy nhất của Coder: Viết logic nghiệp vụ vào ruột các hàm `{ ... }` sao cho test từ **ĐỎ $\rightarrow$ XANH**. **Tuyệt đối KHÔNG thêm function mới, KHÔNG xóa function, KHÔNG sửa signature** — mọi nhu cầu thay đổi contract phải đi qua batch renegotiation (Trụ cột 4).

---

### Trụ cột 4: Nguyên Tắc "Accumulate & Batch" (Không Làm Lẻ Tẻ)
Khi đang code, nếu Coder phát hiện 1 signature bị thiếu tham số hoặc sai kiểu dữ liệu:
- **Tuyệt đối KHÔNG dừng lại ngay lập tức** để bắn về (tránh bẫy Ping-Pong Thrashing).
- **Quy tắc**:
  1. Ghi nhận lỗi/nhu cầu đổi signature đó vào danh sách tạm.
  2. Tiếp tục implement tối đa các phần khác của task (dù biết hiện tại test có thể chưa pass).
  3. Trong quá trình đó, nếu phát hiện thêm các lỗi signature khác thì tiếp tục gom vào.
  4. Đến **cuối turn**, đóng gói toàn bộ thành **1 BATCH REQUEST DUY NHẤT** gửi về.

---

### Trụ cột 5: Trung Gian Bắt Buộc — Main Agent (Hub-and-Spoke)
- **Tuyệt đối cấm giao tiếp ngang hàng (Peer-to-Peer)** giữa Coder và TDD.
- Mọi báo cáo của Coder đều gửi về **Main Agent (Orchestrator Hub)**:
  - **Main Agent thẩm định**: Nếu Coder lười biếng đòi xóa tham số validate $\rightarrow$ Main Agent gạt bỏ ngay, bắt Coder tự xử lý bên trong thân hàm.
  - **Nếu đề xuất hợp lý**: Main Agent chắt lọc danh sách batch, tạo chỉ thị chuẩn xác chuyển giao cho TDD cập nhật lại stub và test.
  - **Sau khi TDD sửa xong**: Main Agent kiểm tra test ĐỎ, cập nhật lại snapshot hash, rồi mới bàn giao lại cho Coder.

---

### Trụ cột 6: 100% Kết Quả Phải Là Typed Schema Result (SP-06 Tier 1)
Không bao giờ dùng regex parse văn bản tự do của AI để ra quyết định trạng thái. Mọi kết quả chuyển giao đều dùng Tool Schemas (Declared Faces):
1. **TDD Step**: Bắt buộc gọi `submit_scaffold_outcome`:
   ```yaml
   status: scaffold_ready | blocked
   stubs: [{ file, symbols: [{ name, kind, signature }] }]
   test_suite: { test_file, red_tests: [], failure_type }
   ```
2. **Coder Step**: Bắt buộc gọi `submit_coder_outcome`:
   ```yaml
   status: completed | renegotiate_signatures | blocked
   summary: "..."
   batch_signature_requests:
     - symbol: "fetchUser"
       file: "user_service.go"
       current_signature: "fun fetchUser(id: String): User"
       proposed_signature: "fun fetchUser(id: String, forceRefresh: Boolean = false): User"
       rationale: "Cần cờ bypass cache khi refresh"
   implementation_progress: "Đã xong 85% logic"
   ```

---

### Trụ cột 7: Ngân Sách An Toàn — `negotiationCap: 5` (phase-scoped, không dùng chung flow cap)

- Trong cấu hình policy của flow: `policy.negotiationCap: 5` (không dùng chung `cap: 5` của review loop — B-10). Code fallback: 5 nếu không khai.
- Nhờ cơ chế Gom Batch (Trụ cột 4), thông thường chỉ cần 1 đến 2 vòng đàm phán là giải quyết xong toàn bộ thay đổi kiến trúc.
- `negotiationCap: 5` cung cấp dư địa rộng rãi cho các task phức tạp, đồng thời là chốt chặn chống tốn token nếu hai agent rơi vào vòng lặp bất đồng ý kiến. Không extend được (escalate khi vượt).

---

## 4. Bảng So Sánh Trước và Sau Khi Có CP-67

| Tiêu chí | Trước CP-67 (Legacy) | Sau CP-67 (Contract-First TDD) |
| :--- | :--- | :--- |
| **Model ở TDD** | Model nhẹ, chỉ viết signature rỗng | **Model xịn (High-Reasoning)**, thiết kế API & Test ĐỎ |
| **Trạng thái test tại TDD** | Test rỗng $\rightarrow$ luôn XANH giả tạo | **Test đầy đủ $\rightarrow$ Compile OK nhưng chạy ĐỎ thật** |
| **Nguy cơ ở Coder** | **Confirmation Bias cực nặng** (tự code, tự viết test khen mình) | **Triệt tiêu 100% Bias** (test đã bị khóa trước khi code) |
| **Quyền sửa chữ ký** | Coder tự do đổi signature tùy tiện | **Khóa cứng qua `r-signature-lock` (AST Hash)** |
| **Xử lý khi cần đổi chữ ký** | Coder tự sửa lén trong file | **Gom BATCH cuối turn gửi Main Agent duyệt** |
| **Giao tiếp Agent** | Không rõ ràng hoặc text tự do | **100% Typed Tool Schemas qua Main Agent** |
| **Ngân sách đàm phán** | Không kiểm soát | **Khóa cứng `negotiationCap: 5` (phase-scoped, không dùng chung flow cap)** |

---

## 5. Kết Luận & Lộ Trình Triển Khai
Tài liệu kế hoạch chi tiết đã được lập tại:
`requirements/07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md`

Các slice công việc tiếp theo (post-review sync B-1/B-2, B-3/B-5, B-4/B-8, B-6/B-7/B-9/B-10, B-8.1/B-8.4, D-1..D-6):
- **P-1**: Xây dựng 2 tool faces `submit_scaffold_outcome` và `submit_coder_outcome` + đăng ký trong manifest; **wire face tới 4 provider adapter** (Claude MCP, Codex, Grok, Opencode) + expose cho child delegate node (B-1/B-2); harness uses existing `submit_review_outcome` + `SubmitFlowControl` bridge, không wire per-face constant vào adapter (B-2 resolve); 5 test signatures.
- **P-2**: Viết rule `r-signature-lock` (signature hash (chỉ signature, không bao gồm body) — B-8.1: mọi thêm/xóa/sửa symbol đều fire) + rule `r-scaffold-red` (enforce compile OK + RED test, suppress r-tests/r-reg cho scaffold turn — B-4); `ast_signatures.go` + `DocumentSymbols` vào LSP client (B-8.4); `FrozenContractRecord` + `TurnResult` + `AgentLoopState` mở rộng field; `LockScaffoldArtifacts` single-write (B-8.3); 15 test signatures.
- **P-2b** (B-11 / Task-383): Static stub-body whitelist + language adapters (Go native, React qua node, Kotlin LSP-anchored, C/C++ tree-sitter sau build tag) + caching + fail-open `Unverified`; land sau P-5; 15 test signatures. Chi tiết §6.
- **P-3**: Viết Prompts (`scaffold-contract-tdd.md`) và Agent Persona (`scaffold-architect.md`) cho Scaffold Architect (TDD); behavior `agent.scaffold` đăng ký ở agentpack (alias `behaviorAliases`, safety topology writer classification) + runner (behavior_registry, spawnable target, writer classification); model-allowlist mở cho `agent.scaffold` (B-5); bỏ `context_profiles.go` reference (file không tồn tại); 3 test signatures.
- **P-4**: Cập nhật Coder Prompts (`implement-scaffold-body.md`) với hợp đồng "Accumulate & Batch" + 3 lệnh cấm (không sửa test, không sửa signature, **không thêm/xóa function** — B-8.1); 2 test signatures.
- **P-5**: Cập nhật Topology `task-harness.yaml` và `vibe-sprint.yaml`: **giữ nguyên node id** (`test_signatures`, `tdd`, `implement`, `coder` — B-7), thêm node `synthesis_negotiation` (B-6), edges negotiation đúng, `policy.negotiationCap: 5` (B-10); `NegotiationRound`/`NegotiationCap` trong loop state; 6 test signatures; `bug-harness`/`rag-harness`/`bug-plan-harness` không đổi.
- **P-6** (mới): Doc-sync & verification & closeout: tạo `CP-67-Test-Steps.md` (toàn bộ test case + verification commands + live evidence — D-4), viết CA cho mỗi slice, sync upstream SS-14 (AC-21), SS-19 (AC-9, siết AC-2), SS-18, SD-24, SP-06 (precedence), SD-20 (§2.10 r-signature-lock, §2.11 r-scaffold-red, §7 precedence); cập nhật CP-64 doc (flag retire note). 46 test signatures tổng (B-11 — xem §6).

---

## 6. Amendment B-11: Static Stub-Body Whitelist (tín hiệu detect thứ 3)

Post-design review phát hiện lỗ hổng của `r-scaffold-red`: tín hiệu ĐỎ là **necessary-but-not-sufficient** — AI viết body logic nhưng logic sai thì test vẫn ĐỎ → pass gate ("viết body nửa vời may mắn ĐỎ"). Ngoài ra rule `len(Tests.Failed) > 0` cho phép smuggle 90% implementation + 1 test đỏ. B-11 bổ sung:

### 6.1 Ba tín hiệu detect body logic ở scaffold turn
1. **GREEN** (hiện có): test chạy XANH → body đã implement.
2. **COMPILE** (hiện có): code không compile.
3. **STATIC (mới — B-11)**: stub-body whitelist — AST walk kiểm tra thân mọi symbol chỉ chứa dạng stub chuẩn; deterministic, không phụ thuộc kết quả test. Đóng cả lỗ hổng "viết body nửa vời/sai cho test ĐỎ" lẫn nhánh smuggle-90%-green (static bắt mọi body non-stub bất kể kết quả test).

### 6.2 Ma trận adapter 4 nhóm ngôn ngữ (một lần walk → Signature + BodyShape)
| Ngôn ngữ | Nguồn AST | Chính xác |
|---|---|---|
| Go | `go/parser` (stdlib) | Exact |
| React (TS/TSX/JS/JSX) | `node` + TypeScript Compiler API (`jsx: true`) | Exact |
| Android Kotlin | LSP `documentSymbol` + `foldingRange` (anchored text) | Near-exact |
| C/C++ | tree-sitter-c/cpp (build tag `treesitter`); mặc định clangd LSP anchored | Exact syntax-level / Near-exact |

Chain fallback chung: exact parser → LSP-anchored → structured regex → `Unverified` (fail-open + evidence).

### 6.3 Stub convention mở rộng (bổ sung Trụ cột 1)
- **C**: đúng 1 statement `return` zero-sentinel (`0`, `-1`, `NULL`, `0.0`, `false`) hoặc `assert(0 && "not implemented")`.
- **C++**: `throw std::runtime_error("not implemented")` / `throw std::logic_error(...)` / `return nullptr` / `return {}`.
- **React/TS**: `throw new Error("not implemented")` / `=> TODO()`.

Nơi logic trốn (whitelist phải cover): Go (`var` initializer, `init()`); Kotlin (`init {}`, property initializer, default param, companion); TS/React (class field arrow, top-level `const`); C/C++ (macro đa-statement, global initializer, lambda, template body). Macro đa-statement trong DeclaredPaths → flag NonStub; thêm `#include` không tính là symbol change; decl ở `.h` + def ở `.c` dedupe về 1 symbol.

### 6.4 Quyết định vận hành
- **Caching** theo `(path, mtime, content hash)` — gate mỗi turn chỉ re-parse file đổi.
- **Fail-open** khi parser không khả dụng: `body_unverified` + evidence, không chặn workflow; parse xong mà non-stub → luôn violation.
- **Không cgo mặc định**: build `CGO_ENABLED=0` giữ binary tĩnh; build tag `treesitter` opt-in cho exact parser Kotlin/C/C++.
- **Task split**: P-2a giữ Task-379 (2 rules + extractor interface + Go + LSP signature extraction); Task-383 mới (P-2b) lo static stub-body whitelist + adapters React/Kotlin/C/C++ + build tag + caching, land sau P-5.
- Tổng test signatures: 28 → **46** (P-2: 15, P-2b: 15).
