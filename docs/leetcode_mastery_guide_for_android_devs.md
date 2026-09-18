# LEETCODE MASTERY PLAYBOOK DÀNH CHO ANDROID KOTLIN DEVELOPER
> **Mục tiêu:** Thoát khỏi bẫy *"Nhìn lời giải thì hiểu, tự code thì tắc"*, tự tin làm chủ các bài phỏng vấn Easy / Medium trong 6 tháng.

---

## PHẦN 1: QUY TRÌNH 4 BƯỚC "GIẢI MÃ ĐỀ BÀI" TRƯỚC KHI GÕ CODE

Lý do lớn nhất khiến bạn bị "tắc" là vì bạn **nhảy vào nghĩ thuật toán quá sớm** mà chưa bóc tách đủ dữ liệu từ đề bài. Hãy tuân thủ quy trình 4 bước sau:

```
[1. Đọc I/O & Kiểu dữ liệu] 
           ↓
[2. Tra cứu Ràng buộc (Constraints) → Đoán Độ phức tạp]
           ↓
[3. Bắt Từ khóa vàng (Keywords Dictionary)]
           ↓
[4. Xác định Bất biến (Loop Invariant) & Viết mã giả ra nháp]
```

### Bước 1: Kiểm tra Input / Output & Cạm bẫy kiểu dữ liệu
* **Số âm có thể xuất hiện không?** (Nếu có: Two Pointers tính tổng sẽ không đơn điệu nếu chưa sort; Sliding Window tính tổng dương sẽ gãy).
* **Mảng rỗng hoặc có 1 phần tử không?** ($N = 0, N = 1$).
* **Có bị tràn số 32-bit Int không?** 
  * Phép nhân: `nums[i] * nums[j]` $\to$ có cần `Long` không?
  * Tính tổng: Prefix Sum có vượt quá $2 \times 10^9$ không?
  * Mid trong Binary Search: Luôn dùng `val mid = left + (right - left) / 2` thay vì `(left + right) / 2`.

---

### Bước 2: Bảng tra cứu "Ràng buộc (Constraints) $\to$ Thuật toán"

Ràng buộc $N$ (kích thước mảng/chuỗi) chính là **"lời nhắc lộ đề"** của người ra đề:

| Giá trị $N$ | Time Complexity tối đa | Hướng thuật toán bắt buộc nghĩ tới |
| :--- | :--- | :--- |
| $N \le 10 \sim 16$ | $O(2^N)$ hoặc $O(N!)$ | **Backtracking / Brute-force / Bitmask DP** (Permutations, Subsets). |
| $N \le 20 \sim 40$ | $O(2^{N/2})$ | **Meet-in-the-middle**, Backtracking có tỉa nhánh mạnh. |
| $N \le 100 \sim 400$ | $O(N^3)$ | 3 vòng lặp, Floyd-Warshall, Matrix multiplication. |
| $N \le 1,000 \sim 2,000$ | $O(N^2)$ | 2 vòng for lồng nhau, 2D Dynamic Programming, 3Sum ($O(N^2)$). |
| **$N \le 10^5 \sim 2 \times 10^5$** | **$O(N \log N)$ hoặc $O(N)$** | **Sorted + Binary Search, Two Pointers, Sliding Window, Monotonic Stack, Prefix Sum, Heap**. *(Đây là mức phổ biến nhất trong phỏng vấn!)* |
| $N \ge 10^6 \sim 10^9$ | $O(\log N)$ hoặc $O(1)$ | **Binary Search on Answer, Toán học, Bit manipulation**. |

> **Nguyên tắc vàng:** Máy chấm LeetCode thực hiện khoảng $10^7 \sim 10^8$ phép tính/giây. Lấy độ phức tạp nhân với $N$, nếu vượt quá $10^8$ thì chắc chắn dính **TLE (Time Limit Exceeded)**.

---

### Bước 3: Từ điển "Từ Khóa Vàng" (Problem Triggers)

#### 1. Phân biệt rõ 3 khái niệm sống còn:
* **`Subarray / Substring` (Mảng/Chuỗi con LIÊN TIẾP):** Các phần tử phải đứng liền kề nhau.
  * $\to$ *Vũ khí chính:* **Sliding Window, Prefix Sum, Kadane's algorithm**.
* **`Subsequence` (Dãy con KHÔNG CẦN LIÊN TIẾP):** Giữ nguyên thứ tự xuất hiện ban đầu nhưng có thể bỏ qua một số phần tử.
  * $\to$ *Vũ khí chính:* **Dynamic Programming, Greedy, Two Pointers (khi so khớp 2 chuỗi)**.
* **`Subset` (Tập con):** Không cần liên tiếp, không quan tâm thứ tự.
  * $\to$ *Vũ khí chính:* **Backtracking, Bitmask, Sort rồi xử lý**.

#### 2. Các cụm từ kích hoạt phản xạ vô điều kiện:

| Từ khóa trong đề | Phản xạ đầu tiên trong não |
| :--- | :--- |
| *"Sorted array"* / *"Array is sorted"* | **Binary Search** hoặc **Two Pointers** (Collision). Không được dùng $O(N^2)$. |
| *"In-place"*, *"O(1) extra memory"* | **Fast-Slow Pointers**, Backward fill, hoặc Đổi dấu chỉ số mảng. |
| *"Contiguous subarray"* + *"Sum equals K"* | **Prefix Sum + HashMap** (hoặc Sliding Window nếu toàn số dương). |
| *"Longest/Shortest substring/subarray with condition..."* | **Sliding Window** (Co giãn 2 con trỏ `left`, `right`). |
| *"At most K..."*, *"At least K..."* | **Sliding Window biến thiên** hoặc Trick $\text{Exact}(K) = \text{AtMost}(K) - \text{AtMost}(K-1)$. |
| *"Minimize the maximum..."* / *"Maximum minimum..."* | **Binary Search On Answer** (Chặt nhị phân trên tập nghiệm). |
| *"All elements appear twice except one"* | **Bitwise XOR** (`a ^ a = 0`). |
| *"Values are in range $[1, N]$"* trong mảng kích thước $N$ | **In-place Hash / Negation trick** (Dùng chính giá trị làm index). |
| *"Next greater element"* / *"Next warmer day"* / *"Stock span"* | **Monotonic Stack**. |
| *"Top K elements"* / *"Kth largest/frequent"* | **Min-Heap (PriorityQueue)** kích thước $K$ hoặc **QuickSelect**. |
| *"Count of elements > N/2"* (Majority) | **Boyer-Moore Voting Algorithm**. |
| *"Find all combinations / permutations"* | **Backtracking (Quay lui)**. |

---

### Bước 4: Chuyển dịch từ Kotlin Functional sang Imperative Invariant

Trong Android, bạn hay viết:
```kotlin
// Không nên dùng trong thuật toán đòi hỏi O(1) memory hoặc kiểm soát con trỏ chặt:
val result = nums.filter { it > 0 }.distinct().sorted() // Tốn nhiều mảng phụ, allocation GC liên tục
```

