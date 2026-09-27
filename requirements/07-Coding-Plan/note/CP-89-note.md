# CP-89 Bản Ghi Chú: Flow có hai cách khởi động — start ngay, hoặc chat rồi mới forward

> Dành cho người sẽ code. Đây là note issue, chưa phải coding plan và chưa phải bugfix. Hành vi hiện tại giữ làm chế độ mặc định. Chế độ mới là một lựa chọn lúc mở run.

- Document ID: `CP-89`
- Title: `Flow launch — immediate start or chat-then-forward`
- Kind: `note`
- Date: `2026-09-27`
- Status: `reviewed` — findings merged (§8); task breakdown: Task-451, Task-452, Task-453, CP-89-Test-Steps
- Related: CP-42 (flow start trên turn đầu), CP-60 (working mode), CP-59 (chat SSOT), `note/CP-89-review.md`

## 1. Issue

Chọn một flow rồi gửi câu đầu tiên thì flow **luôn start ngay**. Main agent không được trả lời câu đó. Các step thật (`ingest_reader`, `cp_reader`, …) spawn trước khi người dùng chốt ý.

Người dùng chưa chắc feature sẽ như thế nào. Họ muốn ngồi chat với main trước. Khi thấy ổn, họ mới bảo forward, và lúc đó flow mới chạy step thật.

Flow mode hiện không có cửa đó. Không có trạng thái "đã chọn flow nhưng chưa cho chạy".

## 2. Case thật — start `vibe-ingest`

Operator ở working mode `vibe`, chọn flow `vibe-ingest`, gửi:

> Tôi muốn một game rắn trên terminal, nhưng chưa chốt: có tường hay không, ăn mồi thì dài ra thế nào, thua khi nào. Bàn với tôi trước khi viết spec.

Việc runner làm hôm nay, trên turn đầu (`turnCount == 0`) khi run đã có `flowRef`:

1. `startTurn` đặt `flowStartOnly = true`.
2. Gọi `startResolvedFlow`. Entry của `vibe-ingest` là `ingest_reader`. Child spawn ngay, prompt của child chính là câu ở trên.
3. Lượt provider của hub bị nuốt. Comment trong `interactive_service.go` nói rõ: hub's first provider turn is suppressed, hub chỉ được gọi lại khi flow tới một node `hub.inline`.
4. `flowEngineDriven` và `vibeAwaitingLock` bật ngay. Run không còn là chat thường.

Kết quả: chưa có cuộc bàn nào với main, spec đã bị nháp. Câu "bàn với tôi trước" không tới main.

Cùng cửa này áp cho mọi flow gắn lúc tạo run hoặc lúc turn đầu: `vibe-cp-ingest`, `vibe-sprint`, `bug-harness`, `task-harness`. `vibe-cp-ingest` còn 422 `invalid_cp_source` nếu câu đầu không trỏ một file CP. Người đang muốn chat dò ý bị chặn trước cả khi được nói chuyện.

## 3. Hai chế độ

Chế độ gắn trên run lúc start, cùng chỗ với `flowRef` và `workingMode`. Sau khi flow đã start thật thì không đổi. Default là chế độ hiện tại, để client cũ không đổi hành vi.

### 3.1 `immediate` — code hiện tại

Prompt đầu tiên là lệnh cho flow.

- Turn đầu có `flowRef` (trên turn, hoặc đã gắn `chatFlowRef` lúc tạo run) thì `startResolvedFlow` chạy.
- Hub không trả lời câu đó.
- Mọi gate của flow (nguồn CP, contract, audit) chạy như hôm nay.

Không sửa đường này. Mọi test flow đang xanh phải vẫn xanh khi client không gửi field mới.

### 3.2 `chat_then_forward` — chế độ mới

Flow được **chọn và ghim**, nhưng step thật chưa chạy.

Trước forward, run là chat với main:

- Provider của hub trả lời bình thường. Nhiều turn thoải mái.
- Không `startResolvedFlow`, không child, không `flowEngineDriven`.
- Không bật `vibeAwaitingLock`. Không chạy hàng rào `invalid_cp_source` trên những câu chat dò ý.
- `chatFlowRef` vẫn nằm trên run, để runner biết lúc forward sẽ mở flow nào.
- Working-mode fence vẫn giữ: không ghim flow vibe khi run đang `dev`, và ngược lại. Fence chạy lúc ghim, không đợi forward.

Forward là một hành động tường minh của user, không phải câu chữ mà model tự hiểu là "ok":

- Turn mang cờ `forwardFlow: true`. UI có nút Forward gửi cờ này. Một slash phía client cũng chỉ được phép gửi cùng cờ đó.
- Runner không suy luận từ chữ "ok", "chốt", "làm đi" trong transcript. Những chữ đó là nội dung chat.

Đúng một lần, trên turn forward:

1. Hàng rào mà hôm nay chạy ở turn đầu chuyển sang chạy ở đây. `vibe-cp-ingest` thiếu source CP thì 422, không spawn.
2. `startResolvedFlow` chạy. Prompt đưa vào entry node là gói đã chốt, không phải mỗi câu đầu tiên:
   - câu user gửi kèm nút Forward, nếu có
   - kèm transcript chat đã settle (user và main) để entry node thấy phần đã bàn
3. Turn forward này là turn `flowStartOnly`: hub không trả lời thêm một lượt chat nữa, vì user đã chuyển sang flow.
4. Từ đây run giống hệt một run `immediate` vừa start xong. Step, gate, hub.inline, audit đi đường cũ.

Không forward lần hai. Turn sau forward là turn flow bình thường, không phải lần chat dò ý mới.

## 4. Vì sao không dùng lại "chỉ turn đầu mới start"

