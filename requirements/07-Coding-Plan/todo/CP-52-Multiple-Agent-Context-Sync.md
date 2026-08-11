# CP-52: Multiple-Agent Context Synchronization Across Parallel Worktrees

## Metadata

- Document ID: `CP-52`
- Title: `Multiple-Agent Context Synchronization Across Parallel Worktrees`
- Feature Keys: `agent-flow-engine, change-contract`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-22`
- Last Updated: `2026-07-22`
- Parent Documents: [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `TBD (chưa tạo Task — xem §6 kế hoạch chia phase)`
- Related Documents: [CP-43: Change Contract And Canonical Intent Signature](../inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [CP-44: Pluggable Context Source Registry](../inprogress/CP-44-Pluggable-Context-Source-Registry.md), [CP-54: Locus-Anchored Relevance Retrieval](./CP-54-Locus-Anchored-Context-Relevance.md) (**prerequisite của P-3** — scorer/locus + chuẩn hóa target dùng chung; P-3 gọi lại, không xây trùng; thứ tự inter-CP: CP-43 → CP-54 → CP-52), [CP-35: Context And Regression Engine Rollout](../done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-51: Durable Turn Dispatch State Machine](../done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [Task-259: source.dependence Context Source](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md)
- Replaces: `None`
- Tags: `agent-flow-engine, change-contract, canonical-head, parallel-agents, worktree, context-sync, shared-reality, semantic-conflict, drift, gitnexus-impact`

## AI Quick View

### Tóm tắt

- Ghi lại một vấn đề thiết kế được nêu trong một bài viết bên ngoài ("The Problem with Parallel Agents"), map nó vào kiến trúc thật của FlowPilot, rồi đề xuất một hướng cải tiến có phạm vi rõ ràng.
- **Vấn đề của bài viết**: khi nhiều AI agent chạy song song trên cùng một codebase, *biệt lập vật lý* (worktree/branch/container) giải quyết được xung đột mã nguồn, nhưng *biệt lập khái niệm* thì không — mỗi agent giữ một snapshot + giả định đã cũ, nên khi tích hợp sẽ vỡ qua **xung đột ngữ nghĩa / hành vi** (merge sạch, type-check pass, unit test pass, nhưng hành vi tích hợp sai). Con người trở thành "message bus" thủ công. Bài viết đóng khung lời giải thành ba tầng **Shared Reality**: (L1) code state, (L2) context state, (L3) intent state.
- **FlowPilot hôm nay**: *bên trong một flow*, nó né được phiên bản khó của vấn đề — chỉ đúng một writer (coder), stage song song duy nhất là **cohort review read-only**, mọi child gập về một hub. Nhưng *ở cấp composition* — người dùng tạo 2 git worktree → add mỗi cái thành **một target project riêng** → chạy flow song song — thì FlowPilot rơi vào đúng chế độ bài viết mô tả, và **không lớp context-sync nào bắc qua được 2 worktree-project** (mọi lớp đều key theo working dir hoặc `project_id`). Đã verify ở cấp code (§4).
- **Cơ hội**: FlowPilot đã sở hữu sẵn đúng những primitive mà một Shared Reality xuyên-agent cần — **Canonical Head** theo từng feature (intent, L3), **drift detection** (staleness, L2), **GitNexus impact** (xếp hạng dependency, L2), **change ledger** (nhật ký verification). Khóa join bắc qua các worktree — `feature_key` — cũng đã tồn tại (head lưu dưới dạng `canonical/<feature_key>.json`). Nước đi còn thiếu là **federate** (liên hợp) Canonical Head + drift signal theo `feature_key` thay vì theo `project_id`.

### Yêu cầu hiện tại

- Duyệt tài liệu này ở trạng thái **draft**, ghi lại (1) vấn đề, (2) tình trạng hiện tại đã verify của FlowPilot, (3) hướng giải pháp đề xuất. Quyết định có nâng lên `approved` và cắt Task con hay không, và có cần một SD riêng hay không (xem `Q-1`).
- CP này **không** đề xuất cho FlowPilot tự động merge code phân kỳ giữa các nhánh song song. Đó là non-goal tường minh (§8).

### Quyết định chính

- `P-1` **Federate theo `feature_key`, không theo `project_id`.** Một nhóm các target project liên quan (ví dụ hai worktree của cùng một repo) dùng chung một **context-engine scope** key theo repo/feature, nhờ đó một Canonical Head do worktree A ghi cho feature X sẽ đọc được bởi worktree B (cũng đụng X). Khóa join đã có sẵn (`canonical/<feature_key>.json`).
- `P-2` **Đồng bộ intent + signal, không đồng bộ code.** L1 (code state) vẫn do git/CI/con người sở hữu. FlowPilot chỉ push/pull L2 (drift + impact signal) và L3 (Canonical Head intent). Không auto rebase/merge xuyên nhánh — đúng rủi ro bài viết cảnh báo và bị từ chối ở đây.
- `P-3` **Xếp hạng trước khi thông báo.** Thông báo xuyen-worktree được lọc theo giao của `feature_key` và **blast-radius từ GitNexus impact** (xây trên Task-259), nên một worktree chỉ được báo về thay đổi của worktree khác khi scope khai báo của nó thật sự phụ thuộc. Đây chính là cơ chế chống "shared noise" mà bài viết yêu cầu.
- `P-4` **Có chặn, non-fatal, tất định.** Drift signal không bao giờ tự kích hoạt một chuỗi re-edit (rủi ro "update loop" của bài viết); mọi tính toán xuyên-project là best-effort và không bao giờ chặn một turn (giống `CP-43 AC-9`).
- `P-5` **Tái sử dụng, không xây lại.** Đặt code trong các module có sẵn `internal/{contextsync,changecontract,structure,changeledger}`; không thêm store engine mới, không vector DB (nhất quán với `SD-17 D-4`).

### Ràng buộc

- Không được đổi topology *bên trong flow* của FlowPilot (single writer + cohort review read-only) — thuộc tính an toàn đó giữ nguyên.
- Local-first: hai worktree trên cùng một máy nên đồng bộ qua một context-engine scope trên filesystem chung trước khi tính tới đường Drive-mediated cross-machine.
- Mở rộng, không phá vỡ, các module `CP-35`/`CP-43` và gate hook sau `finishTurn`.
- Dependency cứng cho phase xếp hạng impact (`P-3`): Task-259 (`source.dependence`) hiện đang `draft` / chưa sẵn sàng (xem `Q-5`).

### Câu hỏi mở

- `Q-1` Hợp đồng Shared Reality xuyen-worktree thuộc về một **SD mới** (như `CP-51` → SD-24), hay là phần mở rộng của `SD-21`/`SD-17`? Nghiêng về: một SD mới ngắn, vì nó giới thiệu khái niệm federation/grouping đứng trên `project_id`.
- `Q-2` **Đơn vị grouping** là gì? Các phương án: (a) tự động gom target project theo git remote URL + heuristic "cùng repo"; (b) người dùng khai báo tường minh một "workspace" liệt kê các project thành viên; (c) chỉ gom theo catalog `feature_key` chung. Nghiêng về: (b) tường minh, để tránh cross-talk bất ngờ.
- `Q-3` **Cùng máy vs khác máy**: bắt đầu với context-engine dir chung trên filesystem cục bộ; hoãn federation qua Drive (folder `context-engine/` hiện đang key theo project — §4).
- `Q-4` **Rebaseline đồng thời** cùng một head `feature_key` bởi hai worktree: last-write-wins, merge, hay flag cho người quyết? Cần một luật trước khi bật push.
- `Q-5` Task-259 sẵn sàng thì mới chạy được `P-3` xếp hạng impact (blocker đã biết: lệch dir-bucket vs symbol input, chưa có latency budget, fallback walk không hủy được, CLI GitNexus chưa verify). Ship `P-1`/`P-2` với bộ lọc chỉ theo `feature_key` trước; thêm impact ranking khi 259 xong.

### Nguồn tham chiếu

- Bài viết bên ngoài: *"The Problem with Parallel Agents"* (đồng bộ ngữ cảnh cho parallel agent; khung ba tầng Shared Reality). Tóm tắt ở §3; bài viết là bối cảnh, không phải nguồn chuẩn tắc của FlowPilot.
- Code đã verify (§4): `internal/contextsync/local.go`, `internal/contextsync/sync.go`, `internal/runner/engine_drive_sync.go`, `internal/runner/gate_hook.go`, `internal/runner/interactive_service.go`, `internal/agentpack/flow-pack/agents/{coder,reviewer}.md`; SD-18/SD-19/SD-20/SD-24; `CP-43 §4.x`.
- `SS-14` US-3, AC-7, AC-8, BR-2; `SD-17` D-3, D-4, D-11; `SD-21` (Canonical Head / intent signature).

## 1. Mục tiêu

Mở rộng context engine của FlowPilot để khi nhiều agent chạy **song song trên các worktree-project riêng biệt của cùng một repository**, chúng chia sẻ được các tầng ngữ cảnh quan trọng cho việc tích hợp đúng — **intent** theo từng feature (Canonical Head, L3) và **drift/impact signal** (L2) — mà không bao giờ tự động merge code của nhau (L1). Đưa con người ra khỏi vai trò "message bus thủ công" cho trường hợp xuyên-agent, đồng thời coi ba rủi ro tự động hóa của bài viết (auto-rebase sai, context dilution, update loop) là acceptance criteria chứ không phải điều nghĩ tới sau.

## 2. Tài liệu đầu vào

- `SD-21` — Change Contract và Canonical Head; artifact intent mà CP này đem federate.
- `SD-17` (D-3 ordered-history, D-4 no-vector, D-11 deferred scope-drift) — context engine mà CP này mở rộng.
- `SS-14` (US-3, AC-7, AC-8) — acceptance criteria về code-context/regression-safety mà federation phải tiếp tục tôn trọng.
- `CP-43`/`CP-44`/`CP-35` — các module `contextsync`/`changecontract`/`structure`/registry đã ship mà công việc này đặt code vào.
- `Task-259` — `source.dependence` (GitNexus impact → agent context); nguồn xếp hạng cho `P-3`.

## 3. Bối cảnh — Bài toán đồng bộ ngữ cảnh cho parallel agent (theo bài viết)

*Mục này ghi lại YÊU CẦU #1: "bài viết đề cập gì".*

### 3.1 Luận điểm cốt lõi

Chi phí *triển khai* đã sụp đổ: các AI coding agent song song có thể mỗi con nhận một task (sửa review, làm UI, đổi billing) và tạo ra code chạy được rất nhanh. Nút thắt mới là **tốc độ tạo thay đổi của AI giờ vượt tốc độ con người cập nhật mô hình tư duy về hệ thống**. Con người bị âm thầm biến thành **message bus**: đọc thay đổi của Agent A, xác định các thread khác bị ảnh hưởng, rồi tóm tắt lại bối cảnh cho Agent B và C và yêu cầu chúng rebase / cập nhật giả định. Khi công việc song song và tần suất merge tăng, chi phí phối hợp có thể tăng nhanh hơn số lượng thread.

### 3.2 Biệt lập vật lý vs biệt lập khái niệm

- **Biệt lập vật lý** (worktree / branch / container) đã được giải quyết — nó ngăn các agent ghi đè file của nhau (merge conflict).
- **Biệt lập khái niệm** mới là vấn đề thật: mỗi agent chỉ sở hữu *cuộc hội thoại, giả định, và snapshot code tại thời điểm nó bắt đầu*. Khi thế giới bên ngoài đổi, agent bị cô lập vẫn tiếp tục làm việc trên dữ liệu cũ.

Ví dụ trong bài viết: Agent A cấu trúc lại API trả về thành `ReviewResult { comments, severity, summary }`; Agent B, từ snapshot cũ hơn, vẫn xây UI theo `ReviewResult { findings, score }`; Agent C viết lại cơ chế retry/state bên dưới. Mỗi cái đều hợp lệ và vượt test cục bộ trong branch riêng — nhưng hệ thống tích hợp thì vỡ.

### 3.3 Từ xung đột mã nguồn tới xung đột ngữ nghĩa

VCS truyền thống xử lý **xung đột dòng**. Rủi ro lớn hơn với parallel agent là **xung đột ngữ nghĩa / hành vi**: code merge sạch, type-check pass, unit test pass, nhưng hành vi tích hợp lại sai. Git mù hoàn toàn với lớp này.

### 3.4 Khung ba tầng Shared Reality

| Tầng | Đồng bộ | Công cụ điển hình | Cảnh báo của bài viết |
|------|---------|-------------------|-----------------------|
| **L1 — Code state** | Agent có code mới nhất | Git rebase / merge / CI | Về cơ bản đã giải quyết |
| **L2 — Context state** | Agent biết các quyết định/task song song | *(còn thiếu)* | Chỉ hữu ích nếu hệ thống **tự phát hiện dependency và xếp hạng impact**; nếu không, thread-visibility biến thành **shared noise** |
| **L3 — Intent state** | Agent hiểu hướng đi chung của sản phẩm để lời giải **hội tụ** | *(còn thiếu)* | Tầng khó nhất |

Hai chiều đồng bộ được đề xuất: **pull-based** (agent/người dùng chủ động đọc thread/PR bên cạnh để hiểu ý định thiết kế, không chỉ nhìn diff) và **push-based** (hệ thống phát hiện thay đổi ở branch cha rồi thông báo / auto-rebase branch con).

### 3.5 Ba rủi ro khi tự động hóa quá sớm

1. **Auto-rebase ≠ tích hợp đúng** — Git có thể merge không conflict trong khi logic nghiệp vụ vẫn mâu thuẫn.
2. **Context dilution** — báo mọi thay đổi cho mọi agent làm ngập context window bằng thông tin không liên quan, khiến chất lượng đầu ra giảm.
3. **Update loop** — thay đổi ở một branch kích hoạt auto-rebase + sửa code ở các branch khác, những sửa đổi mới đó lại kích hoạt cập nhật ở nơi khác, lặp vô hạn.

## 4. Tình trạng hiện tại của FlowPilot

*Mục này ghi lại YÊU CẦU #2: "tình trạng của FlowPilot", kèm bằng chứng code.*

### 4.1 Bên trong một flow, FlowPilot né được phiên bản khó của vấn đề theo kiến trúc

FlowPilot Flow Mode là một **orchestrator đa-provider, một-flow, một-working-tree**, không phải hệ N-worktree-độc-lập. Một flow là DAG (`context → coder → {reviewer_correctness, reviewer_security} → synthesis`, có `back` edge có chặn) với đúng một stage chạy song song:

- **Mỗi thời điểm chỉ một writer**: coder chạy một mình; agent của nó có `[Read, Edit, Write, Bash, Grep, Glob]` (`internal/agentpack/flow-pack/agents/coder.md`).
- **Stage song song là read-only**: reviewer có `[Read, Grep, Glob, Bash]` — không Edit/Write (`internal/agentpack/flow-pack/agents/reviewer.md`). Chúng soi *cùng một* diff với các lăng kính khác nhau.
- **Topology hình sao**: reviewer không nói chuyện ngang hàng; kết quả gập về hub thành một text note hợp nhất (`buildCohortNote`), join bằng barrier `all`.

⇒ Vì hai agent không bao giờ cùng ghi vào một cây code, không có divergence/merge để hòa giải *bên trong một flow*. Kịch bản xung đột ngữ nghĩa của bài viết không phát sinh ở đây.

### 4.2 Ở cấp composition, FlowPilot rơi trọn vào chế độ của bài viết

Không có gì ngăn người dùng: tạo **hai git worktree** của một repo → add **mỗi cái thành một target project riêng** (mỗi cái một `project_id`) → chạy một flow (hoặc agent thường) **song song ở mỗi cái**. Đó chính xác là chế độ "Agent A / Agent B trên các phần khác nhau của cùng một codebase" của bài viết — và FlowPilot khiến việc này cực kỳ dễ.

### 4.3 Đã verify: không lớp context-sync nào bắc qua hai worktree-project

Mọi lớp trong bộ máy "shared reality" của FlowPilot đều key theo working directory hoặc `project_id`. Hai worktree-project vì thế là các **ốc đảo** sạch — không có gì được chia sẻ hay đồng bộ giữa chúng:

| Lớp context | Lưu tại | Key theo | Bắc qua 2 worktree? |
|---|---|---|---|
| Ledger / feature history | `<target>/.flowpilot/ledger/` | working dir | ❌ |
| Feature catalog | `<target>/.flowpilot/catalog/` | working dir | ❌ |
| **Canonical Head (intent, L3)** | `<target>/.flowpilot/canonical/<feature_key>.json` | working dir | ❌ |
| Change Contract | `<target>/.flowpilot/contracts/` — chỉ local, **không cả sync Drive** | working dir | ❌ |
| Dispatch log | `.flowpilot/chats/<project_id>/dispatch.ndjson` | `project_id` | ❌ |
| Drift detection | tính trên spec-hash cục bộ | working dir | ❌ |
| Drive mirror | `context-engine/` bên trong **Drive root của chính project đó** | `project_id` | ❌ |

Bằng chứng:
- `.flowpilot/` nằm dưới `<target>/` (worktree); substrate được tạo từ `dotFlowpilotDir` truyền vào theo từng target (`internal/contextsync/local.go:22-30`; ràng buộc `CP-43` §4.44 "Per-`project_id`, local-first under `<target>/.flowpilot/`").
- Canonical Head là `canonical/<feature_key>.json` và được upload theo **basename** (`internal/contextsync/local.go:60-79`, `sync.go:37`).
- Folder Drive `context-engine/` được resolve **bên trong Drive root của từng project**: `buildEngineDriveSyncer(projectID)` → `ensureChatSessionDriveRoot(projectID)` (`internal/runner/engine_drive_sync.go:26,73-74`). Hai `project_id` ⇒ hai Drive root ⇒ hai folder `context-engine/` cô lập (không cross-sync, và — vì root khác nhau — cũng không đè nhầm lên nhau).
- Cô lập giữa các run đồng thời là chủ đích và đã verify: hai run trên hai project giữ file dispatch tách biệt, không rò rỉ chéo `project_id` (nhật ký verification `CP-51`, kịch bản C9).

### 4.4 Hệ quả (đọc thẳng thắn)

Ở cấu hình parallel-worktree, FlowPilot hiện **làm rất dễ việc sinh ra các thực tại phân kỳ** trong khi cung cấp **zero** đồng bộ L1/L2/L3 giữa chúng. Gate + drift detection của mỗi project chỉ so code của project đó với spec của chính project đó — nó không thể thấy rằng một worktree bên cạnh đã đổi một API dùng chung. Git merge sạch, mỗi gate pass, nhưng hành vi tích hợp có thể vỡ. Con người quay lại làm message bus — đúng failure mode của bài viết.

### 4.5 Những gì đã có sẵn để xây tiếp

FlowPilot đứng ở vị trí hiếm có vì các primitive khó đã tồn tại — chỉ là chúng đang ở phạm vi **per-project**:
- **Canonical Head** (`SD-21`): một tuyên bố intent có thẩm quyền cho mỗi `feature_key`, ký trên **spec** (không phải byte code) — một artifact L3 cụ thể mà hầu hết nền tảng khác không có.
- **Drift detection**: các trạng thái `SpecDrifted` / `CodeDrifted` (`internal/changecontract/`) — một signal staleness của L2.
- **GitNexus impact** (`structure.Dependents`): hôm nay chỉ nuôi việc xếp hạng severity của gate (`internal/runner/gate_hook.go`), chưa vào context của agent — Task-259 sẽ bơm blast-radius vào context.
- **Change ledger**: nhật ký verification có thứ tự, đánh chỉ mục theo commit (`SD-17 D-3`).
- **Tư thế chống rủi ro sẵn có**: FlowPilot không bao giờ auto-merge code phân kỳ (đường Gemini shell-bridge fail **closed** khi main đổi đồng thời); context bị chặn bằng hard byte caps + priority ordering (không flooding); loop có chặn (≤2 reprompt, cap→escalate). Nghĩa là rủi ro #1 và #3 của bài viết đã được phòng thủ một phần về mặt tinh thần.

## 5. Giải pháp — Federate Shared Reality theo `feature_key`

*Mục này ghi lại YÊU CẦU #3: "solution để improve".*

### 5.1 Ý tưởng trung tâm

Nâng Canonical Head + drift signal từ artifact **per-`project_id`** thành artifact **per-`feature_key`, được federate**, chia sẻ bởi một nhóm target project liên quan đã khai báo. Khóa join đã tồn tại (`canonical/<feature_key>.json`); chỉ có *phạm vi folder* hiện đang bị buộc theo project. Federation là thay đổi kiến trúc duy nhất; mọi thứ còn lại là tái sử dụng.

### 5.2 Map vào ba tầng của bài viết

- **L1 — Code state**: **ngoài phạm vi theo quyết định.** Git / CI / con người sở hữu việc hòa giải branch. FlowPilot không được auto-rebase hay auto-merge (rủi ro #1 của bài viết; `P-2`).
- **L2 — Context state**: một worktree đang sửa `feature_key = X` sẽ (a) **pull** Canonical Head chung hiện tại + bất kỳ drift nào từ worktree khác cho X trước turn coder của nó, và (b) khi nó rebaseline X, **push** một signal `spec_drifted` sang các project bên cạnh có scope khai báo giao với X. Thông báo được **xếp hạng** theo GitNexus impact và **lọc** theo giao `feature_key` để các worktree không liên quan không bao giờ bị báo (cơ chế chống "shared noise"; `P-3`).
- **L3 — Intent state**: Canonical Head chung trở thành **mỏ neo hội tụ** — coder của mỗi worktree đọc lại intent có thẩm quyền cho các feature nó đụng tới ở mỗi turn, nhờ đó các lời giải song song uốn về một thiết kế thay vì phân kỳ.

### 5.3 Hai chiều đồng bộ (theo bài viết)

- **Pull-based**: một context source (mở rộng registry `CP-44`) mà, tại hand-off, nạp Canonical Head *đã federate* cho mỗi `feature_key` trong Change Contract của turn, không chỉ head cục bộ.
- **Push-based**: khi một head đã federate bị rebaseline/đổi spec-hash, đánh dấu head của các project bên cạnh cho cùng `feature_key` là `spec_drifted` và surface nó thành gate/attention signal ở turn **kế tiếp** của chúng (không bao giờ là một lệnh sửa cưỡng bức tức thì — có chặn, `P-4`).

### 5.4 Ba rủi ro → acceptance criteria

1. **Không auto-rebase ẩu** → hệ thống chỉ phát *intent/drift signal*; coder bên nhận tự quyết cách hòa giải. DoD khẳng định không có merge code tự động nào xảy ra giữa các worktree.
2. **Không context dilution** → một worktree chỉ nhận signal từ worktree khác **khi** scope khai báo của nó giao với blast-radius của symbol bị đổi ≠ ∅. DoD gồm một test phủ định: feature Y không liên quan không tạo ra thông báo nào ở worktree B.
3. **Không update loop** → drift signal là tư vấn và idempotent; tiêu thụ một signal không bao giờ tự phát ra signal khác. DoD gồm một test ping-pong hai worktree bắt buộc phải kết thúc.

## 6. Kế hoạch chia phase (đề xuất — phụ thuộc `Q-1`/`Q-5`)

- `P-1` **Context-engine scope đã federate.** Giới thiệu một grouping tường minh (một "workspace" gồm các target project thành viên; `Q-2`) resolve ra một context-engine root chung key theo repo/feature thay vì `project_id`. Ưu tiên filesystem cùng máy trước (`Q-3`). *Đụng vào:* `contextsync`, đăng ký project.
- `P-2` **Pull Canonical Head xuyên-project.** Context source mới/mở rộng đọc head đã federate cho các `feature_key` của turn tại hand-off. *Đụng vào:* `changecontract`, registry `CP-44`, `flow_context_package`.
- `P-3` **Push drift có xếp hạng.** Khi rebaseline, đánh dấu head bên cạnh cho cùng `feature_key`; surface ở turn kế tiếp của chúng. Lọc/xếp hạng theo giao `feature_key` ∩ GitNexus blast-radius (**phụ thuộc Task-259**). Scorer/locus + chuẩn hóa target **tái dùng từ CP-54** (prerequisite), không xây lại. *Đụng vào:* `changecontract/drift`, `structure`, `gate_hook`.
- `P-4` **Bơm hội tụ.** Đảm bảo mỗi turn coder đọc lại intent đã federate có thẩm quyền cho các feature của nó. *Đụng vào:* `flow_context_handoff`.
- `P-5` **Federation khác máy (hoãn).** Mở rộng đường Drive `context-engine/` từ per-`project_id` sang một root chung/key theo feature. *Đụng vào:* `engine_drive_sync`. Phụ thuộc `Q-3`/`Q-4`.

## 7. Định nghĩa Hoàn thành (draft)

- Một integration test hai worktree: worktree A đổi hình dạng một API được ghi trong Canonical Head cho feature X; worktree B (scope khai báo có đụng X) nhận được head đã cập nhật **và** một drift signal **trước** turn coder kế tiếp của nó — với **không có** merge code tự động nào giữa hai branch.
- Test phủ định/nhiễu: một worktree chỉ đụng feature Y không liên quan sẽ **không** nhận signal xuyên-project nào.
- Test kết thúc: drift qua lại giữa hai worktree không tạo ra update loop vô hạn.
- Toàn bộ test `contextsync`/`changecontract`/`structure` hiện có vẫn xanh; test mới là additive; `go test ./internal/...` sạch.
- Topology bên trong flow (single writer + cohort read-only) chứng minh được là không đổi.

## 8. Non-Goals

- Auto rebase/merge code xuyên nhánh cho công việc song song phân kỳ (rủi ro #1 của bài viết) — bị từ chối tường minh.
- Một đối tượng state chung, sống, mutable giữa các agent — việc lan truyền vẫn dựa trên file + signal.
- Vector/embedding retrieval (`SD-17 D-4` giữ nguyên).
- Đổi mô hình single-writer / cohort read-only bên trong flow.

## 9. Rủi ro

- **Dependency Task-259** (`Q-5`): xếp hạng impact (`P-3`) bị chặn tới khi `source.dependence` sẵn sàng; giảm nhẹ bằng cách ship `P-1`/`P-2` với bộ lọc chỉ theo `feature_key` trước.
- **Race rebaseline đồng thời** (`Q-4`): hai worktree viết lại cùng một head cần một luật xử lý xung đột đã định trước khi bật push.
- **Bất ngờ khi grouping**: tự động gom theo remote URL có thể gây cross-talk giữa các clone không liên quan; ưu tiên thành viên workspace tường minh (`Q-2`).
- **Mệt mỏi vì signal**: ngay cả signal đã xếp hạng cũng có thể tích tụ; giữ chúng ở mức tư vấn, dedupe, và có chặn.
