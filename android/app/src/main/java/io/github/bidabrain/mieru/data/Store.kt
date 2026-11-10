package io.github.bidabrain.mieru.data

import android.content.Context
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File

/** Single JSON file holding everything the user configures. */
object Store {
    private lateinit var file: File
    private val _data = MutableStateFlow(AppData())
    val data: StateFlow<AppData> = _data.asStateFlow()
    private val io = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val writeLock = Mutex()

    fun init(ctx: Context) {
        file = File(ctx.filesDir, "data.json")
        if (file.exists()) {
            runCatching { AppJson.decodeFromString<AppData>(file.readText()) }
                .onSuccess { _data.value = it }
                .onFailure { Log.e("Store", "load failed", it) }
        }
    }

    fun update(fn: (AppData) -> AppData) {
        synchronized(this) { _data.value = fn(_data.value) }
        val snapshot = _data.value
        io.launch {
            writeLock.withLock {
                val tmp = File(file.path + ".tmp")
                tmp.writeText(AppJson.encodeToString(AppData.serializer(), snapshot))
                tmp.renameTo(file)
            }
        }
    }

    fun updateRouting(fn: (Routing) -> Routing) = update { it.copy(routing = fn(it.routing)) }
    fun updateSettings(fn: (Settings) -> Settings) = update { it.copy(settings = fn(it.settings)) }

    fun upsertProfile(p: Profile) = update { d ->
        val list = if (d.profiles.any { it.id == p.id }) d.profiles.map { if (it.id == p.id) p else it } else d.profiles + p
        d.copy(profiles = list, selectedId = d.selectedId ?: p.id)
    }

    fun addProfiles(ps: List<Profile>) = update { d ->
        d.copy(profiles = d.profiles + ps, selectedId = d.selectedId ?: ps.firstOrNull()?.id)
    }

    fun deleteProfile(id: String) = update { d ->
        val list = d.profiles.filterNot { it.id == id }
        d.copy(profiles = list, selectedId = if (d.selectedId == id) list.firstOrNull()?.id else d.selectedId)
    }

    fun select(id: String) = update { it.copy(selectedId = id) }
}
