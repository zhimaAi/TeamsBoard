import { onBeforeUnmount, ref, shallowRef } from 'vue'

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
      throw new Error('仅支持 PNG、JPEG、WebP 图片')
    }
    if (nextFile.size > MAX_ICON_FILE_SIZE) {
      throw new Error('图片大小不能超过 2MB')
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
