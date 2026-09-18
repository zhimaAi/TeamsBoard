import { onBeforeUnmount, ref, watch } from 'vue'

export function useContentFullscreen(bodyClass = 'goteams-task-fullscreen') {
  const fullscreen = ref(false)

  watch(fullscreen, (value) => {
    document.body.classList.toggle(bodyClass, value)
  })

  onBeforeUnmount(() => {
    document.body.classList.remove(bodyClass)
    fullscreen.value = false
  })

  return { fullscreen }
}
