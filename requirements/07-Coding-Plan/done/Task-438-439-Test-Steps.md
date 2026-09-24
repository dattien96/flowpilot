# Task-438/439 Test Steps — Devin thought_level + ACP Prewarm

- Document ID: `Task-438-439-Test-Steps`
- Title: `Task-438/439 Test Steps`
- Phase: `coding-plan`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-24`
- Last Updated: `2026-09-24`
- Parent Documents: `Task-438, Task-439`
- Child Documents: ``
- Related Documents: `CA-965-Devin-ThoughtLevel-And-StaleModelDetect, CA-966-Devin-ACP-Prewarm-And-ProviderStatus, CA-967-Devin-Process-Exit-Watcher, CA-968-Devin-Silent-ApiKey-Auth`
- Replaces: ``
- Tags: `verification, devin, acp, prewarm, reasoning, provider-status, silent-auth`

## AI Quick View

### Summary

- Verify Task-438 (reasoning → `thought_level` trên ACP schema mới, stale-model
  disable khi detect) và Task-439 (prewarm `devin acp` ở boot/login/switch +
  `provider_status` cold-start event). Automated Go/desktop tests cho logic;
  live HTTP test trên account Devin thật của máy này cho end-to-end.

### Current Ask

- Đã chạy xong: automated suites xanh + live matrix 6/6 trên runner thật
  (`127.0.0.1:47788` rồi `:47789`, binary build từ `7fc921a9` + dirty).
  Desktop DETECT MODEL vào Supabase chỉ cover ở unit level (không ghi DB
  thật trong live test).
- Follow-up (F-2): `authenticate` giờ thử silent `windsurf-api-key` +
  `_meta.api_key` trước — account đã login không còn pop browser PKCE;
  không key hoặc key bị reject → fallback `devin-browser` như cũ.

### Key Decisions

- Live test đi qua HTTP endpoints thật của runner (`/client/workflow-runs`,
  `/turns`, `/events/stream`) — đúng wire contract desktop/TUI consume.
- Cold-start được ép bằng cách kill ACP child của runner test — không đụng
  `devin.exe` của runner hệ thống.
- `provider_status` là event additive: warm turn không emit, cold turn emit
  `connecting → ready` trong khi adapter factory còn block.

### Constraints

- Không edit test cũ; mọi coverage mới là file additive.
- Không kill `devin.exe` không thuộc sở hữu runner test.
- Live test dùng account `ba805096a861115ab7ed4aff743531a7` (connected,
  active) — credential cache sẵn nên PKCE không mở browser.

### Source Refs

- `Task-438`, `Task-439`, `change-audit/CA-965-*.md`, `CA-966-*.md`,
  commits `0859b6b3`, `7fc921a9`.

## 1. Goal

Chứng minh trên nhịp live: (a) model/reasoning map đúng schema ACP mới
(`swe-2-high` + `thought_level`), stale id `swe-2-max` tự resolve sibling;
(b) prewarm chạy khi có connected Devin account mà không cần provider đang
được chọn; (c) cold-start emit `provider_status` thay vì im lặng; (d) flow
chat cũ không đổi cấu trúc.

## 2. Automated Verification

```bash
cd apps/local-runner && go test ./internal/runner/ -run 'TestDevinThoughtLevel|TestDevinCatalogModelFor|TestApplyDevinSessionConfig|TestWarmDevin|TestDevinChatProcessWarm|TestStartTurnDevin' -count=1
cd apps/desktop-flowpilot && npx vitest run src/state/timelineReducer.test.ts src/components/settings/aiProvidersDetect.test.ts
cd apps/desktop-flowpilot && npx tsc --noEmit
cd apps/local-runner && go test ./internal/tui/... -count=1
```

| Nhóm | Test bắt buộc | Pass criteria |
|---|---|---|
| Reasoning schema | `devin_thought_level_test.go` (6 tests): `thought_level` harvest từ session/new configOptions; effort→`medium/high/max` map; `xhigh` round-up `max`; bare model id gửi khi new-schema | send `set_config_option{thought_level}`; legacy catalog vẫn suffix-remap |
| Stale model | `aiProvidersDetect.test.ts` (4 tests) | detected-row vắng khỏi live catalog → `isEnabled:false`; manual row không đụng |
| Prewarm | `devin_prewarm_test.go` (6 tests): boot/login/switch triggers, dedup per-scope, fail-closed khi chưa auth, warm predicate | warm handle đậu chat segment `""`, reuse bởi turn đầu |
| Status event | `timelineReducer.test.ts` (36): `provider_status` → 1 system row update in-place (`connecting`→`ready`/`failed`) | không noise khi warm; `failed` → error row |
| Process exit (F-1) | `TestEnsureDevinProcessExitMarksClosedWithoutStdoutEOF` | kill parent, grandchild giữ stdout → `isClosed()` < 3s qua `cmd.Wait` watcher |
| Silent auth (F-2) | `devin_silent_auth_test.go` (3 tests) | key trong credentials.toml → `windsurf-api-key`+`_meta.api_key`; không key → `devin-browser`; silent RPC error → retry `devin-browser` |

Kết quả: focused Go 16/16, desktop 40/40, typecheck sạch, TUI xanh.
Full runner suite: timeout Windows + `TempDir RemoveAll` flakes — baseline
đã documented (CA-319/342/637), không assertion fail trong vùng đụng tới.

## 3. Manual/Live Test Prep (đã thực hiện)

- Build: `go build -o /tmp/flowpilot-livetest.exe ./apps/local-runner`
  → buildId `7fc921a9cee78fe933d16021c2ffb007c2273d2d`.
- Workspace tạm: `C:\working\flowpilot\.livetest-ws` (xóa sau test).
- Launch: binary serve trên `127.0.0.1:47788`, PID 3396,
  `lifecycleMode=persistent`, account devin `ba805096…` connected+active.
- `curl` cho HTTP calls; event stream subscribe qua
  `GET /client/workflow-runs/run-1/events/stream` → `/tmp/fp-events.ndjson`.

## 4. Live REAL Tests — evidence

### `L-1` Boot prewarm — PASS

Không user action nào. Runner log ngay sau boot:

```
[devin] prewarm start scope="ba805096a861115ab7ed4aff743531a7" reason="boot"
[devin-acp] send {"id":1,"method":"initialize",...}
[devin-acp] send {"id":2,"method":"authenticate","methodId":"devin-browser"}
[devin] prewarm ready scope="ba805096a861115ab7ed4aff743531a7" reason="boot"
```

Warm ≈ **6s** (credential cache → không browser). MCP khởi động trong
prewarm (MCP của Devin, không đổi): `Connecting to MCP server
'flowpilot_jira'`, `Checking for OAuth tokens`, `No OAuth tokens found`,
`Attempting unauthenticated streamable HTTP connection`; `atlassian`
auth-required warn (expected — cần OAuth riêng, unrelated).

### `L-2` Warm turn + new schema — PASS

`POST /client/workflow-runs` → `run-1` (`chatId=cht_ad7eb85f272b`,
`providerSessionId=thread-2`). Turn: `prompt="reply with exactly: PONG"`,
`model="devin/swe-2-high"`, `reasoningEffort="max"`.

Wire log thật:

```
session/new → "pebble-plow"
set_config_option{model, "swe-2-high"}        → OK
set_config_option{mode, "smart"}              → OK
set_config_option{thought_level, "max"}       → OK
session/prompt → agent_message_chunk "P","ONG"
```

Events: `turn_started` 08:41:34.047 → first delta 08:41:41 (~7s ttft) →
`token_usage_updated` (20570 in / 30 out). **Không có** `provider_status` —
đúng: process đã warm. Không còn `Invalid value` — `swe-2-high` là id hợp
lệ của catalog mới (display name "SWE-2"); reasoning đi qua knob
`thought_level` riêng thay vì suffix id.

### `L-3` Stale model id live — PASS

Turn-17: `model="devin/swe-2-max"` (row stale từ catalog cũ),
`reasoningEffort="high"`. Log:

```
resolved_model="devin/swe-2-max" effort="high"
session/load session="pebble-plow"
set_config_option{model, "swe-2-high"}   → OK   ← sibling fallback
set_config_option{thought_level, "high"} → OK   (resp: currentValue="high")
```

`swe-2-max` không còn trong catalog → `devinCatalogModelFor` resolve về
`swe-2-high` cùng family. Không provider error, không silent wrong-model.
Provider `turn_stats` vẫn label `"SWE-2 Max"` (display naming của Devin —
ghi nhận nguyên trạng).

### `L-4` Cold-start `provider_status` — PASS

Kill ACP child của runner test (PID 21804) → handle chết được reap → turn
mới đi cold path. Events (UTC):

```
08:49:16.549  provider_status "connecting"  "Devin is starting — first run may open a browser sign-in."
08:49:24.957  provider_status "ready"       "Devin agent ready."        (~8.4s handshake)
08:49:24.969  turn_started  (turn-17)
08:49:28.588  message_delta "COLD-OK"       (~3.6s ttft) → turn_completed
```

`connecting` emit đúng lúc adapter factory còn block — progress có tên thay
vì màn hình đứng.

### `L-5` Old-chat compatibility — PASS

Cold turn trên **cùng run-1** resume `pebble-plow` qua `session/load` trên
process mới tinh: run/chat/session id không đổi, không session model song
song, `provider_status` chỉ là event row additive (replay sẽ hiện lại —
đúng lịch sử), warm turn không emit.

### `L-6` Silent API-key auth, no browser — PASS

Runner :47789 (binary dirty trên `7fc921a9` + silent-auth), account
`ba805096…` connected, `credentials.toml` có `windsurf_api_key`.

Boot prewarm — wire log + log của chính `devin.exe` (PID 13772):

```
16:49:13 send authenticate {methodId:"windsurf-api-key", _meta:{api_key:"[redacted]"}}
         devin.exe: ACP: authenticate #1 (had_credentials=true,
                    method_id=windsurf-api-key, meta_keys=["api_key"])
         devin.exe: ACP: API key provided directly via authenticate meta
