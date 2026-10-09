import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  cancelUpdateDownload,
  checkForUpdates,
  deferUpdate,
  downloadUpdate,
  getUpdateState,
  installUpdate,
  isDesktopRuntime,
  onUpdateState,
} from '@/composables/useDesktop'

export const useUpdateStore = defineStore('update', () => {
  const state = ref<DesktopUpdateState | null>(null)
  const modalOpen = ref(false)
  const checking = ref(false)
  let unsubscribe: (() => void) | null = null
  let initialized = false
  let initialization: Promise<void> | null = null
  let activeChecks = 0

  function initialize(): Promise<void> {
    if (!isDesktopRuntime() || initialized) return Promise.resolve()
    if (initialization) return initialization
    initialization = (async () => {
      try {
        unsubscribe = onUpdateState(next => { state.value = next })
        const initial = await getUpdateState()
        if (!state.value) state.value = initial
        initialized = true
      } catch (error) {
        unsubscribe?.()
        unsubscribe = null
        throw error
      } finally {
        initialization = null
      }
    })()
    return initialization
  }

  async function check(manual: boolean) {
    await initialize()
    if (!isDesktopRuntime()) return null
    checking.value = ++activeChecks > 0
    try {
      const result = await checkForUpdates(manual)
      state.value = result
      if (result.showPrompt) modalOpen.value = true
      return result
    } finally {
      checking.value = --activeChecks > 0
    }
  }

  async function later() {
    await deferUpdate()
    modalOpen.value = false
  }

  async function close() {
    if (state.value?.status === 'downloading') await cancelUpdateDownload()
    await later()
  }

  async function download() {
    state.value = await downloadUpdate()
  }

  async function cancelDownload() {
    await cancelUpdateDownload()
    modalOpen.value = false
  }

  async function install() {
    const result = await installUpdate()
    if (result.manual) modalOpen.value = false
    return result
  }

  function dispose() {
    unsubscribe?.()
    unsubscribe = null
    initialized = false
  }

  return { state, modalOpen, checking, initialize, check, later, close, download,
    cancelDownload, install, dispose }
})
