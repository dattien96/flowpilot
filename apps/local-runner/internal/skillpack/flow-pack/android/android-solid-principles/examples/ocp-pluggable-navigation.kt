package com.example.app.navigation

// open interface cho phép các module tự định nghĩa
interface AppRoute {
    val route: String
}

// Module tính năng tự định nghĩa
data object HomeRoute : AppRoute {
    override val route = "home"
}

// Pluggable NavGraph với Hilt Multibindings
// (Thêm code cấu hình Dagger/Hilt ở đây)
