import 'vue-router'

declare module 'vue-router' {
  interface RouteMeta {
    /** Page title (for top context bar) */
    title?: string
    /** Main menu key */
    menu?: string
    /** Whether a local browser Session is required (default true) */
    requiresSession?: boolean
    /** Whether cloud login is required */
    requiresCloud?: boolean
  }
}
