# LSP vs AST/UAST

# CP-63 \[Flowpilot] Kiến Trúc Code Intelligence: LSP, GitNexus, AST & UAST

Tài liệu này tổng hợp toàn bộ bản chất kỹ thuật, cơ chế hoạt động, sự phân tầng và chiến lược triển khai của **LSP (Language Server Protocol)** trong FlowPilot (đối chiếu với **Oh-My-Pi - OMP** và công nghệ **GitNexus** hiện có).



Xuất phát từ quá trình làm Flowpilot chúng ta quay lại động vào khái niệm cây cấu trúc của language

***



## 1. Sự Khác Biệt Giữa FlowPilot Và Cách OMP Đang Làm Với LSP

### Triết lý của Oh-My-Pi (OMP)

* **Triết lý:** *"A coding agent with the IDE wired in"*.
* **Cách OMP dùng LSP:** OMP nhúng LSP trực tiếp vào vòng lặp của Agent ở tầng Terminal để phục vụ mục đích **vi mô**:
  &#x20; 1\. *Live Compiler Diagnostics:* Agent sửa file xong, nhận lỗi compiler tức thì (<200ms) từ LSP để tự sửa typo/type error trước khi chạy lệnh test.
  &#x20; 2\. *Structurally Correct Refactor:* Tận dụng các lệnh LSP như `workspace/applyEdit`, `rename`, `goToDefinition` để refactor chuẩn xác tuyệt đối theo AST của trình biên dịch, thay vì dùng regex hay find-and-replace mù quáng.
  &#x20; 3\. *Hashline Edits:* Neo các thay đổi code theo content-hash để chống trôi dòng (drift).
* **Hạn chế của OMP:** OMP là một Terminal Agent đơn lẻ. Nó **chỉ có góc nhìn vi mô cục bộ**. Nó không có khái niệm luồng nghiệp vụ toàn hệ thống (Execution Flows), không đo lường được rủi ro lan truyền (Blast Radius) xuyên module, và không có cơ chế chặn cổng (Gate Enforcement). OMP cũng né tránh các ngôn ngữ phức tạp như Kotlin/Android.



OK bỏ qua việc OMP nó theo tư tưởng scope nhỏ. chúng ta chỉ học nó ở phần LSP. Tức là

* Thay vì AI code -> k biết ok hay không -> phải build + test mới biết code sai -> dôi khi sai chỉ dấu ;
* LSP giúp phát hiện luôn. k cần build gì hết. Như kiểu chúng ta dev - code trên IDE, viết sai cũ pháp -> đỏ ngay k cần build

-> Giảm đc rất nhiều token cho những lỗi typo kiểu này.



### Triết lý của FlowPilot (CP-63)

* **FlowPilot là nền tảng điều phối công nghệ toàn diện (AI Engineering Platform):** Có Flow Mode, Change Contract, FlowGate, Regression Engine.
* **Cách FlowPilot dùng LSP:** Không chỉ dừng lại ở việc hỗ trợ Agent gõ code, FlowPilot dùng LSP như một **Cổng Kiểm Duyệt Cú Pháp Tự Động (Automated Pre-Gate Compiler Guard)**:
  * Khi Agent ghi file (`EventFileChanged`), LSP server phân tích vi sai ngay lập tức.
  * Nếu có lỗi cú pháp hoặc sai kiểu dữ liệu, hệ thống tự động kích hoạt cơ chế `pendingGateRepromptPrompt` để bắt Agent sửa ngay trong RAM.
  * Nhờ đó, Agent không lãng phí token chạy các pipeline kiểm thử nặng nề (`./gradlew test`, `go test ./...`) khi code còn chưa compile nổi.

***

## 2. Kiến Trúc Hai Tầng Song Song: Macro (GitNexus) Và Micro (LSP)

