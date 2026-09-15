# AI Engineering & Technical Interview Playbook: From Fundamentals to Harness Architecture

> **Dành cho**: Kỹ sư phần mềm (đặc biệt là Mobile / Systems / Backend Developers) muốn làm chủ bản chất của AI Agent, kiến trúc Harness Engineering (Senior / Lead / Staff AI Engineer).
> **Tham chiếu thực chiến**: Dự án thực tế **FlowPilot** kết hợp bài học từ **oh-my-pi (omp)** và **GoClaw**.

---

## MỤC LỤC

- [LỜI MỞ ĐẦU: SỰ DỊCH CHUYỂN TỪ APP DEVELOPER SANG AI SYSTEMS ENGINEER](#lời-mở-đầu)
- [PHẦN I: NỀN TẢNG AI & AGENTIC TỪ CON SỐ 0 (FUNDAMENTALS)](#phần-i-nền-tảng-ai--agentic-từ-con-số-0)
  - [1. Bản chất hoạt động của Large Language Models (LLMs)](#1-bản-chất-hoạt-động-của-llm)
  - [2. Trục tiến hóa: Prompt $\to$ Context $\to$ Workflow $\to$ Harness](#2-trục-tiến-hóa)
  - [3. Tại sao Markdown (.md) là "tiếng mẹ đẻ" của AI Agent?](#3-tại-sao-markdown-là-tiếng-mẹ-đẻ-của-ai-agent)
  - [4. AI Skill là gì? (Phân biệt với Prompt thô)](#4-ai-skill-là-gì)
  - [5. AI Hooks & Interceptors](#5-ai-hooks--interceptors)
  - [6. Model Context Protocol (MCP) toàn diện](#6-model-context-protocol-mcp)
- [PHẦN II: KỸ NGHỆ NÂNG CAO – HARNESS & AGENTIC ARCHITECTURE](#phần-ii-kỹ-nghệ-nâng-cao--harness-architecture)
  - [1. Pluggable Context Catalog: Toàn cảnh các nguồn ngữ cảnh](#1-pluggable-context-catalog-toàn-cảnh-các-nguồn-ngữ-cảnh)
  - [2. Kiểm soát sự bất định: The Oracle Rule, Hệ thống Flow Gates & Schema-First](#2-kiểm-soát-sự-bất-định-the-oracle-rule-hệ-thống-flow-gates--schema-first)
  - [3. Tính bền vững: State Machine, CAS & Linearization trong Go](#3-tính-bền-vững-state-machine--cas)
  - [4. Đa Agent (Multi-Agent): Hub-and-Spoke & 2-Owner Debate](#4-đa-agent-multi-agent)
  - [5. Chat Continuity SSOT: Đổi model không mất trí nhớ](#5-chat-continuity-ssot)
  - [6. Tại sao AI Hooks không thể thay thế được Harness Operating Platform?](#6-tại-sao-ai-hooks-không-thể-thay-thế-được-harness-operating-platform)
- [PHẦN III: BỘ CÂU HỎI & TRẢ LỜI (Q&A INTERVIEW PLAYBOOK)](#phần-iii-bộ-câu-hỏi--trả-lời-phỏng-vấn-thực-chiến)
  - [15 kịch bản phỏng vấn hóc búa từ cơ bản đến Staff/Principal Engineer](#15-kịch-bản-phỏng-vấn)

---

# LỜI MỞ ĐẦU

Trong làn sóng AI hiện nay, thị trường phân hóa thành 3 nhóm kỹ sư:
1. **AI User**: Dùng ChatGPT/Cursor để gõ code nhanh hơn, phụ thuộc hoàn toàn vào gợi ý.
2. **AI Wrapper Developer**: Gọi API OpenAI, dùng LangChain tạo chatbot RAG cơ bản; gặp lỗi hồi quy, tràn token hoặc vòng lặp là bất lực.
3. **AI Systems / Harness Engineer**: Hiểu bản chất bất định của LLM, xây dựng các rào chắn kỹ thuật (Harness, Flow Gates, State Machines, AST Graph) để biến LLM thành một cỗ máy sản xuất phần mềm chính xác, tin cậy và có thể kiểm toán.

---

# PHẦN I: NỀN TẢNG AI & AGENTIC TỪ CON SỐ 0

## 1. Bản chất hoạt động của LLM

- **Next-Token Prediction**: Bản chất LLM không "suy nghĩ" như con người. LLM là một mạng nơ-ron sâu (Transformer Architecture) tính toán xác suất có điều kiện:
  $$P(w_{n} \mid w_1, w_2, \dots, w_{n-1})$$
  Nó chỉ đơn thuần đoán xem từ/token tiếp theo có khả năng xuất hiện cao nhất là gì dựa trên ngữ cảnh được cung cấp.
- **Tokenization**: Từ ngữ được cắt thành các mẩu nhỏ (sub-words gọi là token). Ví dụ: 1000 từ tiếng Anh $\approx$ 1300 tokens; tiếng Việt có thể tốn nhiều token hơn do phân mảnh âm tiết.
- **Temperature & Top-p**:
  - `Temperature (0.0 -> 1.0)`: Độ ngẫu nhiên. Khi viết code, ta để `Temperature = 0.0` hoặc `0.1` để câu trả lời có tính tất định (deterministic), nhất quán cao nhất. Khi viết văn, brainstorming thì để $0.7 - 0.9$.
  - `Top-p (Nucleus Sampling)`: Giới hạn tập hợp token ứng viên dựa trên tổng xác suất tích lũy.
- **Context Window & Vấn đề "Lost in the Middle"**:
  - Dù model quảng cáo có 200k hay 1M tokens context, khả năng chú ý (Attention) giảm mạnh ở khoảng giữa của prompt. Nhồi quá nhiều code thô sẽ làm giảm độ chính xác và gây ảo giác (hallucination).

## 2. Trục tiến hóa: Prompt $\to$ Context $\to$ Workflow $\to$ Harness

```
[Prompt Engineering]    -->  Viết câu lệnh hay, ép vai, few-shot trong 1 phiên chat.
        │
        ▼
[Context Engineering]   -->  Chọn đúng file, đúng symbol, đúng lịch sử commit, cắt giảm token thừa.
        │
        ▼
[Workflow Engineering]  -->  Đóng gói tác vụ thành các bước tuần tự có input/output schema và human approval gate.
        │
        ▼
[Harness Engineering]   -->  Môi trường runtime hoàn chỉnh: State Machine, Crash Recovery, Flow Gates,
                             Test Oracle, Node Isolation, Tool Defense (FlowPilot đang làm).
```

## 3. Tại sao Markdown (.md) là "tiếng mẹ đẻ" của AI Agent?

Tại sao toàn bộ các tool agent hàng đầu (Cursor, Aider, Claude Code, FlowPilot) đều lưu tài liệu và prompt dạng `.md`?
1. **Cấu trúc phân cấp tự nhiên (Structural Hierarchy)**: `#`, `##`, `###` tương đương với cấu trúc cây dữ liệu giúp thuật toán Attention nhận diện ranh giới ngữ cảnh rõ rệt.
2. **Mật độ thông tin cao (High Token Density)**: Không tốn thẻ đóng mở rườm rà như XML/HTML hay dấu ngoặc nhọn JSON, giúp tiết kiệm từ 20-30% token context window.
3. **Phân tích cú pháp xác định (Deterministic AST Parsing)**: Trình phân tích markdown trong Go/Node có thể dễ dàng bóc tách tiêu đề, bảng, checkbox (`- [ ]`) mà không cần tốn 1 token LLM nào.

## 4. AI Skill là gì? (Phân biệt với Prompt thô)

- **Prompt thô**: Đoạn text nhập vào khung chat, dùng một lần rồi trôi đi, không có tính tái sử dụng và không ràng buộc tool.
- **AI Skill**: Một gói tri thức kỹ thuật độc lập (Modular Capability Package) gồm 3 phần:
  1. `YAML Frontmatter`: Metadata (name, description, required_tools, triggers).
  2. `Markdown Instructions`: Quy tắc nghiệp vụ, các tình huống biên (edge cases), phong cách code.
  3. `Scripts / Tools đi kèm`: Script Bash/Python mà agent được phép gọi khi kích hoạt skill.
- *Cơ chế hoạt động*: Agent chỉ nạp danh mục tiêu đề skill vào context (tiết kiệm token). Khi gặp bài toán phù hợp (trigger match), agent mới kéo toàn bộ nội dung skill vào bộ nhớ làm việc.

## 5. AI Hooks & Interceptors

Tương tự như Lifecycle Callbacks trong Android (`onCreate`, `onResume`, `onDestroy`) hay Middlewares trong Backend, Agent Runtime sử dụng **Hooks**:
- **Pre-prompt Hook**: Chạy trước khi gửi yêu cầu tới LLM. Dùng để: Đóng gói context (`BudgetPacker`), kiểm tra token limit, chèn convention dự án.
- **Post-response Hook (Flow Gate)**: Chạy ngay khi LLM vừa kết thúc lượt sinh token (turn). Dùng để: Kiểm tra Git diff, chạy unit test, kiểm tra vi phạm Definition of Done.
- **Stop / Cancel Interceptor**: Xử lý an toàn khi người dùng nhấn hủy giữa chừng, đảm bảo không để lại tiến trình zombie hay file bị ghi dở dang.

## 6. Model Context Protocol (MCP) toàn diện

- **MCP là gì?**: Giao thức chuẩn hóa mã nguồn mở do Anthropic công bố, giải quyết bài toán "kết nối AI với thế giới bên ngoài".
- **Vấn đề trước khi có MCP**: Mỗi công cụ AI phải tự viết connector riêng cho Jira, GitHub, Slack, DB (N công cụ $\times$ M dịch vụ = $N \times M$ tích hợp).
- **Kiến trúc MCP**:
  - `MCP Host`: Ứng dụng điều phối (FlowPilot, Claude Desktop, Cursor).
  - `MCP Client`: Client nằm trong Host duy trì kết nối 1-1 với các Server.
  - `MCP Server`: Chương trình nhẹ cung cấp dịch vụ chuyên biệt (Jira MCP Server, Google Drive MCP Server, GitNexus MCP Server).
- **3 Trụ cột của MCP**:
  1. `Prompts`: Mẫu câu lệnh có sẵn được server định nghĩa.
  2. `Resources`: Dữ liệu thụ động chỉ đọc (file, schema database, ticket info).
  3. `Tools`: Hàm thực thi có side-effect (tạo commit, gửi tin nhắn, gọi API).

---

# PHẦN II: KỸ NGHỆ NÂNG CAO – HARNESS & AGENTIC ARCHITECTURE

Đây là phần phản ánh chính xác kiến trúc đẳng cấp đã được hiện thực hóa trong **FlowPilot**:

## 1. Pluggable Context Catalog: Toàn cảnh các nguồn ngữ cảnh

Thay vì nhồi nhét mã nguồn thô vô tội vạ vào prompt gây tràn context và hiện tượng *"Lost in the middle"*, một hệ thống Harness chuyên nghiệp quản lý một **Danh mục nguồn ngữ cảnh cắm rút (Pluggable Context Catalog)**. Mỗi nguồn phục vụ một góc nhìn nhận thức riêng biệt:

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                    FLOWPILOT PLUGGABLE CONTEXT CATALOG                      │
├───────────────────────────────┬─────────────────────────────────────────────┤
│ 1. Project Conventions        │ Quy chuẩn code, kiến trúc, style guide      │
│ 2. Canonical Intent Head      │ Bản tuyên ngôn ý đồ nghiệp vụ & digital SHA │
│ 3. Negative Knowledge         │ Tri thức phủ định: các phương án đã bị loại │
│ 4. Change Ledger              │ Lịch sử commit tuần tự có gắn feature key   │
│ 5. Code Structure & AST       │ GitNexus call-graphs & blast-radius         │
│ 6. Locus-Anchored Excerpts    │ Đoạn code neo chính xác vào phạm vi sửa đổi │
│ 7. Sprint Handoff Artifacts   │ Truyền bối cảnh "tại sao làm thế" qua sprint│
│ 8. External MCP Context       │ Jira tickets, Figma specs, Crashlytics logs │
│ 9. Semantic Artifact Memory   │ Vector RAG qua pgvector từ các run trước    │
│ 10. Budget Packer             │ Phân bổ hạn ngạch token section chống tràn  │
└───────────────────────────────┴─────────────────────────────────────────────┘
```

### Chi tiết các nguồn ngữ cảnh cốt lõi:
1. **Conventions & Repo-as-Config**: Nạp các quy chuẩn kiến trúc (`AGENTS.md`, `CLAUDE.md`, coding conventions) như một contract section bất biến ở đầu prompt.
2. **Canonical Intent Head & Chữ ký số SHA-256**: Mỗi tính năng có một bản ghi Canonical Head chứa phát biểu ý định nghiệp vụ tối thượng kèm chữ ký `sha256(feature_key + spec_hashes + canonical_behavior)`. Đây là chiếc mỏ neo ngăn AI đi lệch spec ban đầu.
3. **Negative Knowledge (Tri thức phủ định) & Superseding Decision Records**: Khi phát triển một tính năng qua nhiều lần thử nghiệm ($A \to B \to C \to A$), hệ thống thu gọn các lần lặp lại và lưu vết rõ ràng: *Phương án B, C đã bị từ chối vì lý do gì*. AI đọc vào sẽ biết ngay **những điều tuyệt đối không được làm lại**, tránh đi vào ngõ cụt.
4. **Change Ledger (Commit-Indexed History)**: Lịch sử commit được lập chỉ mục theo thời gian và gắn thẻ `feature_key`. Nguyên tắc: Commit mới nhất là chân lý kỹ thuật hiện tại.
5. **Code Structure & AST (GitNexus)**: Đồ thị phụ thuộc, cây gọi hàm (call-graph) và luồng thực thi (execution flows) giúp tính toán chính xác phạm vi ảnh hưởng (blast radius) trước khi gõ code.
6. **Locus-Anchored Excerpts**: Chỉ trích xuất các hàm/lớp nằm trong tâm điểm thay đổi (locus) thay vì nạp cả file hàng ngàn dòng.
7. **Sprint Handoff Artifacts (`sprint_handoff.v1`)**: Cuối mỗi sprint, node audit xuất ra artifact handoff. Sprint tiếp theo nạp artifact này làm bối cảnh ưu tiên cao, giúp AI hiểu được *tại sao sprint trước lại quyết định như vậy*.
8. **External MCP Context**: Kết nối trực tiếp với Jira (User story, AC), Figma (tokens, colors), và Firebase Crashlytics (stack traces thực tế).
9. **Semantic Artifact Memory**: Tìm kiếm vector tương đồng qua `pgvector` trên Supabase đối với các tài liệu PRD, Tech Design cũ.
10. **Budget Packer**: Phân bổ trần token cố định cho từng section (ví dụ: Conventions 20%, Files 40%, Memory 30%). Nếu vượt ngưỡng, hệ thống tự động thay thế code thô bằng các bản tóm tắt cấu trúc.

## 2. Kiểm soát sự bất định: The Oracle Rule, Hệ thống Flow Gates & Schema-First

LLM vốn có tính ngẫu nhiên (non-deterministic). Để đưa LLM vào sản xuất phần mềm nghiêm ngặt, FlowPilot xây dựng một mạng lưới kiểm soát phòng thủ nhiều lớp:

### 2.1 The Oracle Rule: Chân lý Kiểm thử Tuyệt đối
- **Bản chất của vấn đề**: Khi code mới làm gãy unit test có sẵn, xu hướng tự nhiên của LLM là sửa assertions của bài test cũ để lệnh test báo exit code 0. Điều này biến một lỗi hồi quy nguy hiểm thành code đã ship.
- **Quy tắc Oracle**: Bộ test suite đã chạy xanh trước khi bắt đầu sửa code (`ensureBaseline`) được coi là **Chân lý Tối thượng (Ground Truth / Oracle)**.
- **Cơ chế Hard-Block**: Nếu một test cũ bị fail sau khi sửa code, hệ thống kích hoạt cơ chế `always-block`. AI tuyệt đối không có quyền tự ý sửa test cũ để làm code pass; mọi xung đột giữa test cũ và yêu cầu mới bắt buộc phải leo thang lên con người phê duyệt qua thẻ quyết định.

### 2.2 Bộ sưu tập 20+ Cổng Kiểm duyệt Tự động (Flow Gate Engine)
Mỗi lượt sinh code (turn) của AI đều bị Runner chặn lại để chạy qua bộ cổng kiểm soát toàn diện:

| Nhóm Kiểm Soát | Mã Gate | Ý nghĩa & Điều kiện kích hoạt | Hành động |
| :--- | :--- | :--- | :--- |
| **Kiểm thử & Hồi quy** | `r-tests` | Toàn bộ test suite phải pass hoặc có giải trình hợp lệ | `block` |
| | `r-reg` | Phát hiện lỗi hồi quy thực sự so với baseline ban đầu | `block` |
| | `r-additive-tests` | Phát hiện file test cũ bị chỉnh sửa trái phép (bảo vệ test) | `reprompt` |
| | `r-newtest` | Viết code tính năng mới nhưng không kèm file test mới | `reprompt` |
| | `r-dep` | Phát hiện xóa code đang có các module khác phụ thuộc | `block` |
| **Phạm vi & Hợp đồng** | `r-contract` | Ép AI phải tự tuyên bố Change Contract trước khi sửa code | `reprompt` |
| | `r-scope` | Bắt quả tang sửa file/symbol nằm ngoài phạm vi khai báo | `warn` / `block` |
| | `r-spec-drift` | Đặc tả nghiệp vụ của tính năng bị thay đổi so với Canonical Head | `warn` |
| | `r-code-drift` | Code thay đổi làm lệch hướng so với ý đồ ban đầu | `warn` |
| | `r-attach-spec` | Tính năng chưa có spec nay được gắn spec mới (cần duyệt re-baseline) | `approve` |
| | `r-retire` | Tính năng bị đổi tên, sáp nhập hoặc bỏ đi | `approve` |
| **Quản trị Kỹ thuật** | `r-dod-present` | Bắt buộc tài liệu Task/Bug phải có mục Definition of Done (`- [ ]`) | `reprompt` |
| | `r-dod-complete`| Đánh dấu Done nhưng còn checkbox trống và thiếu giải trình | `block` |
| | `r-ca` | Bắt buộc tạo ghi chú Change-Audit (`change-audit/CA-*.md`) | `reprompt` |
| | `r-fk` | Bắt buộc commit message phải gắn đúng `[feature-key]` | `reprompt` |
| | `r-task` / `r-bug` | Bắt buộc cập nhật tài liệu Task/Bug tương ứng khi tham chiếu | `reprompt` |
| | `r-artifact-*` | Bắt buộc sinh đủ file output và cấu trúc tiêu đề theo hợp đồng | `reprompt` |
| **Bảo vệ Đặc tả (Vibe)**| `r-requirement` | Chữ ký test signature bị lệch so với đặc tả nghiệp vụ đã khóa | `block` |

### 2.3 Schema-First 4 Tầng (T0 – T3)
FlowPilot không bao giờ parse văn bản tự do của LLM để đưa ra các quyết định điều hướng:
- **T0 (Deterministic)**: Code Go thuần, regex markdown, phân tích git diff (0-token cost).
- **T1 (Transport Schema)**: Ràng buộc bằng Tool-call JSON Schema (ví dụ: `submit_review_outcome` per-AC evidence).
- **T2 (Validation & Reprompt)**: Go runner kiểm tra, chỉ reprompt tối đa 1 lần nếu sai schema.
- **T3 (Fail-Closed)**: Đóng băng/park tác vụ, hiển thị Decision Card cho con người, không đoán mò.

### 2.4 Node Isolation: Khóa quyền Read-Only ở tầng Hạ tầng (Silent-Deny)
Dặn dò trong prompt ("Hãy chỉ đọc code, đừng sửa file nhé") là không đáng tin cậy. FlowPilot áp dụng **Node Isolation**:
- Các agent đóng vai trò thẩm định như **Reviewer**, **Scout**, **Owner** bị thu hồi quyền ghi ở cấp tầng mạng/process.
- Mọi tool call cố tình ghi file hay chạy lệnh nguy hiểm đều bị runner chặn đứng (`silent-deny`) trước khi chạm vào ổ cứng.

## 3. Tính bền vững: State Machine, CAS & Linearization trong Go

Để bảo đảm hệ thống không bao giờ bị deadlock hoặc duplicate turn khi mất mạng/crash:
- **8-State Forward-Only State Machine**:
  $$\text{prepared} \to \text{send\_claimed} \to \text{send\_started} \to \text{provider\_accepted} \to \text{terminal\_*}$$
- **CAS (Compare-And-Swap) trên Revision**: Mọi thay đổi trạng thái đều phải kiểm tra phiên bản dữ liệu trước khi ghi, chống race condition giữa nhiều goroutines.
- **Stop Linearization Point**:
  - Nếu user nhấn `STOP` **trước** CAS `send_started` $\implies$ Hủy ngay, không tốn 1 byte gửi lên LLM (`stopped_before_send`).
  - Nếu user nhấn `STOP` **sau** CAS `send_started` $\implies$ Lệnh gửi đã bay đi, kích hoạt cờ hủy provider CLI (`cancelled_in_flight`).

## 4. Đa Agent (Multi-Agent): Hub-and-Spoke & 2-Owner Debate

- **Tại sao dùng Hub-and-Spoke thay vì Peer-to-Peer?**
  - Peer-to-Peer (các agent tự do chat với nhau) dễ dẫn đến bão tin nhắn, vòng lặp vô tận và mất khả năng kiểm soát.
  - Hub-and-Spoke: **Main Hub Agent** là điều phối viên duy nhất. Sub-agents chỉ nhận nhiệm vụ từ Hub, trả về kết quả cấu trúc và tắt tiến trình.
- **Vibe Mode 2-Owner Debate**:
  - Thay vì để 1 LLM tự đánh giá (self-reflection thường thiên vị), Vibe mode sinh ra 2 Agent Owner độc lập (`Owner_1` và `Owner_2`) phản biện chéo nhau dựa trên tài liệu System Specs đã khóa (`SS-Lock`). Chỉ khi 2 Owner đồng thuận, code mới được đi tiếp.

## 5. Chat Continuity SSOT

- Phân tách độc lập giữa:
  - `chatId`: Phiên hội thoại dài hạn của người dùng (User Experience).
  - `runId` / `leg`: Từng chặng thực thi trên một AI Provider cụ thể.
- Khi người dùng đổi model giữa chừng (ví dụ: đang dùng Claude Haiku chuyển sang Codex GPT-5): Runner thực hiện **Linearized Switch 3 pha**: Đóng leg cũ $\to$ Đóng gói Handoff Envelope $\to$ Khởi tạo leg mới và tiếp tục cuộc trò chuyện mượt mà.

## 6. Tại sao AI Hooks không thể thay thế được Harness Operating Platform?

Nhiều lập trình viên lầm tưởng: *"Claude Code hay Cursor đã có Hooks (Pre-prompt, Post-tool, Stop hooks), vậy cần gì một Harness Platform như FlowPilot?"*.
Thực tế, **Hook chỉ là 'Chân phanh / Cảm biến va chạm'**, còn FlowPilot là **'Chiếc xe tự lái hoàn chỉnh'**:

| Tiêu chí so sánh | AI Hooks (Claude / Cursor / Git Hooks) | Harness Operating Platform (FlowPilot) |
| :--- | :--- | :--- |
| **Bản chất trạng thái** | **Stateless & Ephemeral**: Hook là script chạy thoáng qua, thoát tiến trình là mất sạch bộ nhớ, không biết hôm qua đã làm gì. | **Stateful & Durable State Machine**: Máy trạng thái 8 bước lưu trên DB với CAS; đứt mạng hay crash tự chuyển sang `uncertain`/`repair_required` an toàn. |
| **Phạm vi điều phối** | **Single Agent**: Chỉ bám vào vòng đời của 1 phiên AI duy nhất; không thể quản lý đồ thị nhiều agent. | **Multi-Agent Topology**: Điều phối Hub-and-Spoke, Coder $\leftrightarrow$ Reviewer Loop, và mô hình tranh biện 2-Owner Debate. |
| **Quyền hạn an toàn** | Dựa vào prompt dặn dò; hook chỉ cảnh báo sau khi file đã bị ghi hoặc câu lệnh đã chạy. | **Node Isolation (Silent-Deny)**: Tước quyền ghi đĩa ở tầng hạ tầng mạng/process cho Reviewer/Owner; không thể phá hoại dù prompt bảo làm. |
| **Ngữ cảnh (Context)**| Chỉ `echo` thêm text vào prompt một cách thô sơ, dễ gây tràn token window và "Lost in the middle". | **Pluggable Context Catalog & Budget Packer**: Quản lý 10 nguồn ngữ cảnh, lưu vết Negative Knowledge, và phân bổ hạn ngạch token chặt chẽ. |
| **Cổng duyệt (Gates)** | Hộp thoại terminal tạm bợ (`y/n`), tắt máy là mất, không thể làm việc nhóm. | **Enterprise Approval Gate**: Lưu trong DB (`WAITING_USER_APPROVAL`), PM/Lead duyệt bất đồng bộ từ xa qua Web Dashboard. |
| **Phạm vi nền tảng** | **Vendor-locked**: Hook của Claude chỉ chạy trên Claude; đổi máy tính là mất sạch context. | **Cross-Device & Cross-Provider**: Đổi máy tính chỉ cần bind lại path; đổi model (Claude $\to$ GPT $\to$ Grok) bằng Chat-SSOT mượt mà. |

> **Ẩn dụ kinh điển**: Có một chiếc phanh xịn (Hook) không có nghĩa là bạn đã có một chiếc xe tự lái (Harness Platform). FlowPilot có sử dụng Hook bên trong (`gate_hook.go`), nhưng Hook chỉ là một cảm biến cấp thấp dưới sự chỉ huy của toàn bộ hệ thống Harness!

---

# PHẦN III: BỘ CÂU HỎI & TRẢ LỜI PHỎNG VẤN THỰC CHIẾN (Q&A PLAYBOOK)

Dưới đây là 15 câu hỏi thực chiến, kèm câu trả lời mẫu theo phong cách **Senior / Lead Engineer**:

---

### Câu 1: "Sự khác biệt cốt lõi giữa Prompt Engineering, Context Engineering và Harness Engineering là gì?"
- **Trả lời mẫu**:
  > *"Prompt Engineering chỉ là tầng bề mặt: viết câu lệnh thế nào cho model hiểu trong một lần gọi đơn lẻ. 
  > Context Engineering giải quyết bài toán cung cấp đúng và đủ tri thức: chọn lọc chính xác symbol nào, tài liệu nào, lịch sử nào vào context window mà không gây tràn token hay nhiễu.
  > Còn Harness Engineering là tầng hạ tầng bao bọc: quản lý vòng đời của Agent, kiểm soát trạng thái bằng State Machine, thiết lập các Flow Gate chặn đứng lỗi hồi quy, cô lập quyền hạn (Read-Only) và đảm bảo tính bền vững (Crash-recovery). Tôi xây dựng FlowPilot chính là để dịch chuyển trọng tâm từ viết prompt sang Harness Engineering."*

---

### Câu 2: "Khi context của một dự án phần mềm lên đến hàng triệu token, bạn giải quyết bài toán Context Window và chi phí thế nào?"
- **Trả lời mẫu**:
  > *"Tôi áp dụng 3 kỹ thuật cốt lõi trong FlowPilot:
  > Thứ nhất là **Pluggable Context Catalog (10 Nguồn Ngữ Cảnh Chuyên Biệt)**: Không nhồi mã nguồn thô vô tội vạ. Tùy theo giai đoạn, hệ thống chỉ nạp đúng nguồn cần thiết: Code Graph (GitNexus/AST) cho cấu trúc gọi hàm và blast radius, Locus-anchored Excerpts cho đoạn code cần sửa, Commit Ledger cho lịch sử thay đổi, và Negative Knowledge để tránh vết xe đổ đã bị loại bỏ.
  > Thứ hai là **Budget Packer**: Chia nhỏ context window thành các hạn ngạch token cố định cho từng phân vùng (ví dụ: Conventions 20%, Files sửa đổi 40%, Memory Summary 30%). Nếu vượt ngưỡng trần, hệ thống tự động co gọn hoặc thay thế mã nguồn thô bằng các bản tóm tắt cấu trúc.
  > Thứ ba là **Progressive Disclosure**: Khởi đầu bằng danh mục con trỏ (pointer-only); chỉ khi model thật sự cần đi sâu vào một module cụ thể thì mới kích hoạt tool để tải chi tiết."*

---

### Câu 3: "Điểm yếu lớn nhất của AI Coding Assistant hiện nay là gì và bạn giải quyết nó bằng cách nào?"
- **Trả lời mẫu**:
  > *"Điểm yếu lớn nhất không phải là sinh code dở, mà là **Silent Regression (Hồi quy âm thầm)** và **Tampering with Tests (Tự ý sửa test)**. Khi gặp bug, LLM thường có xu hướng sửa assertions của bài test cũ để lệnh test báo xanh, biến một lỗi hồi quy nghiêm trọng thành tính năng đã ship.
  > Trong FlowPilot, tôi thiết lập **The Oracle Rule (`SP-06`/`SS-14`)**: Trước khi agent sửa code, runner chụp baseline test suite. Nếu test cũ bị gãy sau khi chạy code, Flow Gate kích hoạt cơ chế `always-block`. Agent tuyệt đối không có quyền sửa test cũ trừ khi có sự phê duyệt tường minh từ con người qua thẻ quyết định."*

---

### Câu 4: "MCP (Model Context Protocol) giải quyết vấn đề gì khác biệt so với Function Calling truyền thống?"
- **Trả lời mẫu**:
  > *"Rất nhiều người nhầm lẫn MCP chỉ là một cách gọi khác của Function Calling, nhưng thực chất đây là bước chuyển dịch mang tính kiến trúc từ **Point-to-Point Coupling** sang **Open Standard Protocol**:
  > 
  > 1. **Vấn đề của Function Calling truyền thống**:
  >    - **Phân mảnh $N \times M$**: Function Calling gắn chặt vào từng SDK của nhà cung cấp (OpenAI có schema riêng, Anthropic có format riêng). Nếu có 3 model (Claude, GPT, Grok) và 5 nguồn dữ liệu (Google Drive, Jira, GitHub, Postgres, Slack), bạn phải tự viết và bảo trì $3 \times 5 = 15$ bộ adapter.
  >    - **In-process & Rủi ro bảo mật**: Function calling chạy trực tiếp trong tiến trình (in-process) của host application. Client phải cầm trực tiếp OAuth token, API keys của bên thứ 3 và tự lo vòng đời kết nối.
  >    - **Hẹp phạm vi**: Chỉ hỗ trợ *Tools* (hàm thực thi), không có chuẩn chung cho *Resources* (đọc dữ liệu thụ động qua URI) hay *Prompts* (mẫu câu lệnh tái sử dụng).
  > 
  > 2. **Sự vượt trội của MCP (Model Context Protocol)**:
  >    - **Chuẩn hóa $N + M$ qua JSON-RPC 2.0**: Tách biệt hoàn toàn vai trò MCP Host (Client) và MCP Server. Viết một MCP Server duy nhất, nó có thể cắm ngay vào Claude Desktop, Cursor, FlowPilot Runner hay bất kỳ AI client nào hỗ trợ MCP.
  >    - **Out-of-Process Isolation**: MCP Server chạy như một tiến trình độc lập (child process qua `stdio` hoặc remote qua `SSE`). AI host không cần biết đến thông tin xác thực nội bộ của server, lỗi crash ở server không làm sập ứng dụng chính.
  >    - **3 Trụ cột tiêu chuẩn**: Chuẩn hóa cả `Tools`, `Resources` (ví dụ `gdrive://folder/file_id`), và `Prompts`.
  > 
  > 3. **Ví dụ thực chiến: Tôi đã tự viết Google Drive Proxy MCP Server trong FlowPilot (`google_drive_proxy_mcp.go`)**:
  >    Trong FlowPilot, thay vì hardcode Function Calling thô, tôi tự tay lập trình một MCP Server hoàn chỉnh bằng Go theo chuẩn JSON-RPC 2.0 giao tiếp qua `stdin`/`stdout`. Kiến trúc này mang lại 3 giá trị đột phá mà Function Calling không thể làm được:
  >    - **Dynamic Capability Negotiation (Phân quyền động theo Mode)**: Server hỗ trợ chế độ `read_only` và `read_write`. Khi một Agent chỉ làm nhiệm vụ nghiên cứu (như Scout hay Reviewer), server chạy ở chế độ `read_only` và trong phản hồi `tools/list` chỉ công bố các công cụ đọc (`search`, `listFolder`, `readGoogleDoc`, `authGetStatus`). Các tool ghi (`createGoogleDoc`, `updateGoogleDoc`, `createFolder`) bị triệt tiêu hoàn toàn ngay từ tầng schema, LLM không thể bị hallucinate để gọi lệnh ghi bừa bãi.
  >    - **Approval Gate Interceptor (Cổng phê duyệt an toàn)**: Function Calling thông thường khi model gọi hàm là máy tính thực thi ngay lập tức. Nhưng với Google Drive MCP của tôi, khi agent gọi `createGoogleDoc` hoặc `updateGoogleDoc` (nếu không bật `yoloMode`), MCP Server sẽ **chặn lại (intercept)**, tạo một yêu cầu phê duyệt (`pending`) trên cơ sở dữ liệu và trả về lỗi chuẩn MCP: `MCP_WRITE_APPROVAL_REQUIRED: FlowPilot created approval request <ID>. Wait for user approval before retrying this exact Google Drive tool call.` Khi Tech Lead bấm Approve trên Dashboard/TUI, server mới thực thi ghi file lên Drive và trả về `WebViewLink`. Nếu Reject, tài liệu người dùng trên Cloud được bảo vệ nguyên vẹn.
  >    - **Cross-Provider Decoupling**: Nhờ chuẩn MCP qua stdio, khi tôi chuyển đổi model của FlowPilot từ Claude 3.7 Sonnet sang OpenAI Codex hay Grok 3, toàn bộ tích hợp Google Drive chạy ngay lập tức mà không phải sửa một dòng code adapter nào."*

---

### Câu 5: "Làm thế nào để xây dựng một quy trình Multi-Agent không bị rơi vào vòng lặp vô hạn (Infinite Loops)?"
- **Trả lời mẫu**:
  > *"Vòng lặp vô hạn xảy ra khi tiêu chí nghiệm thu không đóng (open-ended criteria). Trong FlowPilot, tôi áp dụng nguyên tắc **Kill-Review Closed-Claim Contract (`SP-05`)**:
  > 1. Đóng băng Claim: Mỗi vòng review chỉ chứng minh một khẳng định duy nhất có thể xác thực nhị phân (Pass/Fail).
  > 2. Đặt giới hạn cứng (Safety Cap): Ví dụ tối đa 3 vòng lặp giữa Coder và Reviewer.
  > 3. Schema-First Verdict: Reviewer bắt buộc trả về kết quả theo từng tiêu chí nghiệm thu (per-AC) kèm dẫn chứng `file:line`. Nếu sau số vòng quy định vẫn còn điểm fail, hệ thống dừng lại và chuyển trạng thái `KILL_WITH_FINDINGS` để con người xử lý, tuyệt đối không để 2 agent tự tranh luận vô tận."*

---

### Câu 6: "Tại sao FlowPilot lại chọn mô hình Hub-and-Spoke thay vì để các Agent giao tiếp dạng Mesh (Peer-to-Peer)?"
- **Trả lời mẫu**:
  > *"Mô hình Mesh (Peer-to-Peer) trông rất thú vị trên lý thuyết nhưng là ác mộng trong thực tế production vì: Bão tin nhắn (Message explosion), khó audit vết quyết định, và hiện tượng trôi dạt ngữ cảnh (Context drift) khi Agent A nói nhỏ với Agent B.
  > Hub-and-Spoke chọn **Main Hub làm Nhạc trưởng duy nhất**: Coder chỉ biết việc code, Reviewer chỉ biết review. Mọi trao đổi đều phải quy tụ về Hub để tổng hợp và đánh giá trước khi phân phối tiếp. Điều này giúp hệ thống có thể quan sát (Observable), kiểm toán (Auditable) và dễ dàng can thiệp bằng Human Approval Gates."*

---

### Câu 7: "Làm thế nào để xử lý race condition khi người dùng bấm nút STOP đúng lúc LLM đang stream token hoặc chuẩn bị ghi file?"
- **Trả lời mẫu**:
  > *"Đây là bài toán Concurrency kinh điển trong Systems Engineering. Trong Go Runner của FlowPilot, tôi xây dựng **State Machine 8 bước với Compare-And-Swap (CAS)** trên biến `Revision` (`SD-24`).
  > Tôi xác định một **Linearization Point (Điểm tuyến tính hóa)** duy nhất là lúc chuyển trạng thái từ `send_claimed` sang `send_started`.
  > Nếu lệnh `STOP` của user commit CAS trước điểm này, lệnh gửi bị chặn đứng hoàn toàn, không có byte nào được ghi ra đĩa. Nếu lệnh `STOP` đến sau, runner ghi nhận `StopOutcome = cancelled_in_flight` và truyền tín hiệu context cancellation để ngắt tiến trình con của CLI."*

---

### Câu 8: "Thế nào là Schema-First trong Agentic Design và tại sao nó quan trọng hơn việc dặn dò trong prompt?"
- **Trả lời mẫu**:
  > *"Dặn dò trong prompt ('Hãy trả lời theo định dạng JSON sau...') chỉ là khuyến nghị ở tầng ngữ nghĩa; LLM vẫn có thể sinh thừa lời xin lỗi hoặc sai định dạng khi gặp bài toán khó.
  > **Schema-First (`CP-62`)** là đưa việc kiểm soát xuống **tầng giao vận (Transport Layer)**:
  > Chúng tôi định nghĩa JSON Schema cho các Tool Call. Nếu LLM trả về sai schema, tầng mạng từ chối ngay. Runner chỉ cho phép tối đa 1 lần reprompt sửa lỗi. Hơn nữa, với các node cần an toàn như Reviewer, chúng tôi áp dụng **Node Isolation (Silent-Deny)**: Chặn hẳn tool ghi file ở tầng Go runner, biến quyền Read-Only thành luật bất biến của hạ tầng chứ không dựa vào sự tự giác của prompt."*

---

### Câu 9: "Là một Android Developer, bạn đã mang những tư duy gì từ Mobile sang việc xây dựng AI Harness Platform này?"
- **Trả lời mẫu**:
  > *"Mobile Development dạy cho tôi tư duy sống còn về:
  > 1. **Resource Constraints & Lifecycle**: Trong Android, bộ nhớ và pin rất hạn chế, Activity có thể bị hủy bất cứ lúc nào. Khi làm AI, Context Window cũng hạn chế hệt như RAM; tôi áp dụng kỹ thuật quản lý bộ nhớ đệm và lifecycle để nạp/hủy context mượt mà.
  > 2. **Offline-First & Local Persistence**: Mạng có thể đứt bất thình lình. Go Runner của FlowPilot luôn lưu trạng thái cục bộ vào file/database trước khi thực thi lệnh bên ngoài.
  > 3. **Unidirectional Data Flow (UDF) & Concurrency**: Quen thuộc với StateFlow/Coroutines giúp tôi dễ dàng thiết kế state machine và xử lý đa luồng an toàn cho Agent."*

---

### Câu 10: "Làm sao ngăn chặn Prompt Injection và đảm bảo an toàn khi cho Agent chạy lệnh Terminal (Bash)?"
- **Trả lời mẫu**:
  > *"Chúng tôi áp dụng nguyên tắc **Phòng thủ theo chiều sâu (Defense-in-depth)**:
  > Thứ nhất: Phân quyền theo vai trò (Role-based Tool Permission). Các agent Reviewer/Owner hoàn toàn bị tước quyền gọi các lệnh sửa đổi file hoặc chạy shell nguy hiểm.
  > Thứ hai: Bộ phân loại lệnh (Command Classifier) kiểm tra danh sách đen các lệnh phá hoại (`rm -rf`, format disk, sửa git remote).
  > Thứ ba: Human-in-the-loop Gate (`SP-01`). Mọi tác vụ có rủi ro cao (như commit code, push git, release) đều phải dừng lại ở Approval Gate để người dùng xác nhận trực tiếp."*

---

### Câu 11: "Sự khác biệt giữa FlowPilot và các framework như LangChain hay CrewAI là gì?"
- **Trả lời mẫu**:
  > *"LangChain hay CrewAI là các thư viện trừu tượng hóa (Libraries/Frameworks) ở tầng ứng dụng, chủ yếu phục vụ viết code Python/TypeScript nhanh cho các tác vụ chat/rag thông thường.
  > FlowPilot là một **Harness Operating Platform**. Nó giải quyết các bài toán kỹ thuật hạ tầng mà các framework trên thường bỏ qua: Quản lý tiến trình CLI cục bộ trên máy dev, tích hợp sâu vào Git/AST, lưu trữ trạng thái bền vững trên PostgreSQL, kiểm soát quy trình kiểm thử (Oracle Rule) và giao diện trực quan cho cả Developer (Dev mode) lẫn người dùng phi kỹ thuật (Vibe mode)."*

---

### Câu 12: "Chat-SSOT trong FlowPilot giải quyết bài toán gì khi người dùng muốn đổi model giữa chừng? Việc tóm tắt Handoff này do AI làm hay Runner làm?"
- **Trả lời mẫu**:
  > *"Trong hầu hết các công cụ AI hiện nay, mỗi phiên chat bị gắn chặt (tightly coupled) với một process CLI và một provider cụ thể. Nếu bạn đang trao đổi dở dang với Claude 3.7 mà muốn đổi sang OpenAI Codex hay Grok, bạn buộc phải mở cửa sổ mới và mất sạch toàn bộ ngữ cảnh trước đó.
  > 
  > FlowPilot giải quyết bài toán này bằng cơ chế **Chat Continuity SSOT (`SD-26`)**:
  > Chúng tôi phân tách độc lập giữa **`chatId`** (phiên trải nghiệm hội thoại logic duy nhất của người dùng) và **`runId` / `leg`** (từng chặng thực thi vật lý trên một model cụ thể). Khi người dùng đổi model giữa chừng, hệ thống thực hiện quy trình **Switch Linearization 3 pha**: Đóng leg cũ $\to$ Đóng gói Handoff Envelope $\to$ Mở leg mới trên provider mới và tiếp tục cuộc trò chuyện mượt mà.
  > 
  > **Về việc tóm tắt Handoff: Do AI làm hay Go Runner làm?**
  > Đây là câu hỏi rất hay về mặt kiến trúc. Trong FlowPilot, chúng tôi **không phó mặc hoàn toàn cho AI cũng không làm cứng bằng tay**, mà áp dụng mô hình **Phối hợp phân tầng (Hybrid Tiered Architecture)** giữa Go Runner và AI Model:
  > 
  > 1. **Go Runner đảm nhận việc Điều phối Cấu trúc & Giới hạn Dung lượng (Deterministic Layer)**:
  >    - Runner trích xuất lịch sử các lượt chat sạch từ đĩa (`transcriptTurnsFromRun`), lọc bỏ các tin nhắn rác hoặc gate reprompt.
  >    - Runner áp trần dung lượng cứng 64KB (`handoffMaxBytes = 64KB`). Runner tự tính toán cắt tỉa UTF-8 (`packConversationTurns`), chèn các chỉ dấu ranh giới (`[Earlier conversation omitted...]` hoặc `[turn truncated...]`).
  >    - Runner đóng gói phong bì **Handoff Envelope** với chữ ký chuẩn `[FlowPilot cross-provider chat handoff]` và thông tin `Source provider`, `Source run`.
  > 
  > 2. **AI Model (Tier Siêu Nhẹ) đảm nhận việc Tóm tắt Ngữ nghĩa (Semantic Summarization)**:
  >    - **Background Rolling Summary**: Khi cuộc trò chuyện rơi vào trạng thái rảnh rỗi (idle timer 5 phút sau lượt chat cuối) hoặc khi người dùng bấm nút *'Gen summary'*, Runner kích hoạt tiến trình nền gọi một **model AI giá rẻ/siêu nhẹ** của chính provider đó (như Claude Haiku hoặc GPT mini) để đọc các turn thuộc về `feature_key` và cô đọng thành các bullet points súc tích ($\le 900$ bytes) lưu vào `changeledger.ChatSummaryLedger`.
  >    - **Deterministic Fallback trong Go Runner (`heuristicSummarizeTurns`)**: Nếu model tóm tắt bị lỗi mạng, hết quota hoặc không có kết nối, **chính Go Runner** sẽ tự động chạy thuật toán heuristic nội bộ (trích xuất Goal từ câu prompt đầu tiên của user + gom các câu chốt) để sinh summary ngoại tuyến (offline-safe), không bao giờ để gãy tiến trình.
  > 
  > 3. **Cơ chế Thang chuyển tiếp Handoff 3 nấc (Handoff Ladder)** khi đổi model:
  >    - **Nấc `hybrid`**: Nếu đã có bản tóm tắt cache từ trước $\implies$ Runner nhét bản tóm tắt này vào block `<conversation_summary>`, kết hợp với các turn chat gần nhất trong `<previous_conversation>`.
  >    - **Nấc `target_summary`**: Nếu chưa có bản tóm tắt sẵn và lịch sử cũ bị cắt bớt do vượt quá trần 64KB $\implies$ Runner gửi chỉ thị cho **AI Model mới (Target Model)**: *'No cached FlowPilot summary was available... First summarize the previous conversation for yourself, then continue from the latest user intent'* (lúc này chính model mới sẽ tự đọc và tóm tắt lại cho nó trước khi trả lời).
  >    - **Nấc `raw`**: Nếu toàn bộ cuộc trò chuyện ngắn gọn vừa vặn trong trần 64KB $\implies$ Runner giữ nguyên toàn bộ lịch sử nguyên bản mà không cần tóm tắt."*

---

### Câu 13: "Vibe Mode trong FlowPilot hoạt động ra sao? 2-Owner Debate có gì vượt trội hơn Single-agent Self-reflection?"
- **Trả lời mẫu**:
  > *"Vibe Mode dành cho người dùng muốn tự động hóa từ yêu cầu thô đến code hoàn chỉnh mà không cần can thiệp từng bước.
  > Điểm mấu chốt là **Cổng khóa SS-Lock**: Người dùng phải duyệt khóa đặc tả trước khi code.
  > Sau đó, để thay thế con người duyệt các gate trung gian, chúng tôi cho **2 Agent Owner độc lập tranh luận đối kháng**. Single-agent thường mắc lỗi 'tự đồng tình' (confirmation bias) – tự mình sinh ra lỗi rồi tự mình khen đúng. Hai Owner với prompt độc lập sẽ soi xét các kẽ hở của nhau, và chỉ khi đạt được đồng thuận có cấu trúc thì quy trình mới được phép đi tiếp."*

---

### Câu 14: "Tại sao FlowPilot lại tách biệt giữa Business Workflow (Supabase) và Local AI Session (Go Runner)?"
- **Trả lời mẫu**:
  > *"Đây là bài toán cân bằng giữa **Cộng tác đám mây (Cloud Collaboration)** và **Môi trường thực thi cục bộ (Local Execution)**:
  > Metadata của project, định nghĩa workflow, lịch sử chạy và tài liệu artifact cần lưu trên Cloud (Supabase) để cả team PM, Lead, Dev có thể xem bất đồng bộ từ bất kỳ đâu.
  > Nhưng mã nguồn, compiler, Git repo và môi trường test lại nằm trên máy cá nhân của Developer. Go Runner đóng vai trò là cầu nối: kéo định nghĩa tác vụ từ Cloud về, tương tác với source code thật tại chỗ, chạy test thật, rồi đồng bộ kết quả ngược lại Cloud."*

---
