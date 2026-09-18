import { onBeforeUnmount, ref, shallowRef } from 'vue'
import { t } from '@/i18n'

export const MAX_ICON_FILE_SIZE = 2 * 1024 * 1024
export const ICON_FILE_ACCEPT = 'image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp'

const SUPPORTED_ICON_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])

export function useIconFile() {
  const file = shallowRef<File>()
  const previewUrl = ref('')

  function releasePreview() {
    if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
    previewUrl.value = ''
  }

  function selectFile(nextFile: File) {
    if (!SUPPORTED_ICON_TYPES.has(nextFile.type)) {
      throw new Error(t('components.errors.iconType'))
    }
    if (nextFile.size > MAX_ICON_FILE_SIZE) {
      throw new Error(t('components.errors.iconSize'))
    }

    releasePreview()
    file.value = nextFile
    previewUrl.value = URL.createObjectURL(nextFile)
  }

  function reset() {
    releasePreview()
    file.value = undefined
  }

  onBeforeUnmount(reset)

  return {
    file,
    previewUrl,
    selectFile,
    reset,
  }
}
