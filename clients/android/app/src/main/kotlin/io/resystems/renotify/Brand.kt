// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package io.resystems.renotify

import android.content.Context
import android.graphics.Typeface
import android.text.SpannableString
import android.text.Spanned
import android.text.style.ForegroundColorSpan
import android.text.style.RelativeSizeSpan
import android.text.style.StyleSpan
import android.text.style.TypefaceSpan

/**
 * Centralised brand identity for the Renotify app (M-05).
 * Defines colours, font loading, and branded text helpers.
 */
object Brand {

    // --- Colours ---

    /** Brand red — used for "Re" prefix and logo accent. */
    const val PRIMARY = 0xFF993333.toInt()

    /** Brand grey — used for "notify"/"systems" suffix. */
    const val TEXT_PRIMARY = 0xFF555555.toInt()

    /** Secondary text — status, metadata, timestamps. */
    const val TEXT_SECONDARY = 0xFF999999.toInt()

    /** Dark header/surface background. */
    const val HEADER_BG = 0xFF202020.toInt()

    /** Button background. */
    const val BUTTON_BG = 0xFF444444.toInt()

    /** Button text. */
    const val BUTTON_TEXT = 0xFFFFFFFF.toInt()

    /** Primary text on light backgrounds. */
    const val TEXT_DARK = 0xFF333333.toInt()

    /** Subtle divider/border colour. */
    const val DIVIDER = 0xFFDDDDDD.toInt()

    /** Dashboard flow status indicator (green). */
    const val STATUS_ACTIVE = 0xFF4CAF50.toInt()

    /** Stop button red. */
    const val ACTION_STOP = 0xFFCC3333.toInt()

    /** Link/load-more blue. */
    const val LINK = 0xFF1A73E8.toInt()

    // --- Fonts ---

    private var dancingScript: Typeface? = null
    private var montserrat: Typeface? = null

    /** Dancing Script Bold — used for "Re" prefix. */
    fun dancingScript(context: Context): Typeface {
        return dancingScript ?: Typeface.createFromAsset(
            context.assets,
            "fonts/DancingScript-Bold.ttf"
        ).also { dancingScript = it }
    }

    /** Montserrat Regular — used for "notify"/"systems". */
    fun montserrat(context: Context): Typeface {
        return montserrat ?: Typeface.createFromAsset(
            context.assets,
            "fonts/Montserrat-Regular.ttf"
        ).also { montserrat = it }
    }

    // --- Branded text ---

    /**
     * Build a SpannableString with the "Re" prefix in Dancing
     * Script Bold [PRIMARY] and the remainder in Montserrat
     * Regular [TEXT_PRIMARY].
     *
     * Usage: `brandedName("Renotify")` or
     *        `brandedName("Resystems")`
     */
    fun brandedName(
        context: Context,
        name: String,
        textSize: Float? = null
    ): SpannableString {
        val prefixLen = 2 // "Re"
        val span = SpannableString(name)

        // "Re" — Dancing Script Bold, brand red, 1.4× larger.
        span.setSpan(
            CustomTypefaceSpan(dancingScript(context)),
            0, prefixLen,
            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        span.setSpan(
            StyleSpan(Typeface.BOLD),
            0, prefixLen,
            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        span.setSpan(
            ForegroundColorSpan(PRIMARY),
            0, prefixLen,
            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        span.setSpan(
            RelativeSizeSpan(1.4f),
            0, prefixLen,
            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)

        // Remainder — Montserrat Regular, brand grey.
        span.setSpan(
            CustomTypefaceSpan(montserrat(context)),
            prefixLen, name.length,
            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        span.setSpan(
            ForegroundColorSpan(TEXT_PRIMARY),
            prefixLen, name.length,
            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)

        return span
    }

    /**
     * Custom TypefaceSpan that accepts a [Typeface] instance
     * directly (the framework TypefaceSpan only accepts a font
     * family string before API 28).
     */
    private class CustomTypefaceSpan(
        private val typeface: Typeface
    ) : android.text.style.MetricAffectingSpan() {

        override fun updateDrawState(
            tp: android.text.TextPaint
        ) {
            tp.typeface = typeface
        }

        override fun updateMeasureState(
            tp: android.text.TextPaint
        ) {
            tp.typeface = typeface
        }
    }
}
