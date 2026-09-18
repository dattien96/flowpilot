# ĐẶC TẢ KIẾN TRÚC CHI TIẾT: 7 SHARED CORE PACKAGES (FLOWPILOT STUDIO)

> **Mục tiêu:** Xây dựng nền tảng dùng chung (Shared Infrastructure & Domain Foundation) cho bộ 7 ứng dụng thực chiến thị trường Mỹ/Tier-1. Toàn bộ kiến trúc tuân thủ nghiêm ngặt chuẩn **Android SOLID, Clean Architecture 3 lớp, và Monorepo Turborepo 1-way dependency**.
> **Nguyên tắc cốt lõi:** $0 Server Cost, 100% Local-First, Zero Backend Dependency.

---

## 🏛️ 1. TỔNG QUAN HỆ THỐNG & NGUYÊN TẮC PHÂN LỚP

Hệ thống được tổ chức theo mô hình **Multi-Module Monorepo** (Turborepo + pnpm / npm workspaces).

```
flowpilot-mobile-studio/
├── packages/                                  # REUSABLE CORE PACKAGES (100% DÙNG CHUNG)
│   ├── core-ui/                               # Design System, Tokens, Glove-friendly Inputs, Gauges, Pad
│   ├── core-billing/                          # RevenueCat IAP abstraction, Paywall UI, Entitlement Cache
│   ├── core-ads/                              # Google AdMob Manager, Frequency Capping, Natural Breakpoints
│   ├── core-storage/                          # MMKV Key-Value + SQLite Repository Abstraction (Local-first)
│   ├── core-pdf/                              # HTML-to-PDF Engine (expo-print) & Clean Print Templates
│   ├── core-security/                         # Legal Disclaimers, EULA Dialogs, Numerical Sanitization
│   └── core-common/                           # Imperial Math, Fractions, Currencies, Local Notifications
│
└── apps/                                      # 7 THIN CLIENT APPS (CHỈ CHỨA DOMAIN & UI ĐẶC THÙ)
    ├── proquote/                              # App 1: Báo giá thợ US ($49.99 Lifetime)
    ├── towsafe/                               # App 2: Cân tải trọng xe bán tải & RV ($14.99 Lifetime)
    ├── cutcraft/                              # App 3: Tối ưu cắt ván gỗ 2D ($14.99 Lifetime / $2.99/mo)
    ├── sparkycalc/                            # App 4: Tính ống gen & dây điện NEC ($19.99 Lifetime)
    ├── shiftsync/                             # App 5: Lịch ca kíp & tính giờ làm ($14.99/yr Sub)
    ├── flipcalc/                              # App 6: Tính lãi buôn đồ cũ eBay/Poshmark ($19.99 Lifetime)
    └── docuscan/                              # App 7: Quét hóa đơn sang PDF (AdMob Cày Traffic)
```

---

## 📐 2. QUY TẮC PHỤ THUỘC (DEPENDENCY MATRIX)

Tuân thủ nghiêm ngặt **Quy tắc 1 chiều (One-Way Dependency Rule)** từ tư duy Android Native:

```
[Apps: proquote, towsafe...] ──► Phụ thuộc vào ──► [packages/core-*]
[packages/core-*]           ──► TUYỆT ĐỐI KHÔNG phụ thuộc ngược vào bất kỳ App nào
[packages/core-*]           ──► Hạn chế phụ thuộc chéo (core-ui KHÔNG import core-storage)
```

| Core Package | Phụ thuộc bên ngoài (Third-party SDK) | Phụ thuộc nội bộ (Internal) |
| :--- | :--- | :--- |
| **`core-ui`** | `react-native`, `nativewind`, `lucide-react-native` | Không |
| **`core-billing`** | `react-native-purchases` (RevenueCat), `zustand` | `core-storage` (MMKV cache) |
| **`core-ads`** | `react-native-google-mobile-ads` | `core-billing` (để kiểm tra `isPro`) |
| **`core-storage`** | `react-native-mmkv`, `expo-sqlite` | Không |
| **`core-pdf`** | `expo-print`, `expo-sharing` | `core-ui` (chỉ dùng CSS template) |
| **`core-security`** | `react-native` | Không |
| **`core-common`** | `expo-notifications` | Không |

---

## 🔍 3. ĐẶC TẢ CHI TIẾT TỪNG CORE PACKAGE

---

### 3.1. `packages/core-ui` – Design System & Giao Diện Thực Chiến Cho Thợ

