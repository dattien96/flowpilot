---
version: 6
name: android-superapp-architecture
description: >-
  Kiến trúc và chuẩn chia module cho ứng dụng Android quy mô lớn (Superapp).
  Bao gồm quy tắc composite build, Core vs Foundation vs Tooling, Feature API
  cross-feature communication, Hilt DI composition, chiến lược Database đa module,
  và ma trận dependency.
---

# Android Superapp Architecture — Skill Guide

> **Dùng khi nào:** Thiết kế module mới, review kiến trúc, onboard dự án lớn, hoặc
> quyết định "cái này đặt ở đâu?".
>
> **Không dùng khi:** Viết UI thuần (xem `compose-ui`), soi SOLID (xem `android-solid-scan`).

---

## 1. Tổng quan cấu trúc Module & Phân lớp

Kiến trúc chia thành **ba repo composite** + **bốn lớp module**. Mục tiêu: giảm build time,
tránh circular dependency, tái sử dụng Foundation cross-app, và giữ feature độc lập.

```text
┌──────────────────────── AppStartTooling ─────────────────────────┐
│  conventions · libs.versions.toml · KSP · dependency policies    │
└──────────────────────────────────────────────────────────────────┘

┌────────────────── AppStartCoreFoundation (0% Hilt) ──────────────┐
│  domainfoundation · appdesign · presentation                     │
│  list/image/security/storage:  api (+ ui-port) → impl            │
│  network-retrofit (provider-specific)                            │
└──────────────────────────────────────────────────────────────────┘

┌─────────────────────────── AppStart ─────────────────────────────┐
│                                                                  │
│  :app  →  :di  →  binds everything                               │
│              │                                                   │
│              ├─► :core:{domain,navigation,network,database,…}    │
│              │         ▲                                         │
│              │         └── :core:di-qualifiers (leaf)            │
│              │                                                   │
│              └─► :feature:<name>                                 │
│                    api ←──────── other features                  │
│                    presentation → domain ← data ← datasource     │
│                                        │                         │
│                                        └──► foundation *-api     │
│                                                                  │
│  Foundation *-impl và provider concrete chỉ vào qua :app/:di     │
│  (và :core:network đối với network-retrofit)                     │
└──────────────────────────────────────────────────────────────────┘
```

---

## 2. Ba Repo Composite

```text
AppStartTooling              ← convention plugins, version catalog, KSP
AppStartCoreFoundation       ← capability kỹ thuật cross-app (0% Hilt)
AppStart                     ← sản phẩm: :app, :core, :di, :feature:*
```

`AppStart/settings.gradle.kts` dùng `includeBuild` cho Tooling và Foundation, kèm
`dependencySubstitution` map artifact `com.datnguyen.a76.foundation:*` → project Foundation.

### 2.1 Tooling (`AppStartTooling`)

| Thành phần | Việc làm |
|---|---|
| `gradle/libs.versions.toml` | Version catalog dùng chung App + Foundation |
| `build-logic` convention plugins | Chuẩn hóa Android/Kotlin/Compose/Hilt/KSP |
| `feature-dependency-policy` | Chặn dependency sai hướng giữa feature layers |
| `DiBoundaryPolicy` | Cưỡng chế quy tắc `:di` / qualifier |
| `ksp-annotations` + `ksp-processor` | Sinh binding (ví dụ `@UseCase`) |

Convention plugin chính:

| Plugin id | Dùng cho |
|---|---|
| `appstart.kotlin.library` | Pure Kotlin (`:domain`, `:api`, qualifiers…) |
| `appstart.android.library` | Android library (`:data`, `:datasource`, `:presentation`…) |
| `appstart.android.compose` | Presentation Compose |
| `appstart.hilt` | Hilt + KSP trên module được phép |
| `appstart.feature-dependency-policy` | Policy biên feature |

**Tooling KHÔNG chứa business feature và KHÔNG chứa Hilt `@Module` của app.**

### 2.2 Foundation (`AppStartCoreFoundation`)

Capability **kỹ thuật tái sử dụng cross-app**. **KHÔNG biết Hilt.** App khác tái sử dụng 100%
qua composite build; **KHÔNG** tái sử dụng `:core` của AppStart.

