# CA-636: Bound renderer output — a wedged console can never freeze the event loop

## What

TUI chat "treo" lặp lại sau CA-633 dù đã có mọi fix input-side (log pid 12048,
binary mới nhất, 13:05:46): session load xong +4s, `sessionLoadTimeoutMsg`
xử lý lúc 13:06:31.886, rồi **0 KeyMsg suốt 48 phút** — user copy-paste vào ô
chat, phím không bao giờ tới `Update`. Process sống, **0 CPU, toàn bộ thread
waiting** (blocked, không phải spin). Tới 13:54:51 user bấm phím thì mọi thứ
sống lại ngay (rồi thoát).

## Why

Windows console block output: khi user **đang giữ text selection trong
terminal** (chọn để copy / QuickEdit select / mark mode), `WriteConsole` sẽ
BLOCK cho tới khi user bấm phím (hoặc Esc/click) để clear selection — hành vi
có tài liệu rõ (python bpo-26744, DaniWeb "WriteFile() function in printf
freezes", SO "QuickEdit mode in Command Prompt freeze applications").

Bubble Tea v1.3.10 renderer: goroutine `listen()` (ticker ~60fps) gọi
`flush()` — `flush()` **giữ `r.mtx` rồi mới `r.out.Write(...)`**
(standard_renderer.go:283). Nếu console write bị block:

1. `flush()` giữ `r.mtx` vô hạn.
2. Event loop chặn tại `p.renderer.write(model.View())` → `r.mtx.Lock()`.
3. Event loop không bao giờ quay lại `<-p.msgs` → paste keys không được
   dispatch → conhost queue 64 slot tràn → phím drop ở OS level.
4. Hồi phục: user bấm phím → selection clear → `WriteConsole` trả về → `r.mtx`
   nhả → event loop chạy lại → phím dồn tới ngay.

Mọi fix trước (CA-610 mouse filter, CA-612/630/631 paste reject, CA-615
clipboard timeout, CA-621 View log throttle, CA-633 compose cache) đều gia cố
**input side**. Không fix nào bound **output write** — một console write bị
kẹt vẫn đóng băng cả event loop. Đó là lý do "fix nhiều lần vẫn treo".

## Fix

- **`output_queue.go` (mới)**: `queuedOutput` — writer queue-backed, một
  goroutine drain duy nhất:
  - `Write(p)` luôn trả về ngay: enqueue vào FIFO `ch` (cap 8); nếu queue đầy
    (console bị kẹt) thì **drop frame** + log giới hạn 5s (`output queue full
    — dropping frame`). Renderer repaint mỗi tick nên frame bị drop không
    mất gì.
  - `drain()` là goroutine duy nhất có thể stall trên console bị block —
    event loop và input reader không bao giờ chờ console.
  - Implement `term.File` (`Fd()`/`Read()`/`Close()` delegate về `os.Stdout`)
    để bubbletea giữ nguyên `ttyOutput` detection → `checkResize` /
    `WindowSizeMsg` vẫn hoạt động (không phá CA-584 width / CA-633 sidebar).
- **`app.go` `Run()`**: `tea.WithOutput(newQueuedOutput(os.Stdout))` — chỉ ở
  Run() (interactive), không đổi `tuiProgramOpts()` (test cũ len==3 giữ nguyên;
  headless/print không dùng program).

Will not undo: CA-610/612/615/621/630/631/633, reject guard, mouse filter,
compose cache, paste token, clipboard timeouts.

## Tests (additive, no old edit)

- `tui/app/output_queue_test.go`:
  - `TestQueuedOutput_BlockedConsoleWriteReturnsImmediately` — console bị
    block (gateWriter giữ write tới khi unblock): `Write` phải trả về trong
    <500ms (event loop không đóng băng), sau khi unblock queue drain ra.
  - `TestQueuedOutput_HealthyConsoleWritesInOrder` — console khoẻ: frames
    drain đúng thứ tự, payload nguyên vẹn.
  - `TestQueuedOutput_ImplementsTermFile` — `Fd()` delegate về file thật
    (term.File contract → resize vẫn hoạt động).
  - `TestQueuedOutput_DropLogIsThrottled` — log drop không spam.
- `go vet ./internal/tui/app` clean; `go test ./internal/tui/app -count=1`
  xanh 14.5s (old suite untouched).

## Provider parity

Agnostic — không nhánh `providerKey`; writer thuần output layer.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-636
change_type: bugfix
summary: bound renderer output via queuedOutput (single drain goroutine, drop-on-full, term.File kept) so a Windows console output block (text selection freezes WriteConsole until a keypress) can never hold the renderer mutex and freeze the event loop — fixes recurring paste-hang (log 12048: 0 KeyMsg 48min, recovery on keypress)
# --->8---