---
name: react-native-solid-principles
description: Chuẩn mực SOLID thực chiến cho React Native và TypeScript, chuyển giao từ tư duy kiến trúc Android Native sang TypeScript/Expo. Áp dụng khi thiết kế component, custom hook, service, repository, và review code.
version: 6
---

# React Native & TypeScript SOLID Principles

Tài liệu hướng dẫn áp dụng triệt để 5 nguyên tắc SOLID vào React Native / Expo với TypeScript, chuyển hóa trực tiếp từ các quy tắc chẩn đoán cơ học của Android Native.

---

## 1. S — Single Responsibility Principle (SRP)
> "Một class, hook, hoặc component chỉ nên có đúng MỘT lý do duy nhất để thay đổi."

### ❓ 4 Câu hỏi chẩn đoán cơ học:
1. **Câu hỏi "VÀ":** Tóm tắt chức năng của component/hook này trong 1 câu duy nhất. Nếu xuất hiện chữ **"VÀ"**, **"ĐỒNG THỜI"**, **"CŨNG"** ➔ **FAILED (Vi phạm S)**.
2. **Ai sửa?** Liệt kê các nhóm yêu cầu có thể bắt sửa file này. Nếu Design team (giao diện), Backend team (format API), và Product team (luật nghiệp vụ) đều có lý do mở file này ra sửa ➔ **FAILED**.
3. **Test được không?** Muốn test một hành vi nghiệp vụ mà phải mock JSX, Navigation, hay Native modules không liên quan ➔ **FAILED**.
4. **Import lạc tầng:** Trong file UI/Hook có thấy import trực tiếp SQLite, MMKV, hoặc API client không? ➔ **FAILED**.

### ❌ BEFORE (Vi phạm SRP):
```tsx
// ❌ Component ôm trọn: Giao diện + Gọi API/DB + Luật tính toán + State điều hướng
export function QuoteScreen({ route, navigation }: any) {
  const [items, setItems] = useState<any[]>([]);
  const [total, setTotal] = useState(0);

  // Vi phạm: Logic nghiệp vụ tính thuế/giảm giá nằm lẫn trong UI
  const calculateTotal = (rawItems: any[]) => {
    let sub = rawItems.reduce((acc, i) => acc + i.price * i.qty, 0);
    let tax = sub * 0.08; // 8% VAT
    let discount = sub > 1000 ? sub * 0.1 : 0; // Luật giảm giá giấu trong UI
    return sub + tax - discount;
  };

  const handleSave = async () => {
    // Vi phạm: Gọi trực tiếp database/native storage trong component
    await db.executeSql('INSERT INTO quotes ...');
    Alert.alert('Thành công', 'Đã lưu');
    navigation.goBack();
  };

  return (
    <View className="p-4">
      {items.map(i => <Text key={i.id}>{i.name} - ${i.price}</Text>)}
      <Text>Tổng: ${total}</Text>
      <TouchableOpacity onPress={handleSave}><Text>Lưu</Text></TouchableOpacity>
    </View>
  );
}
```

### ✅ AFTER (Tuân thủ SRP — Tách 3 lớp rõ ràng):
```tsx
// 1. DOMAIN LAYER (Pure TypeScript - 0% React/UI dependencies)
// file: features/quotes/domain/calculateQuoteTotal.ts
export function calculateQuoteTotal(subtotal: number, taxRate: number, discountPolicy: DiscountPolicy): QuotePriceSummary {
  const tax = subtotal * taxRate;
  const discount = discountPolicy.calculateDiscount(subtotal);
  return { subtotal, tax, discount, total: subtotal + tax - discount };
}

// 2. PRESENTATION CONTROLLER / HOOK (Tương đương ViewModel trong Android)
// file: features/quotes/hooks/useQuoteEditor.ts
export function useQuoteEditor(quoteRepo: IQuoteRepository) {
  const [items, setItems] = useState<QuoteItem[]>([]);
  const [isSaving, setIsSaving] = useState(false);

  const priceSummary = useMemo(() => {
    return calculateQuoteTotal(calculateSubtotal(items), 0.08, StandardDiscountPolicy);
  }, [items]);

  const saveQuote = async () => {
    setIsSaving(true);
    const result = await quoteRepo.save({ items, summary: priceSummary });
    setIsSaving(false);
    return result;
  };

  return { items, priceSummary, isSaving, saveQuote };
}

// 3. PURE DUMB UI COMPONENT (Chỉ hiển thị, nhận event)
// file: features/quotes/components/QuoteEditorView.tsx
export function QuoteEditorView({ items, priceSummary, onSave }: QuoteEditorViewProps) {
  return (
    <View className="p-4">
      <ItemList items={items} />
      <PriceSummaryCard summary={priceSummary} />
      <PrimaryButton title="Lưu Báo Giá" onPress={onSave} />
    </View>
  );
}
```

---

