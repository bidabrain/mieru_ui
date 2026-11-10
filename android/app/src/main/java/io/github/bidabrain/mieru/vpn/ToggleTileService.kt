package io.github.bidabrain.mieru.vpn

import android.app.PendingIntent
import android.content.Intent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import io.github.bidabrain.mieru.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/** Quick-settings tile to connect / disconnect without opening the app. */
class ToggleTileService : TileService() {
    private var job: Job? = null

    override fun onStartListening() {
        job = CoroutineScope(Dispatchers.Main).launch {
            VpnState.status.collect { s ->
                qsTile?.apply {
                    state = when (s) {
                        VpnStatus.Running, VpnStatus.Starting -> Tile.STATE_ACTIVE
                        else -> Tile.STATE_INACTIVE
                    }
                    if (Build.VERSION.SDK_INT >= 29) subtitle = if (s == VpnStatus.Running) VpnState.profileName else null
                    updateTile()
                }
            }
        }
    }

    override fun onStopListening() {
        job?.cancel()
    }

    override fun onClick() {
        if (VpnState.isActive) {
            VpnController.stop(this)
            return
        }
        if (VpnController.prepare(this) == null) {
            VpnController.start(this)
        } else {
            // Consent dialog needs an activity.
            val i = Intent(this, MainActivity::class.java)
                .setAction(MainActivity.ACTION_CONNECT)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            if (Build.VERSION.SDK_INT >= 34) {
                startActivityAndCollapse(PendingIntent.getActivity(this, 0, i, PendingIntent.FLAG_IMMUTABLE))
            } else {
                @Suppress("DEPRECATION")
                startActivityAndCollapse(i)
            }
        }
    }
}
