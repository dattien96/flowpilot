---
name: react-native-conventions
description: Quy chuẩn dự án React Native & Expo cho FlowPilot. Định nghĩa cấu trúc thư mục, quy tắc TypeScript, styling NativeWind và quản lý dependencies.
version: 6
---

# React Native & Expo Project Conventions

Quy chuẩn chính thức cho các dự án React Native (Expo) phát triển cùng FlowPilot.

---

## 1. Nền Tảng Kỹ Thuật (Tech Baseline)
* **Framework:** React Native 0.74+ / Expo SDK 51+ (New Architecture enabled).
* **Language:** TypeScript 5.0+ bật `"strict": true`.
* **Routing:** App thin-client dùng `src/navigation/RootNavigator.tsx` tích hợp Navigation Guard (xem `react-native-appstart-architecture`); `apps/_template` scaffold dùng Expo Router file-based (`app/(tabs)/`).
* **Styling:** NativeWind v4 (Tailwind CSS for React Native).
* **State Management:** **Zustand Micro-Store đóng vai ViewModel** cho mọi màn hình/feature (xem `react-native-appstart-architecture` §4); Local React Hooks (`useState`, `useReducer`) chỉ cho ephemeral UI state trong component.
* **App Layout:** Mọi app trong `apps/` tuân thủ contract 5 phân lớp `src/{api, domain, data, datasource, presentation}` (xem `react-native-appstart-architecture`).
* **Monetization:** `react-native-purchases` (RevenueCat SDK).
* **Data Persistence:** Local-First (`expo-sqlite` hoặc `react-native-mmkv`).

---

## 2. Quy Tắc Bắt Buộc Khi Sinh Code (Enforced Coding Rules)
1. **Dumb UI Components:** Mọi UI component chỉ nhận props và hiển thị. Cấm gọi API, SQLite hay side-effects phức tạp bên trong body render.
2. **Zustand Micro-Store (ViewModel Pattern):** Toàn bộ state và logic xử lý sự kiện của màn hình/feature phải nằm trong Zustand micro-store (ví dụ: `useQuoteEditorStore`), screen chỉ render dumb UI và dispatch action.
3. **Pure Domain Logic:** Các hàm tính toán tiền tệ, thuế, chu kỳ lịch là pure TypeScript functions, không phụ thuộc vào React hay UI framework.
4. **Không Dùng `any`:** Mọi state, prop, DTO và error phải có kiểu dữ liệu rõ ràng.
5. **Fail-Closed Error Handling:** Mọi tác vụ async phải bọc trong `try-catch` và trả về `Result<T, E>` hoặc xử lý lỗi có UI hiển thị rõ ràng, cấm nuốt lỗi im lặng.