#### Mục tiêu:
Cung cấp toàn bộ Design Tokens và các UI component nguyên tử (Atomic UI), đặc biệt hỗ trợ **chế độ sử dụng bằng găng tay lao động (Glove-friendly)** và **môi trường ánh sáng khắc nghiệt (High-contrast Dark Mode)**.

#### Cấu trúc thư mục:
```text
packages/core-ui/
├── src/
│   ├── tokens/
│   │   ├── colors.ts            # Bảng màu: Brand, Safety Green, Alert Yellow, Danger Red
│   │   ├── typography.ts        # Monospace numbers, Large Headings, Small labels
│   │   └── spacing.ts           # Grid 4px/8px chuẩn
│   ├── components/
│   │   ├── Button/
│   │   │   ├── Button.tsx       # Variants: primary, secondary, outline, destructive
│   │   │   ├── GloveButton.tsx  # Min-height 56dp, padding 16dp, haptic mạnh
│   │   │   └── types.ts
│   │   ├── Input/
│   │   │   ├── TextInput.tsx
│   │   │   ├── NumericInput.tsx # Bàn phím số chuyên dụng, auto-format
│   │   │   ├── FractionInput.tsx# Nhập phân số thợ mộc (1/8", 1/4", 3/8", 1/2")
│   │   │   └── types.ts
│   │   ├── Gauge/
│   │   │   ├── MetricGauge.tsx  # Vòng cung bán nguyệt Xanh / Vàng / Đỏ
│   │   │   └── LinearBar.tsx    # Thanh tiến trình phần trăm
│   │   ├── Signature/
│   │   │   ├── SignaturePad.tsx # Khung cảm ứng ký ngón tay SVG
│   │   │   └── types.ts
│   │   ├── Badge/
│   │   │   └── StatusBadge.tsx  # Huy hiệu trạng thái: SAFE, OVERLOAD, PAID...
│   │   ├── Dialog/
│   │   │   ├── BottomSheet.tsx
│   │   │   └── ConfirmDialog.tsx
│   │   └── EmptyState/
│   │       └── EmptyState.tsx   # Hiển thị khi danh sách trống kèm nút bấm tạo mới
│   └── index.ts                 # Feature API duy nhất
```

#### Quy chuẩn SOLID áp dụng:
* **SRP (Single Responsibility):** Mỗi component chỉ vẽ và xử lý tương tác UI của chính nó, không chứa business logic hay gọi database.
* **ISP (Interface Segregation):** Tách riêng `GloveButtonProps` khỏi `StandardButtonProps`, không ép component thông thường phải nhận các tham số thừa của chế độ găng tay.

---

### 3.2. `packages/core-billing` – IAP RevenueCat & Paywall Tối Ưu Chốt Sale

#### Mục tiêu:
Đóng gói toàn bộ logic thanh toán In-App Purchase (IAP) của Apple App Store và Google Play thông qua RevenueCat SDK, trừu tượng hóa qua Interface `IBillingService` (áp dụng triệt để nguyên lý Dependency Inversion - DIP).

#### Cấu trúc thư mục:
```text
packages/core-billing/
├── src/
│   ├── domain/
│   │   ├── models/
│   │   │   ├── Entitlement.ts       # Enum: 'pro_access', 'unlimited_vehicles'...
│   │   │   ├── PackagePlan.ts       # Lifetime vs Monthly vs Annual
│   │   │   └── PurchaseResult.ts    # Success | Cancelled | Error
│   │   └── ports/
│   │       └── IBillingService.ts   # Interface thuần túy
│   ├── infrastructure/
│   │   ├── RevenueCatService.ts     # Implementation gọi SDK react-native-purchases
│   │   └── MockBillingService.ts    # Phục vụ chạy Unit Test và Preview UI
│   ├── presentation/
│   │   ├── hooks/
│   │   │   ├── useBilling.ts        # Hook lấy danh sách gói & hàm mua
│   │   │   └── useIsPro.ts          # Hook kiểm tra quyền Pro 0ms từ RAM
│   │   ├── store/
│   │   │   └── billingStore.ts      # Zustand Store cache entitlement
│   │   └── components/
│   │       └── PaywallModal.tsx     # Giao diện Paywall chuẩn thương mại
│   └── index.ts
```

#### Hợp đồng Interface (`IBillingService`):
```typescript
export interface IBillingService {
  initialize(apiKey: string): Promise<void>;
  getPackages(): Promise<PackagePlan[]>;
  purchase(packageId: string): Promise<PurchaseResult>;
  restore(): Promise<boolean>;
  checkEntitlement(entitlementId: string): Promise<boolean>;
}
```

