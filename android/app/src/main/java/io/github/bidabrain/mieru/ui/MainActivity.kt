package io.github.bidabrain.mieru.ui

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.lifecycle.lifecycleScope
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.core.userMessage
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.ui.theme.MieruTheme
import io.github.bidabrain.mieru.update.Updater
import io.github.bidabrain.mieru.vpn.VpnController
import io.github.bidabrain.mieru.vpn.VpnState
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    /** Text of a mierus:// link opened from outside, shown as an import prompt. */
    val pendingImport = mutableStateOf<String?>(null)

    private val vpnConsent = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        if (VpnController.prepare(this) == null) VpnController.start(this)
        else VpnState.error.value = "需要授予 VPN 权限才能连接"
    }

    private val permissions = registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        // Denial is fine: notifications are cosmetic, and without local-network access only LAN targets fail.
        requestVpn()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.auto(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.auto(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT),
        )
        super.onCreate(savedInstanceState)
        handleIntent(intent)
        setContent {
            val data by Store.data.collectAsState()
            MieruTheme(themeMode = data.settings.theme, dynamicColor = data.settings.dynamicColor) {
                AppRoot(activity = this)
            }
        }
        lifecycleScope.launch { Updater.autoCheck() }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        when {
            intent?.action == ACTION_CONNECT -> connect()
            intent?.data?.scheme.equals("mierus", ignoreCase = true) -> pendingImport.value = intent?.dataString
        }
    }

    fun connect() {
        if (Store.data.value.selected == null) {
            VpnState.error.value = "请先添加一个节点"
            return
        }
        val need = buildList {
            if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
            if (Build.VERSION.SDK_INT >= 37) add(PERMISSION_LOCAL_NETWORK)
        }.filter { ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED }
        if (need.isEmpty()) requestVpn() else permissions.launch(need.toTypedArray())
    }

    private fun requestVpn() {
        val consent = VpnController.prepare(this)
        if (consent != null) vpnConsent.launch(consent) else VpnController.start(this)
    }

    fun disconnect() = VpnController.stop(this)

    /** Import helper used by the clipboard / QR / deep-link paths. */
    fun importText(text: String): Result<Int> = runCatching {
        val ps = Core.parseLinks(text)
        Store.addProfiles(ps)
        ps.size
    }.recoverCatching { throw IllegalArgumentException(it.userMessage()) }

    companion object {
        const val ACTION_CONNECT = "io.github.bidabrain.mieru.CONNECT"
        const val PERMISSION_LOCAL_NETWORK = "android.permission.ACCESS_LOCAL_NETWORK"
    }
}
