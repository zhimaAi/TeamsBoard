import {
  inject,
  onScopeDispose,
  provide,
  toValue,
  watch,
  type InjectionKey,
  type MaybeRefOrGetter,
} from 'vue'

const APP_TITLE = 'Teams Desk Agent'

type TitleSource = MaybeRefOrGetter<string | null | undefined>

interface TitleEntry {
  title: string
}

interface TitleRegistration {
  update: (title: string | null | undefined) => void
  dispose: () => void
}

interface DocumentTitleContext {
  register: () => TitleRegistration
}

const documentTitleKey: InjectionKey<DocumentTitleContext> = Symbol('document-title')

function normalizeTitle(title: string | null | undefined) {
  return title?.trim() || ''
}

export function provideDocumentTitle(fallbackSource: TitleSource) {
  const entries: TitleEntry[] = []

  function syncTitle() {
    if (typeof document === 'undefined') return
    const overrideTitle = [...entries].reverse().find((entry) => entry.title)?.title
    const pageTitle = overrideTitle || normalizeTitle(toValue(fallbackSource))
    document.title = pageTitle ? `${pageTitle}-${APP_TITLE}` : APP_TITLE
  }

  watch(() => toValue(fallbackSource), syncTitle, { immediate: true })

  provide(documentTitleKey, {
    register() {
      const entry: TitleEntry = { title: '' }
      entries.push(entry)

      return {
        update(title) {
          entry.title = normalizeTitle(title)
          syncTitle()
        },
        dispose() {
          const index = entries.indexOf(entry)
          if (index >= 0) entries.splice(index, 1)
          syncTitle()
        },
      }
    },
  })
}

export function useDocumentTitle(titleSource: TitleSource) {
  const context = inject(documentTitleKey)
  if (!context) return

  const registration = context.register()
  const stop = watch(
    () => toValue(titleSource),
    (title) => registration.update(title),
    { immediate: true },
  )

  onScopeDispose(() => {
    stop()
    registration.dispose()
  })
}
