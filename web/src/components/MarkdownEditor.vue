<template>
  <div
    ref="hostRef"
    class="markdown-editor"
    :class="{ 'is-disabled': disabled }"
  ></div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
import { MAX_IMAGE_SIZE, TASK_IMAGE_ACCEPT } from '@/composables/useTaskImageAttachments'
import { useAppI18n } from '@/i18n'

const { t, locale } = useAppI18n()

const props = withDefaults(
  defineProps<{
    modelValue?: string
    placeholder?: string
    disabled?: boolean
    cacheId?: string
    onUploadImages?: (files: File[]) => Promise<string[]>
  }>(),
  {
    modelValue: '',
    placeholder: '',
    disabled: false,
    cacheId: '',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  ready: []
}>()

const hostRef = ref<HTMLElement>()
let editor: Vditor | undefined
let applyingExternalValue = false
let destroyed = false
let editorReady = false
let editorVersion = 0

const VDITOR_CDN = `${import.meta.env.BASE_URL}vditor`.replace(/\/$/, '')

function currentValue() {
  return editorReady && editor ? editor.getValue() : props.modelValue
}

function syncFromEditor() {
  if (!editorReady || !editor || applyingExternalValue) return
  emit('update:modelValue', editor.getValue())
}

function handleUpload(files: File[]): string | null | Promise<null> {
  if (!props.onUploadImages) return t('components.markdown.uploadUnavailable')
  if (!files.length) return null
  return insertUploadedMarkers(files)
}

async function insertUploadedMarkers(files: File[]): Promise<null> {
  const onUploadImages = props.onUploadImages
  if (!onUploadImages) {
    editor?.tip(t('components.markdown.uploadUnavailable'))
    return null
  }
  try {
    const markers = await onUploadImages(files)
    if (markers.length) {
      editor?.insertValue(markers.join('\n'))
      syncFromEditor()
    }
  } catch (error) {
    editor?.tip(error instanceof Error ? error.message : t('components.markdown.uploadFailed'))
  }
  return null
}

function createEditor() {
  const host = hostRef.value
  if (!host || destroyed) return
  const version = ++editorVersion

  editor = new Vditor(host, {
    cdn: VDITOR_CDN,
    mode: 'ir',
    theme: 'classic',
    icon: 'ant',
    lang: locale.value === 'en-US' ? 'en_US' : 'zh_CN',
    height: '100%',
    placeholder: props.placeholder || t('components.markdown.placeholder'),
    value: props.modelValue,
    cache: { enable: false, id: props.cacheId || 'task-description' },
    toolbar: [
      'headings',
      'bold',
      'italic',
      'strike',
      '|',
      'list',
      'ordered-list',
      'check',
      '|',
      'quote',
      'line',
      'code',
      'inline-code',
      'link',
      'table',
      '|',
      'undo',
      'redo',
      ...(props.onUploadImages ? (['|', 'upload'] as const) : []),
    ],
    toolbarConfig: {
      pin: true,
    },
    counter: { enable: false },
    outline: { enable: false, position: 'left' },
    preview: {
      delay: 200,
      hljs: { enable: false },
      markdown: {
        toc: false,
        footnotes: false,
        mark: true,
        sanitize: true,
        codeBlockPreview: false,
        mathBlockPreview: false,
      },
      theme: {
        current: 'light',
        path: `${VDITOR_CDN}/dist/css/content-theme`,
      },
    },
    upload: {
      accept: TASK_IMAGE_ACCEPT,
      max: MAX_IMAGE_SIZE,
      multiple: true,
      filename: (name) => name,
      handler: handleUpload,
    },
    input() {
      syncFromEditor()
    },
    blur() {
      syncFromEditor()
    },
    after() {
      if (destroyed || version !== editorVersion) return
      editorReady = true
      applyingExternalValue = true
      editor?.setValue(props.modelValue || '', true)
      applyingExternalValue = false
      if (props.disabled) editor?.disabled()
      else editor?.enable()
      emit('ready')
    },
  })
}

onMounted(() => {
  void nextTick(() => createEditor())
})

watch(
  () => props.modelValue,
  (value) => {
    if (!editorReady || !editor) return
    if (value === editor.getValue()) return
    applyingExternalValue = true
    editor.setValue(value || '', true)
    applyingExternalValue = false
  },
)

watch(
  () => props.disabled,
  (disabled) => {
    if (!editorReady || !editor) return
    if (disabled) editor.disabled()
    else editor.enable()
  },
)

watch(locale, async () => {
  syncFromEditor()
  editorVersion += 1
  const version = editorVersion
  editorReady = false
  editor?.destroy()
  editor = undefined
  await nextTick()
  if (!destroyed && version === editorVersion) createEditor()
})

onBeforeUnmount(() => {
  destroyed = true
  editorVersion += 1
  editorReady = false
  editor?.destroy()
  editor = undefined
})

defineExpose({
  getValue: currentValue,
  focus() {
    editor?.focus()
  },
  resize() {
    const host = hostRef.value
    const vditorEl = host?.querySelector<HTMLElement>('.vditor')
    if (host && vditorEl) vditorEl.style.height = '100%'
  },
})
</script>

<style scoped>
.markdown-editor {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
}

.markdown-editor :deep(.vditor) {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

.markdown-editor :deep(.vditor-content),
.markdown-editor :deep(.vditor-ir) {
  min-height: 0;
  flex: 1;
}

.markdown-editor.is-disabled :deep(.vditor-toolbar) {
  pointer-events: none;
}
</style>
