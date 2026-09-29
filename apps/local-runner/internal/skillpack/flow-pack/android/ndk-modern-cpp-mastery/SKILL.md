---
version: 6
name: ndk-modern-cpp-mastery
description: >-
  Enforces enterprise C++20 and Android NDK guidelines distilled from Lessons 1-16.
  Use whenever generating, refactoring, or reviewing C++, JNI, CMake, or native memory code
  in TiToCompose and PrivaVault (:core:security-rasp, :core:crypto-ndk, :core:shredder-ndk, :core:vault-core).
---

# NDK & Modern C++20 Mastery Skill (Enterprise Standards)

This skill encodes the hard engineering principles from **Lessons 1 to 16** of the Native NDK curriculum. All AI agents generating native code for PrivaVault or TiToCompose **MUST strictly adhere** to these laws.

---

## 🏛️ LAW 1: RAII & RESOURCE WRAPPERS (Lessons 2, 3, 4, 7)
* **Never use raw pointers or raw file descriptors without an RAII wrapper.**
* Wrap file descriptors (`int fd`) using an RAII struct/class that automatically calls `close(fd)` in its destructor.
* Wrap `mmap()` buffers using an RAII guard that calls `munmap(ptr, size)` in its destructor.
* Wrap OS resources (`ANativeWindow`, `FILE*`, `EVP_CIPHER_CTX*`) with Custom Deleters in `std::unique_ptr`.
* **Zero Cost Custom Deleter (Empty Base Optimization - EBO):** Use stateless struct functors for deleters:
  ```cpp
  struct FdCloser { void operator()(int* fd) const noexcept { if (fd && *fd >= 0) { ::close(*fd); delete fd; } } };
  ```

---

## ⚡ LAW 2: MOVE SEMANTICS & RULE OF 5 (Lessons 3, 8)
* Whenever a class manages an OS resource directly:
  1. **Delete Copy Constructor and Copy Assignment Operator:**
     ```cpp
     MyResource(const MyResource&) = delete;
     MyResource& operator=(const MyResource&) = delete;
     ```
  2. **Implement Move Constructor & Move Assignment Operator with `noexcept`:**
     ```cpp
     MyResource(MyResource&& other) noexcept;
     MyResource& operator=(MyResource&& other) noexcept;
     ```
* Pass large chunks (e.g. 64KB video chunks, payloads) using `std::move` or `std::span<const uint8_t>`, never copy by value.

---

## 🚀 LAW 3: ZERO-COPY JNI & DIRECT BYTEBUFFER (Lesson 13)
* **Never use `jbyteArray` + `GetByteArrayElements` for high-throughput streaming (e.g. video, files > 12KB).**
* For streaming media, decryption, or file shredding, pass a Direct `ByteBuffer` from Kotlin:
  ```cpp
  void* rawBuffer = env->GetDirectBufferAddress(jbuffer);
  jlong capacity = env->GetDirectBufferCapacity(jbuffer);
  if (!rawBuffer) return; // Validate null
  ```
* For small coordinates/arrays (< 1KB), use `Get<Type>ArrayRegion` onto the native C++ stack memory (no heap allocation, no release call needed).

---

## 🔒 LAW 4: JNI CACHING & LOCAL REFERENCE INTEGRITY (Lessons 11, 12)
* **Never call `FindClass` or `GetMethodID` inside hot loops or per-frame calls.**
* Perform all reflections inside `JNI_OnLoad`:
  - `jclass` is a Local Reference on the Java Heap $\to$ **MUST call `env->NewGlobalRef(localClass)`**.
  - `jmethodID` and `jfieldID` are internal VM offsets $\to$ Cache directly in `static` variables without `NewGlobalRef`.
* **Local Reference Table Limit (< 512 entries per thread):**
  - When iterating over collections or repeatedly allocating native references, wrap the loop with `env->PushLocalFrame(16)` and `env->PopLocalFrame(nullptr)` or call `env->DeleteLocalRef(obj)`.
* **Thread Safety (`JNIEnv*`):**
  - Never share `JNIEnv*` across threads. Cache `JavaVM*` globally and call `jvm->AttachCurrentThread` / `DetachCurrentThread` on native background threads.

---

## 🛡️ LAW 5: CPU CACHE LINE ALIGNMENT & FALSE SHARING (Lesson 10)
* Any multi-threaded data structure (Lock-free Ring Buffer, concurrent queues, multi-threaded progress tracking) must align its atomic indices:
  ```cpp
  alignas(64) std::atomic<uint64_t> head_{0};
  alignas(64) std::atomic<uint64_t> tail_{0};
  ```
* Prevent False Sharing across CPU L1 cache lines (64 bytes).

---

## 🗄️ LAW 6: LINUX VIRTUAL MEMORY & MMAP (Lesson 9)
* For persistent Key-Value storage (MMKV architecture) and Virtual Binary Containers (`.pvault`):
  - Use `mmap(nullptr, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0)`.
  - Always call `fdatasync(fd)` or `msync()` when critical security data must hit physical flash storage.
  - Understand the difference between Virtual Address Space allocation (`mmap`) and Physical RAM usage (Pages are faulted in on-demand).

---

## 🧱 LAW 7: ENTERPRISE MODULAR CMAKE & CLEAN ARCHITECTURE (Lesson 14)
* Enforce 2 distinct layers in every native feature:
  1. **Core Static Library (`.a`):** 100% pure C++20, zero dependencies on `jni.h` or Android SDK. Can be tested independently via Google Test (GTest).
  2. **JNI Bridge Shared Library (`.so`):** Thin translation layer that unpacks JNI objects, calls the core `.a`, and packages results.
* Enforce modern target-based CMake:
  - `add_library(core_target STATIC ...)`
  - `target_include_directories(core_target PUBLIC include)`
  - `target_link_libraries(bridge_target PRIVATE core_target log android)`
  - `target_compile_features(core_target PUBLIC cxx_std_20)`

---

## 🧰 LAW 8: SANITIZERS & MEMORY SAFETY (Lesson 16)
* All native code must compile clean under AddressSanitizer (`-fsanitize=address`) and LeakSanitizer (`-fsanitize=leak`).
* Always zero out sensitive keys in RAM immediately after use:
  ```cpp
  #include <cstring>
  // Use memset_s or explicit memory barrier zeroization to prevent compiler optimization
  volatile uint8_t* p = key;
  while (len--) *p++ = 0;
  ```
* Guard sensitive Master Key memory regions using `mprotect(ptr, size, PROT_NONE)` when idle.
