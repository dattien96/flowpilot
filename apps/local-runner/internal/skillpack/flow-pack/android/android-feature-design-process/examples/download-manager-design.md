# Ví dụ Thiết kế: Download Manager Feature

## 0. Yêu cầu (Requirements)
- Cho phép người dùng tải file tài liệu (PDF, Docx) về máy từ URL.
- Hiển thị % tiến trình tải trên UI.
- Có thể pause, resume, hoặc cancel.
- Tải ngầm kể cả khi tắt app (background).

## 1. HLD (Phase 1)

**D0: Bounded Contexts**
- Context: `FileManagement`, `NetworkOperations`.
- Quyết định: Tạo feature module riêng `:feature:downloader` vì nó phức tạp và độc lập với các nghiệp vụ khác.

**D1: Module Graph**
- `:feature:downloader:api` (Interface cho các tính năng khác gọi tải file)
- `:feature:downloader:impl` (Gồm Presentation, Domain, Data)

**D2: Owner & Data Flow**
- Owner của tiến trình tải: Background Service (WorkManager) là SSOT.
- Flow: UI -> Command -> WorkManager -> Progress Stream (Flow) -> UI.

**D3: Data Lifecycle**
- Trạng thái tải: Persistent (DataStore hoặc DB) để lỡ app crash mở lên vẫn biết đang tải dở.
- File tải xong: Lưu ở Storage của OS.

**D4 & D5: Cross-feature Boundary**
Module `:api` sẽ expose:
```kotlin
interface FileDownloader {
    fun download(url: String, fileName: String): String // return downloadId
    fun observeProgress(downloadId: String): Flow<DownloadState>
}
```

## 2. LLD (Phase 2 - SOLID Gate)

**Interface nội bộ trong Domain:**
```kotlin
interface DownloadEngine {
    suspend fun executeDownload(task: DownloadTask)
    fun getProgress(): Flow<Int>
}
```

**Probe Gate Check:**
- **S:** `DownloadEngine` chỉ lo việc thực thi tải, không lo việc lưu DB, không lo UI. (Pass)
- **O:** Nếu thêm giao thức tải FTP, chỉ cần tạo `FtpDownloadEngine` thay vì sửa HTTP engine hiện tại. (Pass)
- **L:** Các Engine không ném exception lạ, lỗi mạng được wrap thành `Result.Error`. (Pass)
- **I:** Giao diện ngắn gọn, không có hàm dư thừa. (Pass)
- **D:** `DownloadEngine` nằm ở tầng Domain, `WorkManagerDownloadEngine` nằm ở tầng Data/Platform phụ thuộc ngược vào Domain. Tên hàm không nói tới `WorkManager` hay `OkHttp`. (Pass)