16:49:16 recv {"id":2,"result":{}}          ← auth OK, busy 23ms
16:49:16 prewarm ready reason="boot"
```

**Không có** dòng `Starting browser-based PKCE authentication flow`, không
callback listener `127.0.0.1:xxxxx` nào được mở, không browser nào pop.
So sánh trước fix (`L-1`): `methodId:"devin-browser"` → PKCE listener +
browser kể cả khi `had_credentials=true`.

Turn sau đó trên warm handle: `session/new → sassy-reward`,
`set_config_option{model,"swe-2-high"}` + `{thought_level,"max"}` OK,
trả `SA-OK` ~11s.

Fallback probe trên binary thật (không đụng credentials của user):
`authenticate{windsurf-api-key, _meta.api_key:"bogus-…"}` →
`{"id":2,"error":{"code":-32603,"message":"…invalid api key"}}` — đúng
shape RPC error mà `devinAuthenticate` bắt để retry `devin-browser`.
Trình tự silent→browser đã chứng minh ở unit test
(`SilentAuthFailureFallsBackToBrowser`); live PKCE pop không chạy vì sẽ
mở browser thật trên máy user.

## 5. Finding F-1 — dead-process detection lag → FIXED live

**Quan sát ban đầu:** `taskkill /PID 21804` lúc ~15:43:3x nhưng dispatcher
chỉ thấy EOF lúc **15:46:41** — trễ ~142s. Turn-10 reuse dead handle, ghi
`session/load` vào pipe không đầu đọc → `turn_failed "stream closed: EOF"`.

**Cơ chế:** `devin.exe` spawn MCP children inherit stdout pipe handle của
ACP parent trên Windows → parent chết nhưng pipe vẫn mở → read loop không
nhận EOF → `closed` flag không set → handle trông "warm". Detection chỉ
dựa vào EOF là pre-existing blind spot (không phải regression).

**Fix:** watcher goroutine `cmd.Wait()` → `dispatcher.fail("devin acp
process exited")` ngay khi OS process exit — độc lập stdout EOF
(`devin_process.go`). `Wait()` cũng đóng parent-side pipes → unblock read
loop. `fail` idempotent nên race với readLoop/h.close() an toàn.

**Unit repro:** `TestEnsureDevinProcessExitMarksClosedWithoutStdoutEOF` —
fake-bin `sh -c` spawn `sleep 60 &` grandchild giữ stdout trước handshake,
kill parent → assert `isClosed()` trong 3s. Red trước fix (fail sau 3.07s),
green sau (0.08s).

**Live re-verify (build dirty trên `7fc921a9`, runner :47788):**

```
taskkill /PID 23348 (prewarmed ACP child)
09:19:44.295  provider_status "connecting"  ← dead handle detect NGAY
09:19:53.807  provider_status "ready"       (~9.5s cold handshake)
09:19:53.809  turn_started turn-260531
              session/load "brook-monkey" trên process mới → OK
09:19:59.028  message_delta "F1-OK" → token_usage
```

So với trước fix: turn-10 treo 142s → EOF fail. Sau fix: turn sau kill
đi cold path sạch, resume session cũ, trả lời trong ~15s tổng.

## 6. Log & Audit Evidence

- CA-965 (Task-438), CA-966 (Task-439), CA-967 (F-1), CA-968 (silent auth);
  commits `0859b6b3`, `7fc921a9`, `f0cd4b39`.
- `gitnexus_detect_changes` trước commit: 44 symbols, medium risk, đúng
  scope khai báo.
- Live artifacts: runner log `/tmp/fp-serve.log`, `/tmp/runner-sa.log`,
  event stream `/tmp/fp-events.ndjson`, `/tmp/sa-events.log`;
  devin.exe log `devin_20260924-164913_13772.log` (silent-auth proof).
- Cleanup: runner test PID 3396 + 19236 killed, ACP child 23348/13772
  terminated; `devin.exe` PID 16612 + 1708 (hệ thống) không đụng;
  `.livetest-ws`, `.livetest-ws2` đã xóa.

## 7. Verification Complete When

- [x] §2 automated xanh (Go focused 16/16; desktop 40/40; tsc; TUI).
- [x] `L-1` → `L-6` ticked trên runner+account thật.
- [x] F-1 fixed (process-exit watcher) + live re-verified: kill → cold path
      + `provider_status` ngay, `session/load` resume, ~15s tổng.
- [x] F-2 silent auth live: `windsurf-api-key`+`_meta.api_key` → auth ~3s,
      zero browser/PKCE listener; bogus key → RPC error (fallback trigger);
      warm turn + thought_level vẫn nguyên.
- [ ] Desktop DETECT MODEL chạy thật vào Supabase (unit-covered; cần
      operator bấm trên UI để xác nhận stale row `swe-2-max` bị disable).
- [ ] Live PKCE pop sau fallback chưa exercise (cần revoke credentials thật
      → sẽ mở browser trên máy user; quyết định bỏ qua, unit + probe
      invalid-key đã cover trigger).