Khi đưa LSP vào, FlowPilot **không loại bỏ GitNexus** mà nâng cấp hệ thống lên mô hình **Nhận thức mã nguồn&#x20;****2 tầng****&#x20;(Two-Tier Intelligence)**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 TẦNG 1: VĨ MÔ (MACRO PLANE) - GITNEXUS - <ĐÃ CÓ>            │
│                                                                             │
│  - Góc nhìn: Toàn bộ Repository, quan hệ liên module, đồ thị tri thức.      │
│  - Sức mạnh: Đo Blast Radius (Low/Med/High/Critical), nhận diện 300 luồng   │
│              nghiệp vụ (Execution Flows), bảo vệ kiến trúc dự án.           │
│  - Vị trí áp dụng: Giai đoạn Planning, Architecture, Gate duyệt Scope.      │
│  - Điểm yếu: Đồ thị tĩnh (stale khi code thay đổi), không bắt được lỗi typo.│
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                         Kiểm soát Scope & Rủi ro
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                   TẦNG 2: VI MÔ (MICRO PLANE) - LSP - <HỌC TỪ OMP VÀ THÊM>  │
│                                                                             │
│  - Góc nhìn: Cục bộ trong file và package đang mở trên RAM.                │
│  - Sức mạnh: Trình biên dịch thực thụ; phát hiện Syntax Error, Type Mismatch│
│              trong <200ms; cung cấp Goto Definition, Find References chuẩn.  │
│  - V vị trí áp dụng: Giai đoạn Coding, Live Edit Feedback Loop, Pre-Gate.   │
│  - Điểm yếu: Không hiểu ngữ cảnh kiến trúc hệ thống hay luồng dữ liệu rộng.│
└─────────────────────────────────────────────────────────────────────────────┘
```

| Tiêu chí           | **GitNexus** (Macro Plane)                      | **LSP Runtime** (Micro Plane)                        |
| ------------------ | ----------------------------------------------- | ---------------------------------------------------- |
| **Bản chất**       | Đồ thị quan hệ mã nguồn (Knowledge Graph)       | Trình biên dịch chạy ngầm (Live Compiler Backend)    |
| **Phạm vi**        | Toàn hệ thống, liên thư mục, luồng nghiệp vụ    | Từng dòng code, từng file, cấu trúc kiểu dữ liệu     |
| **Độ trễ**         | Vài giây đến vài phút (cần re-index)            | **< 200 mili-giây** (trực tiếp trên RAM)             |
| **Nhiệm vụ chính** | Ngăn sửa ngoài phạm vi (Scope Drift), đo rủi ro | Bắt lỗi cú pháp, sai type, thiếu import ngay lập tức |
|                    | Nói chung thằng này mang scope tổng quan        | Mang ý nghĩa bắt typo nhỏ nhất                       |

Mỗi thằng sẽ có 1 nhiệm vụ làm cho hệ thống 2 lớp ok hơn

***

## 3. Giải Thích Chi Tiết LSP Là Gì? **(Language Server Protocol)**

### 3.1 Nguồn gốc và bài toán $M \times N$

Trước năm 2016, nếu có $M$ trình soạn thảo (VS Code, Sublime, Vim, Emacs, FlowPilot) và $N$ ngôn ngữ (Go, TS, Python, Kotlin, Rust), dev phải viết $M \times N$ plugin tích hợp riêng lẻ.

Microsoft tạo ra **LSP (Language Server Protocol)** để chuẩn hóa: chỉ cần $M$ Client và $N$ Server nói chung 1 ngôn ngữ giao tiếp JSON-RPC.



Tức là thay vì cặp VsCode - GO, VsCode-TS, Flowpilot-GO, Flowpilot-TS thì giờ chỉ cần

VsCode / Flowpilot ---- TRUNG GIAN LSP ---- GO/TS 

* Như call Api vậy. dùng HTTP/json làm tiêu chuẩn
* Như MCP vậy



Còn dương nhiên chúng ta k cần care vụ Vscode hay gì hết

Ở đay chúng ta chỉ có 1\*N chứ không phải M\*N

Flowpilot -- LSP --- Go/java/ts/python/......



### 3.2 Cơ chế hoạt động trong hệ điều hành

LSP hoạt động dưới dạng **2 tiến trình (Processes) độc lập** giao tiếp với nhau qua đường ống **Standard I/O (stdin / stdout)**:

```
┌────────────────────────┐      Standard I/O Pipes       ┌────────────────────────┐
│     flowpilot.exe      │   (stdin / stdout không mạng) │      gopls / pyright   │
│     (LSP Client)       │ ◄───────────────────────────► │      (LSP Server)      │
└────────────────────────┘                               └────────────────────────┘
```

* **Định dạng gói tin:** Dùng Content-Length framing (tương tự HTTP đơn giản):
  ```http
  Content-Length: 124\r\n
  \r\n
  {"jsonrpc":"2.0","method":"textDocument/didChange","params":{...}}
  ```

&#x20; \`\`\`



Có 2 điểm tương đồng cực kỳ thú vị giữa cách ta gọi **LSP** và cách ta gọi **Codex / Claude / MCP** trong FlowPilot:

#### 3.2.1 Bản chất của RPC (Remote Procedure Call) là gì?



Xem [JSON-RPC](assets://./workspace/28ed06e3-7267-4a7b-8298-5e62b9bdd576/hTMNgVrLNtnOyhF4_Gko-)



RPC nghĩa là: *"Tôi coi một hàm ở một tiến trình khác (hoặc máy chủ khác) như thể một hàm nội bộ của tôi. Tôi gửi tên hàm (**`method`**), tham số (**`params`**), và nó trả về kết quả (**`result`**)."*

Chuẩn **JSON-RPC 2.0** mà LSP sử dụng có format như sau:

**Client gửi (Request):**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "textDocument/definition",
  "params": { "uri": "file:///main.go", "position": { "line": 42, "character": 10 } }
}
```