Khi làm LeetCode, trước khi code hãy viết **3 câu bất biến (Invariants)** ra nháp:
1. `Con trỏ đại diện cho cái gì?` (Ví dụ: `slow` là đuôi của mảng sạch đã lọc; `fast` là thám thính).
2. `Điều kiện để con trỏ di chuyển là gì?` (Ví dụ: gặp số khác `val` thì copy `nums[slow++] = nums[fast]`).
3. `Vòng lặp dừng khi nào?` (`left < right` hay `left <= right`?).

---

# PHẦN 2: HỆ THỐNG PHÂN NHÓM MICRO-ARCHETYPES TOÀN DIỆN
*(Bao gồm toàn bộ bài trong list của bạn + Các bài kinh điển bổ sung cần phải biết)*

---

## NHÓM 1: TWO POINTERS (HAI CON TRỎ)

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "LOẠI TRỪ KHÔNG GIAN TÌM KIẾM $N \times N$":**  
> Dân chuyên Tin không nghĩ Two Pointers là "chạy con trỏ hú họa". Họ hình dung bài toán như một ma trận tọa độ 2D kích thước $N \times N$ đại diện cho tất cả các cặp nghiệm `(left, right)`.  
> Khi mảng đã sắp xếp và `nums[left] + nums[right] > target`: Vì mảng tăng dần nên `nums[right]` cộng với bất kỳ số nào bên phải `left` cũng đều sẽ $> target$. Do đó, việc giảm `right--` tương đương với **loại bỏ vĩnh viễn cả một cột $N$ nghiệm khả dĩ** chỉ bằng 1 phép so sánh! Đó là lý do Two Pointers thu hẹp không gian từ $O(N^2)$ về $O(N)$.

