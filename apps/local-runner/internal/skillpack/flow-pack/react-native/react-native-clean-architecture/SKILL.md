---
name: react-native-clean-architecture
description: Phân định ranh giới Business, Data và UI/Presentation logic trong React Native & TypeScript theo chuẩn Clean Architecture. Hướng dẫn cấu trúc layer, mapping data và chống leak phụ thuộc.
version: 6
---

# React Native Clean Architecture: Ranh Giới Business, Data & UI Logic

Tài liệu hướng dẫn phân định ranh giới kiến trúc bằng **Ý nghĩa của quyết định**, không phân chia cảm tính theo vị trí class hay tên file.

---

## 1. BA RANH GIỚI BẮT BUỘC (ARCHITECTURAL BOUNDARIES)

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                    1. PRESENTATION LAYER (UI & HOOKS)                       │
│  - React Functional Components (Dumb UI với NativeWind/Tailwind)             │
│  - Custom Hooks (Đóng vai trò ViewModel: Quản lý UI State & User Events)     │
│  - UiModels & Formatting (Dịch Domain Model sang Text/Color/Icon hiển thị)   │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                         Phụ thuộc vào Abstraction
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       2. DOMAIN LAYER (BUSINESS CORE)                       │
│  - Pure TypeScript: Entities, Value Objects, Policies, Use Cases            │
│  - Ports (Interfaces): IQuoteRepository, IBillingService                    │
│  - 0% imports từ 'react', 'react-native', 'expo-*', 'sqlite', hay API SDK    │
└──────────────────────────────────────▲──────────────────────────────────────┘
                                       │
                         Triển khai Abstraction (DIP)
                                       │
┌──────────────────────────────────────┴──────────────────────────────────────┐
│                    3. DATA & INFRASTRUCTURE LAYER                           │
│  - DataSources: SQLite Client, MMKV Storage, HTTP Client (Axios/Fetch)      │
│  - DTOs (Data Transfer Objects) & Mappers (DTO ➔ Domain Model)              │
│  - Repository Implementations: Quản lý Cache, Fallback, Caching Policy      │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. PHÂN BIỆT THEO Ý NGHĨA QUYẾT ĐỊNH

### 2.1. Business Logic (Thuộc Domain Layer)
Trả lời câu hỏi: **"Quy tắc nghiệp vụ của sản phẩm là gì, dù đổi sang Web, CLI hay TV vẫn phải đúng?"**
* Ai đủ điều kiện nhận ưu đãi?
* Thuật toán tính tiền giờ làm thêm (OT x1.5 sau 8 tiếng, ca đêm +30%).
* Tính tổng tiền hóa đơn: `Subtotal + Tax% - Discount`.
* Giới hạn logic: Đơn hàng không được âm tiền, thú cưng phải có ngày sinh hợp lệ.
* ⚠️ **Luật cứng:** Không chứa tên thẻ HTML, màu sắc hex, icon, pixel, hay chuỗi đã dịch theo ngôn ngữ UI.

### 2.2. Data Logic (Thuộc Data Layer)
Trả lời câu hỏi: **"Lấy và lưu trữ dữ liệu ở đâu, bằng công cụ gì?"**
* Đọc từ SQLite trước, nếu rỗng thì gọi API.
* Chuyển đổi (Mapping) từ `quote_dto.json` sang `Quote` entity của Domain.
* Xử lý Cache TTL, pagination cursor, retry policy.
* ⚠️ **Luật cứng:** Mapper trong Data chỉ dịch cấu trúc dữ liệu, **tuyệt đối không tự ý áp đặt luật nghiệp vụ** (ví dụ: Mapper không được tự tính giảm giá 10%).

### 2.3. UI / Presentation Logic (Thuộc Presentation Layer)
Trả lời câu hỏi: **"Người dùng nhìn thấy state như thế nào và tương tác ra sao?"**
* Trạng thái `PercentageDiscount(10)` sẽ được vẽ thành Badge màu đỏ với chữ `"Giảm 10%"`.
* Khi đang lưu dữ liệu thì hiển thị ActivityIndicator.
* Bấm nút "Báo giá" thì mở Modal ký tên ngón tay.
* Định dạng tiền tệ theo Locale: `$1,250.00` (Mỹ) vs `1.250,00 €` (Đức).
* ⚠️ **Luật cứng:** Domain không bao giờ trả về trường `shouldShowBadge` hay `badgeColor`. Presentation tự quyết định cách hiển thị dựa trên kết quả Domain.

---

## 3. BA PHÉP THỬ NHANH ĐỂ BIẾT CODE THUỘC TẦNG NÀO

| Câu hỏi kiểm tra | Nếu câu trả lời là CÓ | Tầng chịu trách nhiệm |
| :--- | :--- | :--- |
| Đổi từ React Native sang Web hoặc Node.js CLI mà luật này vẫn phải đúng? | Đây là Business Meaning | **Domain Layer** |
| Đổi từ SQLite sang MMKV hoặc từ Axios sang Fetch mà logic thay đổi? | Đây là Integration/Persistence Policy | **Data Layer** |
| Cùng một kết quả nhưng trên Điện thoại hiện nút bấm, trên Máy tính bảng hiện danh sách chia đôi màn hình? | Đây là Presentation Policy | **Presentation Layer** |

---

## 4. COMMAND-QUERY SEPARATION (CQS) TRONG REACT NATIVE

Một hàm hoặc effect chỉ được làm 1 trong 2 việc:
1. **Query:** Lấy dữ liệu và trả về kết quả (Không làm biến đổi state bên trong).
2. **Command:** Thực thi một hành động làm biến đổi dữ liệu (Không dùng để hiển thị trực tiếp).

### ❌ VI PHẠM CQS:
```tsx
// ❌ Hàm vừa lấy danh sách vừa âm thầm thay đổi state nội bộ trong lúc render
function getActiveShifts() {
  const shifts = db.getShifts();
  if (shifts.length === 1) {
    setIsSingleShift(true); // Gây re-render vô hạn hoặc loop effect!
  }
  return shifts;
}
```

### ✅ TUÂN THỦ CQS:
* Quá trình Render JSX là **Pure Query (Hàm thuần khiết)**.
* Mọi hành vi làm thay đổi dữ liệu (Command) **bắt buộc phải nằm trong Event Handlers** (`onPress`, `onSubmit`) hoặc các hàm controller rõ ràng.

---

## 5. RACE CONDITIONS & GENERATION TOKEN TRONG ASYNC HOOKS

Khi người dùng chuyển tab nhanh hoặc gõ tìm kiếm liên tục, các tác vụ async chạy ngầm rất dễ trả về kết quả cũ đè lên kết quả mới.

### ✅ Giải pháp Generation Token / AbortController:
```tsx
export function useSearchQuotes(quoteRepo: IQuoteRepository) {
  const [results, setResults] = useState<Quote[]>([]);
  const generationRef = useRef(0);

  const search = useCallback(async (query: string) => {
    // Tăng token mỗi lần gọi mới
    const currentGeneration = ++generationRef.current;

    const data = await quoteRepo.search(query);

    // Chỉ cập nhật state nếu token hiện tại vẫn là mới nhất
    if (currentGeneration === generationRef.current) {
      setResults(data);
    }
  }, [quoteRepo]);

  return { results, search };
}
```
