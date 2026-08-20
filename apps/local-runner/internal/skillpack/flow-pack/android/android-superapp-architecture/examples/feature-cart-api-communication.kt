/*
 * Ví dụ thực tế về giao tiếp Cross-Feature sử dụng mô hình API / Impl.
 *
 * Yêu cầu: Feature ProductDetail (Chi tiết sản phẩm) cần thêm sản phẩm vào Giỏ hàng (Cart).
 * Khó khăn: ProductDetail không được phụ thuộc trực tiếp vào Cart để tránh tightly-coupled.
 * Giải pháp: Tạo module :feature:cart:api chứa interface CartFeatureApi.
 */

// =======================================================
// MODULE: :feature:cart:api
// PATH: feature/cart/api/src/main/java/com/app/feature/cart/api/CartFeatureApi.kt
// =======================================================
package com.app.feature.cart.api

/**
 * Giao diện public để các feature khác tương tác với Cart.
 * DTOs (Data Transfer Objects) cũng nên được định nghĩa trong module này.
 */
data class CartItemDto(
    val productId: String,
    val quantity: Int
)

interface CartFeatureApi {
    suspend fun addToCart(item: CartItemDto): Boolean
    suspend fun getCartItemCount(): Int
}


// =======================================================
// MODULE: :feature:cart:data (hoặc :feature:cart:impl)
// PATH: feature/cart/data/src/main/java/com/app/feature/cart/data/CartFeatureApiImpl.kt
// Phụ thuộc (Dependencies): project(":feature:cart:api")
// =======================================================
package com.app.feature.cart.data

import com.app.feature.cart.api.CartFeatureApi
import com.app.feature.cart.api.CartItemDto
import javax.inject.Inject

/**
 * Implementation thực tế. Chú ý class này là 'internal',
 * ngăn chặn các module khác khởi tạo trực tiếp.
 */
internal class CartFeatureApiImpl @Inject constructor(
    private val cartRepository: CartRepository // internal repository of cart feature
) : CartFeatureApi {

    override suspend fun addToCart(item: CartItemDto): Boolean {
        // Logic nghiệp vụ nội bộ của Cart
        return cartRepository.insertOrUpdate(item.productId, item.quantity)
    }

    override suspend fun getCartItemCount(): Int {
        return cartRepository.getTotalItems()
    }
}


// =======================================================
// MODULE: :di (Hoặc :app nếu không có :di riêng)
// PATH: di/src/main/java/com/app/di/CartModule.kt
// Phụ thuộc (Dependencies): project(":feature:cart:api"), project(":feature:cart:data")
// =======================================================
package com.app.di

import com.app.feature.cart.api.CartFeatureApi
import com.app.feature.cart.data.CartFeatureApiImpl
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

@Module
@InstallIn(SingletonComponent::class)
abstract class CartBindingModule {

    @Binds
    internal abstract fun bindCartFeatureApi(
        impl: CartFeatureApiImpl
    ): CartFeatureApi
}


// =======================================================
// MODULE: :feature:product-detail:presentation
// PATH: feature/product-detail/presentation/src/main/java/com/app/feature/productdetail/ProductDetailViewModel.kt
// Phụ thuộc (Dependencies): project(":feature:cart:api")
// =======================================================
package com.app.feature.productdetail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.app.feature.cart.api.CartFeatureApi
import com.app.feature.cart.api.CartItemDto
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class ProductDetailViewModel @Inject constructor(
    // Inject interface CartFeatureApi, hoàn toàn KHÔNG biết về CartFeatureApiImpl
    private val cartFeatureApi: CartFeatureApi
) : ViewModel() {

    fun onAddToCartClicked(productId: String) {
        viewModelScope.launch {
            val success = cartFeatureApi.addToCart(CartItemDto(productId, 1))
            if (success) {
                // Update UI: Show success toast
            }
        }
    }
}