#### Đặc tính kỹ thuật:
* **Zero-Lag Hydration:** Khi mở app, trạng thái `isPro` được nạp tức thì từ MMKV (0ms) vào Zustand store. Giao diện không bao giờ bị nhấp nháy chuyển từ Free sang Pro trước mắt người dùng.

---

### 3.3. `packages/core-ads` – Điều Phối AdMob & Điểm Ngắt Tự Nhiên

#### Mục tiêu:
Quản lý hiển thị quảng cáo AdMob (Banner, Interstitial, Rewarded) theo nguyên tắc **tôn trọng thợ, chỉ hiện tại điểm ngắt tự nhiên (Natural Breakpoint)** và **tự động vô hiệu hóa 100% khi người dùng mua Pro**.

#### Cấu trúc thư mục:
```text
packages/core-ads/
├── src/
│   ├── manager/
│   │   ├── AdManager.ts         # Singleton quản lý nạp trước quảng cáo
│   │   ├── FrequencyCapper.ts   # Bộ đếm thời gian (3-5 phút) & số lần thao tác
│   │   └── AdConfig.ts          # ID quảng cáo thật vs ID test
│   ├── components/
│   │   ├── AdaptiveBanner.tsx   # Banner dưới đáy màn hình
│   │   └── RewardedAdButton.tsx # Nút xem video nhận lượt tính năng
│   └── index.ts
```

#### Logic điều tiết:
```typescript
export async function triggerNaturalBreakpoint(actionName: string): Promise<void> {
  if (useBillingStore.getState().isPro) return; // Đã mua Pro -> Bỏ qua
  if (!FrequencyCapper.canShowInterstitial()) return; // Chưa đủ thời gian giãn cách -> Bỏ qua
  
  await AdManager.showInterstitial();
  FrequencyCapper.recordImpression();
}
```

---

### 3.4. `packages/core-storage` – Lưu Trữ Cục Bộ (Local-First MMKV & SQLite)

#### Mục tiêu:
Thực hiện cam kết **$0 Server Cost**. Toàn bộ dữ liệu của người dùng được lưu trữ cục bộ an toàn trên máy khách:
* **MMKV:** Lưu cài đặt cấu hình, đơn vị đo (Imperial/Metric), token quyền Pro.
* **SQLite (`expo-sqlite`):** Lưu trữ các bảng dữ liệu quan hệ (danh sách đơn báo giá, hồ sơ xe bán tải, danh sách ván cắt, lịch ca kíp).

#### Cấu trúc thư mục:
```text
packages/core-storage/
├── src/
│   ├── kv/
│   │   ├── MMKVStorage.ts       # Đọc/Ghi Key-Value đồng bộ cực nhanh
│   │   └── StorageKeys.ts       # Định nghĩa Enum các Key chuẩn
│   ├── db/
│   │   ├── DatabaseClient.ts    # Khởi tạo kết nối expo-sqlite
│   │   ├── MigrationEngine.ts   # Cơ chế chạy script nâng cấp bảng tự động
│   │   ├── BaseRepository.ts    # Generic CRUD Repository <T>
│   │   └── types.ts
│   └── index.ts
```

#### Hợp đồng `BaseRepository<T>`:
```typescript
export interface IBaseRepository<T> {
  findById(id: string): Promise<T | null>;
  findAll(): Promise<T[]>;
  save(entity: T): Promise<void>;
  saveBatch(entities: T[]): Promise<void>;
  delete(id: string): Promise<void>;
}
```

---

### 3.5. `packages/core-pdf` – Cỗ Máy Sinh Tài Liệu Thương Mại Chuẩn In Ấn

#### Mục tiêu:
Biến toàn bộ kết quả tính toán thành tài liệu PDF thương mại chất lượng cao (A4 hoặc US Letter) bằng cách biên dịch HTML + Tailwind CSS thông qua `expo-print` và chia sẻ qua `expo-sharing`.

