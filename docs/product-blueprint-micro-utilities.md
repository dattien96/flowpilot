# MASTER BLUEPRINT: DANH MỤC 7 ỨNG DỤNG MICRO-UTILITY (FLOWPILOT STUDIO)

> **Chiến lược Barbell:** Kết hợp **2 Ứng dụng cày Ads/Traffic khổng lồ** + **5 Ứng dụng chuyên thu tiền IAP/Lifetime giá trị cao**. Chi phí vận hành $0 (Local-First, Zero Server), kiến trúc Monorepo React Native (Expo) chia sẻ Core dùng chung.

---

## 📊 1. MA TRẬN TỔNG QUAN 7 ỨNG DỤNG CHIẾN LƯỢC (US / TIER-1)

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│             DANH MỤC 7 ỨNG DỤNG CHIẾN LƯỢC FLOWPILOT STUDIO (US/TIER-1)                │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 🚀 NHÓM 1: CỖ MÁY CÀY TRAFFIC & ADMOB ADS KHỔNG LỒ (EVERGREEN ASO)                     │
│   1. DocuScan   │ Quét tài liệu & Hóa đơn sang PDF   │ AdMob Ads (70%) + $4.99 Lifetime│
│   2. FlipCalc   │ Tính lãi & phí sàn eBay/Poshmark   │ AdMob Ads (50%) + $19.99 Lifetime│
├────────────────────────────────────────────────────────────────────────────────────────┤
│ 💎 NHÓM 2: BỘ NĂM THỰC CHIẾN / BLUE-COLLAR WTP CỰC CAO ($15 - $50, AN TOÀN PHÁP LÝ)    │
│   3. ProQuote   │ Báo giá & Hóa đơn thợ sửa chữa US  │ $49.99 Lifetime (0% Ads)        │
│   4. TowSafe    │ Cân tải trọng xe bán tải & RV kéo  │ $14.99 Lifetime (0% Ads)        │
│   5. CutCraft   │ Sơ đồ cắt gỗ 2D tối ưu cho thợ mộc │ $14.99 Lifetime / $2.99/mo      │
│   6. SparkyCalc │ Tính ống gen & cỡ dây điện chuẩn NEC│ $19.99 Lifetime (0% Ads)        │
│   7. ShiftSync  │ Lịch ca kíp & Tính giờ công y tá   │ $1.99/mo / $14.99/yr Sub        │
└────────────────────────────────────────────────────────────────────────────────────────┘
```



---

## 🎯 2. CHIẾN LƯỢC GẮN ADS TOÀN HỆ THỐNG (ADS INTEGRATION STRATEGY)

### 2.1. Nguyên Tắc Cốt Lõi: "Ads Là Đòn Bẩy Bán IAP"
* Không spam quảng cáo gây ức chế (tránh bão 1 sao từ người dùng).
* **Chỉ hiển thị Interstitial Ad tại Điểm ngắt tự nhiên (Natural Breakpoint):** Khi người dùng đã hoàn thành xong việc (Ví dụ: Nén xong video, Xuất xong PDF). Lúc này tâm lý người dùng thoải mái và chấp nhận xem quảng cáo 5 giây.
* **Tần suất khống chế (Frequency Capping):** Tối đa 1 quảng cáo toàn màn hình trong vòng 3 đến 5 phút; chỉ hiện sau mỗi 2 lần thực hiện thao tác thành công.
* Dưới mỗi quảng cáo luôn có nút bấm kích cầu: *"🚀 Nâng cấp $4.99 để xóa vĩnh viễn quảng cáo"*.

### 2.2. Bảng Phân Bổ Ads Theo Ngữ Cảnh Từng App

| Ứng dụng | Mức độ Ads | Banner đáy | Interstitial (Toàn màn hình) | Rewarded (Xem nhận thưởng) |
| :--- | :--- | :--- | :--- | :--- |
| **1. ZipClip** *(Nén video)* | 💰 **CỰC CAO** | Có (Adaptive) | Có (Sau mỗi 2 lượt nén thành công) | Có (Xem 1 ad để nén hàng loạt 5 file) |
| **2. DocuScan** *(Scan PDF)* | 💰 **CỰC CAO** | Có (Adaptive) | Có (Sau khi bấm Lưu/Chia sẻ PDF) | Có (Xem 1 ad để nhận diện chữ OCR) |
| **3. ProQuote** *(Báo giá thợ)* | 🚫 **0% ADS** | Không | Không (Tránh làm mất uy tín thợ khi gặp khách) | Không (100% thu tiền từ IAP $49.99) |
| **4. ShiftSync** *(Lịch ca kíp)* | ⚠️ **THẤP** | Có | Chỉ hiện khi bấm xuất báo cáo Timesheet | Không |
| **5. StepFlow** *(Timer ADHD)* | ⚠️ **THẤP** | Không | CẤM trong lúc đếm giờ tập trung | Có (Xem 1 ad để mở routine VIP 24h) |
| **6. PawVault** *(Sổ tiêm)* | ⚠️ **THẤP** | Có | Chỉ hiện khi bấm xuất thẻ Pet Passport PDF | Không |
| **7. FocusZen** *(Cai nghiện ĐT)*| 🚫 **0% ADS** | Không | Không (App tối giản cấm quảng cáo) | Không (100% thu tiền từ IAP) |

---

## 📱 3. CHI TIẾT ĐẦY ĐỦ 7 ỨNG DỤNG (SPECS & SCREENS)

---

### SẢN PHẨM 1: ZipClip – Video & Photo Batch Compressor (Cỗ máy Ads #1)
* **Vấn đề:** Video 4K quá nặng (100MB - 500MB), không gửi được qua Email (<25MB), Discord (<10MB), Zalo. Các app nén hiện tại spam quảng cáo 30s hoặc bẫy $7.99/tuần.
* **Giải pháp kỹ thuật:** Dùng `react-native-compressor` (gọi trực tiếp `MediaCodec` trên Android và `AVAssetExportSession` trên iOS) ➔ App nhẹ < 14MB, nén siêu tốc, máy mát.
* **Màn hình (4 màn):**
  1. *Home & Preset Selector:* Thanh dung lượng bộ nhớ máy + Nút chọn Video/Ảnh + 4 Preset mục tiêu (`<25MB Email/Zalo`, `<10MB Discord`, `Tiết kiệm 80% bộ nhớ`, `Chuẩn TikTok 1080p`).
  2. *Processing View:* Vòng tròn tiến trình % kèm bộ đếm dung lượng giảm trực tiếp theo thời gian thực: `140 MB ➔ ~18 MB (-87%)`.
  3. *Result & Comparison:* Video Player so sánh chất lượng Trước vs Sau + Nút Lưu/Chia sẻ/Xóa file gốc. (Ads trigger sau khi bấm Lưu).
  4. *Paywall:* $4.99 Lifetime (Nén hàng loạt 50 file + Xóa 100% quảng cáo).

---

### SẢN PHẨM 2: DocuScan – Smart Offline PDF Document Scanner (Cỗ máy Ads #2)
* **Vấn đề:** Nhu cầu chụp hóa đơn, hợp đồng, chứng minh thư, bài tập thành file PDF cực kỳ lớn. Các app lớn như CamScanner, Adobe Scan ép đăng ký gói tuần $9.99 hoặc chèn watermark to đùng.
* **Giải pháp:** App scan tài liệu cục bộ, tự động cắt góc, tăng độ tương phản đen trắng, gom nhiều ảnh thành 1 file PDF sắc nét, hoàn toàn miễn phí (đổi lại xem quảng cáo ngắn).
* **Màn hình (4 màn):**
  1. *Document Vault:* Lưới hiển thị các tài liệu đã scan kèm thumbnail trang đầu + Nút Chụp quét hoặc Chọn từ thư viện.
  2. *Camera & Edge Crop:* Camera chụp liên tục nhiều trang (Trang 1, 2, 3...) + Khung căn chỉnh 4 góc mép giấy + 4 bộ lọc (B&W, Magic Color, Grayscale, Gốc).
  3. *PDF Preview & Export:* Xem lướt các trang + Đổi thứ tự trang + Nút Lưu PDF và Chia sẻ. (Ads trigger sau khi bấm Lưu/Chia sẻ).
  4. *Paywall:* $4.99 Lifetime (Xóa vĩnh viễn quảng cáo, xóa dòng bản quyền chân trang, mở khóa OCR xuất file Word).

---

### SẢN PHẨM 3: ProQuote – Contractor Quick Estimate & Invoice (IAP $49.99)
* **Vấn đề:** Thợ sơn, sửa ống nước, cắt cỏ tại Mỹ/Âu kiếm $80 - $150/giờ. Cần chốt đơn và gửi báo giá PDF ngay trên xe tải trong 30 giây trước khi khách gọi đội khác.
* **An toàn pháp lý:** 100% an toàn. Đóng vai trò như "Quyển hóa đơn bán lẻ", người dùng tự thỏa thuận. Kèm Disclaimer miễn trừ trách nhiệm kế toán/thuế.
* **Màn hình (5 màn):**
  1. *Dashboard:* Danh sách báo giá phân loại theo tabs: Nháp, Đã gửi, Đã chốt, Đã thu tiền.
  2. *Quick Builder:* Thông tin khách (chọn từ danh bạ) + Danh mục dịch vụ (Line items) + Tính tạm tính, thuế %, giảm giá %.
  3. *Signature & Photos:* Khung cảm ứng cho khách ký tên bằng ngón tay + Đính kèm 2 ảnh hiện trường trước khi làm.
  4. *Preview & Share:* Sinh file PDF chuyên nghiệp trong 0.5s có logo thợ, bảng kê chi tiết, chữ ký khách. 1 chạm gửi qua iMessage, SMS, WhatsApp.
  5. *Paywall:* $49.99 Lifetime hoặc $9.99/tháng (Không giới hạn báo giá + Logo cá nhân + Đổi màu template).

---

### SẢN PHẨM 4: ShiftSync – Work Shift Calendar & Wage Tracker (Sub $14.99/yr)
* **Vấn đề:** Y tá, điều dưỡng, bảo vệ, công nhân làm việc ca xoay vòng phức tạp (2 ngày, 2 đêm, 4 nghỉ). Hay bị công ty tính sai tiền làm thêm giờ (OT) và phụ cấp ca đêm.
* **An toàn pháp lý:** Công cụ lịch cá nhân thuần túy. Tiền lương chỉ là ước tính tham khảo cá nhân.
* **Màn hình (4 màn):**
  1. *Shift Calendar Grid:* Lưới lịch tháng mã màu ca trực (🟡 Sáng, 🔵 Chiều, 🟣 Đêm, 🟢 Nghỉ) + Đổi ca nhanh 1 chạm + Thẻ tóm tắt thu nhập tạm tính.
  2. *Pattern Generator:* Thiết lập chu kỳ lặp tự động -> Tự động điền lịch cho cả năm tiếp theo trong 0.5s.
  3. *Wage & OT Engine:* Cài đặt mức lương cơ sở + Hệ số OT (sau 8h x1.5, ca đêm +30%, ngày lễ x2.0) + Biểu đồ giờ làm theo tuần.
  4. *Paywall & Export:* Xuất file PDF bảng chấm công gửi cho quản lý + Mua gói Pro ($1.99/tháng hoặc $14.99/năm).

---

### SẢN PHẨM 5: StepFlow – ADHD Visual Routine & Micro-Step Timer (IAP $29.99)
* **Vấn đề:** Người mắc hội chứng ADHD và người hay trì hoãn tại Mỹ/Âu bị "tê liệt ý chí" trước việc lớn ("Dọn phòng", "Học bài"). Họ bị mù thời gian, không cảm nhận được thời gian trôi.
* **An toàn pháp lý:** Thuộc danh mục Productivity (Năng suất), hoàn toàn không phải app y tế.
* **Màn hình (4 màn):**
  1. *Routine Hub:* Thư viện các routine mẫu (Morning Kickstart, Room Reset, Deep Work Launch, Sleep Prep).
  2. *Visual Timer & Runner:* Vòng tròn màu pastel co lại theo thời gian + Chỉ hiển thị đúng 1 bước nhỏ 2 phút + Âm thanh Dopamine ding! + Rung haptic khi xong.
  3. *Task Deconstructor:* Nhập 1 việc gây sợ hãi -> App tự chia thành 4 bước siêu nhỏ 3 phút.
  4. *Paywall:* $29.99 Lifetime hoặc $3.99/tháng (Tạo không giới hạn routine + Âm thanh Brown Noise + Rung nâng cao).

---

### SẢN PHẨM 6: PawVault – Pet Vaccine & Health Records Vault (IAP $12.99)
* **Vấn đề:** 85 triệu hộ gia đình Mỹ nuôi chó mèo. Khi gửi thú cưng vào khách sạn (Boarding) hay đi máy bay, bắt buộc phải xuất trình giấy tiêm phòng Dại (Rabies) còn hạn. Giấy tờ thường bị lạc mất.
* **An toàn pháp lý:** Album lưu trữ hồ sơ cá nhân. Không đưa ra lời khuyên y tế.
* **Màn hình (5 màn):**
  1. *Pet Profile List:* Danh sách thú cưng kèm thẻ căn cước (Pet Passport Card) + Huy hiệu tình trạng vaccine (Xanh/Đỏ).
  2. *Vaccine Vault:* Phân loại chuẩn y tế (Dại, Parvo, Thuốc ve rận) + Chụp ảnh giấy chứng nhận tiêm phòng.
  3. *Smart Reminders:* Cài đặt Local Push Notification nhắc trước 30 ngày và trước 7 ngày khi vaccine hết hạn.
  4. *Pet Passport View:* Xem trước và 1-chạm xuất file PDF thẻ y bạ gửi cho khách sạn thú cưng / bác sĩ.
  5. *Paywall:* $12.99 Lifetime hoặc $1.49/tháng (Quản lý nhiều thú cưng không giới hạn + Đồng bộ gia đình).

---

### SẢN PHẨM 7: FocusZen – Minimalist Launcher & Screen Timer (IAP $29.99)
* **Vấn đề:** Giới trẻ Gen Z và dân văn phòng Mỹ/Âu theo trào lưu Digital Detox / Dumbphone, muốn cai nghiện lướt TikTok/Reels vô thức mà không cần mua thêm điện thoại cục gạch.
* **Màn hình (3 màn):**
  1. *Minimalist Home (Android):* Giao diện đen trắng tối giản, chỉ có Giờ + Ngày + 4 tên app thiết yếu dạng chữ (loại bỏ toàn bộ icon màu sắc).
  2. *Blocker & Delay Settings (iOS/Android):* Chọn app cần chặn + Màn hình trì hoãn hít thở sâu 10 giây trước khi mở app gây nghiện.
  3. *Paywall:* $29.99 Lifetime hoặc $2.99/tháng.

---

### 🌟 3.2. DANH SÁCH 6 ỨNG DỤNG TIỀM NĂNG MỚI (HYPER-NICHE BLUE OCEANS - TIER-1 / US)

---

### SẢN PHẨM 8 (ỨNG VIÊN): TowSafe – RV & Truck Towing / Payload Calculator (IAP $14.99)
* **Vấn đề:** 11+ triệu chủ xe bán tải & RV tại Bắc Mỹ luôn bị quá tải trọng thùng xe (Payload bottleneck) gây nguy cơ lật xe, đứt thắng, mất bảo hiểm. Hiện họ phải dùng Excel phức tạp chia sẻ trên r/GoRVing.
* **Giải pháp:** 100% Offline-First. Nhập thông số tem cửa xe (GVWR, Payload, GCWR) + rơ-moóc + vé cân trạm cân 3 trục (CAT Scale ticket). Đồng hồ đo Xanh/Vàng/Đỏ chỉ rõ quá tải ở trục nào.
* **Màn hình (4 màn):**
  1. *Rig Dashboard:* Đồng hồ đo an toàn tổng quát (GVWR, Payload %, Tongue Weight %, Axle Limits).
  2. *Vehicle & Door Sticker Setup:* Nhập chỉ số tem cửa xe bán tải & thông số rơ-moóc.
  3. *CAT Scale Calculator:* Nhập 3 số đo từ vé trạm cân để tự động tính trọng lượng lưỡi kéo thực tế (Actual Tongue Weight).
  4. *Safety Report & Paywall:* Xuất PDF chứng nhận an toàn trước chuyến đi + IAP $14.99 Lifetime (mở khóa nhiều xe kéo).

---

### SẢN PHẨM 9 (ỨNG VIÊN): CutCraft – 2D Cut List Optimizer & Lumber Waste (IAP $14.99)
* **Vấn đề:** Gỗ dán Plywood cao cấp giá $80-$130/tấm. Thợ mộc và DIY tính nhẩm thủ công mất 45 phút, dễ cắt nhầm gây hỏng gỗ. Các web app thì lag trong xưởng mộc bụi bặm không có mạng.
* **Giải pháp:** Thuật toán 2D Bin-packing tính toán tức thì ngay trên thiết bị. Tính cả độ dày mạch cưa (Blade Kerf 1/8") và thớ gỗ (Grain Direction).
* **Màn hình (4 màn):**
  1. *Material & Stock Setup:* Khổ ván (4x8 ft, 1220x2440 mm), độ dày mạch cưa, hướng vân.
  2. *Cut List Editor:* Nhập danh sách chi tiết cần cắt (Chiều dài x Rộng x Số lượng).
  3. *Visual Cut Diagram:* Sơ đồ ván 2D trực quan, đánh số thứ tự từng nhát cắt tối ưu nhất.
  4. *Print & Paywall:* Xuất PDF 1 trang sắc nét in ra dán bàn cưa + IAP $14.99 Lifetime / $2.99/tháng.

---

### SẢN PHẨM 10 (ỨNG VIÊN): SparkyCalc – NEC Conduit Fill & Wire Sizing (IAP $19.99)
* **Vấn đề:** Thợ điện Mỹ kéo dây điện qua ống gen bắt buộc tuân theo bảng NEC (tối đa 40% thể tích). Vi phạm sẽ bị thanh tra bắt tháo dỡ đền hàng nghìn USD. Nơi làm việc tầng hầm không có sóng điện thoại.
* **Giải pháp:** Tra cứu chuẩn NEC 2020/2023/2026 hoàn toàn Offline. Nút bấm to cho người đeo găng tay (Glove-friendly), Dark Mode chuẩn phòng kỹ thuật ngầm.
* **Màn hình (4 màn):**
  1. *Conduit Fill Calculator:* Chọn loại ống (EMT, PVC, RMC), cỡ ống + Chọn loại dây (THHN, Romex) ➔ Thanh % thể tích đầy.
  2. *Voltage Drop & Ampacity:* Tính độ sụt áp theo chiều dài dây dẫn.
  3. *Inspector PDF Report:* Xuất báo cáo chứng minh công thức chuẩn NEC đưa cho thanh tra công trình.
  4. *Paywall:* $19.99 Lifetime hoặc $2.99/tháng (0% quảng cáo).

---

### SẢN PHẨM 11 (ỨNG VIÊN): FlipCalc – Reseller Profit & Dimension Checker (Ads + IAP $19.99)
* **Vấn đề:** Dân săn đồ cũ (Thrifters, eBay/Poshmark sellers) cần biết trong 5 giây món đồ mua $10 có lãi không sau khi trừ 13.25% phí sàn eBay + cước ship bưu điện cồng kềnh.
* **Giải pháp:** So sánh lợi nhuận ròng song song giữa eBay vs Poshmark vs Mercari vs Bán tiền mặt ngay tại chỗ. Tích hợp bảng cước cân nặng và kích thước thùng hộp USPS.
* **Màn hình (4 màn):**
  1. *Multi-Platform Compare:* Nhập Giá mua + Giá bán dự kiến ➔ Bảng so sánh lợi nhuận ròng thời gian thực giữa 4 sàn.
  2. *Shipping Box Tier:* Đo kích thước hộp -> Cảnh báo cước khối lượng quy đổi (Dimensional weight).
  3. *Inventory & Sourcing Log:* Sổ lưu các món đã gom để cuối năm xuất CSV khai thuế Schedule C.
  4. *Paywall & Ads:* Banner đáy + $19.99 Lifetime xóa ads và mở khóa lưu kho vô hạn.

---

### SẢN PHẨM 12 (ỨNG VIÊN): GutLog – Low-FODMAP & IBS Elimination Diary (IAP $29.99)
* **Vấn đề:** Hàng triệu bệnh nhân ruột kích thích (IBS) cần ăn kiêng đào thải để tìm món gây dị ứng. Các app đếm calo không theo dõi được nhóm chất FODMAP và thang phân y tế Bristol.
* **Giải pháp:** 100% Local-First (an toàn tuyệt đối bảo mật y tế). Ghi chép món ăn trong 2 chạm, ghi nhận triệu chứng sau 2-4h, bảng ma trận tự động tìm ra thực phẩm gây kích ứng cao nhất.
* **Màn hình (4 màn):**
  1. *Food & Meal Log:* Nhập nhanh món ăn phân loại theo nhãn FODMAP (Đỏ/Vàng/Xanh).
  2. *Symptom Tracker:* Đánh giá cơn đau bụng (1-5), chướng bụng, và Thang phân Bristol (Type 1-7).
  3. *Correlation Insights:* Biểu đồ ma trận chỉ điểm thức ăn gây kích ứng cao nhất.
  4. *Doctor Report & Paywall:* Xuất PDF gửi bác sĩ tiêu hóa + IAP $29.99 Lifetime / $4.99/tháng.

---

### SẢN PHẨM 13 (ỨNG VIÊN): QuiltMath – Quilting & Fabric Yardage Calculator (IAP $9.99)
* **Vấn đề:** May chăn ghép vải (Quilting) ở Mỹ là thị trường 4.2 tỷ USD. Tính toán yard vải cắt viền, vải lót, tam giác ghép rất phức tạp. Thiếu vải bị lệch lô màu làm hỏng cả tấm chăn 3 tháng công.
* **Giải pháp:** Giao diện font to, phong cách vintage rõ ràng cho phụ nữ trung niên. Tính chính xác yardage cho từng phần chăn, trừ hao đường may 1/4 inch chuẩn.
* **Màn hình (4 màn):**
  1. *Quilt Calculators Hub:* Danh mục tính: Vải lót (Backing), Vải viền (Binding), Khung (Border), Khối Flying Geese.
  2. *Fabric Stash Inventory:* Kho vải vụn cá nhân (chụp ảnh, ghi kích thước yard có sẵn).
  3. *Project Summary:* Tổng hợp số yard cần mua ở tiệm vải.
  4. *Paywall:* $9.99 Lifetime mở khóa toàn bộ công thức và kho vải không giới hạn.

---

## 🏗️ 4. KIẾN TRÚC MONOREPO & CHIA MODULE CHUẨN

Toàn bộ 7 app được xây dựng trong 1 Monorepo duy nhất, chia sẻ toàn bộ Core Modules trong `packages/`:

```text
flowpilot-mobile-studio/
├── packages/                                  # REUSABLE CORE PACKAGES
│   ├── core-ui/                               # Design tokens, Buttons, Tailwind Theme
│   ├── core-billing/                          # RevenueCat IAP & Paywall View
│   ├── core-ads/                              # Google AdMob Manager & Frequency Capping
│   ├── core-storage/                          # SQLite / MMKV Repository abstraction
│   ├── core-pdf/                              # HTML-to-PDF Engine (expo-print)
│   ├── core-security/                         # Disclaimers, EULA & Sanitization
│   └── core-common/                           # Date utils, Currency, File helpers
│
└── apps/                                      # 7 APPS THIN CLIENTS
    ├── docuscan/                              # App 1: Scan PDF & Hóa đơn (Ads + IAP)
    ├── flipcalc/                              # App 2: Tính lãi buôn đồ cũ eBay/Poshmark (Ads + IAP)
    ├── proquote/                              # App 3: Báo giá thợ US ($49.99 IAP)
    ├── towsafe/                               # App 4: Tải trọng xe bán tải & RV ($14.99 IAP)
    ├── cutcraft/                              # App 5: Tối ưu cắt gỗ 2D ($14.99 IAP)
    ├── sparkycalc/                            # App 6: Tính ống gen & dây điện NEC ($19.99 IAP)
    └── shiftsync/                             # App 7: Lịch ca kíp & giờ làm ($14.99/yr Sub)
