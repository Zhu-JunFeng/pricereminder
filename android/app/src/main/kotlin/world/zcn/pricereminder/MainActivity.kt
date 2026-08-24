package world.zcn.pricereminder

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.runtime.LaunchedEffect
import androidx.core.content.ContextCompat
import world.zcn.pricereminder.monitor.PriceMonitorService
import world.zcn.pricereminder.ui.PriceReminderApp
import world.zcn.pricereminder.ui.PriceReminderTheme

class MainActivity : ComponentActivity() {
    private val viewModel: MainViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (savedInstanceState == null) startMonitor()
        setContent {
            PriceReminderTheme {
                val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                    if (!granted) viewModel.markNotificationPermissionUnavailable()
                }
                LaunchedEffect(Unit) {
                    if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(
                            this@MainActivity, Manifest.permission.POST_NOTIFICATIONS
                        ) != PackageManager.PERMISSION_GRANTED
                    ) {
                        permission.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                }
                PriceReminderApp(
                    viewModel = viewModel,
                    onStartMonitor = {
                        startMonitor()
                        if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                            permission.launch(Manifest.permission.POST_NOTIFICATIONS)
                        }
                    },
                    onStopMonitor = {
                        stopService(Intent(this, PriceMonitorService::class.java))
                    },
                )
            }
        }
    }

    private fun startMonitor() {
        ContextCompat.startForegroundService(this, Intent(this, PriceMonitorService::class.java))
    }
}