#### Cấu trúc thư mục:
```text
packages/core-pdf/
├── src/
│   ├── templates/
│   │   ├── DocumentHeader.ts    # Logo, thông tin thợ, ngày giờ, số chứng từ
│   │   ├── DocumentFooter.ts    # Số trang (Trang 1/2), ghi chú miễn trừ trách nhiệm
│   │   ├── TableRenderer.ts     # Bảng bóc tách khối lượng / bảng kê tự ngắt trang
│   │   ├── SvgDiagramEmbed.ts   # Nhúng bản vẽ sơ đồ cắt gỗ 2D vào PDF
│   │   └── SignatureBox.ts      # Khung nhúng chữ ký số của khách
│   ├── engine/
│   │   ├── PdfGenerator.ts      # HTML String -> File PDF URI (expo-print)
│   │   └── ShareService.ts      # Mở Native Share Dialog (expo-sharing)
│   └── index.ts
```

#### Trường hợp sử dụng ở cả 7 app:
1. `proquote`: Hóa đơn & Báo giá kèm chữ ký khách hàng.
2. `towsafe`: Bảng kiểm định an toàn tải trọng xe kéo trước khi lên cao tốc.
3. `cutcraft`: Bản vẽ kỹ thuật sơ đồ cắt ván gỗ 2D in ra dán bàn cưa.
4. `sparkycalc`: Báo cáo trích dẫn chuẩn mã NEC nộp cho thanh tra công trình.
5. `shiftsync`: Bảng chấm công tổng hợp giờ làm & tiền tăng ca nộp cho quản lý.
6. `flipcalc`: Bảng kê khai chi phí hàng tồn kho phục vụ khai thuế Schedule C.
7. `docuscan`: Tập hợp các trang scan thành file PDF hoàn chỉnh.

---

### 3.6. `packages/core-security` – Miễn Trừ Pháp Lý & Bảo Vệ An Toàn Số Liệu

#### Mục tiêu:
Bảo vệ lập trình viên và studio khỏi các rủi ro pháp lý tại thị trường Mỹ (tránh kiện tụng về an toàn xe cộ, điện nước) và ngăn ngừa lỗi sập app (Crash Prevention).

#### Cấu trúc thư mục:
```text
packages/core-security/
├── src/
│   ├── legal/
│   │   ├── DisclaimerModal.tsx  # Bắt buộc người dùng bấm chấp thuận khi mở app lần đầu
│   │   ├── LegalDisclaimerTexts.ts # Đoạn văn miễn trừ: "Informational & Planning Aid Only"
│   │   └── PrivacyPolicyModal.tsx # Xem chính sách bảo mật cục bộ (tuân thủ Apple 5.1)
│   ├── math/
│   │   └── SafeMath.ts          # Chống chia cho 0, khử NaN, chặn số âm phi lý
│   └── index.ts
```

---

### 3.7. `packages/core-common` – Bộ Toán Imperial & Đổi Đơn Vị Mỹ

#### Mục tiêu:
Cung cấp các công thức tính toán đặc thù cho thị trường Mỹ (hệ đo lường Imperial: lbs, inches, feet) và các hàm tiện ích thời gian, tiền tệ.

#### Cấu trúc thư mục:
```text
packages/core-common/
├── src/
│   ├── imperial/
│   │   ├── FractionCalculator.ts # Cộng trừ nhân chia phân số inch: 3/8" + 5/16" = 11/16"
│   │   └── UnitsConverter.ts     # lbs <-> kg, feet <-> meter, sq ft <-> m2
│   ├── currency/
│   │   └── UsdFormatter.ts       # Định dạng tiền tệ: $1,499.00
│   ├── date/
│   │   ├── DateHelpers.ts        # Thao tác ngày tháng, tính chu kỳ ca kíp
│   │   └── ICalGenerator.ts      # Tạo file lịch .ics để đồng bộ Google/Apple Calendar
│   ├── notifications/
│   │   └── LocalNotifications.ts # Đặt lịch nhắc cục bộ bằng expo-notifications
│   └── index.ts
```

---

## 🚀 4. QUY CHUẨN THI CÔNG DÀNH CHO FLOWPILOT RUNNER

Khi FlowPilot bước vào giai đoạn code:
1. **Tuần tự triển khai Core:** Dựng từ package nền tảng trước (`core-common`, `core-storage`, `core-security`) ➔ dựng UI & Engine (`core-ui`, `core-billing`, `core-ads`, `core-pdf`).
2. **Quy tắc kiểm thử Additive Only:** Toàn bộ use case của từng Core Package phải có Unit Test độc lập (chạy Jest / React Native Testing Library) trước khi tích hợp vào các App con.
3. **Tuyệt đối không vi phạm Feature API:** Mọi file trong các App con chỉ được phép import từ `@flowpilot/core-ui`, không được import sâu vào đường dẫn con (ví dụ: cấm `@flowpilot/core-ui/src/internal/...`).