## 2. O — Open/Closed Principle (OCP)
> "Thực thể phần mềm phải MỞ cho việc mở rộng (thêm tính năng mới), nhưng ĐÓNG cho việc sửa đổi (không sửa code cũ)."

### ❓ Phân biệt Tập ĐÓNG vs Tập MỞ:
* **Tập ĐÓNG (Cố định):** Trạng thái tải `Idle | Loading | Success | Error` hoặc Giới tính `Male | Female`. Dùng `switch-case` hoặc `union type` hoàn toàn hợp lệ vì không có khả năng mở rộng bất thường.
* **Tập MỞ (Biến thể mở rộng):** Các loại thanh toán (Stripe, RevenueCat, ApplePay), các loại file nén (Video, Photo, Audio), các preset nén. **Tuyệt đối cấm viết `switch(type)` cứng ở Core.**

### ❌ BEFORE (Vi phạm OCP):
```tsx
// ❌ Khi thêm 1 loại Preset nén mới (như WebP, 4K), bắt buộc phải mở file core này ra sửa
export function compressMedia(file: MediaFile, preset: string) {
  if (preset === 'EMAIL') {
    return compressForEmail(file);
  } else if (preset === 'DISCORD') {
    return compressForDiscord(file);
  } else if (preset === 'TIKTOK') {
    return compressForTikTok(file);
  }
  throw new Error(`Chưa hỗ trợ preset: ${preset}`);
}
```

### ✅ AFTER (Tuân thủ OCP — Registry / Strategy Pattern):
```tsx
// 1. Định nghĩa Strategy Interface
export interface ICompressionStrategy {
  readonly id: string;
  canHandle(file: MediaFile): boolean;
  compress(file: MediaFile): Promise<CompressedResult>;
}

// 2. Registry Engine (Không bao giờ phải sửa file này khi thêm strategy mới)
export class CompressionRegistry {
  private strategies = new Map<string, ICompressionStrategy>();

  register(strategy: ICompressionStrategy): void {
    this.strategies.set(strategy.id, strategy);
  }

  get(id: string): ICompressionStrategy {
    const strategy = this.strategies.get(id);
    if (!strategy) throw new Error(`Strategy ${id} chưa được đăng ký`);
    return strategy;
  }
}

// 3. Thêm Preset mới chỉ cần tạo file mới và gọi register() tại App Entry, 0% sửa code Core!
export const EmailCompressionStrategy: ICompressionStrategy = {
  id: 'EMAIL_25MB',
  canHandle: (f) => f.type === 'video',
  compress: async (f) => { /* logic nén dưới 25MB */ }
};
```

---

## 3. L — Liskov Substitution Principle (LSP)
> "Class con hoặc implementation phải thay thế hoàn toàn class cha/interface mà không làm nổ vỡ chương trình hay phá vỡ bất biến (Invariants)."

### 🚩 3 Red Flags nhận diện vi phạm:
1. **"Con Hư" (TODO / Exception / No-op):** Implementation quăng lỗi `throw new Error("Not implemented")` hoặc để hàm rỗng `() => {}` cho xong chuyện.
2. **"Hỏi Giấy Tờ" (Type Checking `typeof` / `instanceof`):** Caller phải kiểm tra `if (service instanceof MockService)` để gọi hàm khác.
3. **"Sentinel Values" để nuốt lỗi ngầm:** Hàm thất bại nhưng trả về chuỗi rỗng `""`, `-1`, hoặc `null` thay vì báo lỗi minh bạch.

### ❌ BEFORE (Vi phạm LSP):
```tsx
interface IBillingService {
  purchase(productId: string): Promise<PurchaseResult>;
  restorePurchases(): Promise<void>;
}

// ❌ MockBillingService phá vỡ hợp đồng của interface cha
export class MockBillingService implements IBillingService {
  async purchase(productId: string) {
    // Nuốt lỗi hoặc trả giá trị rỗng phá vỡ luồng
    return null as any; 
  }
  async restorePurchases() {
    // Con hư: Không làm gì và cũng không báo lỗi
    throw new Error("Mock không hỗ trợ restore!");
  }
}
```

### ✅ AFTER (Tuân thủ LSP — Result Pattern & Fail-Closed):
```tsx
// Sử dụng Discriminated Union cho kết quả minh bạch
export type Result<T, E = Error> = 
  | { readonly ok: true; readonly value: T }
  | { readonly ok: false; readonly error: E };

export interface IBillingService {
  purchase(productId: string): Promise<Result<CustomerInfo, BillingError>>;
  restorePurchases(): Promise<Result<CustomerInfo, BillingError>>;
}

export class MockBillingService implements IBillingService {
  async purchase(productId: string): Promise<Result<CustomerInfo, BillingError>> {
    return {
      ok: true,
      value: { isPro: true, activeSubscriptions: [productId] }
    };
  }

  async restorePurchases(): Promise<Result<CustomerInfo, BillingError>> {
    return { ok: true, value: { isPro: true, activeSubscriptions: [] } };
  }
}
```

