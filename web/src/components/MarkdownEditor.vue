<template>
  <div
    ref="rootRef"
    class="markdown-editor"
    :class="{ 'is-disabled': disabled }"
  >
    <div ref="hostRef" class="markdown-editor-host"></div>
    <!--
      S-UI-11: 浮动工具栏（仅 floatingToolbar === true 时启用）。
      只在真正选中文本（非空选区）时才显示，折叠光标或选区离开编辑器都隐藏，
      避免光标停留时工具栏一直悬浮遮挡编辑区；
      定位基于选区 Range 与组件根矩形，不依赖 vditor 私有 API；
      不使用 v-show 之外的显隐控制，未定位完成前以 opacity 隐藏，避免位置跳变（边界 E16）。
    -->
    <div
      v-show="floatingVisible"
      ref="floatingRef"
      class="floating-toolbar"
      :class="{ 'is-ready': floatingReady }"
      role="toolbar"
      :aria-label="t('components.markdown.floatingToolbar')"
      :style="floatingStyle"
    >
      <!--
        工具按钮使用 `mousedown.prevent` 以保留编辑器选区（点击时不让编辑区失焦）；
        该处理与 S-UI-12「不注册 contextmenu 处理器、不阻止右键默认行为」无关。
      -->
      <button
        v-for="tool in FLOATING_TOOLS"
        :key="tool.key"
        type="button"
        class="floating-tool"
        :aria-label="t(tool.label)"
        @mousedown.prevent="applyFloatingTool(tool.key)"
      >
        <component :is="tool.icon" />
      </button>
      <span class="floating-divider" />
      <span class="floating-actions">
        <slot name="floating-actions" :text="floatingSelectionText" />
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { h, nextTick, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
import {
  BoldOutlined,
  CodeOutlined,
  ItalicOutlined,
  LinkOutlined,
  OrderedListOutlined,
  StrikethroughOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons-vue'
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
    /** S-UI-11: 是否启用跟随编写位置的浮动工具栏（默认 false，保持任务域既有调用方行为不变） */
    floatingToolbar?: boolean
  }>(),
  {
    modelValue: '',
    placeholder: '',
    disabled: false,
    cacheId: '',
    floatingToolbar: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  ready: []
  /** S-UI-13: 编辑器选区变化；`text` 为空表示选区已离开编辑器或已折叠 */
  'selection-change': [info: EditorSelectionInfo]
}>()

type EditorSelectionInfo = {
  text: string
  prefix: string
  suffix: string
  rect: DOMRect | null
}

type FloatingToolKey =
  | 'bold'
  | 'italic'
  | 'strike'
  | 'inlineCode'
  | 'link'
  | 'quote'
  | 'unorderedList'
  | 'orderedList'

const rootRef = ref<HTMLElement>()
const hostRef = ref<HTMLElement>()
const floatingRef = ref<HTMLElement>()
let editor: Vditor | undefined
let applyingExternalValue = false
let destroyed = false
let editorReady = false
let editorVersion = 0

const VDITOR_CDN = `${import.meta.env.BASE_URL}vditor`.replace(/\/$/, '')

// 选区前后文定位片段长度（CX-2：DOM 级前后各 ≤32 字，不做真实行号）
const SELECTION_CONTEXT_LENGTH = 32
// 浮动工具栏与选区之间的最小间距，同时用作容器内边距约束（边界 E16）
const FLOATING_GAP = 8

// 引用命令没有对应的 Ant 图标：用渲染函数输出 markdown 引用标记字形，
// 可访问名称由按钮的 aria-label 提供（避免在模板中写裸文本）。
const QuoteToolIcon = () => h('span', { class: 'floating-tool-glyph', 'aria-hidden': 'true' }, '>')

const FLOATING_TOOLS: Array<{ key: FloatingToolKey; label: string; icon: Component }> = [
  { key: 'bold', label: 'components.markdown.bold', icon: BoldOutlined },
  { key: 'italic', label: 'components.markdown.italic', icon: ItalicOutlined },
  { key: 'strike', label: 'components.markdown.strike', icon: StrikethroughOutlined },
  { key: 'inlineCode', label: 'components.markdown.inlineCode', icon: CodeOutlined },
  { key: 'link', label: 'components.markdown.link', icon: LinkOutlined },
  { key: 'quote', label: 'components.markdown.quote', icon: QuoteToolIcon },
  {
    key: 'unorderedList',
    label: 'components.markdown.unorderedList',
    icon: UnorderedListOutlined,
  },
  {
    key: 'orderedList',
    label: 'components.markdown.orderedList',
    icon: OrderedListOutlined,
  },
]

const floatingVisible = ref(false)
const floatingReady = ref(false)
const floatingSelectionText = ref('')
const floatingStyle = ref<Record<string, string>>({})
let floatingSequence = 0
let lastEmittedSelectionKey = ''

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
    // S-UI-10: 顶部固定工具栏保留（与浮动工具栏并存）
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
      hljs: { enable: false,lineNumber:true },
      // 让 setPadding 计算出的左右内边距落到 minPadding=35px，从而让正文撑满编辑器宽度。
      // 设成足够大的值后，(editorWidth - maxWidth) / 2 必为负，再被 Math.max(35, ...) 夹回 35。
      maxWidth: 99999,
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

/* ---------------- S-UI-11 / S-UI-13：浮动工具栏与选区信息 ---------------- */

/** 即时渲染模式下的正文容器（仅用渲染后的 DOM 类名做定位，不使用 vditor 私有 API） */
function editorContentRoot(): HTMLElement | null {
  const host = hostRef.value
  if (!host) return null
  return (
    host.querySelector<HTMLElement>('.vditor-ir .vditor-reset') ||
    host.querySelector<HTMLElement>('.vditor-ir')
  )
}

/** 选区边界所在的块级元素（`.vditor-reset` 的直接子元素），用于取前后文片段 */
function selectionBlock(node: Node): HTMLElement | null {
  const root = editorContentRoot()
  if (!root) return null
  let current: Node | null = node
  while (current && current !== root) {
    if (current.parentNode === root && current instanceof HTMLElement) return current
    current = current.parentNode
  }
  return root
}

function offsetWithin(block: HTMLElement, boundary: Node, offset: number): number | null {
  const probe = document.createRange()
  probe.selectNodeContents(block)
  try {
    probe.setEnd(boundary, offset)
  } catch {
    return null
  }
  return probe.toString().length
}

function normalizeContext(value: string) {
  return value.replace(/\s+/g, ' ').trim()
}

/** 折叠光标位于空行/行首时可能得到全零矩形，退化为块级元素矩形（边界 E16） */
function readRangeRect(range: Range): DOMRect | null {
  const rect = range.getBoundingClientRect()
  if (rect.width || rect.height || rect.top || rect.left) return rect
  const rects = range.getClientRects()
  if (rects.length) return rects[0]
  return selectionBlock(range.startContainer)?.getBoundingClientRect() ?? null
}

function readEditorSelection(): EditorSelectionInfo | null {
  const root = editorContentRoot()
  const selection = window.getSelection()
  if (!root || !selection || !selection.rangeCount) return null
  const range = selection.getRangeAt(0)
  const container = range.commonAncestorContainer
  const element = container instanceof Element ? container : container.parentElement
  if (!element || !root.contains(element)) return null

  let prefix = ''
  let suffix = ''
  const startBlock = selectionBlock(range.startContainer)
  if (startBlock) {
    const startOffset = offsetWithin(startBlock, range.startContainer, range.startOffset)
    if (startOffset !== null) {
      const blockText = startBlock.textContent || ''
      prefix = normalizeContext(
        blockText.slice(Math.max(0, startOffset - SELECTION_CONTEXT_LENGTH), startOffset),
      )
    }
  }
  const endBlock = selectionBlock(range.endContainer)
  if (endBlock) {
    const endOffset = offsetWithin(endBlock, range.endContainer, range.endOffset)
    if (endOffset !== null) {
      const blockText = endBlock.textContent || ''
      suffix = normalizeContext(blockText.slice(endOffset, endOffset + SELECTION_CONTEXT_LENGTH))
    }
  }
  return { text: selection.toString(), prefix, suffix, rect: readRangeRect(range) }
}

function emitSelectionChange(info: EditorSelectionInfo | null) {
  const key = info ? `${info.text}#${info.rect ? 1 : 0}` : 'none'
  if (key === lastEmittedSelectionKey) return
  lastEmittedSelectionKey = key
  emit('selection-change', info ?? { text: '', prefix: '', suffix: '', rect: null })
}

function positionFloatingToolbar(rect: DOMRect | null) {
  const host = rootRef.value
  const toolbar = floatingRef.value
  if (!host || !toolbar || !rect) return
  const hostRect = host.getBoundingClientRect()
  const width = toolbar.offsetWidth
  const height = toolbar.offsetHeight

  // 水平：默认与选区左缘对齐；右侧越界则改为右缘对齐；仍越界则贴住容器右侧
  let left = rect.left - hostRect.left
  if (left + width > hostRect.width - FLOATING_GAP) {
    left = rect.right - hostRect.left - width
  }
  const maxLeft = Math.max(FLOATING_GAP, hostRect.width - width - FLOATING_GAP)
  left = Math.min(Math.max(left, FLOATING_GAP), maxLeft)

  // 垂直：默认浮在选区上方；上方空间不足则翻转到选区下方（边界 E16）
  let top = rect.top - hostRect.top - height - FLOATING_GAP
  if (top < FLOATING_GAP) top = rect.bottom - hostRect.top + FLOATING_GAP
  const maxTop = Math.max(FLOATING_GAP, hostRect.height - height - FLOATING_GAP)
  top = Math.min(Math.max(top, FLOATING_GAP), maxTop)

  floatingStyle.value = { left: `${Math.round(left)}px`, top: `${Math.round(top)}px` }
}

function hideFloatingToolbar() {
  floatingSequence += 1
  floatingVisible.value = false
  floatingReady.value = false
  floatingSelectionText.value = ''
  emitSelectionChange(null)
}

/** 读取当前选区（光标或选中文本）并刷新浮层位置；只有非空选区才显示，折叠光标或选区不在编辑器内则隐藏 */
async function refreshFloatingToolbar() {
  if (!props.floatingToolbar || destroyed) return
  const sequence = ++floatingSequence
  const info = readEditorSelection()
  // 折叠光标（info.text 为空）也走隐藏分支：避免光标进入编辑器后工具栏一直悬浮遮挡编辑区，
  // 同时 hideFloatingToolbar 会向调用方 emit 空 text 的选区变化以便清理相关 UI 状态
  if (!info || !info.text) {
    hideFloatingToolbar()
    return
  }
  floatingSelectionText.value = info.text
  if (!floatingVisible.value) floatingReady.value = false
  floatingVisible.value = true
  await nextTick()
  if (sequence !== floatingSequence || !floatingVisible.value) return
  positionFloatingToolbar(info.rect)
  floatingReady.value = true
  emitSelectionChange(info)
}

function handleDocumentSelectionChange() {
  void refreshFloatingToolbar()
}

function handleEditorScroll() {
  if (!floatingVisible.value) return
  void refreshFloatingToolbar()
}

function currentSelection() {
  return editor?.getSelection() ?? ''
}

/** 用 vditor 公开 API 包裹选中内容（updateValue 会替换当前选中内容） */
function wrapSelection(before: string, after: string) {
  const selected = currentSelection()
  if (!selected) return
  editor?.updateValue(`${before}${selected}${after}`)
  syncFromEditor()
  void refreshFloatingToolbar()
}

/** 用 vditor 公开 API 在光标处插入 markdown 片段 */
function insertAtCursor(markdown: string) {
  editor?.insertValue(markdown)
  syncFromEditor()
  void refreshFloatingToolbar()
}

/** S-UI-11: 浮动工具栏动作，全部走 vditor 公开 API（updateValue / insertValue） */
function applyFloatingTool(key: FloatingToolKey) {
  switch (key) {
    case 'bold':
      wrapSelection('**', '**')
      break
    case 'italic':
      wrapSelection('*', '*')
      break
    case 'strike':
      wrapSelection('~~', '~~')
      break
    case 'inlineCode':
      wrapSelection('`', '`')
      break
    case 'link':
      if (currentSelection()) wrapSelection('[', ']()')
      else insertAtCursor('[]()')
      break
    case 'quote':
      insertAtCursor('> ')
      break
    case 'unorderedList':
      insertAtCursor('- ')
      break
    case 'orderedList':
      insertAtCursor('1. ')
      break
  }
}

/* ---------------- 生命周期 ---------------- */

onMounted(() => {
  void nextTick(() => createEditor())
  rootRef.value?.addEventListener('scroll', handleEditorScroll, true)
  // S-UI-11: 仅在启用浮动工具栏时注册；S-UI-12: 不注册任何 contextmenu 处理器
  if (props.floatingToolbar) {
    document.addEventListener('selectionchange', handleDocumentSelectionChange)
  }
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
  hideFloatingToolbar()
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
  // web/AGENTS.md：组件卸载必须清理监听器
  document.removeEventListener('selectionchange', handleDocumentSelectionChange)
  rootRef.value?.removeEventListener('scroll', handleEditorScroll, true)
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
  /** S-UI-11: 当前选区信息（含前后文定位片段与选区矩形） */
  getSelectionInfo(): EditorSelectionInfo | null {
    return readEditorSelection()
  },
  /** S-UI-10: 大纲跳转——定位第 index 个标题元素并滚动到可视区 */
  scrollToHeading(index: number) {
    const host = hostRef.value
    if (!host || index < 0) return
    const headings = host.querySelectorAll<HTMLElement>(
      '.vditor-ir .vditor-reset h1, .vditor-ir .vditor-reset h2, .vditor-ir .vditor-reset h3, ' +
        '.vditor-ir .vditor-reset h4, .vditor-ir .vditor-reset h5, .vditor-ir .vditor-reset h6',
    )
    headings[index]?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  },
})
</script>

<style scoped>
.markdown-editor {
  position: relative;
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
}

.markdown-editor-host {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
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

.floating-toolbar {
  position: absolute;
  z-index: 20;
  display: flex;
  max-width: calc(100% - 16px);
  align-items: center;
  gap: 2px;
  padding: 4px 6px;
  border: 1px solid #f0f0f0;
  border-radius: 12px;
  opacity: 0;
  background: #fff;
  box-shadow:
    0 6px 30px 5px rgba(0, 0, 0, 0.05),
    0 16px 24px 2px rgba(0, 0, 0, 0.04),
    0 8px 10px -5px rgba(0, 0, 0, 0.08);
  transition: opacity 0.12s ease;
}

.floating-toolbar.is-ready {
  opacity: 1;
}

.floating-tool {
  display: inline-flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 8px;
  color: #595959;
  background: transparent;
  cursor: pointer;
  font-size: 15px;
  transition: color 0.18s ease, background 0.18s ease;
}

.floating-tool:hover {
  color: #3157e2;
  background: #f0f1f3;
}

.floating-tool:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

/* 引用命令使用 markdown 标记字形，等宽字体与图标视觉重量对齐 */
.floating-tool-glyph {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 15px;
  font-weight: 600;
  line-height: 1;
}

.floating-divider {
  width: 1px;
  height: 18px;
  margin: 0 4px;
  background: #e2e4e8;
}

.floating-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

@media (prefers-reduced-motion: reduce) {
  .floating-toolbar,
  .floating-tool {
    transition: none;
  }
}
</style>
