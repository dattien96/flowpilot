package com.example.app.foundation

// Vế 2 của DIP: Contract Shape - Không leak impl details
fun interface StaticKeyProvider {
    fun getKey(): String
}

// Provider interface tránh hardcode tên thư viện
interface NativeLibraryProvider {
    fun getLibraryName(): String
}
