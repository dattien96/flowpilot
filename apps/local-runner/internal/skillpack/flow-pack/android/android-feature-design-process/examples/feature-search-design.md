# Ví dụ Thiết kế: Global Search Feature

## 0. Yêu cầu (Requirements)
- Tìm kiếm toàn cục trong app (Users, Posts, Groups).
- Hiển thị gợi ý tìm kiếm (Suggestions) khi đang gõ (debounce 300ms).
- Lưu lịch sử tìm kiếm cục bộ (Recent searches).

## 1. HLD (Phase 1)

**D0: Bounded Contexts**
- Tương tác với nhiều context: `User`, `Post`, `Group`.
- Search bản thân nó là một context aggregator.

**D1: Module Graph**
- `:feature:search` (Chứa UI và logic search).
- Search cần phụ thuộc vào `:core:network` và `:core:database`.

**D2: Owner & Data Flow**
- Lịch sử tìm kiếm: Local DB (SSOT).
- Kết quả tìm kiếm trực tiếp: Network (SSOT), ViewModel chỉ giữ state tạm thời.

**D3: Data Lifecycle**
- Lịch sử (Recent): Persistent (Room DB).
- Kết quả tìm kiếm tạm thời: View-bound (ViewModel).

## 2. LLD (Phase 2 - SOLID Gate)

**Interface `SearchProvider` (DIP):**
Thay vì Search gọi trực tiếp API của User, Post, ta định nghĩa Port ở Search:

```kotlin
// In :feature:search:domain
interface SearchProvider {
    suspend fun search(query: String): List<SearchResultItem>
}

// Model chung của Search
data class SearchResultItem(val id: String, val title: String, val type: SearchType)
```
Các module `:feature:user` hoặc `:feature:post` sẽ implement `SearchProvider` này (nếu ta dùng multi-module plugin architecture).
Hoặc đơn giản hơn, `:feature:search:data` sẽ gọi các API cần thiết và map về `SearchResultItem`.

**Probe Gate Check:**
- **S:** SearchViewModel không tự gọi API, nó ủy quyền cho `SearchUseCase` và `SearchProvider`. (Pass)
- **O:** Thêm mục tìm kiếm "Products": tạo thêm một `ProductSearchProvider` và inject vào danh sách providers của `SearchUseCase`. Không sửa code cốt lõi của logic Search. (Pass)
- **L:** Các Provider trả về list rỗng nếu lỗi, không ném exception sập app. (Pass)
- **I:** Giao diện chỉ có đúng 1 hàm `search()`. (Pass)
- **D:** `SearchProvider` thuộc về Consumer (Search module). Chữ "Provider" ở đây là theo context của ngữ nghĩa, nhưng package của nó nằm ở Search Domain. (Pass)