```

---

## 📅 5. LỘ TRÌNH TRIỂN KHAI CUỐN CHIẾU 7 TUẦN

* **Tuần 1: Khởi tạo Monorepo & Dựng 7 `packages/core-*`:** Setup Turborepo, `core-ui`, `core-billing`, `core-ads`, `core-storage`, `core-pdf`, `core-security`, `core-common`.
* **Tuần 2: Build `ProQuote`:** App IAP giá trị cao nhất ($49.99), kiểm chứng luồng duyệt Store và thanh toán RevenueCat.
* **Tuần 3: Build `TowSafe`:** Tải trọng xe bán tải & RV kéo, kiểm chứng đồng hồ Gauge và tem cửa xe.
* **Tuần 4: Build `CutCraft`:** Thuật toán 2D bin-packing cắt ván gỗ, kiểm chứng xuất bản vẽ PDF xưởng mộc.
* **Tuần 5: Build `SparkyCalc`:** Tra cứu bảng chuẩn NEC thợ điện, giao diện Glove-friendly nút to.
* **Tuần 6: Build `ShiftSync`:** Lịch xoay ca y tá, tạo dòng tiền thuê bao định kỳ (MRR).
* **Tuần 7: Build `DocuScan` & `FlipCalc`:** Hoàn tất 2 cỗ máy cày traffic AdMob và dân săn đồ cũ.

---

## 🔬 6. QUY TRÌNH VERIFY NHU CẦU & ĐỘ CHÍNH XÁC THỊ TRƯỜNG MỸ TỪ VIỆT NAM (REMOTE VALIDATION FRAMEWORK)

> **Nguyên tắc cốt lõi:** Nhóm ứng dụng **Thực chiến (Trades / Blue-Collar / Utility)** dễ verify từ xa hơn 100 lần so với các app phong cách sống (Lifestyle / Social / Dating). Thợ mộc, thợ điện hay dân kéo xe ở Mỹ hành động dựa trên **Toán học, Tiêu chuẩn ngành luật định (Codified Standards)** và **Nỗi đau mất tiền thật**, chứ không phụ thuộc vào thị hiếu văn hóa nhất thời.

### 6.1. 5 Phương Pháp Thực Chiến Kiểm Chứng Từ Xa ($0 Chi Phí)

#### 1. "Đào Mỏ Vàng" Từ Đánh Giá 1-3 Sao Trên US App Store
* **Cơ chế:** Đối thủ đã trả tiền quảng cáo kéo người dùng về, nhưng làm sản phẩm cẩu thả hoặc vòi tiền vô lý. Khách hàng Mỹ có thói quen viết review chê bai rất chi tiết.
* **Cách thực hiện từ VN:** Chuyển vùng App Store sang US (hoặc qua Appfigures/Sensor Tower/web URL `apps.apple.com/us/...`), lọc các bài đánh giá 1-3 sao của top 3 app đối thủ trong 12 tháng qua.
* **Quy luật chuyển hóa:**
  * Khách chửi: *"Ép thuê bao $7.99/tuần cho một phép tính đơn giản"* ➔ **Quyết định:** Bán đứt Lifetime $9.99 - $14.99.
  * Khách chửi: *"Không tính độ dày mạch cưa kerf làm hỏng tấm gỗ"* ➔ **Quyết định:** Đưa Blade Kerf thành tính năng trọng tâm của CutCraft.
  * Khách chửi: *"Vào tầng hầm mất sóng app quay vòng vòng không tính được"* ➔ **Quyết định:** Khắc sâu định vị 100% Offline-First.

#### 2. "Nằm Vùng" Trên Subreddit & Diễn Đàn Chuyên Ngành Mỹ
* Người Mỹ thảo luận chuyên môn sâu trên Reddit và forum độc lập thay vì Facebook:
  * **`towsafe`:** `r/GoRVing` (350k members), `r/traveltrailers` (120k members), `r/f150`, forum `iRV2.com`. Hàng ngày có hàng chục bài đăng ảnh tem cửa xe hỏi kéo rơ-moóc có bị quá tải không và chia sẻ file Excel tự chế.
  * **`cutcraft`:** `r/woodworking` (4.5M members), `r/BeginnerWoodWorking` (600k members). Dân làm mộc liên tục hỏi cách chia ván 4x8 ft và tối ưu đường cưa.
  * **`sparkycalc`:** `r/electricians` (500k members), `r/IBEW`. Thảo luận bảng tra cứu conduit fill chương 9 luật NEC.
  * **`flipcalc`:** `r/Flipping` (300k members). Thảo luận biểu phí mới của eBay/Poshmark và cước bưu điện USPS.
  * **`shiftsync`:** `r/nursing` (800k members). Y tá bàn tán về lịch ca xoay vòng và cách tính OT ca đêm.

#### 3. Căn Cứ Công Thức Chuẩn Đã Được Luật Hóa Công Khai (Codified Standards)
* Toàn bộ công thức tính toán đều là tài liệu công khai, minh bạch, không phải tự sáng chế:
  * **`towsafe`:** Tiêu chuẩn Hiệp hội Công nghiệp RV Mỹ (RVIA) và Hiệp hội Kỹ sư Ô tô (SAE J2807):
    $$\text{Payload} = \text{GVWR} - \text{Curb Weight} - \text{Passengers} - \text{Cargo}$$
    $$\text{Actual Tongue Weight (CAT Scale)} = (\text{Truck Axles with trailer}) - (\text{Truck Axles alone})$$
  * **`sparkycalc`:** Bộ luật Tiêu chuẩn Điện Quốc gia Mỹ (**NEC** - National Electrical Code) Chương 9 (Bảng 1, Bảng 4, Bảng 5): Quy định thể tích chiếm chỗ tối đa (1 dây 53%, 2 dây 31%, 3+ dây 40%).
  * **`cutcraft`:** Thuật toán 2D Bin Packing kinh điển (Guillotine / MaxRects cut) có sẵn benchmark chuẩn quốc tế.

#### 4. "Smoke Test" & Xin Feedback Trực Tiếp Người Mỹ (TestFlight 0 Đồng)
* Khi hoàn thành bản Prototype hoặc TestFlight nội bộ:
* Đăng một bài viết chân thành, phi thương mại lên đúng Subreddit ngành:
  > *"Hey guys, I got frustrated with clunky spreadsheets and predatory $9.99/week calculator apps when figuring out door jamb payload & CAT scale math. So I built a 100% offline, free lightweight tool for myself. Would love to know if the numbers match your real-world rigs: [Link TestFlight/Demo]. Any feedback on missing fields is greatly appreciated!"*
* Người Mỹ trong các sub này phản hồi cực kỳ thẳng thắn, chính xác về mặt kỹ thuật, và sẽ chỉ ra ngay nếu thiếu bất kỳ biến số nào ngoài thực tế (ví dụ: trọng lượng Weight Distribution Hitch).

#### 5. Kiểm Chứng Bằng Dữ Liệu Tìm Kiếm ASO (Search Popularity)
* Dữ liệu tìm kiếm của Apple Search Ads không biết nói dối.
* Các từ khóa `towing calculator`, `cut list optimizer`, `conduit fill calculator`, `contractor estimate` có chỉ số Search Popularity từ **25 đến 45/100** trên App Store Mỹ.
* Đây là khoảng "Sweet Spot" hoàn hảo: Đủ lớn để có dòng người tải tự nhiên ổn định quanh năm, nhưng không quá khổng lồ để các tập đoàn lớn đổ triệu đô vào thâu tóm.

---

### 6.2. Bảng Ma Trận Nguồn Dữ Liệu & Kênh Verify Từng App

| App | Kênh "Nằm Vùng" Chính | Nguồn Công Thức Tiêu Chuẩn | Đối Thủ Cần Đọc 1-Star Review |
| :--- | :--- | :--- | :--- |
| **1. ProQuote** | `r/Carpentry`, `r/Plumbing`, `r/Handyman` | Mẫu báo giá thợ Mỹ (Line items, Tax, 10% Overhead) | Jobber, Invoice Simple, Joist |
| **2. TowSafe** | `r/GoRVing`, `r/traveltrailers`, `iRV2.com` | Tiêu chuẩn SAE J2807, Bảng cân CAT Scale 3 trục | Weigh Safe, TowCalc, LoadMate |
| **3. CutCraft** | `r/woodworking`, `r/BeginnerWoodWorking` | Thuật toán 2D Bin Packing, Quy cách ván 4x8 ft (1220x2440) | CutList Optimizer, SketchCut |
| **4. SparkyCalc** | `r/electricians`, `r/IBEW` | Sổ tay NEC 2020 / 2023 / 2026 Chapter 9 Tables | Ugly's Electrical, Southwire Conduit |
| **5. ShiftSync** | `r/nursing`, `r/ShiftWork` | Chu kỳ xoay ca Pitman, DuPont; Hệ số OT US FLSA | Shift Life, NurseGrid, Work Shift |
| **6. FlipCalc** | `r/Flipping`, `r/eBaySellerAdvice` | Biểu phí eBay Final Value Fee 2026, Cước USPS Priority | Profit Check, FlipProfit, FeeTally |
| **7. DocuScan** | App Store Reviews mục "Document Scanner" | Chuẩn PDF/A-1b, Thuật toán lọc màu tài liệu | CamScanner, Scanner App |


