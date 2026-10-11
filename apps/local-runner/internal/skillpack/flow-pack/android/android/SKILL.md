---
name: android
description: Master Android & KMP Engineering Skill. Central router, implementation playbook, and audit quality gate covering Architecture, SOLID, MVI Delegates, Compose UI, Adaptive Multi-Screen, Edge-to-Edge, Navigation, Memory Leaks, NDK C++20, and Clean Code standards. Use as input guide before implementing and as audit DoD checklist after completing tasks.
version: 1
---

# Android Master Playbook & Audit Gate

> **Mục đích:** Kỹ năng chủ lực (Master Skill) đóng vai trò trung tâm điều phối (Router) và cổng kiểm duyệt chất lượng (Audit Gate) cho toàn bộ quy trình phát triển Android/KMP.
>
> 🎯 **2 Chế độ hoạt động:**
> 1. **Input Mode (Trước khi code):** Định hướng kiến trúc, tra cứu quy chuẩn kỹ thuật và tải đúng các sub-skill chuyên sâu liên quan.
> 2. **Audit / DoD Mode (Sau khi code xong):** Rà soát mã nguồn đối chiếu với bộ tiêu chí xuất xưởng **Master DoD Checklist** để đảm bảo 0% lỗi kiến trúc, memory leak hay vi phạm SOLID.

---

## 🗺️ Navigation Map (Bản đồ điều hướng 13 Sub-Skills)

| Lĩnh vực trọng tâm | Sub-Skill cần nạp | Nhiệm vụ chính & Đường dẫn |
| :--- | :--- | :--- |
| **Kiến trúc Superapp / Module** | `android-superapp-architecture` | Composite builds, phân tầng Core/Foundation/Feature/Tooling, dependency matrix: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-superapp-architecture/SKILL.md) |
| **Kiến trúc Clean 5 Lớp & MVI** | `architecture` | 5 tầng Clean Architecture (`presentation -> domain <- data <- datasource`), MVI Delegate, SAM UseCase: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/architecture/SKILL.md) |
| **Nguyên lý SOLID Android** | `android-solid-principles` | Quy tắc & chẩn đoán SRP, OCP, LSP, ISP, DIP cho Android (cấm Data->Datasource, cấm concrete UseCase): [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-solid-principles/SKILL.md) |
| **Quét vi phạm SOLID** | `android-solid-scan` | Playbook cơ học phát hiện 147+ lỗi SOLID, anti-patterns, bảng đối chiếu mã lỗi: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-solid-scan/SKILL.md) |
| **Mã hóa chiến thuật & Quy chuẩn** | `coding` | Quy ước đặt tên, `@Composable` modifier, single immutable StateFlow, zero monkey-patch seams: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/coding/SKILL.md) |
| **Phong cách Clean Code** | `code-style` | KISS, DRY, YAGNI, null safety (cấm `!!`), immutability, bảo mật không hardcode secret: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/code-style/SKILL.md) |
| **Compose UI & Tái sử dụng** | `compose-ui` | Cây quyết định component, SceneStrategy vs AdaptiveLayout, tách biệt component dùng chung: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/compose-ui/SKILL.md) |
| **Đa màn hình & Tablet / Foldable** | `android-adaptive-multi-screen` | WindowSizeClass (Compact/Medium/Expanded), List-Detail Pane, Two-Pane, posture gập mở, Tier 1 quality: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-adaptive-multi-screen/SKILL.md) |
| **Edge-to-Edge & System Bars** | `edge-to-edge` | Adaptive insets, xử lý thanh điều hướng, status bar, IME bàn phím, tránh che khuất UI: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/edge-to-edge/SKILL.md) |
| **Điều hướng AppStart Nav3** | `android-starter-navigation` | `:core:navigation`, typed route keys, back-stack decoupling, AppRouter/AppNavigator, deep links: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-starter-navigation/SKILL.md) |
| **Phòng chống Memory Leak** | `android-memory-leak` | Vòng đời ViewModel/Scope, GC reachability, context leak, unregister listener, Bitmaps/Buffers: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-memory-leak/SKILL.md) |
| **Native C++20 & NDK Engine** | `ndk-modern-cpp-mastery` | RAII FdGuard, mprotect(PROT_NONE), zero-copy Direct ByteBuffer, POSIX O_SYNC, ASan/LSan: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/ndk-modern-cpp-mastery/SKILL.md) |
| **Tự kiểm tra lỗi thường gặp** | `common-mistakes` | Check-list 4Cs, chống loop sửa sai, không đoán mò schema, chia nhỏ task: [SKILL.md](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/common-mistakes/SKILL.md) |

