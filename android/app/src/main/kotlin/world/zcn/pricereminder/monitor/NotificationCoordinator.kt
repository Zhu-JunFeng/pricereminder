package world.zcn.pricereminder.monitor

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import androidx.core.app.NotificationCompat
import world.zcn.pricereminder.R
import world.zcn.pricereminder.core.AlertTrigger
import java.math.RoundingMode

class NotificationCoordinator(private val context: Context) {
    private val manager = context.getSystemService(NotificationManager::class.java)

    init {
        manager.createNotificationChannel(NotificationChannel(MONITOR_CHANNEL, "实时价格", NotificationManager.IMPORTANCE_LOW))
        manager.createNotificationChannel(NotificationChannel(ALERT_CHANNEL, "价格预警", NotificationManager.IMPORTANCE_DEFAULT))
        manager.createNotificationChannel(NotificationChannel(HEALTH_CHANNEL, "监控状态", NotificationManager.IMPORTANCE_DEFAULT))
    }

    fun monitor(symbol: String, price: String, status: String) = NotificationCompat.Builder(context, MONITOR_CHANNEL)
        .setSmallIcon(android.R.drawable.ic_popup_sync)
        .setContentTitle("$symbol  $price")
        .setContentText(status)
        .setOngoing(true)
        .setOnlyAlertOnce(true)
        .setCategory(NotificationCompat.CATEGORY_SERVICE)
        .build()

    fun alerts(symbol: String, triggers: List<AlertTrigger>) {
        if (triggers.isEmpty()) return
        val summary = triggers.joinToString("；") {
            val direction = if (it.direction.name == "RISE") "上涨" else "下跌"
            val change = it.changePercent.setScale(2, RoundingMode.HALF_UP).toPlainString()
            "${it.windowMinutes}分钟$direction $change%（阈值 ${it.thresholdText}%）"
        }
        manager.notify(("alert:$symbol:${triggers.first().eventTime}").hashCode(), NotificationCompat.Builder(context, ALERT_CHANNEL)
            .setSmallIcon(android.R.drawable.stat_notify_more)
            .setContentTitle("$symbol 价格预警")
            .setContentText(summary)
            .setStyle(NotificationCompat.BigTextStyle().bigText("$summary\n最新价 ${triggers.first().priceText}"))
            .setAutoCancel(true)
            .build())
    }

    fun health(title: String, detail: String) {
        manager.notify(HEALTH_ID, NotificationCompat.Builder(context, HEALTH_CHANNEL)
            .setSmallIcon(android.R.drawable.stat_notify_error)
            .setContentTitle(title)
            .setContentText(detail)
            .setAutoCancel(true)
            .build())
    }

    companion object {
        const val MONITOR_CHANNEL = "price-monitor"
        const val MONITOR_ID = 1001
        private const val ALERT_CHANNEL = "price-alert"
        private const val HEALTH_CHANNEL = "monitor-health"
        private const val HEALTH_ID = 1002
    }
}
