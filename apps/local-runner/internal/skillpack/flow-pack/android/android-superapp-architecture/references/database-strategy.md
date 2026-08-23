# Chiến lược Database Đa Module (Multi-Module Database Strategy)

Trong một ứng dụng quy mô lớn, việc duy trì một CSDL Room duy nhất chứa toàn bộ các Entity là không khả thi vì nó tạo ra bottleneck, tăng thời gian biên dịch (kapt/ksp) và gây ra conflict khi nhiều team cùng sửa.

Chiến lược này tách biệt Database dựa trên **Vòng đời dữ liệu** và **Phạm vi module**.

## 1. Phân loại Database

| Loại Database | Durable DB (Core) | Cache DB (Feature-level) |
| ------------- | ----------------- | ------------------------ |
| **Mục đích** | Dữ liệu cốt lõi, dài hạn, cần chia sẻ giữa nhiều feature. | Dữ liệu cục bộ của feature, cache API, state tạm thời. |
| **Vị trí** | `:core:database` | `:feature:*:data` |
| **Tuổi thọ** | Xóa khi người dùng logout hoặc cài lại app. | Xóa tự do để lấy lại bộ nhớ khi cần. |
| **Entity** | Session, UserProfile, Configs. | ArticleCache, CartItems, VideoFeed. |
| **Migration** | BẮT BUỘC có migration scripts rõ ràng. | Có thể fallback to destructive migration (xóa và tạo lại). |

## 2. Multi-Account Database Naming

Nếu ứng dụng hỗ trợ nhiều tài khoản (multi-account) đăng nhập cùng lúc, database của các account không được lẫn lộn.
Tên file DB phải gắn liền với ID tài khoản.

```kotlin
val dbName = if (userId != null) "app_db_$userId.db" else "app_db_global.db"
```
Khi chuyển account, cần cung cấp một instance `RoomDatabase` mới. Điều này có thể quản lý qua một `SessionManager` kết hợp với Scope của DI.

## 3. SQLCipher & Bảo mật

- Nếu dữ liệu nhạy cảm (PII, Financial), Room Database phải được mã hóa bằng SQLCipher.
- Passphrase phải được sinh ngẫu nhiên từ Keystore (AndroidX Security Crypto) cho mỗi thiết bị, không lưu dưới dạng hard-coded.

## 4. Quản lý Room Database Factory

Để tạo Database thống nhất, `:core:database` (hoặc một foundation database library) nên cung cấp các Factory pattern.

```kotlin
// Ví dụ hàm tiện ích cung cấp qua foundation
inline fun <reified T : RoomDatabase> buildDatabase(
    context: Context,
    dbName: String,
    passphrase: ByteArray?,
    isCache: Boolean
): T {
    val builder = Room.databaseBuilder(context, T::class.java, dbName)
    
    if (isCache) {
        builder.fallbackToDestructiveMigration()
    } else {
        // Cần addMigrations explicitly
    }
    
    if (passphrase != null) {
        val factory = SupportFactory(passphrase)
        builder.openHelperFactory(factory)
    }
    
    return builder.build()
}
```

## 5. Decision Tree (Bảng quyết định cho Developer)

Khi tạo Entity mới, Dev quyết định đặt nó ở đâu?

1. Tính năng này có lưu trữ offline không?
   - **Không** -> Không dùng DB, lưu In-Memory.
   - **Có** -> Tới bước 2.
2. Dữ liệu này có quan trọng không? (Mất đi có gây lỗi nghiêm trọng, hay chỉ mất thời gian tải lại?)
   - **Không (Chỉ là Cache)** -> Đặt vào **Cache DB** tại `:feature:*:data`. Cài đặt destructive migration.
   - **Có (Quan trọng)** -> Tới bước 3.
3. Dữ liệu này có chia sẻ cho các tính năng khác đọc/ghi không?
   - **Không** -> Đặt vào **Durable DB cục bộ** ở `:feature:*:data`, có viết migration script.
   - **Có** -> Đặt vào **Durable DB trung tâm** ở `:core:database`. Viết migration cẩn thận.
