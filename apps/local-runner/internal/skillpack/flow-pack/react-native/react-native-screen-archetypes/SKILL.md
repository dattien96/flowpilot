---
name: react-native-screen-archetypes
description: Quy chuẩn thiết kế bố cục màn hình (5 Screen Layout Archetypes) và công thái học ngón tay cái (1-Thumb Interaction Rule) cho ứng dụng React Native & Expo. Định hình visual hierarchy, hero visual, input cards, và sticky CTA buttons.
version: 6
---

# React Native Screen Archetypes & UI/UX Composition

Tài liệu quy chuẩn dành cho FlowPilot Design & Coder Agents khi thiết kế và hiện thực hóa các màn hình ứng dụng React Native / Expo. Đảm bảo mọi màn hình sinh ra đều đạt **chất lượng thẩm mỹ cao (Production-Grade Aesthetics)**, **phân tầng thị giác rõ ràng (Visual Hierarchy)** và **chuẩn công thái học ngón tay cái (1-Thumb Ergonomics)**.

---

## 1. QUY TẮC PHÂN TẦNG CÔNG THÁI HỌC 1 NGÓN CÁI (1-THUMB INTERACTION)

Khi thiết kế màn hình di động, diện tích hiển thị bắt buộc được phân bổ thành 3 tầng chức năng:

```text
┌─────────────────────────────────────────────────────────────┐
│ 1. VÙNG XEM (TOP 30% - VIEW-ONLY / HERO ZONE)               │
│ - Mắt người dùng quét đầu tiên khi vào màn hình.            │
│ - Dành riêng cho: Tiêu đề tối giản, Kết quả tính toán chính,│
│   Đồng hồ đo bán nguyệt (MetricGauge), Số liệu trực quan.   │
│ - ⚠️ CẤM: Đặt các nút hành động chính (Primary CTA) tại đây.│
├─────────────────────────────────────────────────────────────┤
│ 2. VÙNG LỰA CHỌN (MIDDLE 40% - INTERACTION & FORM ZONE)     │
│ - Dành cho việc cuộn lướt và nhập liệu.                     │
│ - Gom nhóm các trường thông tin vào các Thẻ (Cards) có      │
│   border viền mờ (zinc-800), bo góc lớn (rounded-2xl).      │
│ - Sử dụng Segmented Controls, Toggle Chips, Bàn phím số.    │
├─────────────────────────────────────────────────────────────┤
│ 3. VÙNG NGÓN CÁI (BOTTOM 30% - THUMB ACTION ZONE)           │
│ - Khu vực ngón tay cái chạm tới dễ dàng và tự nhiên nhất.   │
│ - Ghim cố định (Sticky Bottom Bar) nút hành động quyết định:│
│   [ GloveButton: "Tính toán", "Xuất PDF", "Nâng cấp Pro" ] │
│ - Đảm bảo cách ly an toàn khỏi vạch Home bar của iPhone.     │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. NĂM KHUÔN MẪU BỐ CỤC KINH ĐIỂN (5 SCREEN ARCHETYPES)

---

### ARCHETYPE 1: THE HERO CALCULATOR (Công Cụ Tính Toán Trực Quan)
* **Ứng dụng tiêu biểu:** `TowSafe` (Tải trọng xe), `SparkyCalc` (Cỡ dây điện), `FlipCalc` (Lợi nhuận đồ cũ).
* **Mục tiêu UX:** Người dùng đổi số ở giữa $\rightarrow$ Kết quả trên đỉnh nhảy số tức thì $\rightarrow$ Bấm nút chốt ở đáy.
* **Cấu trúc Component Tree chuẩn:**
```tsx
<ScreenWrapper title="TowSafe Calculator">
  {/* TOP: Hero Visual */}
  <View className="my-2">
    <MetricGauge 
      value={payloadPercent} 
      label="Tải Trọng Thùng Xe (Payload)" 
      unit="%" 
      warningThreshold={80} 
      dangerThreshold={100} 
    />
  </View>

  {/* MIDDLE: Grouped Input Cards */}
  <ScrollView className="flex-1" keyboardShouldPersistTaps="handled">
    <View className="bg-zinc-900 border border-zinc-800 rounded-2xl p-4 my-2">
      <Text className="text-sm font-bold text-zinc-300 uppercase mb-3">Thông số Xe Bán Tải</Text>
      <NumericInput label="GVWR (lbs)" value={gvwr} onChangeText={setGvwr} />
      <NumericInput label="Tự trọng Curb Weight (lbs)" value={curb} onChangeText={setCurb} />
    </View>

    <View className="bg-zinc-900 border border-zinc-800 rounded-2xl p-4 my-2">
      <Text className="text-sm font-bold text-zinc-300 uppercase mb-3">Hành lý & Hành khách</Text>
      <NumericInput label="Người ngồi (lbs)" value={passengers} onChangeText={setPassengers} />
      <NumericInput label="Hàng thùng xe (lbs)" value={cargo} onChangeText={setCargo} />
    </View>
  </ScrollView>

  {/* BOTTOM: Sticky Thumb Action */}
  <View className="pt-2 border-t border-zinc-800/80">
    <GloveButton title="Kiểm Tra An Toàn Rig" variant="primary" onPress={handleCalculate} />
  </View>
