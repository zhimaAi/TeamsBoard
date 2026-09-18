import dayjs from 'dayjs'
import 'dayjs/locale/en'
import 'dayjs/locale/zh-cn'
import { createI18n, useI18n } from 'vue-i18n'
import {
  DEFAULT_LOCALE,
  FALLBACK_LOCALE,
  LOCALE_STORAGE_KEY,
  LOCALE_OPTIONS,
  getLocaleDefinition,
  isAppLocale,
  type AppLocale,
} from './config'
import {
  datetimeFormats,
  numberFormats,
  type DateTimeSchema,
  type NumberSchema,
} from './formats'
import { type MessageKey, type MessageSchema } from './messages'

function readStoredLocale(): AppLocale {
  if (typeof window === 'undefined') return DEFAULT_LOCALE

  try {
    const storedLocale = window.localStorage.getItem(LOCALE_STORAGE_KEY)
    return isAppLocale(storedLocale) ? storedLocale : DEFAULT_LOCALE
  } catch {
    return DEFAULT_LOCALE
  }
}

const initialLocale = readStoredLocale()
const messages = Object.fromEntries(
  LOCALE_OPTIONS.map((definition) => [definition.code, definition.messages]),
) as Record<AppLocale, MessageSchema>

export const i18n = createI18n<
  [MessageSchema, DateTimeSchema, NumberSchema],
  AppLocale,
  false
>({
  legacy: false,
  globalInjection: false,
  locale: initialLocale,
  fallbackLocale: FALLBACK_LOCALE,
  messages,
  datetimeFormats,
  numberFormats,
  missingWarn: import.meta.env.DEV,
  fallbackWarn: import.meta.env.DEV,
})

function applyLocaleEnvironment(locale: AppLocale) {
  const definition = getLocaleDefinition(locale)
  dayjs.locale(definition.dayjsLocale)

  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale
    document.documentElement.dir = definition.direction
  }

  // 托盘菜单和退出/关闭确认框由 Electron 主进程绘制，读不到渲染层的 vue-i18n 资源，
  // 必须在这里把语言推过去。直接调用桥接而不走 useDesktop，是为了避免
  // i18n -> useDesktop -> i18n 的模块循环依赖（web/AGENTS.md 禁止循环依赖）。
  // 同步失败只影响原生界面语言，不应影响页面已完成的切换。
  if (typeof window !== 'undefined') {
    void window.goteamsDesktop?.setLocale(locale)?.catch(() => {
      // 浏览器模式或旧版宿主没有该能力时忽略。
    })
  }
}

export function ensureLocaleLoaded(locale: AppLocale): boolean {
  return i18n.global.availableLocales.includes(locale)
}

export function getCurrentLocale(): AppLocale {
  return i18n.global.locale.value
}

export function setLocale(locale: AppLocale, options: { persist?: boolean } = {}): boolean {
  if (!isAppLocale(locale) || !ensureLocaleLoaded(locale)) return false

  i18n.global.locale.value = locale
  applyLocaleEnvironment(locale)

  if (options.persist !== false && typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, locale)
    } catch {
      // 存储不可用时仍保留当前会话的语言选择。
    }
  }

  return true
}

export function initializeLocale() {
  setLocale(initialLocale, { persist: false })
}

let storageListener: ((event: StorageEvent) => void) | undefined

export function startLocaleSync() {
  if (typeof window === 'undefined' || storageListener) return

  storageListener = (event) => {
    if (event.key !== LOCALE_STORAGE_KEY) return
    const nextLocale = isAppLocale(event.newValue) ? event.newValue : DEFAULT_LOCALE
    setLocale(nextLocale, { persist: false })
  }
  window.addEventListener('storage', storageListener)
}

export function stopLocaleSync() {
  if (typeof window === 'undefined' || !storageListener) return
  window.removeEventListener('storage', storageListener)
  storageListener = undefined
}

export function useAppI18n() {
  return useI18n<{
    message: MessageSchema
    datetime: DateTimeSchema
    number: NumberSchema
  }, AppLocale>({ useScope: 'global' })
}

export function t(key: MessageKey, named?: Record<string, unknown>): string {
  const globalTranslate = i18n.global.t as unknown as (
    messageKey: MessageKey,
    values?: Record<string, unknown>,
  ) => string
  return globalTranslate(key, named)
}

export type { AppLocale, MessageKey, MessageSchema }