Module Foundation hiện có:

```text
:modules:appdesign
:modules:domainfoundation          ← AppResult, primitive domain dùng chung
:modules:presentation              ← base presentation / helper dùng chung

:modules:list:list-api / list-ui-port / list-compose-impl / list-recyclerview-impl
:modules:image:image-api / image-ui-port / image-impl
:modules:security:security-api / security-impl
:modules:storage:storage-api / storage-impl
:modules:network:network-retrofit  ← provider-specific; chỉ infra app consume
```

### 2.3 App Product (`AppStart`)

Chứa: `:app`, `:di`, `:core:*`, `:feature:*`. Xem §5–§8.

---

## 3. Quy tắc phân định :core vs :foundation

| Tiêu chí | `:foundation` | `:core` |
|---|---|---|
| **Repository** | `AppStartCoreFoundation` | `AppStart` |
| **Phạm vi tái sử dụng** | **Cross-app** — nhiều ứng dụng khác nhau | **Intra-app** — chỉ trong cùng 1 ứng dụng |
| **Biết Hilt?** | ❌ Không — giữ độc lập DI framework | ⚠️ Được dùng `@Inject`, nhưng **KHÔNG** khai `@Module` |
| **Ví dụ** | `image-api`, `security-api`, `network-retrofit` | `:core:network`, `:core:database`, `:core:navigation` |

> **`:core` KHÔNG phải Foundation layer thứ hai.** Đừng thiết kế `:core` với kỳ vọng
> App B dùng lại nó. Logic thật sự cần chia sẻ cross-app thuộc về Foundation.

Khi App B mới được tạo:
- **Tái sử dụng 100% Foundation** qua Gradle Composite Build
- **Tự xây `:core` riêng**, tự xây `:di` riêng phù hợp nghiệp vụ

### `:core` modules

| Module | Việc làm |
|---|---|
| `:core:domain` | Primitive / policy app-level (không phình thành god model) |
| `:core:navigation` | Host navigation contracts / graph registration hooks |
| `:core:network` | Cấu hình mạng app; được phép dùng `network-retrofit` |
| `:core:database` | Xây Room/DB app-level / factory provider |
| `:core:security` | Wiring bảo mật phía app quanh Foundation security |
| `:core:di-qualifiers` | **Chỉ** `@Qualifier` annotation — mọi module được ref |

---

## 4. Foundation api / ui-port / impl Pattern

| Hậu tố | Nội dung | Ai được depend |
|---|---|---|
| `*-api` | Interface + model request/response thuần Kotlin | Feature domain/presentation/data |
| `*-ui-port` | Port UI trung lập engine (list/image) | Presentation |
| `*-impl` / `*-compose-impl` | Adapter provider (Coil, RV, Keystore…) | **Chỉ** `:app` / `:di` |
| `network-retrofit` | Retrofit stack | Chỉ `:core:network`, không feature |

```text
Feature / Presentation ──► foundation:*-api  (và ui-port khi cần)
:app / :di             ──► foundation:*-impl (chọn và bind)
```

---

## 5. Cấu trúc một Feature (chuẩn đích)

### 5.1 Gradle modules

```text
:feature:<name>:api            ← public Kit: interface + model tối thiểu
:feature:<name>:presentation   ← Compose / UI / ViewModel
:feature:<name>:domain         ← use case, gateway/repository ports, domain models
:feature:<name>:data           ← repository impl, datasource ports, mapper
:feature:<name>:datasource     ← Retrofit DTO, Room entity/DAO, provider adapters
```

### 5.2 Chiều phụ thuộc trong feature

```text
presentation ──► domain ◄── data ◄── datasource
                   ▲
                   │
                  api   ◄── feature khác (chỉ api)
```

### 5.3 Quy tắc nội dung