### Type 1.1: Fast & Slow Pointers — Sửa mảng tại chỗ ($O(1)$ Space)
* **Bản chất:** `slow` giữ vị trí cần ghi đè tiếp theo, `fast` duyệt qua từng phần tử.
* **Danh sách của bạn:**
  * **26. Remove Duplicates from Sorted Array** *(Easy)*
  * **27. Remove Element** *(Easy)*
  * **80. Remove Duplicates from Sorted Array II** *(Medium - Điều kiện: `nums[fast] > nums[slow - 2]`)*
  * **287. Find the Duplicate Number** *(Medium - Floyd's Tortoise and Hare)*
* **⭐ Bài kinh điển bổ sung cần luyện:**
  * **141. Linked List Cycle** *(Easy - Nhận biết chu trình bằng rùa và thỏ)*
  * **283. Move Zeroes** *(Easy - Đẩy số 0 về cuối mảng bằng Fast-Slow)*

### Type 1.2: Collision Pointers — Hai đầu thu hẹp (Mảng đã sắp xếp)
* **Bản chất:** `left = 0`, `right = n - 1`. Dựa vào tính đơn điệu của mảng để quyết định tăng `left` hay giảm `right`. Khi mở rộng lên $3$ hoặc $4$ phần tử, ta dùng vòng lặp cố định $1$ hoặc $2$ con trỏ bên ngoài, bên trong chạy Collision Pointers.
* **Danh sách của bạn:**
  * **167. Two Sum II - Input Array Is Sorted** *(Medium - Khởi nguồn)*
  * **15. 3Sum** *(Medium - Cố định 1 phần tử + Two Pointers; nhớ skip duplicates)*
  * **18. 4Sum** *(Medium - Cố định 2 phần tử + Two Pointers)*
  * **977. Squares of a Sorted Array** *(Easy - Điền ngược từ cuối mảng kết quả)*
  * **2824. Count Pairs Whose Sum is Less than Target** *(Easy)*
* **⭐ Bài kinh điển bổ sung cần luyện:**
  * **11. Container With Most Water** *(Medium - Di chuyển cột có chiều cao thấp hơn)*
  * **42. Trapping Rain Water** *(Hard - Two Pointers chặn 2 đầu với `maxLeft` và `maxRight`)*
  * **125. Valid Palindrome** *(Easy - So khớp từ 2 đầu)*

### Type 1.3: Backward Fill — Điền ngược từ đuôi
* **Bản chất:** Nếu ghi từ đầu sẽ đè mất dữ liệu chưa đọc $\to$ Bắt đầu ghi từ index cuối cùng lùi về trước.
* **Danh sách của bạn:**
  * **88. Merge Sorted Array** *(Easy)*
  * **1089. Duplicate Zeros** *(Easy)*

### Type 1.4: Partitioning & Parity (Phân chia mảng)
* **Bản chất:** Đưa các phần tử thỏa mãn tính chất (chẵn/lẻ, màu sắc) về các vùng riêng biệt.
* **Danh sách của bạn:**
  * **75. Sort Colors** *(Medium - Dutch National Flag: 3 con trỏ `low, mid, high`)*
  * **905. Sort Array By Parity** *(Easy)*
  * **922. Sort Array By Parity II** *(Easy)*

### Type 1.5: Subsequence Matching & Two-Array Traverse
* **Bản chất:** 2 con trỏ chạy trên 2 mảng/chuỗi độc lập để so khớp.
* **Danh sách của bạn:**
  * **349. Intersection of Two Arrays** *(Easy)*
  * **350. Intersection of Two Arrays II** *(Easy)*
  * **522. Longest Uncommon Subsequence II** *(Medium)*
  * **524. Longest Word in Dictionary through Deleting** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **392. Is Subsequence** *(Easy - Nền tảng của so khớp chuỗi)*

---

## NHÓM 2: SLIDING WINDOW (CỬA SỔ TRƯỢT)

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "CON SÂU ĐO" (GREEDY EXPAND & RESCUE CONTRACT):**  
> Dân chuyên Tin xem Sliding Window như một **con sâu đo**:  
> - **Đầu trước (`right` - Tham lam / Greedy):** Luôn luôn bò về phía trước để nạp dữ liệu vào trạng thái cửa sổ.  
> - **Đuôi sau (`left` - Cứu vãn / Invalidation Rescue):** Chỉ co lên khi và chỉ khi cửa sổ bị **"vỡ quy tắc"** (vi phạm điều kiện đề bài). Co lại cho đến khi cửa sổ hợp lệ trở lại thì dừng.  
> Mọi bài Sliding Window đều tuân thủ chu trình: `Nạp right -> Co left nếu vi phạm -> Cập nhật max/min`.

### Type 2.1: Fixed Window Size $K$ (Cửa sổ cố định)
* **Bản chất:** Tính tổng/kết quả $K$ phần tử đầu tiên. Sau đó mỗi bước: `window += nums[i] - nums[i - K]`.
* **Danh sách của bạn:**
  * **643. Maximum Average Subarray I** *(Easy)*
  * **1343. Number of Sub-arrays of Size K and Average Greater than or Equal to Threshold** *(Medium)*
  * **1652. Defuse the Bomb** *(Easy - Cửa sổ vòng tròn)*
  * **1984. Minimum Difference Between Highest and Lowest of K Scores** *(Easy - Sort rồi trượt)*
* **⭐ Bài kinh điển bổ sung:**
  * **438. Find All Anagrams in a String** *(Medium - Cửa sổ kích thước bằng độ dài pattern)*

### Type 2.2: Dynamic Window — "At Most K Invalid" (Thu phóng cửa sổ)
* **Bản chất:** `right` luôn tiến lên để nạp phần tử. Khi số lượng vi phạm điều kiện $> K$, tăng `left` để co cửa sổ lại. Cập nhật `maxLen = max(maxLen, right - left + 1)`.
* **Danh sách của bạn:**
  * **1004. Max Consecutive Ones III** *(Medium - Tối đa $K$ số 0)*
  * **1493. Longest Subarray of 1's After Deleting One Element** *(Medium - Chính là bài 1004 với $K = 1$)*
  * **219. Contains Duplicate II** *(Easy - Duy trì HashSet trong cửa sổ độ rộng $K$)*
* **⭐ Bài kinh điển bổ sung:**
  * **3. Longest Substring Without Repeating Characters** *(Medium - Phải làm bài này!)*
  * **209. Minimum Size Subarray Sum** *(Medium - Thu nhỏ cửa sổ tìm min length)*
  * **76. Minimum Window Substring** *(Hard - Trùm cuối của Dynamic Window)*

### Type 2.3: Subarray Counting Trick: $\text{Exact}(K) = \text{AtMost}(K) - \text{AtMost}(K - 1)$
* **Bản chất:** Đếm mảng con có chính xác $K$ phần tử khó giữ tính đơn điệu $\to$ đổi thành hiệu của 2 hàm `atMost`.
* **Danh sách của bạn:**
  * **795. Number of Subarrays with Bounded Maximum** *(Medium)*
  * **930. Binary Subarrays With Sum** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **992. Subarrays with K Different Integers** *(Hard)*

---

## NHÓM 3: BINARY SEARCH (TÌM KIẾM NHỊ PHÂN)

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "HÀM VỊ TỪ ĐƠN ĐIỆU" (PREDICATE FUNCTION $P(x)$):**  
> Dân chuyên Tin **không định nghĩa Binary Search là "tìm kiếm trong mảng đã sắp xếp"**.  
> Binary Search là kỹ thuật **tìm điểm gãy (Boundary)** của một hàm đúng/sai $P(x) \in \{False, True\}$ có tính đơn điệu:  
> $$\text{False, False, False, } \mathbf{[True]}, \text{ True, True}$$  
> **Quy trình 3 bước cho mọi bài:**  
> 1. Quên mảng đi, xác định ẩn số nghiệm cần tìm là $x$ và khoảng nghiệm `[min_x, max_x]`.  
> 2. Viết hàm kiểm tra đơn điệu `fun isValid(x): Boolean` ("Với giá trị $x$ này, có thỏa mãn yêu cầu không?").  
> 3. Viết template 5 dòng: nếu `isValid(mid)` thỏa mãn $\to$ lưu nghiệm và co khoảng nghiệm tìm kết quả tốt hơn.

### Type 3.1: Canonical BS & Boundaries (Cơ bản & Tìm biên)
* **Bản chất:** Kiểm soát chặt chẽ `left <= right`. Chú ý `lower_bound` (vị trí đầu tiên $\ge x$) và `upper_bound` (vị trí đầu tiên $> x$).
* **Danh sách của bạn:**
  * **704. Binary Search** *(Easy)*
  * **35. Search Insert Position** *(Easy - Lower bound)*
  * **744. Find Smallest Letter Greater Than Target** *(Easy - Upper bound)*
  * **1351. Count Negative Numbers in a Sorted Matrix** *(Easy)*
  * **1385. Find the Distance Value Between Two Arrays** *(Easy)*
  * **2089. Find Target Indices After Sorting Array** *(Easy)*
  * **2529. Maximum Count of Positive Integer and Negative Integer** *(Easy)*
* **⭐ Bài kinh điển bổ sung:**
  * **33. Search in Rotated Sorted Array** *(Medium - Tìm kiếm trên mảng bị xoay)*
  * **153. Find Minimum in Rotated Sorted Array** *(Medium)*

### Type 3.2: Binary Search On Answer (Tìm kiếm trên không gian nghiệm) ⭐ *TOP 1 PHỎNG VẤN*
* **Bản chất:** Đề bài hỏi: *"Tìm giá trị nhỏ nhất sao cho..."* hoặc *"Tìm giá trị lớn nhất để..."*. Ta không tìm trên mảng mà tìm kiếm trên khoảng kết quả `[minPossible, maxPossible]`. Viết hàm `isValid(mid): Boolean`.
* **Danh sách của bạn:** *(Các bài dưới đây chia sẻ 80% cấu trúc code!)*
  * **875. Koko Eating Bananas** *(Medium - Tốc độ ăn nhỏ nhất)*
  * **1011. Capacity To Ship Packages Within D Days** *(Medium - Trọng tải tàu nhỏ nhất)*
  * **410. Split Array Largest Sum** *(Hard - Nhưng code y hệt bài 1011)*
  * **1482. Minimum Number of Days to Make m Bouquets** *(Medium)*
  * **1870. Minimum Speed to Arrive on Time** *(Medium)*
  * **2187. Minimum Time to Complete Trips** *(Medium)*

### Type 3.3: 2D Matrix / Multi-array BS
* **Danh sách của bạn:**
  * **240. Search a 2D Matrix II** *(Medium - Dò từ góc trên-phải)*
  * **475. Heaters** *(Medium - BS tìm heater gần nhất cho mỗi nhà)*
  * **4. Median of Two Sorted Arrays** *(Hard - Binary Search trên đường cắt phân hoạch 2 mảng)*

---

## NHÓM 4: PREFIX SUM & DIFFERENCE ARRAY

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "ĐẠO HÀM & TÍCH PHÂN RỜI RẠC" (DISCRETE CALCULUS):**  
> Dân chuyên Tin nhìn mảng số học bằng con mắt của Giải tích toán học:  
> - **Đạo hàm (Difference Array - Mảng hiệu):** Muốn cộng dồn giá trị lên đoạn $[L, R]$, không lặp $O(N)$. Chỉ cần ghi nhận 2 điểm biến thiên ở biên: `diff[L] += val` và `diff[R + 1] -= val`. Cập nhật đoạn giảm từ $O(N) \to O(1)$.  
> - **Tích phân (Prefix Sum - Mảng cộng dồn):** Muốn tính tổng đoạn $[L, R]$, lấy tích phân tại $R$ trừ tích phân tại $L - 1$. Truy vấn đoạn giảm từ $O(N) \to O(1)$.  
> Chúng là cặp phép toán nghịch đảo: Mảng ban đầu $\xrightarrow{\text{Đạo hàm}}$ Mảng Hiệu $\xrightarrow{\text{Tích phân}}$ Mảng ban đầu.


### Type 4.1: Running Sum & Cân bằng hai vế (Equilibrium)
* **Bản chất:** $\text{Sum}(L, R) = \text{prefix}[R] - \text{prefix}[L - 1]$. Điểm cân bằng khi `leftSum == totalSum - leftSum - nums[i]`.
* **Danh sách của bạn:**
  * **303. Range Sum Query - Immutable** *(Easy)*
  * **1480. Running Sum of 1d Array** *(Easy)*
  * **724. Find Pivot Index** & **1991. Find the Middle Index in Array** *(Easy - Hai bài là một)*
  * **1413. Minimum Value to Get Positive Step by Step Sum** *(Easy)*
  * **1732. Find the Highest Altitude** *(Easy)*
  * **2574. Left and Right Sum Differences** *(Easy)*
  * **2640. Find the Score of All Prefixes of an Array** *(Medium)*
  * **3028. Ant on the Boundary** *(Easy)*

### Type 4.2: Prefix/Suffix Products & Math Offsets
* **Bản chất:** Tính toán kết hợp từ 2 phía: `prefix[i-1]` và `suffix[i+1]`.
* **Danh sách của bạn:**
  * **238. Product of Array Except Self** *(Medium - Tích trái $\times$ tích phải)*
  * **1664. Ways to Make a Fair Array** *(Medium - Tổng chẵn lẻ đảo ngôi)*
  * **1685. Sum of Absolute Differences in a Sorted Array** *(Medium)*
  * **1588. Sum of All Odd Length Subarrays** *(Easy)*
  * **2389. Longest Subsequence With Limited Sum** *(Easy - Sort + Prefix Sum + BS)*
  * **2391. Minimum Amount of Time to Collect Garbage** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **560. Subarray Sum Equals K** *(Medium - Dùng HashMap lưu tần suất Prefix Sum: `map[prefix - k]`)*

### Type 4.3: Difference Array / Line Sweep (Mảng hiệu)
* **Bản chất:** Cập nhật đoạn $[L, R]$ với giá trị $+V$: `diff[L] += V`, `diff[R + 1] -= V`. Sau đó chạy Prefix Sum để ra mảng thực.
* **Danh sách của bạn:**
  * **1893. Check if All the Integers in a Range Are Covered** *(Easy)*
  * **2848. Points That Intersect With Cars** *(Easy)*
* **⭐ Bài kinh điển bổ sung:**
  * **253. Meeting Rooms II** *(Medium - Line sweep đếm phòng họp cần thiết)*

---

## NHÓM 5: BIT MANIPULATION (THAO TÁC BIT)

### Type 5.1: The XOR Cancellation (`X ^ X = 0` và `X ^ 0 = X`)
* **Danh sách của bạn:**
  * **136. Single Number** *(Easy)*
  * **1720. Decode XORed Array** *(Easy)*
  * **2433. Find The Original Array of Prefix Xor** *(Medium)*
  * **2997. Minimum Number of Operations to Make Array XOR Equal to K** *(Medium)*
  * **3158. Find the XOR of Numbers Which Appear Twice** *(Easy)*
* **⭐ Bài kinh điển bổ sung:**
  * **268. Missing Number** *(Easy - XOR từ 0 đến N với mảng)*

### Type 5.2: Bit Counting, State & Masks
* **Bản chất:** Thao tác trên bit: kiểm tra bit thứ $i$ (`(num shr i) and 1`), đếm số bit 1 (`Integer.bitCount(num)`).
* **Danh sách của bạn:**
  * **137. Single Number II** *(Medium - Đếm tổng bit từng vị trí modulo 3)*
  * **1356. Sort Integers by The Number of 1 Bits** *(Easy)*
  * **1684. Count the Number of Consistent Strings** *(Easy - 1 số Int làm bitmask cho 26 chữ cái)*
  * **1863. Sum of All Subset XOR Totals** *(Easy)*
  * **2044. Count Number of Maximum Bitwise-OR Subsets** *(Medium)*
  * **2317. Maximum XOR After Operations** *(Medium)*
  * **2859. Sum of Values at Indices With K Set Bits** *(Easy)*
  * **2917. Find the K-or of an Array** *(Easy)*
  * **2932. Maximum Strong Pair XOR I** *(Easy)*
  * **3095. Shortest Subarray With OR at Least K I** *(Easy)*

### Type 5.3: Prefix XOR
* **Danh sách của bạn:**
  * **1442. Count Triplets That Can Form Two Arrays of Equal XOR** *(Medium)*
  * **1738. Find Kth Largest XOR Coordinate Value** *(Medium)*

---

## NHÓM 6: MONOTONIC STACK & QUEUE

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "QUY LUẬT ĐÀO THẢI" (SURVIVAL OF THE FITTEST):**  
> Dân chuyên Tin giải thích Monotonic Stack bằng một quy luật thực tế:  
> *"Một phần tử mới xuất hiện (`nums[i]`), vừa **trẻ hơn** (chỉ số index lớn hơn) lại vừa **mạnh hơn/lớn hơn** các phần tử cũ đang nằm trong Stack. Vậy những phần tử cũ yếu thế hơn đó có bao giờ còn cơ hội làm đáp án (Next Greater) cho bất kỳ ai về sau nữa không? **Không bao giờ.**"*  
> $\implies$ Pop thẳng tay toàn bộ các phần tử yếu hơn ra khỏi Stack. Mỗi phần tử chỉ vào và ra khỏi stack tối đa 1 lần, đạt thời gian tuyến tính $O(N)$ tuyệt đối.

### Type 6.1: Monotonic Stack (Ngăn xếp đơn điệu)
* **Bản chất:** Tìm phần tử lớn hơn/nhỏ hơn đầu tiên bên cạnh trong $O(N)$. Duy trì stack có giá trị tăng dần hoặc giảm dần.
* **Danh sách của bạn:**
  * **496. Next Greater Element I** *(Easy)*
  * **503. Next Greater Element II** *(Medium - Mảng xoay vòng $\to$ duyệt 2 lần với `i % n`)*
  * **1475. Final Prices With a Special Discount in a Shop** *(Easy)*
  * **654. Maximum Binary Tree** *(Medium - Dựng cây bằng Monotonic Stack)*
* **⭐ Bài kinh điển bổ sung:**
  * **739. Daily Temperatures** *(Medium - Bắt buộc phải làm!)*
  * **84. Largest Rectangle in Histogram** *(Hard - Đỉnh cao của Monotonic Stack)*

### Type 6.2: State / Queue Simulation
* **Danh sách của bạn:**
  * **682. Baseball Game** *(Easy)*
  * **1472. Design Browser History** *(Medium)*
  * **1700. Number of Students Unable to Eat Lunch** *(Easy)*
  * **2073. Time Needed to Buy Tickets** *(Easy)*

---

## NHÓM 7: HEAP (PRIORITY QUEUE) & TOP-K

### Type 7.1: Top-K Pattern
* **Bản chất:** Tìm $K$ phần tử lớn nhất $\to$ Dùng **Min-Heap** dung lượng $K$.
* **Danh sách của bạn:**
  * **215. Kth Largest Element in an Array** *(Medium)*
  * **347. Top K Frequent Elements** *(Medium - Map đếm tần suất + Min-Heap/Bucket Sort)*
  * **973. K Closest Points to Origin** *(Medium)*
  * **1337. The K Weakest Rows in a Matrix** *(Easy)*
  * **1985. Find the Kth Largest Integer in the Array** *(Medium)*
  * **2099. Find Subsequence of Length K With the Largest Sum** *(Easy)*

### Type 7.2: Greedy Heap Simulation
* **Danh sách của bạn:**
  * **1046. Last Stone Weight** *(Easy - Max-Heap lấy 2 viên đá to nhất)*
  * **2558. Take Gifts From the Richest Pile** *(Easy)*

---

## NHÓM 8: HASHING, FREQUENCY & IN-PLACE TRICKS

### Type 8.1: Frequency Map & Pair Counting
* **Bản chất:** Duyệt 1 lần: `ans += count[x]; count[x]++`.
* **Danh sách của bạn:**
  * **217. Contains Duplicate** *(Easy)*
  * **594. Longest Harmonious Subsequence** *(Easy)*
  * **1512. Number of Good Pairs** *(Easy)*
  * **1995. Count Special Quadruplets** *(Easy)*
  * **2176. Count Equal and Divisible Pairs in an Array** *(Easy)*
  * **2190. Most Frequent Number Following Key In an Array** *(Easy)*
  * **2206. Divide Array Into Equal Pairs** *(Easy)*
  * **2657. Find the Prefix Common Array of Two Arrays** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **1. Two Sum** *(Easy - Hash Map tra cứu target - nums[i])*
  * **49. Group Anagrams** *(Medium - Hash chuỗi bằng sorted string hoặc frequency count)*

### Type 8.2: In-place Array as Hash Table (Index-as-Key) ⭐ *TRICK ĐẶC BIỆT*
* **Bản chất:** Khi giá trị $nums[i] \in [1, N]$, đổi dấu số tại index `abs(val) - 1` thành âm để đánh dấu sự xuất hiện mà không tốn thêm bộ nhớ.
* **Danh sách của bạn:**
  * **448. Find All Numbers Disappeared in an Array** *(Easy)*
  * **2965. Find Missing and Repeated Values** *(Easy)*
* **⭐ Bài kinh điển bổ sung:**
  * **41. First Missing Positive** *(Hard - Dùng chính mảng làm hash table)*

### Type 8.3: Boyer-Moore Voting Algorithm
* **Bản chất:** Triệt tiêu số phiếu của các ứng viên khác nhau.
* **Danh sách của bạn:**
  * **169. Majority Element** *(Easy - Chiếm $> N/2$)*
  * **229. Majority Element II** *(Medium - Chiếm $> N/3$)*

---

## NHÓM 9: DYNAMIC PROGRAMMING (QUY HOẠCH ĐỘNG) & KADANE

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "CÂY QUYẾT ĐỊNH TAKE OR SKIP" (CHỌN HOẶC BỎ):**  
> DP không phải là ma trận hay bảng 2 chiều. DP là một **CÂY QUYẾT ĐỊNH (Decision Tree)** bị trùng lặp các bài toán con.  
> Tại mỗi bước $i$, ta chỉ có 2 quyết định:  
> 1. **Take (Chọn):** Nhận giá trị `nums[i]`, gánh chịu ràng buộc (nhảy cóc vị trí hoặc trừ dung lượng).  
> 2. **Skip (Bỏ qua):** Giữ nguyên tài nguyên, chuyển sang xét phần tử $i + 1$.  
> 
> **Quy trình 3 bước chuyển từ Đệ quy thành DP:**  
> - *Bước 1:* Viết đệ quy `solve(i, capacity)` thử cả 2 nhánh Take và Skip (chính xác $100\%$ nhưng TLE $O(2^N)$).  
> - *Bước 2 (Top-down Memoization):* Thêm mảng nhớ `memo[i][capacity]` để cache nhánh đã tính $\to$ Triệt tiêu nhánh trùng ($O(N \times C)$).  
> - *Bước 3 (Bottom-up Tabulation):* Lật ngược đệ quy thành 2 vòng `for` chạy từ biên cơ sở lên.  
> 
> 💡 **VỚI KADANE (MẢNG CON LIÊN TIẾP MAX):**  
> Mô hình *"Bắt đầu lại cuộc đời hay Kế thừa di sản"*: `curMax = max(nums[i], curMax + nums[i])`.  
> Tại mỗi bước, tự hỏi: "Tự mình đứng riêng lẻ làm lại từ đầu (`nums[i]`) tốt hơn, hay nhận thêm của cải từ chuỗi liên tiếp cũ (`curMax + nums[i]`) tốt hơn?"

### Type 9.1: Kadane's Algorithm
* **Bản chất:** `currentMax = max(nums[i], currentMax + nums[i])`.
* **Danh sách của bạn:**
  * **53. Maximum Subarray** *(Medium)*
  * **121. Best Time to Buy and Sell Stock** *(Easy - Duy trì `minPrice`)*

### Type 9.2: 1D Linear DP
* **Danh sách của bạn:**
  * **746. Min Cost Climbing Stairs** *(Easy)*
  * **300. Longest Increasing Subsequence** *(Medium - $O(N^2)$ DP hoặc $O(N \log N)$ BS)*
  * **1043. Partition Array for Maximum Sum** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **70. Climbing Stairs** *(Easy - Fibonacci)*
  * **198. House Robber** *(Medium - Chọn hay không chọn)*
  * **139. Word Break** *(Medium)*

### Type 9.3: Knapsack / Coin Change
* **Danh sách của bạn:**
  * **322. Coin Change** *(Medium - Số đồng xu ít nhất)*
  * **518. Coin Change II** *(Medium - Số cách đổi)*

### Type 9.4: 2D Grid DP
* **Danh sách của bạn:**
  * **1277. Count Square Submatrices with All Ones** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **62. Unique Paths** *(Medium)*
  * **64. Minimum Path Sum** *(Medium)*

---

## NHÓM 10: TREE CONSTRUCTION & DIVIDE AND CONQUER
* **Bản chất:** Preorder xác định root đầu tiên, Postorder xác định root cuối cùng. Tra cứu vị trí root trong Inorder để chia thành nhánh cây trái và phải.
* **Danh sách của bạn:**
  * **105. Construct Binary Tree from Preorder and Inorder Traversal** *(Medium)*
  * **106. Construct Binary Tree from Inorder and Postorder Traversal** *(Medium)*
  * **108. Convert Sorted Array to Binary Search Tree** *(Easy)*
  * **654. Maximum Binary Tree** *(Medium)*
  * **889. Construct Binary Tree from Preorder and Postorder Traversal** *(Medium)*

---

## NHÓM 11: BACKTRACKING (QUAY LUI)

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "CÂY KHÔNG GIAN TRẠNG THÁI (STATE-SPACE TREE)":**  
> Mọi bài toán Backtracking (Tập con, Hoán vị, Tổ hợp) đều tuân theo **Thần chú 3 bước bất biến**:  
> 1. **Choose (Ra quyết định):** Đưa lựa chọn hiện tại vào đường đi (`path.add(candidate)`).  
> 2. **Explore (Thám hiểm):** Gọi đệ quy bước tiếp theo (`backtrack(nextIndex)`).  
> 3. **Unchoose (Hoàn tác / Backtrack):** Rút lại quyết định vừa rồi (`path.removeAt(path.lastIndex)`) để nhánh khác tiếp tục thử.

### Type 1.1: Permutations & Subsets
* **Bản chất:** Mẫu template 3 bước: `chọn -> backtrack(next) -> bỏ chọn`.
* **Danh sách của bạn:**
  * **78. Subsets** *(Medium)*
  * **46. Permutations** *(Medium)*
  * **47. Permutations II** *(Medium - Sort và bỏ qua nhánh trùng)*
  * **1980. Find Unique Binary String** *(Medium)*
* **⭐ Bài kinh điển bổ sung:**
  * **39. Combination Sum** *(Medium)*

---

## NHÓM 12: GREEDY & INTERVALS (THAM LAM & KHOẢNG)
* **Bản chất:** Với Interval, luôn bắt đầu bằng việc **Sort theo điểm đầu hoặc điểm cuối**.
* **Danh sách của bạn:**
  * **435. Non-overlapping Intervals** *(Medium - Sort theo end time)*
  * **561. Array Partition** *(Easy)*
  * **605. Can Place Flowers** *(Easy)*
  * **1827. Minimum Operations to Make the Array Increasing** *(Easy)*
  * **2037. Minimum Number of Moves to Seat Everyone** *(Easy)*
  * **2383. Minimum Hours of Training to Win a Competition** *(Easy)*
  * **2656. Maximum Sum With Exactly K Elements** *(Easy)*
  * **2900. Longest Unequal Adjacent Groups Subsequence I** *(Easy)*
* **⭐ Bài kinh điển bổ sung:**
  * **56. Merge Intervals** *(Medium - Bắt buộc phải biết!)*
  * **55. Jump Game** *(Medium)*

---

## NHÓM 13: 2D MATRIX TRAVERSAL & SIMULATION

> 💡 **BẢN CHẤT SƯ PHẠM CHUYÊN TIN — MÔ HÌNH "VẾT DẦU LOANG (DFS) VS MẶT SÓNG NỞ (BFS)":**  
> - **DFS (Vết dầu loang / Thăm dò chiều sâu):** Cắm đầu đi sâu hết một nhánh cho tới khi đụng tường thì lùi lại. Thích hợp cho: đếm số vùng đảo (Number of Islands), kiểm tra tính liên thông, tô màu vùng khép kín (Flood Fill).  
> - **BFS (Mặt sóng loang từng lớp - Level-order):** Nở rộng đều đặn theo từng bán kính $1, 2, 3...$ từ tâm. Thích hợp cho: **Đường đi ngắn nhất (Shortest Path)** trong lưới không có trọng số vì tầng nào gặp đích trước thì chắc chắn là đường ngắn nhất!

* **Danh sách của bạn:**
  * **Biến đổi ma trận:** **832. Flipping an Image**, **867. Transpose Matrix**, **1260. Shift 2D Grid**
  * **Kiểm tra ô/đường chéo:** **766. Toeplitz Matrix**, **1380. Lucky Numbers in a Matrix**, **1582. Special Positions in a Binary Matrix**, **2923. Find Champion I**
  * **Duyệt hàng/cột & Hình học:** **463. Island Perimeter**, **812. Largest Triangle Area**, **892. Surface Area of 3D Shapes**, **1030. Matrix Cells in Distance Order**, **1476. Subrectangle Queries**, **1572. Matrix Diagonal Sum**, **1672. Richest Customer Wealth**, **2125. Number of Laser Beams in a Bank**, **2373. Largest Local Values in a Matrix**, **2500. Delete Greatest Value in Each Row**, **2639. Find the Width of Columns of a Grid**, **2643. Row With Maximum Ones**, **3033. Modify the Matrix**
* **⭐ Bài kinh điển bổ sung:**
  * **200. Number of Islands** *(Medium - BFS/DFS duyệt ma trận)*
  * **54. Spiral Matrix** *(Medium - Đi vòng xoắn ốc)*

---

## NHÓM 14: AD-HOC ARRAY & MATH SIMULATION
* **Danh sách của bạn:**
  * **31. Next Permutation** *(Medium - Thuật toán kinh điển)*
  * **118. Pascal's Triangle** & **119. Pascal's Triangle II** *(Easy)*
  * **506. Relative Ranks**, **1431. Kids With Candies**, **1464. Max Product of Two Elements**, **1470. Shuffle the Array**, **1534. Count Good Triplets**, **1752. Check if Array Is Sorted and Rotated**, **1920. Build Array from Permutation**, **1929. Concatenation of Array**, **2011. Final Value of Variable**, **2032. Two Out of Three**, **2114. Max Words Found in Sentences**, **2549. Count Distinct Numbers on Board**, **2974. Minimum Number Game**

---

# PHẦN 3: LỘ TRÌNH 6 THÁNG CHINH PHỤC EASY/MEDIUM

> **Kỷ luật:** 1 - 1.5 tiếng/ngày. Tối thiểu 5 ngày/tuần. Không làm dàn trải, chỉ học theo từng cụm (Batching).

```
Tháng 1-2: Nền tảng Con trỏ & Tìm kiếm (Two Pointers, Sliding Window, Binary Search, Prefix Sum)
   ↓
Tháng 3: Cấu trúc dữ liệu & Bit (Monotonic Stack, Heap/PQ, Bit Manipulation, In-place Hash)
   ↓
Tháng 4: Cây, Đồ thị & Quay lui (Tree Construction, BFS/DFS Grid, Backtracking)
   ↓
Tháng 5: Tối ưu hóa (Greedy, Intervals, Dynamic Programming)
   ↓
Tháng 6: Thi đấu thử (Mock Interview, Luyện tốc độ, Tư duy giao tiếp thuật toán)
```

### Tháng 1: Làm chủ con trỏ và tìm kiếm (Two Pointers & Binary Search)
* **Tuần 1:** Type 1.1 (Fast-Slow) & Type 1.2 (Collision Pointers).
  * *Mục tiêu:* Tự code được 26, 27, 80, 167, 15 (3Sum). Không được nhìn giải.
* **Tuần 2:** Type 1.3, 1.4, 1.5 + Bổ sung: 11, 42.
* **Tuần 3:** Type 3.1 (Canonical BS & Bound).
* **Tuần 4:** Type 3.2 (Binary Search On Answer).
  * *Thử thách tuần 4:* Làm liền mạch: 875 $\to$ 1011 $\to$ 1482 $\to$ 410.

### Tháng 2: Cửa sổ trượt & Cộng dồn (Sliding Window & Prefix Sum)
* **Tuần 5:** Type 2.1 (Fixed Window) & Type 2.2 (Dynamic Window: 1004, 1493 + bổ sung: 3, 209).
* **Tuần 6:** Type 2.3 (Counting trick) & Type 4.1 (Running Sum, Pivot Index: 724, 1991).
* **Tuần 7:** Type 4.2 (Prefix/Suffix: 238 + bổ sung: 560 Subarray Sum Equals K).
* **Tuần 8:** Type 4.3 (Difference Array: 1893, 2848 + bổ sung: 253 Meeting Rooms II).

### Tháng 3: Bitmask, Monotonic Stack & Heap
* **Tuần 9:** Type 5.1 & 5.2 (Bit Manipulation: 136, 137, 2433, 2997).
* **Tuần 10:** Type 6.1 (Monotonic Stack: 496, 503 + bổ sung: 739 Daily Temperatures).
* **Tuần 11:** Type 7.1 & 7.2 (Min-Heap / Max-Heap: 215, 347, 973, 1046).
* **Tuần 12:** Type 8.2 (In-place Array Index Trick: 448, 2965 + bổ sung: 41) & Type 8.3 (Boyer-Moore).

### Tháng 4: Cây nhị phân, Quay lui & Ma trận
* **Tuần 13:** Type 10 (Dựng cây nhị phân: 105, 106, 108, 654, 889).
* **Tuần 14:** Type 11 (Backtracking: 78, 46, 47 + bổ sung: 39).
* **Tuần 15:** Type 13 (Matrix Traversal + bổ sung: 200 Number of Islands, 54 Spiral Matrix).
* **Tuần 16:** Review tổng hợp các bài đã làm từ Tháng 1 đến Tháng 4 (Spaced Repetition).

### Tháng 5: Tham lam, Khoảng & Quy hoạch động (Greedy, Intervals & DP)
* **Tuần 17:** Type 12 (Intervals: 435 + bổ sung: 56 Merge Intervals, 55 Jump Game).
* **Tuần 18:** Type 9.1 (Kadane: 53, 121) & Type 9.2 (1D Linear DP: 746, 300 + bổ sung: 70, 198).
* **Tuần 19:** Type 9.3 (Coin Change: 322, 518) & Type 9.4 (2D DP: 1277 + bổ sung: 62).
* **Tuần 20:** Làm toàn bộ các bài Ad-hoc/Math còn lại trong list (31 Next Permutation, Pascal's Triangle...).

### Tháng 6: Mock Interview & Luyện tốc độ phỏng vấn
* **Luyện giải đề có bấm giờ:** Easy: 10-15 phút, Medium: 20-25 phút.
* **Quy tắc phỏng vấn Think Aloud:** Vừa giải thích ý tưởng bằng lời (hoặc tiếng Anh) vừa gõ mã giả trước khi code Kotlin.

---

# PHẦN 4: KOTLIN CHEAT SHEET TỐI ƯU CHO PHỎNG VẤN

Để tránh dính lỗi biên dịch hoặc tốn bộ nhớ vô ích trong buổi phỏng vấn bằng Kotlin:

### 1. Khởi tạo mảng nguyên thủy (Tránh Auto-boxing của `Array<Int>`):
```kotlin
val arr = IntArray(size) { 0 }         // Tương đương int[] trong Java, cực nhanh
val matrix = Array(m) { IntArray(n) }   // Ma trận 2D
```

### 2. Thao tác Bit trong Kotlin:
```kotlin
val a = 5
val b = 3
val andVal = a and b      // a & b
val orVal  = a or b       // a | b
val xorVal = a xor b      // a ^ b
val notVal = a.inv()      // ~a
val shlVal = a shl 1      // a << 1
val shrVal = a shr 1      // a >> 1
val count1 = Integer.bitCount(a) // Đếm số bit 1
```

### 3. Khởi tạo PriorityQueue (Heap):
```kotlin
// Min-Heap (Mặc định phần tử nhỏ nhất ở đỉnh)
val minHeap = PriorityQueue<Int>()

// Max-Heap (Phần tử lớn nhất ở đỉnh)
val maxHeap = PriorityQueue<Int>(compareByDescending { it })

// Custom Object Heap (Ví dụ: sắp xếp theo tần suất giảm dần)
val freqHeap = PriorityQueue<Pair<Int, Int>> { a, b -> b.second - a.second }
```

### 4. Hoán đổi 2 phần tử trong mảng tại chỗ:
```kotlin
fun IntArray.swap(i: Int, j: Int) {
    val temp = this[i]
    this[i] = this[j]
    this[j] = temp
}
```

### 5. Binary Search chuẩn template không lo off-by-one:
```kotlin
var left = 0
var right = nums.size - 1
while (left <= right) {
    val mid = left + (right - left) / 2
    if (nums[mid] == target) return mid
    else if (nums[mid] < target) left = mid + 1
    else right = mid - 1
}
return -1 // hoặc return left nếu tìm insertion point (lower bound)
```

---

# PHẦN 5: BỘ 6 TRICK KINH ĐIỂN CỦA TIÊU CHUẨN PHỎNG VẤN (MUST-KNOW INTERVIEW TRICKS)

> **Tư duy phỏng vấn:** Khi gặp bài có trick, **không bao giờ phang trick ngay trong 30 giây đầu**. Hãy đi theo kịch bản:
> 1. Trình bày cách $O(N)$ Space (dùng `HashMap` hoặc `HashSet`).
> 2. Đề xuất cách Sort $O(N \log N)$ Time, $O(1)$ Space (Trade-off).
> 3. Trình bày Trick tối ưu đạt cả $O(N)$ Time và $O(1)$ Space.

---

### Trick 1: Boyer-Moore Voting Algorithm (Bầu cử đa số)
* **Bài áp dụng:** **169. Majority Element**, **229. Majority Element II**.
* **Dấu hiệu:** Tìm phần tử chiếm $> N/2$ (hoặc $> N/3$) lần trong mảng với $O(1)$ extra space.
* **Cơ chế:** Coi mỗi số như 1 ứng viên. Hai ứng viên khác nhau sẽ "bắn tỉa" triệt tiêu 1 phiếu của nhau. Do phần tử đa số chiếm $> 50\%$ số phiếu, sau khi triệt tiêu hết các phần tử đối nghịch, ứng viên còn lại chắc chắn là phần tử đa số.
* **Kotlin Template:**
```kotlin
fun majorityElement(nums: IntArray): Int {
    var candidate = nums[0]
    var count = 0
    for (num in nums) {
        if (count == 0) {
            candidate = num
        }
        count += if (num == candidate) 1 else -1
    }
    return candidate
}
```

---

### Trick 2: In-place Negation / Index-as-Hash Table (Đổi dấu mảng tại chỗ)
* **Bài áp dụng:** **448. Find All Numbers Disappeared in an Array**, **2965. Find Missing and Repeated Values**, **41. First Missing Positive**.
* **Dấu hiệu:** Mảng có kích thước $N$ và tất cả giá trị đều nằm trong đoạn $[1, N]$. Đề yêu cầu $O(N)$ time và $O(1)$ space.
* **Cơ chế:** Thay vì dùng `HashSet` tốn $O(N)$ memory, ta dùng chính các vị trí trong mảng làm bảng băm. Khi gặp giá trị `val = Math.abs(nums[i])`, ta đi đến chỉ số `index = val - 1` và đổi dấu số tại đó thành âm: `nums[index] = -Math.abs(nums[index])`. Số nào ở cuối cùng vẫn mang dấu dương $\implies$ index của nó chưa từng xuất hiện!
* **Kotlin Template (Bài 448):**
```kotlin
fun findDisappearedNumbers(nums: IntArray): List<Int> {
    for (i in nums.indices) {
        val targetIdx = Math.abs(nums[i]) - 1
        if (nums[targetIdx] > 0) {
            nums[targetIdx] = -nums[targetIdx] // Đánh dấu đã thăm
        }
    }
    val result = mutableListOf<Int>()
    for (i in nums.indices) {
        if (nums[i] > 0) {
            result.add(i + 1) // Chưa bị đổi dấu -> chưa từng xuất hiện
        }
    }
    return result
}
```

---

### Trick 3: Bitwise XOR Cancellation (Khử trùng lặp tầng Bit)
* **Bài áp dụng:** **136. Single Number**, **268. Missing Number**, **1720. Decode XORed Array**, **2433. Find The Original Array of Prefix Xor**, **2997. Minimum Number of Operations to Make Array XOR Equal to K**.
* **Dấu hiệu:** Mọi phần tử đều xuất hiện 2 lần ngoại trừ 1 phần tử duy nhất; hoặc tìm số bị thiếu trong đoạn $[0, N]$.
* **Cơ chế:** Phép XOR có 2 tính chất vàng: $x \oplus x = 0$ (tự triệt tiêu) và $x \oplus 0 = x$ (giữ nguyên). Phép XOR có tính giao hoán và kết hợp nên thứ tự các số không quan trọng.
* **Kotlin Template (Bài 136):**
```kotlin
fun singleNumber(nums: IntArray): Int {
    var xorSum = 0
    for (num in nums) {
        xorSum = xorSum xor num
    }
    return xorSum
}
```

---

### Trick 4: Floyd's Tortoise and Hare (Rùa và Thỏ - Phát hiện chu kỳ)
* **Bài áp dụng:** **287. Find the Duplicate Number**, **141. Linked List Cycle**, **142. Linked List Cycle II**.
* **Dấu hiệu:** Tìm số bị trùng trong mảng chỉ đọc không được sửa, $O(1)$ space; hoặc phát hiện vòng lặp linked list.
* **Cơ chế:** Coi mảng như một Linked List với liên kết `i -> nums[i]`. Nếu có số trùng nhau, sẽ có 2 index trỏ về cùng một giá trị $\implies$ tạo thành chu trình (Cycle).
  * **Pha 1:** `slow` nhảy 1 bước (`nums[slow]`), `fast` nhảy 2 bước (`nums[nums[fast]]`). Khi gặp nhau chứng tỏ có chu kỳ.
  * **Pha 2:** Đưa `slow` về điểm xuất phát `nums[0]`. Cả 2 cùng nhảy tốc độ 1 bước. Điểm gặp nhau lần 2 chính là lối vào chu trình (chính là số bị trùng).
* **Kotlin Template (Bài 287):**
```kotlin
fun findDuplicate(nums: IntArray): Int {
    var slow = nums[0]
    var fast = nums[0]
    // Pha 1: Tìm điểm chạm nhau trong vòng lặp
    do {
        slow = nums[slow]
        fast = nums[nums[fast]]
    } while (slow != fast)

    // Pha 2: Tìm cổng vào chu kỳ
    slow = nums[0]
    while (slow != fast) {
        slow = nums[slow]
        fast = nums[fast]
    }
    return slow
}
```

---

### Trick 5: Dutch National Flag - 3-Way Partitioning (Cờ Hà Lan 3 vùng)
* **Bài áp dụng:** **75. Sort Colors**.
* **Dấu hiệu:** Sắp xếp mảng chỉ chứa 3 loại giá trị (ví dụ: 0, 1, 2) trong 1 lần duyệt ($O(N)$ time) và $O(1)$ space.
* **Cơ chế:** Dùng 3 con trỏ:
  * `low`: Biên giới của vùng số 0 (các số trước `low` đều là 0).
  * `mid`: Con trỏ duyệt hiện tại.
  * `high`: Biên giới của vùng số 2 (các số sau `high` đều là 2).
  * Duyệt `mid`: nếu gặp 0 thì đổi chỗ với `nums[low++]`, gặp 2 thì đổi chỗ với `nums[high--]`, gặp 1 thì `mid++`.
* **Kotlin Template (Bài 75):**
```kotlin
fun sortColors(nums: IntArray) {
    var low = 0
    var mid = 0
    var high = nums.size - 1
    while (mid <= high) {
        when (nums[mid]) {
            0 -> {
                nums.swap(low, mid)
                low++
                mid++
            }
            1 -> mid++
            2 -> {
                nums.swap(mid, high)
                high--
                // Lưu ý: Không tăng mid ở đây vì phần tử vừa hoán đổi từ high về chưa được kiểm tra!
            }
        }
    }
}
```

---

### Trick 6: Next Permutation (Dò sườn dốc và đảo ngược)
* **Bài áp dụng:** **31. Next Permutation**, **556. Next Greater Element III**.
* **Dấu hiệu:** Tìm cấu hình hoán vị lớn hơn liền kề tiếp theo theo thứ tự từ điển (Lexicographical order).
* **Cơ chế:** 3 bước bất biến:
  1. Duyệt từ phải qua trái tìm vị trí đầu tiên bị giảm giá trị: $nums[i] < nums[i+1]$ (điểm gãy sườn dốc).
  2. Nếu tìm thấy $i$, tiếp tục duyệt từ phải qua trái tìm số nhỏ nhất nhưng vẫn lớn hơn $nums[i]$ (gọi là $nums[j]$). Hoán đổi $nums[i]$ và $nums[j]$.
  3. Đảo ngược toàn bộ đoạn mảng từ $i + 1$ đến cuối để đoạn đuôi trở thành nhỏ nhất có thể (tăng dần).
* **Kotlin Template (Bài 31):**
```kotlin
fun nextPermutation(nums: IntArray) {
    var i = nums.size - 2
    while (i >= 0 && nums[i] >= nums[i + 1]) {
        i--
    }
    if (i >= 0) {
        var j = nums.size - 1
        while (nums[j] <= nums[i]) {
            j--
        }
        nums.swap(i, j)
    }
    reverse(nums, i + 1, nums.size - 1)
}

private fun reverse(nums: IntArray, start: Int, end: Int) {
    var l = start
    var r = end
    while (l < r) {
        nums.swap(l++, r--)
    }
}
```

---

# PHẦN 6: DANH SÁCH BÀI ĐỐ MẸO / TOÁN DỊ NÊN BỎ QUA (LOW ROI)

> **Khuyến cáo:** Những bài này thuộc dạng "biết mẹo thì xong trong 10 giây, không biết thì nghĩ không ra", không đo lường năng lực lập trình và các công ty công nghệ lớn **hầu như không bao giờ hỏi**. Hãy đánh dấu bỏ qua hoặc chỉ đọc lướt để không lãng phí 6 tháng vàng ngọc:

1. **2549. Count Distinct Numbers on Board:**
   * *Bản chất mẹo:* Nếu $n = 1$ trả về 1, còn lại luôn luôn trả về $n - 1$. Hoàn toàn là một câu đố vui ngữ nghĩa, không có giá trị thuật toán.
2. **812. Largest Triangle Area:**
   * *Bản chất mẹo:* Bắt nhớ công thức hình học giải tích Shoelace (diện tích tam giác qua 3 đỉnh tọa độ Gauss). Phỏng vấn dev phần mềm không ai bắt học thuộc công thức này.
3. **892. Surface Area of 3D Shapes:**
   * *Bản chất mẹo:* Đếm diện tích khối hộp 3D và trừ đi các mặt tiếp xúc chồng chéo. Nặng về tính toán hình học vụn vặt, không rèn luyện tư duy cấu trúc dữ liệu.
4. **1980. Find Unique Binary String:**
   * *Bản chất mẹo:* Có một mẹo toán là Cantor's Diagonal Argument (lấy bit thứ $i$ của chuỗi thứ $i$ rồi đảo ngược). Nếu muốn giải thực chất, hãy dùng **Backtracking / Trie**; không cần học vẹt mẹo Cantor.