---

## 4. I — Interface Segregation Principle (ISP)
> "Không ép Consumer phải phụ thuộc vào những method hoặc props mà nó không sử dụng."

### ❓ Quy tắc cơ học:
1. **Lean Interface:** Interface không quá 5 methods. Nếu có methods mà 100% implementation chỉ trả giá trị giả hoặc không dùng ➔ Tách nhỏ interface.
2. **Không leak 3rd-party SDK:** Public interface không chứa type của Axios (`AxiosResponse`), SQLite (`SQLiteDatabase`), hay Native modules.
3. **Props mỏng cho Component:** Nếu Component nhận > 7-10 props rời rạc ➔ Gom thành `UiState` object hoặc tách component con. Cấm truyền cả một God Object vào component chỉ để đọc 1 trường `id`.

### ❌ BEFORE (Vi phạm ISP):
```tsx
// ❌ Interface bắt buộc mọi repository phải có các hàm không cần thiết
interface IUserRepository {
  getUser(id: string): Promise<User>;
  updateUser(user: User): Promise<void>;
  deleteUser(id: string): Promise<void>;
  exportUserDataToCsv(): Promise<string>; // Chỉ dành cho Admin, ép user repo thường cũng phải có
  syncWithSalesforce(): Promise<void>;    // Ép dính 3rd party
}
```

### ✅ AFTER (Tuân thủ ISP):
```tsx
// Tách nhỏ theo nhu cầu của Consumer
export interface IUserReader {
  getUser(id: string): Promise<User | null>;
}

export interface IUserWriter {
  saveUser(user: User): Promise<void>;
  deleteUser(id: string): Promise<void>;
}

export interface IUserDataExporter {
  exportToCsv(userId: string): Promise<string>;
}
```

---

## 5. D — Dependency Inversion Principle (DIP)
> "1. Module cấp cao không phụ thuộc module cấp thấp. Cả hai phụ thuộc vào Abstraction (Interface)."  
> "2. Abstraction không phụ thuộc vào chi tiết. Chi tiết phụ thuộc vào Abstraction."

### ❓ 2 Bài kiểm tra then chốt:
1. **Consumer Ownership:** Ai sở hữu Interface?
   * Interface `IQuoteRepository` phải nằm ở module/thư mục **Consumer (Domain/Feature)**, tuyệt đối không nằm ở thư mục của SQLite/Network Adapter.
2. **Hình dạng của Port (DIP vế 2):** Đọc file interface, **có đoán được công nghệ bên dưới không?**
   * Nếu interface có chữ `executeSql()`, `Cursor`, `getDoc()`, `CollectionReference` ➔ **FAILED (Bị thủng Abstraction)**.
   * Interface phải nói về **Ý nghĩa nghiệp vụ**: `getQuoteById(id: string)`, `saveQuote(quote: Quote)`.

### ❌ BEFORE (Vi phạm DIP):
```tsx
// ❌ Hook cấp cao import trực tiếp thư viện SQLite cấp thấp
import * as SQLite from 'expo-sqlite';

export function useQuotes() {
  const [quotes, setQuotes] = useState([]);
  
  useEffect(() => {
    // Phụ thuộc trực tiếp vào cú pháp SQLite cụ thể
    const db = SQLite.openDatabaseSync('quotes.db');
    const result = db.getAllSync('SELECT * FROM quotes');
    setQuotes(result);
  }, []);
}
```

### ✅ AFTER (Tuân thủ DIP):
```tsx
// 1. Port thuộc về Domain (Consumer sở hữu)
// file: features/quotes/domain/ports/IQuoteRepository.ts
export interface IQuoteRepository {
  getAll(): Promise<Quote[]>;
  save(quote: Quote): Promise<void>;
}

// 2. Adapter cấp thấp triển khai Port (Chi tiết phụ thuộc Abstraction)
// file: features/quotes/data/SQLiteQuoteRepository.ts
import { IQuoteRepository } from '../domain/ports/IQuoteRepository';

export class SQLiteQuoteRepository implements IQuoteRepository {
  constructor(private readonly db: AppDatabase) {}

  async getAll(): Promise<Quote[]> {
    return this.db.query('SELECT * FROM quotes').map(toDomainQuote);
  }
  async save(quote: Quote): Promise<void> {
    await this.db.execute('INSERT OR REPLACE INTO quotes ...');
  }
}

// 3. Hook cấp cao chỉ phụ thuộc vào Port (Interface)
// file: features/quotes/hooks/useQuotes.ts
export function useQuotes(repo: IQuoteRepository) {
  const [quotes, setQuotes] = useState<Quote[]>([]);
  useEffect(() => {
    repo.getAll().then(setQuotes);
  }, [repo]);
  return { quotes };
}
```