| Module | Được | Không được |
|---|---|---|
| `:api` | fun interface, model tối thiểu, `AppResult` | Android, Hilt, Retrofit, Room, use case impl |
| `:domain` | ports, SAM use case, pure Kotlin models; impl `:api` khi cần | Android/Compose/Retrofit/Room/Hilt `@Module` |
| `:data` | repository impl, datasource **interfaces**, mappers | UI; lộ DTO ra presentation |
| `:datasource` | Retrofit/Room/provider concrete | Domain rules; bị domain depend ngược |
| `:presentation` | UI state, ViewModel, navigation feature graph | Gọi Retrofit/Room trực tiếp; depend feature khác `:domain`/`:data` |

### 5.4 Ví dụ sống: Home

```text
:feature:home:api            GetHomeProductSummary, HomeProductSummary
:feature:home:domain         HomeGateway, use cases, GetHomeProductSummaryImpl
:feature:home:data           repository + datasource ports
:feature:home:datasource     Retrofit/Room adapters → :core:network / :core:database
:feature:home:presentation   Home screens / ViewModels
```

---

## 6. Feature API & Cross-Feature Communication

### 6.1 Pattern

```text
:feature:B:domain|presentation ──► :feature:A:api
:di / :app                     ──► bind A.api → A.domain|data impl
```

**Cấm:**

```text
:feature:B ──X──► :feature:A:domain | :data | :presentation | :datasource
```

### 6.2 Ví dụ: Cart API

```kotlin
// :feature:cart:api — thuần Kotlin, không Android
fun interface GetCartItemCount {
    suspend operator fun invoke(): Int
}

// :feature:cart:domain — impl port
class GetCartItemCountImpl @Inject constructor(
    private val cartRepository: CartRepository
) : GetCartItemCount {
    override suspend fun invoke(): Int = cartRepository.getItemCount()
}

// :feature:home:presentation — consume qua api
@HiltViewModel
class HomeViewModel @Inject constructor(
    private val getCartItemCount: GetCartItemCount
) : ViewModel() {
    // dùng getCartItemCount() không biết Cart impl
}

// :di — bind
@Binds fun bindGetCartItemCount(impl: GetCartItemCountImpl): GetCartItemCount
```

### 6.3 Foundation api/impl vs Feature api

| | Foundation `*-api` / `*-impl` | `:feature:<name>:api` |
|---|---|---|
| Phạm vi | Cross-app kỹ thuật | Intra-app nghiệp vụ giữa feature |
| Owner | Foundation team / platform | Feature team sở hữu bounded context |
| Ví dụ | `image-api`, `storage-api` | `GetHomeProductSummary`, `AddToCart` |
| Impl bind tại | `:app` / `:di` chọn Coil/RV/… | `:di` bind use case/gateway của feature |

---

## 7. Composition Root: :app, :di, :core:di-qualifiers

### 7.1 Ba vai trò DI

| Module | Loại Gradle | Chứa gì | Chiều mũi tên |
|---|---|---|---|
| **`:di`** | `android.library` + Hilt | Toàn bộ `@Module`, `@Provides`, `@Binds`, `@EntryPoint` | **Nó → mọi module.** Chỉ `:app` ref nó |
| **`:core:di-qualifiers`** | `kotlin.library`, 0 dependency | Chỉ `@Qualifier` annotation | **Mọi module → nó.** Leaf tuyệt đối |
| **`:app`** | `android.application` | `@HiltAndroidApp`, `@AndroidEntryPoint`, feature flags | Ref `:di` |

```text
                    :core:di-qualifiers          ← leaf, ai cũng ref được
                     ▲    ▲      ▲    ▲
                     │    │      │    │
   :core:network ────┘    │      │    └──── :feature:home:presentation
   :core:security ────────┘      └────────── :feature:home:domain
                     ▲                              ▲
                     │                              │
                     └──────── :di ─────────────────┘   ← join, ref mọi thứ
                                ▲
                                │
                              :app
```

### 7.2 Vì sao `@Qualifier` KHÔNG ở chung `:di` được

**a. Vòng phụ thuộc — Gradle chặn thẳng.** `:di` ref `:feature:home:presentation` để
`@Binds`. Nếu `:feature:home:presentation` cần `@SecureStore` mà nó nằm ở `:di`
→ circular dependency → build đỏ.