Hôm nay cửa start là `turnCount == 0`. Chat vài lượt thì `turnCount` đã khác 0, cửa đó đóng vĩnh viễn. Chế độ mới không được núp vào cửa đó.

Cần một chốt riêng trên run, ví dụ `flowArm`:

| Giá trị | Ý nghĩa |
|---|---|
| `immediate` | Start ở turn đầu. Default. |
| `pending` | Đã ghim flow, đang chat, chưa start. |
| `started` | Đã forward hoặc đã start immediate. Không start lại. |

`pending` sống qua nhiều turn, qua switch provider (CP-59: pin thuộc chat, forward xảy ra trên leg đang active), và qua restart. Reconstruct thấy `pending` thì trả lại chat, không gọi `startResolvedFlow`. Run Drive-restore đã `started` giữ luật BUG-315: không start lại flow.

## 5. Case vibe sau khi có chế độ mới

Cùng câu rắn ở mục 2, user chọn `chat_then_forward`.

1. Main trả lời: hỏi tường, điểm, điều kiện thua. Chưa có file SS, chưa có child `ingest_reader`.
2. User bàn thêm hai turn. Vẫn chỉ là chat.
3. User bấm Forward, kèm câu "Chốt: không tường, ăn mồi dài thêm một đốt, đâm thân là thua. Viết spec rồi mới code."
4. Lúc này `ingest_reader` mới spawn. Prompt của nó là câu chốt cộng transcript ba turn trước.
5. Phần còn lại của `vibe-ingest` chạy như một start immediate.

Nếu user bỏ đi không forward: run kết thúc như một chat thường có flow đã ghim. Không node, không audit, không SS.

## 6. Việc không làm trong CP này

- Không để model tự quyết "user đã ok, start flow".
- Không đổi thứ tự node trong pack vibe, bug-harness, task-harness.
- Không đổi default. Client không gửi `flowArm` thì vẫn `immediate`.
- Không gộp hai chế độ thành một heuristic theo độ dài prompt.

## 7. Test phải có trước khi vá

1. `immediate`, turn đầu có `flowRef`: child entry spawn, hub không nhận câu đó. Hành vi hôm nay.
2. `chat_then_forward`, ba turn chat không cờ forward: không child, hub có reply, `flowEngineDriven` vẫn false.
3. Cùng run, turn thứ tư `forwardFlow: true`: đúng một entry child, prompt child chứa câu forward và một marker từ turn chat trước.
4. Forward lần hai không spawn entry mới.
5. `vibe-cp-ingest` + `chat_then_forward`: câu chat không có source thì 200 và là chat. Turn forward không có source thì 422, không spawn.
6. Restart khi còn `pending`: reconstruct không start flow. Restart khi đã `started`: không start lần hai.
7. Working mode sai với flow đã chọn vẫn 400 lúc ghim, ở cả hai chế độ.
8. Forward trần `{"forwardFlow": true}` không kèm prompt: phải forward được — không chết ở admission guard (xem §8 F-1).
9. Switch provider khi còn `pending` rồi Forward: pin theo chat/run, không theo leg — forward vẫn start flow.
10. Forward sau khi định nghĩa flow đã ghim bị xóa/corrupt: fail-closed `invalid_flow_definition`, không degrade thành chat.

## 8. Quyết định bổ sung từ review (F-1 → F-7)

Review đầy đủ: `note/CP-89-review.md`. Các điểm cần chốt trước khi code:

- **F-1 (admission)**: `handleStartTurn` đang reject body không có content (BUG-509 guard, `interactive_handlers.go:504-509`). `forwardFlow` phải được thêm vào content-check — nút Forward trần là hợp lệ theo §3.2.
- **F-2 (seam resolve)**: các gate start-flow và `invalid_cp_source` đọc `in.FlowRef` (field turn). Ở `pending`, pin nằm trên `rs.chatFlowRef`. Turn forward phải resolve pin từ `rs.chatFlowRef`/`workflowID` — một seam resolve thứ hai. **Không** nới gate `turnCount == 0` trong `resolveWorkflowFlowRef` (đang gánh BUG-315/BUG-261).
- **F-3 (tách pin khỏi arm)**: `createRun` bật `vibeAwaitingLock`/`vibeSprintBudget` ngay khi có `FlowRef` (`interactive_handlers.go:1244-1247`). Phải tách "pin `chatFlowRef`" khỏi "arm start-markers" theo `flowArm` — `pending` giữ markers tắt.
- **F-4 (CP source lúc forward)**: `validateVibeCpIngestSource` resolve theo `SourceDocID` explicit → token trong prompt của turn đó. Quyết định: `SourceDocID` đã ghim lúc create/launch arm được tính vào validate lúc forward. User không phải paste lại path khi Forward; chat transcript không được scan để suy ra source (fail-closed nếu không có).
- **F-5 (transitions mới)**: `forwardFlow: true` trên run không pin flow nào → 422 typed error, không im lặng thành chat. Definition corrupt giữa pin và forward → `invalid_flow_definition` (mirror `pendingFlowRefInvalidErr`), không degrade thành chat.
- **F-6 (transcript bound)**: prompt entry = câu forward + transcript chat đã settle — phải đi qua `promptpacker` với cap cứng (mặc định theo budget entry-node), chỉ gồm turn user+hub đã settle, thứ tự thời gian tăng dần. Không concat thô.
- **F-7 (nhà của `flowArm`)**: `flowArm` thuộc **run/chat**, không thuộc leg. Ba nhà bắt buộc: (a) field trên session row (`sessions.ndjson`), (b) Drive-restore manifest theo precedent `TurnCount` của BUG-315, (c) `reconstruct` tôn trọng `pending` (không `startResolvedFlow`) và `started` (không arm lại). Switch provider tạo leg mới nhưng `pending` vẫn theo run.
