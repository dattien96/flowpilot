/*
 * Ví dụ về thiết lập Room Database đa module.
 * - :core:database cung cấp Factory
 * - :feature:order:data khai báo database của riêng nó (dạng cache cục bộ).
 */

// =======================================================
// MODULE: :core:database
// PATH: core/database/src/main/java/com/app/core/database/AppDatabaseFactory.kt
// =======================================================
package com.app.core.database

import android.content.Context
import androidx.room.Room
import androidx.room.RoomDatabase

/**
 * Factory tập trung đặt ở core để cấu hình chung cho mọi database 
 * (ví dụ: gỡ lỗi, encryption, cấu hình sqlite).
 */
object AppDatabaseFactory {

    /**
     * Tạo một database với multi-account support.
     * @param context Application context
     * @param userId ID của user đang đăng nhập, dùng để tạo tên file riêng biệt.
     * @param dbNamePrefix Tiền tố tên database (vd: "orders", "cache")
     * @param isCache Nếu true, DB sẽ bị xóa và tạo lại khi có thay đổi schema.
     */
    inline fun <reified T : RoomDatabase> create(
        context: Context,
        userId: String?,
        dbNamePrefix: String,
        isCache: Boolean = false
    ): T {
        val fileName = if (userId != null) {
            "${dbNamePrefix}_${userId}.db"
        } else {
            "${dbNamePrefix}_global.db"
        }

        val builder = Room.databaseBuilder(context, T::class.java, fileName)

        if (isCache) {
            builder.fallbackToDestructiveMigration()
        } else {
            // TODO: Yêu cầu dev phải define migrations explicitly nếu không phải cache
        }

        return builder.build()
    }
}


// =======================================================
// MODULE: :feature:order:data
// PATH: feature/order/data/src/main/java/com/app/feature/order/data/local/OrderDatabase.kt
// =======================================================
package com.app.feature.order.data.local

import androidx.room.Database
import androidx.room.RoomDatabase
import com.app.feature.order.data.local.entity.OrderEntity

/**
 * Database cục bộ của tính năng Order.
 * Nó là internal, không module nào khác được truy cập trực tiếp.
 */
@Database(
    entities = [OrderEntity::class],
    version = 1,
    exportSchema = true
)
internal abstract class OrderDatabase : RoomDatabase() {
    abstract fun orderDao(): OrderDao
}


// =======================================================
// MODULE: :feature:order:data (DI Module)
// PATH: feature/order/data/src/main/java/com/app/feature/order/data/di/OrderDataModule.kt
// =======================================================
package com.app.feature.order.data.di

import android.content.Context
import com.app.core.database.AppDatabaseFactory
import com.app.core.session.SessionManager
import com.app.feature.order.data.local.OrderDao
import com.app.feature.order.data.local.OrderDatabase
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
internal object OrderDataModule {

    @Provides
    @Singleton
    fun provideOrderDatabase(
        @ApplicationContext context: Context,
        sessionManager: SessionManager // giả sử lấy từ core:session
    ): OrderDatabase {
        val currentUserId = sessionManager.getCurrentUserId()
        
        // Gọi Factory từ core để tạo DB, áp dụng policy cache
        return AppDatabaseFactory.create(
            context = context,
            userId = currentUserId,
            dbNamePrefix = "feature_order",
            isCache = true // Orders cache có thể fetch lại từ server
        )
    }

    @Provides
    fun provideOrderDao(database: OrderDatabase): OrderDao {
        return database.orderDao()
    }
}