---

## 🏗️ Section A: Architecture & Module Boundaries

> **Chi tiết tại:** [`android-superapp-architecture`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-superapp-architecture/SKILL.md) & [`architecture`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/architecture/SKILL.md)

### 1. Phân tầng Clean Architecture 5 Lớp
Mọi feature chuẩn mực được cấu trúc theo 5 module độc lập:
```text
┌────────────────────────────────────────────────────────┐
│             :feature:<name>:presentation               │
└───────────────────────────┬────────────────────────────┘
                            │ depends on
                            ▼
┌────────────────────────────────────────────────────────┐
│                :feature:<name>:domain                  │
│  (100% Pure Kotlin. SAM UseCases, Gateway interfaces)  │
└───────────────────────────▲────────────────────────────┘
                            │ depends on
┌───────────────────────────┴────────────────────────────┐
│                 :feature:<name>:data                   │
│  (Implements Gateways. OWNS DataSource Port Interfaces)│
└───────────────────────────▲────────────────────────────┘
                            │ depends on
┌───────────────────────────┴────────────────────────────┐
│              :feature:<name>:datasource                │
│  (Implements DataSource ports: Room, Retrofit, SAF)    │
└────────────────────────────────────────────────────────┘

Giao tiếp cross-feature: :feature:<name>:api (chỉ interface + public models)
```

### 2. Quy tắc Đảo ngược phụ thuộc (DIP Hard Rules):
- **CẤM:** Module `:data` phụ thuộc vào module `:datasource` trong Gradle (`build.gradle.kts` của `:data` tuyệt đối không có `implementation(project(":feature:*:datasource"))`).
- **CHUẨN:** Module `:data` định nghĩa và sở hữu các interface port (ví dụ: `XxxLocalDataSource`). Module `:datasource` phụ thuộc vào `:data` và hiện thực hóa interface đó (`XxxLocalDataSourceImpl`).
- **Composition Root:** Toàn bộ wiring cụ thể giữa Port và Implementation được thực hiện tại `:di` / `:app`.

---

## 💎 Section B: SOLID Principles & Clean Contracts

> **Chi tiết tại:** [`android-solid-principles`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-solid-principles/SKILL.md) & [`android-solid-scan`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-solid-scan/SKILL.md)

1. **SRP (Single Responsibility):**
   - ViewModel không nắm giữ `Context`, `Resources`, Views hay Toast.
   - Mỗi file UseCase chỉ đảm nhận đúng 1 hành động nghiệp vụ.
2. **OCP (Open/Closed):**
   - Mở rộng qua delegates, extension functions, và composition. Hạn chế cấu trúc `when`/`if-else` lồng nhau phức tạp.
3. **LSP (Liskov Substitution):**
   - Interface implementer không được throw `UnsupportedOperationException` trên các phương thức hợp đồng.
4. **ISP (Interface Segregation):**
   - Chia nhỏ interface theo nhu cầu gọi thực tế. Không tạo các "God Interface" chứa hàng chục phương thức không liên quan.