**Server trả lời (Response):**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": { "uri": "file:///user.go", "range": { "start": { "line": 15, "character": 5 } } }
}
```

Khác với REST API (phải dùng HTTP POST/GET/URL), JSON-RPC chỉ quan tâm đúng 3 thứ: `id`, `method`, và `params`.

***

#### 3.2.2 So sánh với cách FlowPilot gọi Codex / Claude

Trong FlowPilot, chúng ta có 2 tầng gọi AI Provider:

| Tiêu chí              | Gọi Cloud API (Anthropic / OpenAI REST)     | Gọi Local Process: **LSP** và **Codex App-Server**                                     |
| --------------------- | ------------------------------------------- | -------------------------------------------------------------------------------------- |
| **Giao thức**         | HTTP REST + SSE (Server-Sent Events)        | **JSON-RPC 2.0**                                                                       |
| **Môi trường truyền** | Qua mạng Internet (Cloud)                   | **Đường ống Standard I/O (stdin/stdout)** trong RAM                                    |
| **Độ trễ (Latency)**  | Chậm (500ms – vài giây do qua mạng)         | **Siêu nhanh (< 1 mili-giây)**                                                         |
| **Khớp lệnh**         | Request/Response một chiều hoặc stream text | **Song công (Bi-directional)**: Server có thể tự push notification về mà không cần hỏi |

Đặc biệt, nếu bạn để ý cách FlowPilot tích hợp với **Codex App-Server** (`apps/local-runner/internal/runner/codex_appserver.go`):

$\rightarrow$ **Codex App-Server cũng dùng chính xác JSON-RPC qua stdio y hệt như LSP!** FlowPilot spawn tiến trình `codex`, gửi JSON-RPC qua stdin và hứng kết quả từ stdout.

***

#### 3.2.3 "Bí mật" thú vị: Giao thức MCP (Model Context Protocol) sinh ra từ đâu?

Bạn đã quen với **MCP (Model Context Protocol)** dùng cho các tool Jira, Firebase, Google Drive trong FlowPilot.

Bản thân giao thức **MCP do Anthropic (Claude) công bố năm 2024 thực chất là sao chép (clone) 100% kiến trúc của LSP (do Microsoft tạo năm 2016)**:

* **LSP:** Chuẩn hóa giao tiếp giữa **IDE** $\longleftrightarrow$ **Trình biên dịch ngôn ngữ** (qua JSON-RPC stdio).
* **MCP:** Chuẩn hóa giao tiếp giữa **AI Agent** $\longleftrightarrow$ **Công cụ / Dữ liệu bên ngoài** (cũng qua JSON-RPC stdio).

Vì vậy, việc bạn xây dựng **LSP Client trong FlowPilot (CP-63)** về mặt lập trình mạng sẽ **giống hệt 100% cách FlowPilot đã viết MCP Client**! Đều là:

1. `exec.Command` bật tiến trình con.
2. Nối `stdin` và `stdout`.
3. Đọc/ghi các gói tin JSON có header `Content-Length`.



### 3.3 Vòng đời một phiên làm việc (Lifecycle)

1. **`initialize`****&#x20;(Khởi tạo):** Client gửi thư mục gốc của project (`rootUri`), Server trả về danh sách năng lực (Capabilities: hỗ trợ diagnostics, rename, definition...).
2. **`textDocument/didOpen`****:** Client báo file `main.go` đang được mở với toàn bộ nội dung ban đầu. Server load vào RAM.
3. **`textDocument/didChange`****:** Khi Agent vừa sửa file, Client gửi nội dung mới (hoặc đoạn diff vi sai).
4. **`textDocument/publishDiagnostics`****&#x20;(Điểm mấu chốt):** Server **tự động bắn thông báo (Push notification)** về Client mà không cần Client phải hỏi:
   ```json
   {
     "method": "textDocument/publishDiagnostics",
     "params": {
       "uri": "file:///path/to/main.go",
       "diagnostics": [
         {
           "range": { "start": { "line": 41, "character": 9 }, "end": { "line": 41, "character": 12 } },
           "severity": 1, // 1 = Error
           "message": "undefined: CalculateTotal"
         }
       ]
     }
   }
   ```

&#x20;  \`\`\`

5\. **`shutdown`****&#x20;&&#x20;****`exit`****:** Đóng kết nối an toàn khi thoát ứng dụng.



### 3.4 Tại sao LSP lại phản hồi siêu nhanh (<200ms)?

* **Giữ file trên RAM:** Server lưu AST và Symbol Table trên bộ nhớ động, không cần chờ ghi đĩa xong mới đọc.
* **Phân tích vi sai (Incremental Parsing):** Khi sửa dòng 42, Server chỉ tính toán lại scope của hàm chứa dòng 42, không build lại toàn bộ project.
* **Không sinh mã (No Codegen/Linking):** LSP chỉ dừng ở tầng Front-end của Compiler (Syntax + Type Check), không làm các bước nặng như gen bytecode, tối ưu hóa, dexing hay linking.

***

## 4. Sự Khác Biệt Giữa Các Ngôn Ngữ Khi Handle LSP

Không phải ngôn ngữ nào cũng hỗ trợ LSP như nhau:

### Nhóm 1: Các ngôn ngữ chuẩn mực (OMP hỗ trợ hoàn hảo)

* **Go (****`gopls`****):** Do chính Google Go Team xây dựng. Nhẹ, chuẩn xác tuyệt đối, tích hợp sẵn trong toolchain.
* **TypeScript / JavaScript (****`vtsls`****&#x20;hoặc&#x20;****`typescript-language-server`****):** Cực mạnh vì VS Code được xây dựng trên nền tảng này. Xử lý alias, barrel file, import module hoàn hảo.
* **Python (****`pyright`****):** Do Microsoft phát triển. Phân tích kiểu dữ liệu (Type Inference) siêu tốc trên Node.js runtime.
* **C / C++ (****`clangd`****):** Dựa trên LLVM frontend, cực kỳ mạnh và chuẩn mực.

###

### Nhóm 2: Trường hợp đặc thù — Android & Kotlin

Đây là lý do **OMP né tránh Kotlin**, và FlowPilot phải xây dựng giải pháp lai (Hybrid):

* **Bản chất của Android Project:** Không chỉ là code Kotlin thuần! Nó là tổ hợp của:
  * Code Kotlin + Code Java gọi chéo nhau.
  * File XML (Layout, AndroidManifest, Values).
  * Code do compiler tự gen (`R.id.*`, `R.layout.*`, ViewBinding, Dagger/Hilt component).
  * Compose Compiler Plugin (biến đổi AST tại compile-time).
* **Thực trạng LSP của Kotlin:**
  * JetBrains không mặn mà với LSP mở vì họ đã có **PSI/IntelliJ Platform nội bộ**.
  * `kotlin-language-server` (bản cộng đồng) chỉ phân tích được cú pháp Kotlin thuần, **không hiểu sâu context Android**. Nó thường xuyên báo lỗi giả hoặc không tìm thấy các class `R` hay hàm của Compose.
* **Chiến lược của FlowPilot cho Android (CP-63 P-7 & P-8):**
  * **Lớp 1 (LSP):** Dùng `kotlin-language-server` để quét lỗi cú pháp và type cơ bản trong 200ms.
  * **Lớp 2 (Gradle Fallback):** Khi LSP báo sạch lỗi cú pháp, kích hoạt `./gradlew compileDebugKotlin` để compiler xịn của Android thẩm định nốt phần XML, R class và Compose.

***

## 5. Các Service Ngôn Ngữ Riêng: Là Tải 3rd Lib Hay Thế Nào?

> **ĐÂY LÀ ĐIỂM CẦN LÀM RÕ TUYỆT ĐỐI:**

> **Language Server KHÔNG PHẢI là thư viện (Library/Dependency) của dự án!**

### Bản chất của Language Server

* Nó **không nằm** trong file `go.mod`, `package.json`, hay `build.gradle.kts` của bạn.
* Nó là **Công cụ dòng lệnh độc lập (Standalone CLI Tool / Daemon)** được cài đặt trên hệ điều hành của lập trình viên, giống như cách bạn cài `git`, `node` hay `docker`.

### Cách các Language Server tồn tại trên máy:

1. **Go:** Được cài qua Go toolchain:
   ```shellscript
   go install golang.org/x/tools/gopls@latest
   # Tạo ra file thực thi: gopls.exe (nằm trong $GOPATH/bin)
   ```
   1. **TypeScript:** Được cài qua npm global:
      ```shellscript
      npm install -g @vtsls/language-server
      # Tạo ra file thực thi: vtsls.cmd (nằm trong AppData/npm)
      ```
   &#x20;  \`\`\`

   3\. **Python:** Được cài qua npm hoặc pip:
   ````shellscript
   npm install -g pyright
   # Tạo ra: pyright-langserver.cmd
   &#x20;  ```
   ````
   1. **C / C++:** Nằm sẵn khi cài đặt bộ công cụ Clang/LLVM (`clangd.exe`).
   2. **Kotlin:** Là một gói binary Java chạy độc lập (`kotlin-language-server.bat`), bên trong chứa jar đã bundle sẵn compiler Kotlin.

