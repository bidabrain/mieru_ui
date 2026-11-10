package io.github.bidabrain.mieru

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.data.Store

class App : Application() {
    override fun onCreate() {
        super.onCreate()
        instance = this
        Store.init(this)
        Core.init(this)
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel(CHANNEL_VPN, "连接状态", NotificationManager.IMPORTANCE_LOW).apply {
                setShowBadge(false)
            },
        )
    }

    companion object {
        const val CHANNEL_VPN = "vpn"
        lateinit var instance: App
            private set
    }
}
