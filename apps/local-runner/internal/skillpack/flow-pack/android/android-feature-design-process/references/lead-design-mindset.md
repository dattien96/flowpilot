# Lead Design Mindset

Đây là 4 bước cốt lõi định hình tư duy của một Senior/Lead/Architect khi thiết kế phần mềm.

## A. Tìm biên (Boundaries)
Phần mềm thực chất là tập hợp các thành phần giao tiếp với nhau qua các đường biên (boundaries).
Khi thiết kế một feature:
1. **Owner:** Ai là người thực sự sở hữu data hoặc logic cốt lõi?
2. **Consumer:** Ai là người sử dụng tính năng/data đó?
3. **Provider:** Ai là người cung cấp cơ chế để thực hiện (VD: Room, Retrofit, hệ điều hành)?

**Nguyên tắc:** Giữ Owner và Consumer không bị phụ thuộc cứng (tight-coupling) vào Provider.

## B. Đặt Port đúng phía (Consumer sở hữu Port)
Sai lầm phổ biến nhất của lập trình viên là thiết kế Interface từ góc nhìn của Provider.

- ❌ **Sai:** Bạn viết `SqlUserRepository` (Provider), sau đó bạn tạo interface `UserRepository` ngay cạnh đó (cùng package), và bắt `UserUseCase` (Consumer) gọi vào. Lớp UseCase đang nói ngôn ngữ của Repository.
- ✅ **Đúng:** Bạn đứng ở `UserUseCase` (Consumer). Bạn định nghĩa tôi cần một cái cổng `UserPort` (hoặc interface `GetUserProfile`) ngay tại package của UseCase. Sau đó ở một module xa xôi, bạn tạo `SqlUserAdapter` implement cái cổng đó.

**Lợi ích:** Khi đó, UseCase hoàn toàn độc lập. Bạn có thể thay DB SQL bằng Firebase mà UseCase không cần thay đổi một dòng code hay đổi cả import package.

## C. Port phải nói NĂNG LỰC, không nói CƠ CHẾ
Đây là vế thứ 2 của Dependency Inversion: "Abstraction không được phụ thuộc vào chi tiết".
Tên hàm, tên class của Port (Interface) chỉ được phép mô tả CÁI GÌ (Năng lực - Capability), không được mô tả CÁCH NÀO (Cơ chế - Mechanism).

- ❌ **Vi phạm (Nói cơ chế):** `interface ImageStorage { fun saveBitmap(bmp: Bitmap); fun uploadHttp(...) }` -> Lộ chi tiết Bitmap (Android OS) và Http (Network).
- ✅ **Chuẩn (Nói năng lực):** `interface AvatarManager { fun updateAvatar(imageBytes: ByteArray) }` -> Không quan tâm lưu DB hay gọi API.

## D. Test 3 dòng (Fake Impl Test)
Làm sao biết Interface mình thiết kế ra đã đủ "độc lập" chưa? Hãy thử làm Test 3 dòng.

Hãy thử viết một `FakeAdapter` (ví dụ `InMemoryAvatarManager`).
- Nếu bạn có thể viết nó trong 3-5 dòng code, trả về mock data một cách dễ dàng, không cần setup Context, không cần mock Network... thì thiết kế của bạn đang RẤT TỐT.
- Nếu để Fake được, bạn phải mock 10 cái class lằng nhằng của framework, thì Interface của bạn đang bị "leak" chi tiết của framework vào trong đó.