###

### FlowPilot quản lý chúng như thế nào?

* FlowPilot **không tải thư viện về nhét vào source code** của bạn.
* Khi khởi động, FlowPilot dùng lệnh tìm kiếm đường dẫn hệ thống (`exec.LookPath("gopls")`, `exec.LookPath("pyright")`).
* Nếu tìm thấy binary trong máy $\rightarrow$ FlowPilot spawn tiến trình đó lên, như 1 binary PATH ĐỘC LẬP THÔI.
* Nếu người dùng chưa cài $\rightarrow$ FlowPilot log cảnh báo thân thiện: *"Chưa tìm thấy gopls, bạn hãy chạy lệnh&#x20;**`go install...`**&#x20;để bật tính năng kiểm tra lỗi tức thì"*, và tự động chuyển sang chế độ fallback bình thường mà không làm hỏng ứng dụng.

***

## 6. AST & UAST Nằm Ở Đâu Trong Bức Tranh Tổng Quát Này?

Để hiểu toàn bộ kiến trúc, hãy nhìn vào **đường đi của một đoạn mã** từ khi lập trình viên gõ cho tới khi Agent nhìn thấy lỗi:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          1. SOURCE CODE VĂN BẢN                             │
│       File: MainActivity.kt (Kotlin)          File: Utils.java (Java)       │
└──────────────────────┬───────────────────────────────────────┬──────────────┘
                       │ (Parser đọc chữ)                      │ (Parser đọc chữ)
                       ▼                                       ▼
