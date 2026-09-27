---
name: react-native-appstart-architecture
description: Chuẩn Enterprise Clean Architecture 5 phân lớp (AppStart contract) cho mọi thin-client app trong FlowPilot Studio monorepo — api, domain, data, datasource, presentation — kèm boundary mappers zero-leakage và Zustand micro-store làm ViewModel. Áp dụng khi tạo hoặc refactor app dưới apps/.
version: 6
---

# React Native AppStart 5-Layer Architecture (Thin Client Contract)

Mọi app trong `apps/` của FlowPilot Studio monorepo là một **thin client** tuân thủ cứng contract 5 phân lớp chuyển giao từ Android Native (chuẩn AppStart). Skill này là hợp đồng về cấu trúc folder và chiều phụ thuộc. Ranh giới ngữ nghĩa "code này thuộc tầng nào" xem `react-native-clean-architecture`; ma trận phụ thuộc giữa `apps/` và `packages/` xem `react-native-multi-module-monorepo`.

---

## 1. APP SKELETON BẮT BUỘC

```text
apps/<app-name>/
├── src/
│   ├── api/                             # 1. PUBLIC CONTRACTS (EXPOSE RA BÊN NGOÀI)
│   │   ├── I<Capability>UseCase.ts      # Public UseCase Interface (e.g. ICalculateTowSafety)
│   │   └── models/                      # Public Models phục vụ module khác
│   ├── domain/                          # 2. DOMAIN LAYER (CANONICAL SOURCE OF TRUTH)
│   │   ├── model/                       # Authoritative Business Entities
│   │   ├── gateway/                     # I<App>Gateway Interface (Hợp đồng cho Data layer)
│   │   └── usecase/                     # <Capability>UseCaseImpl implements interface từ api/
│   ├── data/                            # 3. DATA LAYER (REPOSITORY COORDINATION)
│   │   ├── model/                       # Data Models
│   │   ├── datasource/                  # I<Entity>LocalDataSource Interface (Hợp đồng DataSource)
│   │   └── repository/                  # <App>RepositoryImpl (Chỉ giữ Interface của DataSource)
│   ├── datasource/                      # 4. DATASOURCE LAYER (SQLITE & MMKV DRIVERS)
│   │   ├── database/                    # Sqlite<Entity>LocalDataSource implements interface từ data/
│   │   ├── cache/                       # Mmkv<Scope>DataSource
│   │   └── mapper/                      # <Entity>DbMapper (SQLite Entity <-> Domain/Data Model)
│   ├── presentation/                    # 5. PRESENTATION LAYER (ZUSTAND VIEWMODEL + DUMB UI)
│   │   ├── model/                       # <Screen>UiState, <Widget>UiModel (format riêng cho UI/UX)
│   │   ├── mapper/                      # <Screen>UiMapper (Domain Model -> UI Model)
│   │   ├── stores/                      # Zustand Micro-Stores (thay thế Google ViewModel)
│   │   └── screens/                     # Dumb Screens (tương đương @Composable Android)
│   ├── navigation/
│   │   └── RootNavigator.tsx            # Navigation Guard chặn nếu chưa Accept EULA
│   └── App.tsx                          # Composition root: inject implementations vào ports
├── app.json                             # Expo Config (Bundle ID: com.flowpilot.<app>)
└── package.json
```

> **Biến thể expo-router:** Nếu app dùng `expo-router` (file-based), `navigation/RootNavigator.tsx` được thay bằng thư mục `app/` chứa route files **mỏng** — mỗi route chỉ render screen từ `src/presentation/screens/`; EULA guard đặt trong `app/_layout.tsx`. Năm tầng `src/` không đổi.

---

## 2. NĂM PHÂN LỚP & QUY TẮC CỨNG

| Layer | Chứa | Quy tắc bất biến |
| :--- | :--- | :--- |
| **`api/`** | Public UseCase Interface, Public Model/DTO liên module | Module ngoài CHỈ ĐƯỢC phụ thuộc `api/`. Tuyệt đối không import `domain/`, `data/`, `datasource/`, `presentation/` của app khác hay từ ngoài vào sâu. |
| **`domain/`** | Domain Model (canonical truth), Gateway Interface, UseCase Impl | Pure TypeScript. CẤM import `react`, `react-native`, `expo-sqlite`, `react-native-mmkv` hay bất kỳ UI/driver lib nào. |
| **`data/`** | Data Model, DataSource Interface, Repository Impl | `RepositoryImpl` CHỈ CẦM interface của DataSource qua constructor — không biết nguồn là SQLite, MMKV hay API. |
| **`datasource/`** | SQLite/MMKV implementation cụ thể, DB Entity, DTO, DbMapper | Bọc toàn bộ truy cập đĩa/mạng; cô lập cấu trúc bảng, không rò rỉ lên trên. |
| **`presentation/`** | UI Model, UiMapper, Zustand micro-store, Dumb Screens | Screen chỉ render và dispatch action; mọi state/logic nằm trong store. |

