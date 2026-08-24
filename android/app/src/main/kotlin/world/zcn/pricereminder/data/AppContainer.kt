package world.zcn.pricereminder.data

import android.content.Context
import world.zcn.pricereminder.BuildConfig

class AppContainer(context: Context) {
    val localStore = LocalStore(context)
    val apiClient = ApiClient(localStore, BuildConfig.PRICE_REMINDER_SERVER_URL)
}