</ScreenWrapper>
```

---

### ARCHETYPE 2: THE DOCUMENT VAULT (Kho Lưu Trữ & Lưới Tài Liệu)
* **Ứng dụng tiêu biểu:** `DocuScan` (Scan hóa đơn), `PawVault` (Sổ tiêm thú cưng).
* **Mục tiêu UX:** Tìm kiếm cực nhanh, thẻ tài liệu có thumbnail trực quan, nút tạo mới to bản.
* **Cấu trúc Component Tree chuẩn:**
```tsx
<ScreenWrapper title="Hồ Sơ Thú Cưng">
  {/* TOP: Search bar + Status Filter Chips */}
  <View className="flex-row gap-2 my-2">
    <SearchInput placeholder="Tìm tên thú cưng..." value={search} onChangeText={setSearch} />
    <FilterChip label="Tất cả" active={filter === 'all'} onPress={() => setFilter('all')} />
    <FilterChip label="Cần tiêm" active={filter === 'urgent'} onPress={() => setFilter('urgent')} />
  </View>

  {/* MIDDLE: List of Cards with Badges */}
  <FlatList
    data={pets}
    renderItem={({ item }) => (
      <Pressable className="bg-zinc-900 border border-zinc-800 rounded-2xl p-4 my-2 flex-row items-center justify-between">
        <View className="flex-row items-center gap-3">
          <Avatar image={item.avatarUri} fallback={item.name[0]} />
          <View>
            <Text className="text-lg font-bold text-zinc-100">{item.name}</Text>
            <Text className="text-xs text-zinc-400">{item.breed} • {item.age}</Text>
          </View>
        </View>
        <StatusBadge status={item.vaccineStatus} />
      </Pressable>
    )}
  />

  {/* BOTTOM: Sticky Action Button */}
  <View className="pt-2 border-t border-zinc-800/80">
    <GloveButton title="➕ Thêm Hồ Sơ Mới" variant="secondary" onPress={handleAddNew} />
  </View>
