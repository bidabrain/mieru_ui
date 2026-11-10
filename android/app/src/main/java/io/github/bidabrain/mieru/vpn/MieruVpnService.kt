package io.github.bidabrain.mieru.vpn

import android.Manifest
import android.app.Notification
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.IpPrefix
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import java.net.InetAddress
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import io.github.bidabrain.mieru.App
import io.github.bidabrain.mieru.R
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.core.userMessage
import io.github.bidabrain.mieru.data.AppData
import io.github.bidabrain.mieru.data.PerAppMode
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.data.formatSpeed
import io.github.bidabrain.mieru.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

class MieruVpnService : VpnService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val lock = Mutex()
    private var tun: ParcelFileDescriptor? = null
    private var notifyJob: Job? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> scope.launch { lock.withLock { teardown() }; stopSelf() }
            ACTION_RESTART -> {
                goForeground("正在重新连接…")
                scope.launch { lock.withLock { teardownCore(); bringUp() } }
            }
            else -> {
                // VpnService may also be started by the system (always-on VPN): action is null.
                goForeground("正在连接…")
                scope.launch { lock.withLock { if (!Core.isRunning) bringUp() } }
            }
        }
        return START_NOT_STICKY
    }

    private fun bringUp() {
        VpnState.error.value = null
        VpnState.set(VpnStatus.Starting)
        try {
            val d = Store.data.value
            val profile = d.selected ?: error("请先添加一个节点")
            val pfd = buildTun(d, profile.name) ?: error("VPN 权限被拒绝或已被其它 VPN 占用")
            tun = pfd
            Core.start(d, profile, pfd.fd)
            VpnState.profileName = profile.name
            VpnState.startedAt = System.currentTimeMillis()
            VpnState.set(VpnStatus.Running)
            startNotifyLoop()
        } catch (t: Throwable) {
            Log.e(TAG, "start failed", t)
            VpnState.error.value = t.userMessage()
            teardown()
            stopSelf()
        }
    }

    private fun buildTun(d: AppData, session: String): ParcelFileDescriptor? {
        val s = d.settings
        val b = Builder()
            .setSession(session)
            .setMtu(9000)
            .addAddress("198.18.0.1", 30) // must match libcore.TunAddress4
            .addRoute("0.0.0.0", 0)
            .addDnsServer("198.18.0.2")
            .setConfigureIntent(
                PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE),
            )
        if (s.ipv6) {
            b.addAddress("fdfe:dcba:9876::1", 126)
            b.addRoute("::", 0)
        }
        // "Bypass LAN": keep private ranges out of the tun entirely, so LAN traffic never
        // depends on this app (Android 17 also gates LAN access behind a runtime permission).
        if (d.routing.bypassLan && Build.VERSION.SDK_INT >= 33) {
            val lan = listOf("10.0.0.0" to 8, "172.16.0.0" to 12, "192.168.0.0" to 16, "169.254.0.0" to 16, "224.0.0.0" to 4) +
                if (s.ipv6) listOf("fc00::" to 7, "fe80::" to 10) else emptyList()
            lan.forEach { (addr, len) -> runCatching { b.excludeRoute(IpPrefix(InetAddress.getByName(addr), len)) } }
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) b.setMetered(false)

        // The core's own sockets must never loop back into the tun: keep this app outside it.
        val self = packageName
        val list = s.perAppList.filter { it != self }
        when {
            s.perAppMode == PerAppMode.PROXY && list.isNotEmpty() ->
                list.forEach { runCatching { b.addAllowedApplication(it) } }
            s.perAppMode == PerAppMode.BYPASS -> {
                b.addDisallowedApplication(self)
                list.forEach { runCatching { b.addDisallowedApplication(it) } }
            }
            else -> b.addDisallowedApplication(self)
        }
        return b.establish()
    }

    private fun teardownCore() {
        notifyJob?.cancel()
        runCatching { Core.stop() }
        runCatching { tun?.close() }
        tun = null
    }

    private fun teardown() {
        if (VpnState.status.value != VpnStatus.Stopped) VpnState.set(VpnStatus.Stopping)
        teardownCore()
        VpnState.set(VpnStatus.Stopped)
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
    }

    private fun goForeground(text: String) {
        ServiceCompat.startForeground(
            this, NOTIFY_ID, notification(text, null),
            if (Build.VERSION.SDK_INT >= 34) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0,
        )
    }

    private fun startNotifyLoop() {
        notifyJob?.cancel()
        notifyJob = scope.launch {
            val nm = NotificationManagerCompat.from(this@MieruVpnService)
            while (isActive) {
                val t = Core.traffic()
                val canPost = Build.VERSION.SDK_INT < 33 ||
                    ContextCompat.checkSelfPermission(this@MieruVpnService, Manifest.permission.POST_NOTIFICATIONS) ==
                    PackageManager.PERMISSION_GRANTED
                if (canPost) {
                    runCatching {
                        nm.notify(NOTIFY_ID, notification("已连接 · ${VpnState.profileName}", "↑ ${formatSpeed(t.up)}   ↓ ${formatSpeed(t.down)}"))
                    }
                }
                delay(2000)
            }
        }
    }

    private fun notification(title: String, text: String?): Notification {
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE,
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, MieruVpnService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, App.CHANNEL_VPN)
            .setSmallIcon(R.drawable.ic_stat)
            .setContentTitle(title)
            .setContentText(text)
            .setContentIntent(open)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .addAction(0, "断开", stop)
            .build()
    }

    override fun onRevoke() {
        scope.launch { lock.withLock { teardown() }; stopSelf() }
    }

    override fun onDestroy() {
        runCatching { teardownCore() }
        if (VpnState.status.value != VpnStatus.Stopped) VpnState.set(VpnStatus.Stopped)
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        private const val TAG = "MieruVpn"
        private const val NOTIFY_ID = 1
        const val ACTION_START = "io.github.bidabrain.mieru.START"
        const val ACTION_STOP = "io.github.bidabrain.mieru.STOP"
        const val ACTION_RESTART = "io.github.bidabrain.mieru.RESTART"
    }
}
