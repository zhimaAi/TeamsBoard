import 'vue-router'
import type { MessageKey } from '@/i18n/messages'

declare module 'vue-router' {
  interface RouteMeta {
    /** Localized page title key */
    titleKey?: MessageKey
    /** Page title (for top context bar) */
    title?: string
    /** Main menu key */
    menu?: string
    /** Main content layout; custom pages manage their own padding and background */
    contentLayout?: 'default' | 'custom'
    /** Whether the main content container handles vertical scrolling (default true) */
    isScroll?: boolean
    /** Whether a local browser Session is required (default true) */
    requiresSession?: boolean
    /** Whether cloud login is required */
    requiresCloud?: boolean
  }
}