5. **DIP (Dependency Inversion):**
   - **Domain SAM Use Cases:** Mọi Use Case ở `:domain` **BẮT BUỘC** là `fun interface` (Single Abstract Method) kèm class `*Impl.kt` riêng.
     ```kotlin
     fun interface ExecuteActionUseCase {
         suspend operator fun invoke(param: Param): AppResult<Outcome>
     }
     class ExecuteActionUseCaseImpl @Inject constructor(
         private val gateway: ActionGateway
     ) : ExecuteActionUseCase { ... }
     ```
   - **CẤM Monkey-Patch Test Seams:** Tuyệt đối không tạo `internal var seam = usecase::invoke` hoặc tạo wrapper adapters nội bộ (`*LocalOps`) trong Repository/ViewModel để né class unmockable. Phải dùng interface và mock bằng Mokkery.

---

## 📱 Section C: Presentation & MVI ViewModel Delegate

> **Chi tiết tại:** [`architecture`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/architecture/SKILL.md) & [`coding`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/coding/SKILL.md)

### 1. Bộ ba MVI Hợp nhất (State, Intent, Effect)
Mỗi màn hình phải có đầy đủ:
- **`XxxScreenState.kt`**: Một data class duy nhất, bất biến (`val`). **CẤM** phân mảnh thành nhiều `StateFlow` độc lập (`_queue`, `_status`, `_uiState`) trong ViewModel.
- **`XxxIntent.kt`**: Sealed interface định nghĩa mọi ý định tương tác từ User và System. UI chỉ phát intent qua `sendIntent(intent)`.
- **`XxxEffect.kt`**: Sealed interface kế thừa `UiEffect` cho các sự kiện dùng một lần (Navigation, Toast, Snackbar, Activity Result Launchers).

### 2. ViewModel Delegate Composition
ViewModel kế thừa `ViewModel()` và triển khai pattern `MviCapability by delegates`:
```kotlin
@HiltViewModel
class XxxViewModel @Inject constructor(
    private val actionUseCase: ActionUseCase, // SAM interface!
    private val delegates: MviDelegates<XxxScreenState, XxxIntent, XxxEffect> = MviDelegates(XxxScreenState())
) : ViewModel(), MviCapability<XxxScreenState, XxxIntent, XxxEffect> by delegates {

    init {
        delegates.bindScope(viewModelScope)
        delegates.observeIntents { intent ->
            when (intent) {
                is XxxIntent.Submit -> handleSubmit(intent.data)
                is XxxIntent.Retry -> handleRetry()
            }
        }
    }
}
```

### 3. Phân tách Cấu trúc Screen Compose
- **`XxxScreen` (Entry Point):** Public `@Composable`. Nhận `viewModel`, thu thập state bằng `collectAsStateWithLifecycle()`, lắng nghe effect bằng `LaunchedEffect(Unit)`, quản lý ActivityResultLaunchers, truyền pure state + `viewModel::sendIntent` xuống Content.
- **`XxxContent` (State Router):** Internal `@Composable`. Không biết ViewModel. Phân luồng Loading/Error/Success.
- **`XxxSuccessView` / Components:** Internal UI rendering chi tiết.

---

## 🎨 Section D: UI, Compose, Multi-Screen & Edge-to-Edge

> **Chi tiết tại:** [`compose-ui`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/compose-ui/SKILL.md), [`android-adaptive-multi-screen`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-adaptive-multi-screen/SKILL.md) & [`edge-to-edge`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/edge-to-edge/SKILL.md)

1. **Chuẩn Compose Tacticals:**
   - Mọi `@Composable` **BẮT BUỘC** có tham số `modifier: Modifier = Modifier`.
   - Ưu tiên tái sử dụng design system primitives (`CommonText`, `CommonButton`, design tokens) thay vì dựng lại từ đầu.
