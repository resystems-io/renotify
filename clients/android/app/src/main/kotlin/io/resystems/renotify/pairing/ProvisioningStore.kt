package io.resystems.renotify.pairing

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Persistent storage for provisioning credentials received from
 * the QR pairing code. Implementations must encrypt data at rest.
 */
interface ProvisioningStore {
    fun save(payload: ProvisioningPayload)
    fun load(): ProvisioningPayload?
    fun clear()
    fun isPaired(): Boolean
}

/**
 * [ProvisioningStore] backed by [EncryptedSharedPreferences].
 * Uses AES-256-GCM encryption via Android Keystore.
 */
class EncryptedProvisioningStore(context: Context) :
    ProvisioningStore {

    private val prefs: SharedPreferences

    init {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        prefs = EncryptedSharedPreferences.create(
            context,
            PREFS_FILE,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme
                .AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme
                .AES256_GCM
        )
    }

    override fun save(payload: ProvisioningPayload) {
        prefs.edit()
            .putInt(KEY_VERSION, payload.version)
            .putString(KEY_HOST, payload.host)
            .putInt(KEY_PORT, payload.port)
            .putString(KEY_TOKEN, payload.token)
            .putString(KEY_CERT, payload.certFingerprint)
            .putString(KEY_USERNAME, payload.username)
            .apply()
    }

    override fun load(): ProvisioningPayload? {
        val host = prefs.getString(KEY_HOST, null) ?: return null
        val token = prefs.getString(KEY_TOKEN, null) ?: return null
        val cert = prefs.getString(KEY_CERT, null) ?: return null
        val username = prefs.getString(KEY_USERNAME, null)
            ?: return null
        val version = prefs.getInt(KEY_VERSION, -1)
        val port = prefs.getInt(KEY_PORT, -1)
        if (version < 0 || port < 0) return null

        return ProvisioningPayload(
            version = version,
            host = host,
            port = port,
            token = token,
            certFingerprint = cert,
            username = username
        )
    }

    override fun clear() {
        prefs.edit().clear().apply()
    }

    override fun isPaired(): Boolean = load() != null

    companion object {
        private const val PREFS_FILE = "renotify_provisioning"
        private const val KEY_VERSION = "v"
        private const val KEY_HOST = "h"
        private const val KEY_PORT = "p"
        private const val KEY_TOKEN = "t"
        private const val KEY_CERT = "c"
        private const val KEY_USERNAME = "u"
    }
}
