import { computed } from 'vue'
import { LOCALE_OPTIONS, getLocaleDefinition } from '@/i18n/config'
import { i18n, setLocale } from '@/i18n'

export function useLocale() {
  const locale = computed(() => i18n.global.locale.value)
  const currentOption = computed(() => getLocaleDefinition(locale.value))
  const antLocale = computed(() => currentOption.value.antLocale)

  return {
    locale,
    options: LOCALE_OPTIONS,
    currentOption,
    antLocale,
    setLocale,
  }
}