**b. `:di` bắt buộc là Android library.** Nhiều module đang là pure Kotlin
(`core/domain`, `feature/home/domain`). Nếu qualifier ở `:di`, module thuần Kotlin
bị kéo thành module Android chỉ để gọi tên annotation.

**c. Tần suất thay đổi.** `:di` đổi liên tục; qualifier vài tháng mới thêm.
Nếu mọi module ref `:di`, thêm `@Provides` → recompile cả repo.

### 7.3 DiBoundaryPolicy — Cưỡng chế bằng plugin

Plugin `com.datnguyen.a76.appstart.di-boundary-policy` áp ở **root project** — không phải
từng module. Nếu `:core:network` thêm `implementation(project(":di"))`:

```text
* What went wrong:
:core:network must not depend on :di. Only :app may, because :di depends
on every module and the reverse edge is a cycle.
```

### 7.4 Năm quy tắc DI

| # | Quy tắc |
|---|---|
| **R1** | Không viết `@Module` khi `@Inject constructor` làm được |
| **R2** | `@Module` viết tay còn lại → về `:di` |
| **R3** | `@Qualifier` → `:core:di-qualifiers`, không nơi nào khác |
| **R4** | Type `internal` → module sở hữu mở factory public; binding vẫn ở `:di` |
| **R5** | `buildConfigField` nằm ở module khai variant làm nó biến thiên |

---

## 8. Chiến lược Database Đa Module

### 8.1 Sự thật ngược trực giác

Ở quy mô super app, thiết kế phổ biến **KHÔNG** phải một `AppDatabase` khổng lồ:

```text
KHÔNG:  :core:database  →  AppDatabase(200 entity, 60 DAO, 1 file .db, version 87)
CÓ:     mỗi bounded context một database riêng
        :core:database chỉ giữ HẠ TẦNG — không sở hữu entity nào
```

### 8.2 Tách theo vòng đời dữ liệu

| Loại | Ví dụ | Khi logout | Migration | Nên ở đâu |
|---|---|---|---|---|
| **Durable / user-owned** | đơn nháp, tin nhắn chưa gửi | giữ / xoá có chủ ý | Bắt buộc viết + test | DB riêng |
| **Cache** | feed, sản phẩm, banner | xoá thoải mái | destructive OK | DB riêng |
| **Preference / flag** | đã xem onboarding, theme | tuỳ | — | DataStore, **không Room** |

### 8.3 Phân tầng module

```text
:core:database                    ← HẠ TẦNG. KHÔNG có @Entity nào.
   ├─ AppDatabaseFactory          builder chung
   ├─ Encryption (SQLCipher), WAL, QueryCallback logging
   └─ Set<DatabaseBuilderCustomizer>  (multibinding)

:feature:order:data     ← OrderDatabase   (internal) + entity + DAO (internal)
:feature:chat:data      ← ChatDatabase    (internal)
:core:usersession:data  ← SessionDatabase (durable, tách hẳn khỏi cache)
```

**Luật then chốt:** `@Database`, `@Entity`, `@Dao` để `internal`. Thứ duy nhất
public ra khỏi module là interface `LocalDataSource` do domain định nghĩa.

### 8.4 Multi-account

```kotlin
// Tên file DB mang scope user → đổi account = MỞ FILE KHÁC
val name = "order_${session.scopeId}.db"
```

### 8.5 Migration rules

1. `exportSchema = true` và commit JSON vào git
2. `@AutoMigration` cho thêm cột/bảng; viết tay cho đổi kiểu/tách bảng
3. Test migration bắt buộc: từng cặp version + chuỗi v1→vNow
4. Version cục bộ theo từng DB (OrderDatabase v7, ChatDatabase v3)
5. CI chặn: PR đổi `@Entity` mà không kèm schema JSON → fail

### 8.6 Bảng quyết định