┌──────────────────────────────────────┐    ┌─────────────────────────────────┐
│     2. KAST / Kotlin PSI             │    │       2. Java AST / Java PSI    │
│  (Cây cú pháp riêng của Kotlin)      │    │    (Cây cú pháp riêng của Java) │
└──────────────────────┬───────────────┘    └──────────────────┬──────────────┘
                       │                                       │
                       └───────────────────┬───────────────────┘
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                   3. UAST (Universal Abstract Syntax Tree)                  │
│  Tầng cây hợp nhất do JetBrains & Android Tooling tạo ra.                   │
│  Chuẩn hóa: UClass, UMethod, UCallExpression.                               │
│  (Nơi Android Lint hoạt động: 1 rule quét được cả Java và Kotlin).          │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      4. SEMANTIC MODEL & TYPE RESOLVER                      │
│  Bộ não của Compiler: Tra cứu kiểu dữ liệu, giải nghĩa biến, xác định lỗi.  │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                                       ▼ (Đóng gói thành JSON chuẩn)
┌─────────────────────────────────────────────────────────────────────────────┐
│                   5. GIAO THỨC LSP (Language Server Protocol)               │
│  Cái miệng để giao tiếp: Đóng gói danh sách lỗi thành JSON-RPC format
|                                                                qua stdio.   │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                                       ▼ (Bắn qua Standard I/O Pipe)
┌─────────────────────────────────────────────────────────────────────────────┐
│                  6. FLOWPILOT RUNNER & AGENT (Client)                       │
│  Nhận JSON lỗi -> Báo ngay cho Agent sửa lỗi trước khi chạy Test / Gate.   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Phép ẩn dụ dành cho kỹ sư:

