import type { Locale } from 'ant-design-vue/es/locale'
import enUS from 'ant-design-vue/es/locale/en_US'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import { enUSMessages, zhCNMessages, type MessageSchema } from './messages'

export type AppLocale = 'zh-CN' | 'en-US'

export interface LocaleDefinition {
  code: AppLocale
  autonym: string
  shortLabel: string
  direction: 'ltr' | 'rtl'
  antLocale: Locale
  dayjsLocale: string
  messages: MessageSchema
}

export const DEFAULT_LOCALE: AppLocale = 'zh-CN'
export const FALLBACK_LOCALE: AppLocale = 'zh-CN'
export const LOCALE_STORAGE_KEY = 'goteams.locale'

export const LOCALE_OPTIONS: readonly LocaleDefinition[] = [
  {
    code: 'zh-CN',
    autonym: '简体中文',
    shortLabel: '中',
    direction: 'ltr',
    antLocale: zhCN,
    dayjsLocale: 'zh-cn',
    messages: zhCNMessages,
  },
  {
    code: 'en-US',
    autonym: 'English',
    shortLabel: 'EN',
    direction: 'ltr',
    antLocale: enUS,
    dayjsLocale: 'en',
    messages: enUSMessages,
  },
]

export function isAppLocale(value: unknown): value is AppLocale {
  return LOCALE_OPTIONS.some((option) => option.code === value)
}

export function getLocaleDefinition(locale: AppLocale): LocaleDefinition {
  return LOCALE_OPTIONS.find((option) => option.code === locale) ?? LOCALE_OPTIONS[0]
}