| Câu hỏi | "Có" ⇒ |
|---|---|
| Dữ liệu mất đi user có mất gì không? | DB durable, migration nghiêm túc, **cấm** destructive |
| Server tải lại < 1s? | DB cache, destructive hợp lệ, có TTL |
| Chỉ là cờ boolean / lựa chọn? | DataStore, **không** Room |
| Module có thể thành DFM? | **Bắt buộc** DB riêng, ngay từ đầu |
| Luôn phải JOIN với bảng feature khác? | Cùng bounded context — **đừng** tách |
| Chứa PII / token / chat? | Cân nhắc SQLCipher |
| Có chuyển tài khoản? | Tên file DB mang scope user |

---

## 9. Dependency Matrix

| From → To | Cho phép? |
|---|---|
| presentation → domain | ✅ |
| data → domain | ✅ |
| datasource → data (ports) | ✅ |
| domain → feature api (cùng feature) | ✅ khi impl public port |
| feature B → feature A **api** | ✅ |
| feature B → feature A domain/data/presentation | ❌ |
| feature → foundation `*-api` | ✅ |
| feature → foundation `*-impl` | ❌ |
| feature → `:core:network` / database | Chỉ `:datasource` |
| feature → `:di` | ❌ |
| bất kỳ → `:core:di-qualifiers` | ✅ |
| `:app` → `:di` | ✅ (duy nhất) |
| `:di` → feature/core/foundation impl | ✅ để bind |

Kiểm tra bằng grep:

```bash
# Tìm dependency sai hướng
grep -rn 'project(":' --include="*.kts" .

# Xác nhận không feature nào ref :di
grep -rn 'project(":di")' --include="*.kts" feature/
```

---

## 10. Checklist Feature Mới

1. Tạo `:presentation` / `:domain` / `:data`; thêm `:datasource` nếu có Retrofit/Room/provider.
2. Thêm `:api` **chỉ khi** feature khác cần gọi nghiệp vụ của feature này.
3. Gắn convention + `feature-dependency-policy`.
4. Domain thuần Kotlin; map DTO/entity/domain/UI tại biên.
5. Không viết `@Module` trong feature — đăng ký bind tại `:di`.
6. Qualifier mới → `:core:di-qualifiers`, không bỏ vào `:di`.
7. Consume Foundation qua `*-api`; để `:app`/`:di` chọn `*-impl`.
8. Không depend `:domain` của feature khác — dùng `:api`.
9. Database: quyết định Durable vs Cache (§8.6), tên file mang scope user.
10. `exportSchema = true`, thêm migration test.

---

## FORCE Rules

1. **Cấm cyclic dependency.** Dùng `api/impl` pattern để phá vòng lặp. DiBoundaryPolicy cưỡng chế.
2. **Foundation KHÔNG chứa Hilt annotations.** Không `@Inject`, `@Module`, `javax.inject.*`.
   Foundation DI-neutral → app bắc cầu qua `:di`.
3. **`@Qualifier` chỉ ở `:core:di-qualifiers`.** Đặt nơi khác → circular risk, Android leak vào Kotlin.
4. **`@Module` viết tay chỉ ở `:di`.** KSP-generated module ở nguyên tại chỗ. `@Inject constructor` ưu tiên.
5. **Không dùng 1 AppDatabase khổng lồ.** Tách DB theo bounded context + vòng đời dữ liệu.
6. **`@Database`, `@Entity`, `@Dao` phải `internal`.** Public surface là interface `LocalDataSource`.
7. **Feature không depend `:domain`/`:data`/`:presentation` của feature khác.** Chỉ `:api`.
8. **Foundation `*-impl` chỉ vào qua `:app`/`:di`.** Feature chỉ biết `*-api`.

---

## Tài liệu tham khảo

| File | Nội dung |
|---|---|
| `references/module-dependency-matrix.md` | Ma trận dependency chi tiết + grep commands |
| `references/core-vs-foundation-hilt.md` | Hilt DI rules R1-R5, DiBoundaryPolicy, internal factory pattern |
| `references/database-strategy.md` | Database đa module chi tiết, migration, multi-account, SQLCipher |
| `examples/feature-cart-api-communication.kt` | Demo cross-feature communication hoàn chỉnh |
| `examples/module-structure-example.md` | Ví dụ Home feature đầy đủ |
| `examples/database-setup-example.kt` | AppDatabaseFactory + OrderDatabase internal |