* **AST / KAST:** Giống như **View Hierarchy** của một Activity riêng lẻ trong RAM.
* **UAST:** Giống như tầng **Giao diện dùng chung (Common UI Interface)** có thể hiển thị trên cả màn hình điện thoại lẫn màn hình đồng hồ.
* **LSP:** Giống như **REST API Endpoint**. Bản thân LSP không chứa dữ liệu; nó chỉ là cái cổng API để client từ bên ngoài query dữ liệu từ cây AST/UAST nằm trong RAM của Server.
* **Agent:** Là người dùng cuối (Client App), chỉ quan tâm gọi API và nhận danh sách lỗi để sửa.

***

## 7. Liên Tưởng Kiến Trúc: Sự Tương Đồng Giữa Runner Provider Adapter Và LSP

Một cách rất trực quan để hiểu bản chất của LSP trong FlowPilot là **so sánh với cơ chế đa Provider trong Runner hiện tại**:

### 7.1 Bảng đối chiếu song song

| Thành phần                                        | Kiến trúc AI Provider trong FlowPilot Runner                                                                              | Kiến trúc LSP trong Code Intelligence                                                                                                                      |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Bên tiêu thụ (Consumer)**                       | **`InteractiveService`****&#x20;(Runner Core):** Điều phối luồng, không cần biết model nào đang chạy.                     | **`LSPClient`****&#x20;trong Runner:** Điều phối diagnostics, không cần biết ngôn ngữ bên dưới là gì.                                                      |
| **Tầng trừu tượng (Abstraction Layer)**           | **`ProviderAdapter`****&#x20;Interface:** Chuẩn hóa các hàm `StartTurn`, `CancelTurn`, bắn ra `ProviderEvent`.            | **Giao thức LSP (JSON-RPC Protocol):** Chuẩn hóa các thông điệp `didOpen`, `didChange`, `publishDiagnostics`.                                              |
| **Các Adapter cụ thể (Concrete Implementations)** | - `ClaudeAdapter` (Anthropic API)&#xA;- `CodexAdapter` (OpenAI API)&#xA;- `GrokAdapter` (xAI API)&#xA;- `OpenCodeAdapter` | - `gopls` (Adapter cho Go)&#xA;- `vtsls` (Adapter cho TypeScript)&#xA;- `pyright` (Adapter cho Python)&#xA;- `kotlin-language-server` (Adapter cho Kotlin) |
| **Dữ liệu chuẩn hóa đầu ra**                      | `ProviderEvent` (`EventFileChanged`, `EventTurnCompleted`...)                                                             | `Diagnostic` (`file`, `line`, `character`, `severity`, `message`)                                                                                          |

### 7.2 Lợi thế vượt trội: FlowPilot không cần tự viết Adapter cho từng ngôn ngữ!

