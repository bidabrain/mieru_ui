package io.github.bidabrain.mieru.vpn

import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.core.content.ContextCompat
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

enum class VpnStatus { Stopped, Starting, Running, Stopping }

object VpnState {
    private val _status = MutableStateFlow(VpnStatus.Stopped)
    val status: StateFlow<VpnStatus> = _status.asStateFlow()

    /** Last start error, cleared on the next attempt. */
    val error = MutableStateFlow<String?>(null)

    @Volatile var startedAt: Long = 0L
    @Volatile var profileName: String = ""

    internal fun set(s: VpnStatus) {
        _status.value = s
    }

    val isActive: Boolean get() = _status.value == VpnStatus.Running || _status.value == VpnStatus.Starting
}

object VpnController {
    /** Returns the system consent intent if the user still has to approve the VPN. */
    fun prepare(ctx: Context): Intent? = VpnService.prepare(ctx)

    fun start(ctx: Context) = send(ctx, MieruVpnService.ACTION_START)
    fun stop(ctx: Context) = send(ctx, MieruVpnService.ACTION_STOP)

    /** Apply changed settings: restart only when connected. */
    fun reloadIfRunning(ctx: Context) {
        if (VpnState.status.value == VpnStatus.Running) send(ctx, MieruVpnService.ACTION_RESTART)
    }

    private fun send(ctx: Context, action: String) {
        val i = Intent(ctx, MieruVpnService::class.java).setAction(action)
        if (action == MieruVpnService.ACTION_STOP) ctx.startService(i)
        else ContextCompat.startForegroundService(ctx, i)
    }
}