</ScreenWrapper>
```

---

### ARCHETYPE 3: THE STEP WIZARD (Quy Trình Tạo Từng Bước)
* **Ứng dụng tiêu biểu:** `ProQuote` (Báo giá thợ), `CutCraft` (Tối ưu ván gỗ).
* **Mục tiêu UX:** Giảm tải nhận thức (Cognitive Load). Không dồn 20 ô nhập liệu vào 1 trang; chia thành Step 1 $\rightarrow$ Step 2 $\rightarrow$ Step 3.
* **Đặc tính:**
  * Thanh tiến trình Step Bar (`Step 2/3: Kích thước ván`).
  * Nút "Quay lại" (Secondary) song song với nút "Tiếp tục" (Primary).

---

### ARCHETYPE 4: THE FOCUS RUNNER (Đồng Hồ Đếm Giờ & Tập Trung)
* **Ứng dụng tiêu biểu:** `StepFlow` (ADHD Micro-Step Timer), `FocusZen` (Cai nghiện điện thoại).
* **Mục tiêu UX:** Tối giản 100%, không viền thừa, chữ số khổng lồ (Display font), tạo cảm giác thư thái và tập trung.
* **Đặc tính:**
  * Đồng hồ tròn lớn ở trung tâm.
  * Chỉ hiển thị đúng 1 dòng nhiệm vụ nhỏ đang chạy.
  * Nút Play/Pause lớn dạng xúc giác (Haptic Tap).

---

### ARCHETYPE 5: THE HIGH-CONVERTING PAYWALL (Màn Hình Mở Khóa Pro)
* **Ứng dụng tiêu biểu:** Xuất hiện ở cả 7 app khi chạm tính năng khóa hoặc bấm Nâng cấp.
* **Mục tiêu UX:** Tạo độ tin cậy tối đa, làm nổi bật giá trị gói Lifetime $14.99 - $49.99 so với gói tháng.
* **Cấu trúc Component Tree chuẩn:**
```tsx
<ModalWrapper>
  {/* TOP: Value Badge & Hero Title */}
  <View className="items-center my-4">
    <View className="w-16 h-16 rounded-full bg-amber-500/20 items-center justify-center mb-3">
      <CrownIcon size={32} color="#f59e0b" />
    </View>
    <Text className="text-2xl font-black text-zinc-50 text-center">Nâng Cấp Pro Vĩnh Viễn</Text>
    <Text className="text-sm text-zinc-400 text-center mt-1">Mở khóa toàn bộ tính năng độc quyền cho thợ chuyên nghiệp</Text>
  </View>

  {/* MIDDLE: Feature Bullets */}
  <View className="bg-zinc-900 border border-zinc-800 rounded-2xl p-4 my-3 gap-3">
    <BenefitRow title="Không giới hạn lưu trữ hồ sơ xe / đơn hàng" />
    <BenefitRow title="Xuất báo cáo PDF chuẩn in ấn không watermark" />
    <BenefitRow title="Xóa bỏ 100% quảng cáo vĩnh viễn" />
  </View>

  {/* Plan Selector: Highlight Lifetime Package */}
  <View className="flex-row gap-3 my-2">
    <PlanCard 
      title="Trọn Đời (Lifetime)" 
      price="$14.99" 
      badge="TIẾT KIỆM NHẤT" 
      selected={true} 
    />
    <PlanCard 
      title="Hàng Tháng" 
      price="$2.99/tháng" 
      selected={false} 
    />
  </View>

  {/* BOTTOM: Sticky Purchase CTA + Restore */}
  <View className="mt-4">
    <GloveButton title="🚀 Nâng Cấp Ngay - $14.99" variant="primary" onPress={handlePurchase} />
    <Pressable onPress={handleRestore} className="py-2">
      <Text className="text-xs text-center text-zinc-400">Khôi phục giao dịch mua (Restore Purchases)</Text>
    </Pressable>
  </View>
</ModalWrapper>
```

---

## 3. CHECKLIST KIỂM ĐỊNH THIẾT KẾ (DESIGN GATE)

Mọi màn hình do AI sinh ra phải vượt qua 5 tiêu chí:
1. **[ ] Nhận diện Archetype:** Màn hình này thuộc khuôn mẫu nào trong 5 Archetype trên? (Không được lai tạp vô tổ chức).
2. **[ ] Điểm Neo Thị Giác (Hero Anchor):** Mắt người dùng nhìn vào đâu đầu tiên? (Phải có 1 phần tử nổi bật nhất: Gauge, Biểu đồ, hoặc Số tiền).
3. **[ ] Nút Bấm Vùng Ngón Cái (Thumb Reach):** Nút hành động chính có nằm ở 30% đáy màn hình không?
4. **[ ] Kích Thước Tối Thiểu:** Mọi nút bấm và trường nhập liệu có đạt chiều cao tối thiểu 48dp (chế độ thường) hoặc 56dp (chế độ găng tay thợ)?
5. **[ ] Độ Tương Phản Dark Mode:** Nền tối Zinc-950, chữ Zinc-50/Zinc-400, viền Zinc-800. Tuyệt đối không dùng màu đen tuyền `#000000` gây gắt mắt.
