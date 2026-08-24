package world.zcn.pricereminder

import android.app.Application
import world.zcn.pricereminder.data.AppContainer

class PriceReminderApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}