* **Với AI Provider (Claude, Codex, Grok):** Vì mỗi hãng AI dùng định dạng REST/SSE khác nhau, team FlowPilot - là mình - bắt buộc phải **tự tay code từng Adapter riêng - impl parts** (`claude_adapter.go`, `codex_event_mapper.go`, `grok_event_mapper.go`...).
* **Với LSP:** Microsoft và cộng đồng mã nguồn mở đã chuẩn hóa và hiện thực hóa toàn bộ các Adapter này. Họ viết impl parts luôn cho. ta chỉ việc navigate thôi
  * Google đã viết sẵn `gopls` cho Go.
  * Microsoft đã viết sẵn `pyright` cho Python.
  * LLVM đã viết sẵn `clangd` cho C/C++.
     
    &#x20; $\rightarrow$ **FlowPilot chỉ cần viết đúng DUY NHẤT 1 file LSP Client chuẩn** (`internal/lsp/client.go`). Client này có thể "cắm và chạy" (plug-and-play) với mọi Language Server hiện có trên thế giới.

### 7.3 Khả năng mở rộng (Extensibility)

Khi FlowPilot cần hỗ trợ một ngôn ngữ mới (ví dụ sau này thêm Rust hay Dart/Flutter), hệ thống **không cần viết thêm Parser hay Analyzer mới**. Ta chỉ cần khai báo một dòng trong `platform_registry.go`:

```go
"rust": { Binary: "rust-analyzer", Args: []string{} },
"dart": { Binary: "dart", Args: []string{"language-server"} },
```

Toàn bộ chu trình bắt lỗi, auto-reprompt và code intelligence sẽ tự động hoạt động mà không phải sửa đổi một dòng logic nào trong core Runner.

***

## 8. Tổng Kết Kiến Trúc Cho CP-63

1. **Macro Plane (GitNexus):** Giữ vai trò Thống đốc (Governor) — kiểm soát rủi ro kiến trúc, đo Blast Radius và chặn vi phạm Scope trong Change Contract.
2. **Micro Plane (LSP):** Giữ vai trò Lính gác (Guard) — kiểm tra cú pháp và kiểu dữ liệu trong **<200ms**, tự động reprompt Agent sửa lỗi ngay tại chỗ.
3. **Cơ chế Process:** FlowPilot làm Process Master, quản lý các LSP Server Process độc lập theo từng dự án/ngôn ngữ qua Standard I/O Pipes.
4. **Android Strategy:** Kết hợp **Kotlin LSP** (bắt nhanh lỗi cú pháp) + **Gradle Fallback** (bảo đảm an toàn tuyệt đối cho các thành phần đặc thù của Android).
5. **Thiết kế chuẩn hóa (Design Pattern):** Tương tự như cách `InteractiveService` trừu tượng hóa các AI Provider qua `ProviderAdapter`, `LSPClient` trừu tượng hóa mọi trình biên dịch ngôn ngữ qua giao thức chuẩn JSON-RPC.

###

Quan trọng nhất:**Các ông lớn (****`gopls`****,****`vtsls`****,****`pyright`****,****`clangd`****) đã chọn JSON-RPC rồi.**&#x46;lowPilot muốn nói chuyện được với`gopls`của Google hay`pyright`của Microsoft thì bắt buộc phải dùng chung ngôn ngữ JSON-RPC mà họ đã quy chuẩn.



# **KSP và APT là một thế giới hoàn toàn khác (LSP-AST-UAST)****.**

**ĐÚNG 100%! KSP và APT là một thế giới hoàn toàn khác.**

Nếu **LSP** là *"Giao thức giao tiếp để bắt lỗi cú pháp trong <200ms"*, thì **KSP và APT** thuộc về thế giới: **Sinh mã nguồn tự động lúc biên dịch (Compile-Time Code Generation)**.

***

### 1. Bản Chất Của APT và KSP Là Gì?

Trong Android, bạn rất hay dùng các thư viện như **Room, Retrofit, Dagger/Hilt, Moshi, Glide**. Bạn chỉ cần gắn annotation:

```kotlin
@Entity
data class User(val id: Int, val name: String)

@Dao
interface UserDao {
    @Query("SELECT * FROM user")
    fun getAll(): List<User>
}
```

Bạn **chỉ viết interface**, không hề viết code truy vấn SQL bằng tay! Vậy ai là người viết class `UserDao_Impl.java` thật sự để query SQLite?

$\rightarrow$ Đó chính là nhiệm vụ của **APT** và **KSP**! Chúng đọc các `@Annotation` và **tự động sinh thêm file code mới** vào thư mục `build/generated/`.

***

### 2. Sự Tiến Hóa: APT $\rightarrow$ KAPT $\rightarrow$ KSP

