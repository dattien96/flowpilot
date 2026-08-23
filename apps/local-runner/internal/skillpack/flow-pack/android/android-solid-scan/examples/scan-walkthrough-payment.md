# Walkthrough: Quét tính năng Payment

Hướng dẫn từng bước cách tìm lỗi SOLID trong `feature:payment`.

## Bước 1: OCP (Open/Closed)
Tôi chạy: `grep -r "when (" ./feature-payment -A 5`
- **Kết quả**: 
```kotlin
when (paymentType) {
    "CREDIT" -> { /* 50 dòng code xử lý */ }
    "PAYPAL" -> { /* 40 dòng code xử lý */ }
    "CASH" -> { /* 20 dòng code xử lý */ }
}
```
- **Phân tích**: 🔴 Vi phạm OCP. Mỗi lần thêm phương thức thanh toán mới, hàm này lại phình to. Nên áp dụng Strategy pattern đa hình.

## Bước 2: LSP (Liskov)
Chạy: `grep -rn "TODO(" ./feature-payment`
- **Kết quả**: Trong `CashPaymentHandler.kt` có đoạn: `override fun refund() = TODO("Cash cannot be refunded online")`.
- **Phân tích**: 🔴 Vi phạm LSP. Thừa kế `PaymentHandler` nhưng không hỗ trợ `refund()`.

## Bước 3: ISP (Interface Segregation)
Chạy: `grep -rn "interface " ./feature-payment -A 10`
- **Kết quả**: Tìm thấy `PaymentProcessor` interface chứa 12 methods (pay, refund, hold, release, generateInvoice, ...).
- **Phân tích**: 🔴 Vi phạm ISP. Các khách hàng (như Cash handler) không cần `refund` hay `hold`. Phải tách thành `Refundable`, `Payable`, v.v.

## Kế hoạch sửa
1. Tách interface `PaymentProcessor`.
2. Áp dụng Strategy Pattern để loại bỏ khối `when` khổng lồ.
3. Không ném lỗi bằng `TODO()` cho trường hợp `Cash`, thay đổi thiết kế interface.
