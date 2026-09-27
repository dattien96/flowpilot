---
name: react-native-multi-module-monorepo
description: Chuẩn mực tổ chức dự án Multi-Module Monorepo (Turborepo / pnpm) cho React Native & Expo, kế thừa tư duy phân chia module từ Android Native (Foundation, Core, Feature, API vs Impl).
version: 6
---

# React Native Multi-Module Monorepo: Kiến Trúc Chia Module Chuẩn

Tài liệu hướng dẫn tổ chức danh mục nhiều ứng dụng (App Studio / SuperApp) trong một Monorepo duy nhất, ánh xạ trực tiếp từ kiến trúc Gradle Multi-Module của Android sang TypeScript/pnpm workspaces.

---

## 1. BẢN ĐỒ TỔNG QUAN HỆ THỐNG MODULE

```text
flowpilot-mobile-studio/
├── packages/                                  # REUSABLE CORE PACKAGES (DÙNG CHUNG)
│   ├── core-ui/                               # Tương đương :foundation:ui (Design System, Tokens, Tailwind)
│   ├── core-billing/                          # Tương đương :core:billing (RevenueCat Adapter & Paywall)
│   ├── core-ads/                              # Tương đương :core:ads (AdMob Manager, Frequency Capping)
│   ├── core-storage/                          # Tương đương :core:database (SQLite / MMKV Repository abstraction)
│   ├── core-pdf/                              # Tương đương :core:document (HTML-to-PDF Engine qua expo-print)
│   ├── core-security/                         # Tương đương :foundation:security (Disclaimer, Sanitization)
│   └── core-common/                           # Tương đương :foundation:common (Date utils, Currency, Types)
│
└── apps/                                      # 7 THIN CLIENT APPS (CHỈ CHỨA DOMAIN & UI ĐẶC THÙ)
    ├── towsafe/                               # App #1: Cân tải trọng xe bán tải & RV (IAP $14.99) — PHÁT SÚNG MỞ MÀN
    ├── cutcraft/                              # App #2: Tối ưu cắt ván gỗ 2D (IAP $14.99)
    ├── proquote/                              # App #3: Báo giá thợ sửa chữa (IAP $49.99)
    ├── sparkycalc/                            # App #4: Tính ống gen & dây điện NEC (IAP $19.99)
    ├── shiftsync/                             # App #5: Lịch ca kíp & tính giờ làm (Sub $14.99/yr)
    ├── flipcalc/                              # App #6: Tính lãi buôn đồ cũ eBay/Poshmark (IAP $19.99)
    └── docuscan/                              # App #7: Quét hóa đơn sang PDF (AdMob cày traffic)
```

**Build order bất biến (theo blueprint):** `towsafe` ➔ `cutcraft` ➔ `proquote` ➔ `sparkycalc` ➔ `shiftsync` ➔ `flipcalc` ➔ `docuscan`. TowSafe là app mở màn vì nó dựng toàn bộ 7 `packages/core-*` để các app sau tái sử dụng ~90%.

---

## 2. QUY TẮC CHIỀU PHỤ THUỘC (DEPENDENCY DIRECTION RULES)

1. **Chiều phụ thuộc một chiều (Strict Downward Arrow):**
   * `apps/*` ➔ phụ thuộc vào `packages/core-*`.
   * `packages/core-*` ➔ **TUYỆT ĐỐI KHÔNG ĐƯỢC** phụ thuộc ngược lại vào `apps/*` hoặc bất kỳ feature cụ thể nào.
2. **Không chứa danh từ riêng trong Core/Foundation:**
   * Trong `packages/core-*`, cấm chứa tên app cụ thể (ví dụ: cấm chứa `"proquote"`, `"shiftsync"` trong URL scheme, storage key, hay log).
   * Mọi định danh app phải được truyền vào qua tham số khởi tạo hoặc Config Provider từ tầng `apps/*`.
3. **Phân biệt Foundation vs Core:**
   * **Foundation (`core-ui`, `core-common`, `core-security`):** Dùng chung cho toàn bộ dự án, hoàn toàn trung lập, không biết gì về logic nghiệp vụ.
   * **Core Feature (`core-billing`, `core-pdf`):** Cung cấp các adapter kỹ thuật cấp cao nhưng chỉ phụ thuộc vào hợp đồng (Contracts/Interfaces).

---

## 3. TÁCH BIỆT API VÀ IMPLEMENTATION TRONG FEATURE

Trong một Feature module, luôn áp dụng quy tắc **Public API Surface** như cách Android chia module `:feature:xxx:api` và `:feature:xxx:impl`:

```text
features/quotes/
├── index.ts                     # PUBLIC API CONTRACT (Chỉ export những gì bên ngoài được phép thấy)
├── domain/                      # DOMAIN LOGIC
│   ├── ports/
│   │   └── IQuoteRepository.ts  # Public Interface
│   └── models/
│       └── Quote.ts             # Public Domain Model
├── components/
│   └── QuoteScreen.tsx          # Public Screen Entry Component
└── internal/                    # TOÀN BỘ NỘI BỘ BỊ GIẤU KÍN (INTERNAL)
    ├── data/
    │   ├── SQLiteQuoteDataSource.ts
    │   └── QuoteDto.ts
    └── utils/
        └── mathHelpers.ts
```

* **Luật xuất khẩu (Export Rule):** Bên ngoài chỉ được phép:
  ```tsx
  import { QuoteScreen, type Quote, type IQuoteRepository } from '@/features/quotes';
  ```
  **Cấm tiệt việc import sâu vào nội bộ:**
  ```tsx
  // ❌ VI PHẠM ĐÓNG GÓI:
  import { SQLiteQuoteDataSource } from '@/features/quotes/internal/data/SQLiteQuoteDataSource';
  ```

* **Ánh xạ sang Thin Client App:** Trong phạm vi một app dưới `apps/`, Public API Surface tương ứng thư mục `src/api/` (UseCase interface + public model), còn implementation ẩn trong `src/{domain,data,datasource,presentation}` theo contract 5 phân lớp — xem `react-native-appstart-architecture`.

---

## 4. INVERSION OF CONTROL (IOC) TẠI APP COMPOSITION ROOT

Vì không có Hilt/Dagger với annotation processor trong TypeScript, chúng ta thực hiện IoC tại **App Composition Root** (`app/_layout.tsx`) bằng **React Context & Factory Pattern**:

```tsx
// 1. Tạo Context chứa các Ports (Interfaces)
// file: packages/core-billing/src/BillingContext.tsx
export const BillingContext = createContext<IBillingService | null>(null);

export function useBilling(): IBillingService {
  const service = useContext(BillingContext);
  if (!service) throw new Error('useBilling phải được dùng bên trong BillingProvider');
  return service;
}

// 2. Tại Composition Root của từng App cụ thể, Inject Implementation thật:
// file: apps/proquote/app/_layout.tsx
const revenueCatBilling = new RevenueCatBillingService({ apiKey: process.env.EXPO_PUBLIC_REVENUECAT_KEY! });

export default function RootLayout() {
  return (
    <BillingContext.Provider value={revenueCatBilling}>
      <Stack />
    </BillingContext.Provider>
  );
}
```

* **Lợi ích:** 
  * Khi viết Unit Test hoặc Storybook, bạn chỉ cần thay `revenueCatBilling` bằng `new MockBillingService()` mà không phải sửa một dòng code nào trong các màn hình UI.
  * Thỏa mãn 100% nguyên tắc Dependency Inversion (DIP).