2. **Multi-Screen & Adaptive Layouts:**
   - Phân biệt 2 cấp độ Adaptive:
     - **Navigation-Level:** Dùng `SceneStrategy` (Nav3 `rememberListDetailSceneStrategy()`) cho bố cục Master-Detail trên Tablet/Desktop.
     - **Screen-Level:** Dùng `AdaptiveLayout` cho biến đổi Portrait vs Landscape hoặc Phone vs Tablet trong phạm vi một màn hình.
   - Hỗ trợ các breakpoints WindowSizeClass: Compact (<600dp), Medium (600–839dp), Expanded (≥840dp).
   - Kiểm thử hiển thị trên Foldable (tư thế Tabletop, Book posture) và Desktop resizeable window.
3. **Edge-to-Edge & System Insets:**
   - Kích hoạt Edge-to-Edge toàn diện: UI vẽ tràn dưới Status Bar và Navigation Bar.
   - Sử dụng `WindowInsets.safeDrawing`, `statusBars`, `navigationBars`, hoặc `ime`.
   - Xử lý bàn phím ảo bằng `Modifier.imePadding()` hoặc `windowInsetsPadding(WindowInsets.ime)` để không che khuất text input và button hành động.

---

## 🧭 Section E: Navigation & Screen Flow

> **Chi tiết tại:** [`android-starter-navigation`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-starter-navigation/SKILL.md)

1. **Phân tách Điều hướng Decoupled:**
   - Điều hướng tập trung tại `:core:navigation`. Các feature không bao giờ phụ thuộc trực tiếp lẫn nhau.
   - Sử dụng Type-safe route keys (`AppRoute` / `NavKey`).
2. **Back-Stack State Management:**
   - Screen chỉ phát event điều hướng hoặc nhận callback từ NavGraph; không tự ý khởi tạo Router bên trong Composable cấp thấp.
   - Hỗ trợ Deep links và State preservation khi quay trở lại back-stack.

---

## 🛡️ Section F: Lifecycle, Memory Leak & Performance Safety

> **Chi tiết tại:** [`android-memory-leak`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/android-memory-leak/SKILL.md)

1. **Vòng đời State Collection:**
   - **TUYỆT ĐỐI KHÔNG** dùng `collectAsState()` thông thường cho dữ liệu UI. **BẮT BUỘC** dùng `collectAsStateWithLifecycle()` để tự động dừng collection khi app ở background.
2. **Coroutine Scopes & Context Leaks:**
   - Chỉ sử dụng `viewModelScope` trong ViewModel. Không dùng `GlobalScope` hoặc giữ tham chiếu `Activity`/`View` Context trong Singleton hay ViewModel.
   - Hủy đăng ký (unregister) các Callback/Listener trong `onCleared()` hoặc `DisposableEffect(onDispose)`.
3. **Direct Buffers & Memory Footprint:**
   - Sử dụng Zero-Copy Direct ByteBuffers cho streaming hoặc NDK crypto. Thu hồi tài nguyên bộ nhớ kịp thời, tránh rò rỉ native heap.

---

## ⚡ Section G: Native C++20 & NDK Engine

> **Chi tiết tại:** [`ndk-modern-cpp-mastery`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/ndk-modern-cpp-mastery/SKILL.md)

1. **RAII & FdGuard:**
   - Quản lý file descriptor qua RAII guards. Không để rò rỉ descriptor khi JNI ném Exception.
   - Tách rời quyền sở hữu qua `pfd.detachFd()` từ Kotlin, bàn giao quyền đóng duy nhất cho `FdGuard` bên C++.
2. **Bảo mật Bộ nhớ:**
   - Khóa trang bộ nhớ chứa key nhạy cảm bằng `mlock()` và bảo vệ bằng `mprotect(PROT_NONE)`.
   - Xóa sạch bộ nhớ nhạy cảm bằng `OPENSSL_cleanse()` hoặc volatile memset trước khi giải phóng.