### Chiều phụ thuộc một chiều

```text
presentation ──depends──▶ api (interfaces) + domain (models/usecase contract)
data         ──depends──▶ domain (gateway interface) + api
datasource   ──implements──▶ data/datasource interface
```

Domain và `api/` không phụ thuộc ngược vào bất kỳ tầng nào bên dưới — mọi quan hệ đi xuống đều qua interface (DIP).

---

## 3. BOUNDARY MAPPERS — ZERO LEAKAGE

Mỗi tầng sở hữu model riêng, mọi quá cảnh ranh giới đều đi qua mapper:

```text
DbEntity / DTO  (datasource)  ←─DbMapper─→  Domain Model  (domain)  ──UiMapper──▶  UI Model  (presentation)
```

1. **Domain Model là chuẩn nhất** — thực thể nghiệp vụ bất biến.
2. **UI Model tối ưu cho render** — đã format sẵn đơn vị, màu Xanh/Vàng/Đỏ, chuỗi `$1,499.00`.
3. **DbEntity phản ánh schema** — cột SQLite / trường JSON.
4. **Cấm rò rỉ tuyệt đối:** không ném DbEntity lên screen, không truyền UiModel xuống repository.

---

## 4. PRESENTATION = ZUSTAND MICRO-STORE (VIEWMODEL)

Zustand micro-store đóng vai **ViewModel** (thay vì custom hook loẻng): một store nhỏ per screen/feature, giữ `State`, nhận `Intent` (actions), phát `Effect` một chiều.

```typescript
// file: apps/<app>/src/presentation/stores/useDashboardStore.ts
import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import { appStorage } from '@flowpilot/core-storage';

interface DashboardState {
  // State
  inputs: DashboardInputs;
  result: DashboardUiModel | null;
  // Intents (actions)
  setInputs: (inputs: DashboardInputs) => void;
  recalculate: () => void;
}

export const useDashboardStore = create<DashboardState>()(
  persist(
    (set, get) => ({
      inputs: DEFAULT_INPUTS,
      result: null,
      setInputs: (inputs) => {
        set({ inputs });
        get().recalculate(); // reactive: nhập đến đâu tính đến đó
      },
      recalculate: () => {
        const domain = calculateSafety(get().inputs); // gọi UseCase từ domain/api
        set({ result: mapToDashboardUi(domain) });      // qua UiMapper
      },
    }),
    {
      name: 'dashboard-store',
      storage: createJSONStorage(() => appStorage), // persist xuống MMKV, hydrate 0ms
    },
  ),
);
```

Quy tắc:
* Screen subscribe qua **selector có chọn lọc** (`useStore((s) => s.result)`) để tránh re-render thừa.
* State bền (settings, hồ sơ xe, cờ Pro, `hasAcceptedLegalTerms`) tự đồng bộ xuống MMKV qua middleware `persist`.
* Store gọi UseCase qua **interface từ `api/`/`domain/`**, không import trực tiếp implementation ở `data/`/`datasource/` — implementation được inject tại composition root (`App.tsx` / provider).

---

## 5. GATE CHECKLIST TRƯỚC KHI COI APP ĐẠT CONTRACT

* [ ] App có đủ 5 thư mục `src/{api,domain,data,datasource,presentation}` và không file nghiệp vụ nào nằm ngoài.
* [ ] `domain/` không import `react`/`react-native`/`expo-*`/driver cụ thể (`grep` phải ra 0 kết quả).
* [ ] Mọi consumer ngoài app chỉ import từ `src/api/` (hoặc `apps/<app>/api` entrypoint).
* [ ] Mọi quá cảnh ranh giới đi qua mapper — không DbEntity/UiModel leak.
* [ ] State màn hình nằm trong Zustand micro-store; screen là dumb UI.
* [ ] Navigation guard kiểm tra cờ pháp lý (`hasAcceptedLegalTerms`) trước khi vào luồng chính.