| Công nghệ                              | Sinh ra cho                         | Cơ chế hoạt động                                                                                                                                               | Điểm yếu chí mạng                                                                    |
| -------------------------------------- | ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| **APT** *(Annotation Processing Tool)* | **Java** thuần                      | Đi kèm trình biên dịch `javac`, đọc annotation và gen file `.java`.                                                                                            | Không hiểu ngôn ngữ Kotlin.                                                          |
| **KAPT** *(Kotlin APT)*                | **Kotlin** (Thời kỳ đầu)            | Để các lib Java (như Dagger) chạy được trên Kotlin, `kapt` phải biến toàn bộ code Kotlin thành các file **Java giả mạo (Java Stubs)** rồi mới ném cho APT đọc. | **Cực kỳ chậm!** Làm thời gian build Gradle tăng gấp đôi, gấp ba.                    |
| **KSP** *(Kotlin Symbol Processing)*   | **Kotlin** hiện đại (Google tạo ra) | **Đọc trực tiếp cây AST / Symbol của Kotlin Compiler**, không cần tạo Java Stubs, sinh code trực tiếp.                                                         | **Nhanh gấp 2 - 3 lần kapt.** Chuẩn mực hiện tại của Android (Room, Hilt, Moshi...). |

***

### 3. Bảng Phân Biệt Rạch Ròi 3 Khái Niệm Bạn Vừa Học

Để đầu óc không bị lẫn lộn giữa một "rừng" chữ viết tắt:

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│ 1. TẦNG CẤU TRÚC DỮ LIỆU (AST / UAST)                                            │
│    - Nhiệm vụ: Biểu diễn code thô thành cây đối tượng trong RAM.                 │
│    - Thời điểm: Khi bất kỳ công cụ nào parse text code.                          │
├───────────────────────────────────────────────────────────────────────────────────┤
│ 2. TẦNG SINH CODE TỰ ĐỘNG (KSP / APT / KAPT)                                     │
│    - Nhiệm vụ: Đọc @Annotation trên cây AST để tự đẻ ra file code mới.           │
│    - Thời điểm: Lúc bạn bấm RUN / BUILD Gradle (Compile-time).                   │
├───────────────────────────────────────────────────────────────────────────────────┤
│ 3. TẦNG GIAO TIẾP & BÁO LỖI (LSP)                                                │
│    - Nhiệm vụ: Bắn tin nhắn JSON-RPC báo "dòng 42 sai type kìa" qua stdio.       │
│    - Thời điểm: Ngay khi Agent đang gõ phím / ghi file (<200ms).                 │
└───────────────────────────────────────────────────────────────────────────────────┘
```

***

### 4. Mối Liên Hệ Đắt Giá: Tại Sao KSP Lại Làm Khó Thằng LSP Trong Android?

Hiểu được **KSP**, bạn sẽ hiểu thấu lý do vì sao tôi thiết kế **P-8 (Gradle Fallback)** trong kế hoạch **CP-63**:

1. **LSP Server chỉ đọc code trên RAM:** Khi Agent vừa sửa file, LSP chỉ phân tích code hiện có trong vài trăm mili-giây.
2. **LSP không thể chạy KSP:** KSP là một tác vụ nặng của Gradle (chạy mất vài giây đến vài chục giây). LSP không thể mỗi lần gõ một ký tự lại kích hoạt KSP chạy lại được.
3. **Hậu quả:**
   * Giả sử Agent vừa thêm một `@Dao` mới trong Room.
   * Code cần gọi class sinh tự động `AppDatabase_Impl`.
   * Thằng LSP mở file ra sẽ gạch đỏ kêu toáng lên: *"Ủa, class&#x20;**`AppDatabase_Impl`**&#x20;ở đâu ra, tôi không thấy?"*.
   * **Chỉ khi nào Gradle build thật sự chạy**, KSP mới sinh ra file đó trong thư mục `build/generated/ksp/` để hệ thống nhìn thấy.

$\rightarrow$ **Chính KSP/APT là nguyên nhân khiến Android bắt buộc phải có Gradle Fallback ở Phase cuối!** LSP giúp Agent dọn sạch các lỗi logic/type cơ bản, còn Gradle mới là chốt chặn cuối cùng kích hoạt KSP sinh mã hoàn chỉnh.