3. **Hiệu năng & Kiểm thử Native:**
   - Sử dụng POSIX Direct I/O (`O_SYNC`), lock-free ring buffer cho audit logs.
   - Biên dịch và xác minh không rò rỉ bộ nhớ với ASan (AddressSanitizer) và LSan (LeakSanitizer).

---

## 🧹 Section H: Coding Style, Hygiene & Agent Anti-Patterns

> **Chi tiết tại:** [`code-style`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/code-style/SKILL.md), [`coding`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/coding/SKILL.md) & [`common-mistakes`](file:///Users/tiendat/Desktop/android/PrivateVault/.agents/skills/common-mistakes/SKILL.md)

1. **Clean Code & Null Safety:**
   - Cấm lạm dụng toán tử ép kiểu non-null `!!`. Dùng `?.let`, `?: return`, hoặc `requireNotNull`.
   - Hàm ngắn gọn (< 40 dòng), đơn trách nhiệm.
   - Không commit hardcode API secrets hoặc credentials.
2. **Cognitive Agent Self-Check:**
   - Xác nhận 4Cs (Context, Command, Constraints, Criteria) trước khi viết code.
   - Chia nhỏ các yêu cầu phức tạp, không làm nhiều việc cùng lúc.
   - Không lặp lại vòng lặp sửa lỗi mù quáng (Infinite Failure Loop); tìm đúng nguyên nhân gốc rễ và sửa chính xác tại tầng vi phạm.

---

## ✅ Section I: Master Definition of Done (DoD Checklist)

*Dùng làm bảng kiểm soát trước khi đóng task hoặc audit code:*

### 1. Kiến trúc & Phân lớp (Architecture & Gradle)
- [ ] Module `:data` **KHÔNG** phụ thuộc vào `:datasource` trong file `build.gradle.kts`.
- [ ] Interface DataSource Port nằm trong `:data`, được implement bởi `:datasource`.
- [ ] Mọi Use Case trong `:domain` đều là SAM `fun interface` kèm file `*Impl.kt` riêng.
- [ ] ViewModel chỉ inject SAM Use Case interfaces, **KHÔNG** inject concrete class.
- [ ] Không có circular dependencies giữa các module feature.

### 2. Presentation & MVI Delegate
- [ ] ViewModel sử dụng pattern `MviCapability<State, Intent, Effect> by delegates`.
- [ ] State màn hình được gom thành **1 immutable Data Class duy nhất** (`UiState`).
- [ ] Tương tác được mô hình hóa qua sealed interface `XxxIntent`.
- [ ] Sự kiện one-off được mô hình hóa qua sealed interface `XxxEffect`.
- [ ] Tuyệt đối **KHÔNG** có monkey-patch `var` function seams trong ViewModel hay Repository.

### 3. UI, Compose & Đa màn hình
- [ ] Mọi `@Composable` đều có tham số `modifier: Modifier = Modifier`.
- [ ] Sử dụng `collectAsStateWithLifecycle()` thay cho `collectAsState()`.
- [ ] Phân tách rõ ràng: `XxxScreen` (Stateful/Binding) $\to$ `XxxContent` (Stateless).
- [ ] Giao diện hỗ trợ Edge-to-Edge và xử lý insets (Status Bar, Navigation Bar, IME bàn phím).
- [ ] Bố cục giao diện hiển thị đúng trên các kích thước màn hình (Phone, Tablet/Foldable).

### 4. Kiểm thử & Chất lượng (Testing & Hygiene)
- [ ] Không sử dụng toán tử `!!`.
- [ ] Unit test cho UseCases và ViewModels mock trực tiếp các interface bằng Mokkery.
- [ ] Các callback, listeners, coroutine jobs được hủy an toàn, không gây memory leak.
- [ ] Nếu có C++/NDK: đảm bảo quản lý file descriptor bằng RAII, xóa bộ nhớ nhạy cảm trước khi free.
- [ ] Mã nguồn biên dịch sạch sẽ, không có warning nghiêm trọng.
