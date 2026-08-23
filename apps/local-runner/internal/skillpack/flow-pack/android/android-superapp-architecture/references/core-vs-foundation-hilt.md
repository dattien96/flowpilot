# Core vs Foundation & Hilt DI Rules

Một trong những vấn đề thường gặp ở kiến trúc đa module quy mô lớn là ranh giới giữa `:core` và `:foundation`, cũng như cách thức tích hợp Dependency Injection (DI) bằng Hilt/Dagger mà không gây ra coupling quá mạnh.

## 1. Sự khác biệt cốt lõi

### `:foundation` (Thư viện nền tảng)
- Là các thành phần **hoàn toàn độc lập** với cấu trúc nghiệp vụ (business) của ứng dụng.
- Ví dụ: Hệ thống UI component (Design System), Network Client (OkHttp/Retrofit wrappers), Logger, Utilities.
- Phải được thiết kế để có thể "bê" sang một dự án Android khác mà không cần sửa đổi gì.
- **TUYỆT ĐỐI KHÔNG** chứa bất kỳ logic nào liên quan đến Dependency Injection Framework cụ thể (Dagger, Hilt, Koin). Nếu thư viện cần khởi tạo, nó phải cung cấp dạng Builder hoặc Factory.

### `:core` (Thành phần nghiệp vụ cốt lõi)
- Chứa các logic chung của dự án hiện tại, dùng bởi nhiều tính năng (features).
- Ví dụ: `core:network` (chứa Interceptors gắn token đặc thù app), `core:database`, `core:session`.
- **ĐƯỢC PHÉP** sử dụng Hilt để bind các dependencies. Đóng vai trò là cầu nối (adapter) giữa framework và các module `:foundation`.

## 2. Các quy tắc (Rules) Hilt DI

### R1: Foundation là Framework-Agnostic
Không có bất kỳ sự xuất hiện nào của `@Inject`, `@Module`, hay `@InstallIn` bên trong `:foundation`. Các class trong Foundation phải được tạo bằng constructor hoặc factory method thông thường.

### R2: Module `:core` chịu trách nhiệm Binding
Các module `:core` (hoặc `:di`) sẽ khởi tạo các đối tượng từ `:foundation` và đưa chúng vào Dagger Graph.
```kotlin
// Trong :core:network
@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {
    @Provides
    fun provideFoundationLogger(): FoundationLogger {
        return FoundationLogger.Builder().build()
    }
}
```

### R3: Tách biệt `@Qualifier` vào `:core:di-qualifiers`
Trong Dagger/Hilt, `@Qualifier` giúp phân biệt các instance cùng kiểu (VD: `Retrofit` cho Auth vs `Retrofit` cho API thường).
Nếu bạn định nghĩa `@Qualifier` trong `:core:network`, và `:feature:home` cần inject đối tượng đó, `:feature:home` sẽ phải phụ thuộc vào `:core:network`. Điều này phá vỡ việc đóng gói.
**Giải pháp:** Tạo một module riêng `core:di-qualifiers` chỉ chứa các annotation.
```kotlin
// Trong :core:di-qualifiers
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class AuthNetwork

@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class ApiNetwork
```
Các feature chỉ cần phụ thuộc vào `:core:di-qualifiers` (rất nhẹ, không ảnh hưởng build time).

### R4: Interface đóng vai trò cổng giao tiếp (DI Boundary)
Nếu một module `:feature:A` cần gọi `:feature:B`, `:feature:A` chỉ được thấy `interface BApi`. Khai báo `@Binds` của `BApi` trỏ đến `BImpl` phải được đặt trong `:di` hoặc `:feature:B:data` (nếu dùng component module) để A không bao giờ biết đến BImpl.

### R5: DiBoundaryPolicy plugin
Tích hợp script hoặc plugin Gradle để fail build ngay nếu phát hiện `:foundation` có dependency là `com.google.dagger:hilt-android` hoặc nếu một feature phụ thuộc trực tiếp vào `:di` module.
