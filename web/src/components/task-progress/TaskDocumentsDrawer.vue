<template>
  <a-drawer
    :open="open"
    :width="drawerWidth"
    :closable="false"
    :mask-closable="false"
    :body-style="{ padding: 0, overflow: 'hidden' }"
    root-class-name="task-documents-drawer"
    @close="requestClose"
  >
    <div class="documents-drawer-shell">
      <div class="drawer-resize-handle" role="separator" tabindex="0"
        :aria-label="t('workflows.task.progress.resizeDocuments')" aria-orientation="vertical"
        :aria-valuemin="minDrawerWidth" :aria-valuemax="viewportWidth" :aria-valuenow="drawerWidth"
        @pointerdown="startResize" @pointermove="moveResize" @pointerup="finishResize"
        @pointercancel="finishResize" @lostpointercapture="finishResize" @keydown="resizeWithKeyboard" />
      <header class="documents-header">
        <div class="documents-heading">
          <span class="step-avatar">
            <img
              v-if="currentStep?.avatar"
              :src="currentStep.avatar"
              :alt="currentStep.name"
            />
            <span v-else>{{ initials(currentStep?.name) }}</span>
          </span>
          <h2>{{ currentStep?.name || t('workflows.task.progress.drawerCurrentStep') }}</h2>
          <button
            type="button"
            class="open-directory-button"
            :disabled="!taskDirectory || openingDirectory"
            :aria-label="openingDirectory ? t('workflows.task.progress.openingFolder') : t('workflows.task.progress.openInFolder')"
            :title="openingDirectory ? t('workflows.task.progress.openingFolder') : t('workflows.task.progress.openInFolder')"
            @click="openTaskDirectory"
          >
            <LoadingOutlined
              v-if="openingDirectory"
              spin
            />
            <img
              v-else
              :src="openDirectoryIcon"
              alt=""
              aria-hidden="true"
            />
          </button>
        </div>
        <div class="documents-header-actions">
          <button
            type="button"
            class="close-button"
            :aria-label="t('workflows.task.progress.closeDocuments')"
            @click="requestClose"
          >
            <CloseOutlined />
          </button>
        </div>
      </header>

      <div class="documents-workspace" :class="{ 'is-directory-collapsed': sidebarCollapsed }">
        <aside v-show="!sidebarCollapsed" class="file-sidebar">
          <div
            v-if="treeLoading"
            class="sidebar-state"
          >
            <a-spin />
          </div>
          <div
            v-else-if="treeError"
            class="sidebar-state sidebar-error"
          >
            <span>{{ treeError }}</span>
            <a-button
              size="small"
              @click="loadTree"
            >
              <ReloadOutlined />{{ t('common.actions.retry') }}
            </a-button>
          </div>
          <a-empty
            v-else-if="!treeData.length"
            :image="false"
            :description="t('workflows.task.progress.noDocuments')"
          />
          <a-tree
            v-else
            block-node
            :tree-data="treeData"
            :selected-keys="selectedKeys"
            :expanded-keys="expandedKeys"
            @expand="expandedKeys = $event.map(String)"
            @select="handleSelect"
          >
            <template #title="{ title, isDir, fileType, meta, source }">
              <div class="file-tree-node">
                <FolderOutlined
                  v-if="isDir"
                  class="file-node-icon folder-icon"
                />
                <FileMarkdownOutlined
                  v-else-if="isMarkdown(fileType)"
                  class="file-node-icon markdown-icon"
                />
                <FileImageOutlined
                  v-else-if="isImageType(fileType)"
                  class="file-node-icon image-icon"
                />
                <FileTextOutlined
                  v-else-if="isTextType(fileType)"
                  class="file-node-icon text-icon"
                />
                <FileOutlined
                  v-else
                  class="file-node-icon"
                />
                <span
                  class="file-node-name"
                  :title="title"
                  >{{ title }}</span
                >
                <span
                  v-if="meta"
                  class="file-node-meta"
                  >{{ meta }}</span
                >
                <!-- 非目录且拿得到绝对路径的文件，提供一键引用到输入框 -->
                <button
                  v-if="!isDir && source?.absolute_path"
                  type="button"
                  class="file-node-reference"
                  :title="t('workflows.task.progress.referenceToInput')"
                  :aria-label="t('workflows.task.progress.referenceToInput')"
                  @click.stop="emit('reference', source)"
                >
                  <LinkOutlined />
                </button>
              </div>
            </template>
          </a-tree>
        </aside>

        <main class="file-workspace">
          <div class="file-toolbar">
            <button
              type="button"
              class="directory-collapse-btn"
              :title="sidebarCollapsed ? t('workflows.task.progress.expandDirectory') : t('workflows.task.progress.collapseDirectory')"
              :aria-label="sidebarCollapsed ? t('workflows.task.progress.expandDirectory') : t('workflows.task.progress.collapseDirectory')"
              :aria-expanded="!sidebarCollapsed"
              @click="sidebarCollapsed = !sidebarCollapsed"
            >
              <img
                :src="toggleDirectoryIcon"
                alt=""
                aria-hidden="true"
                :class="{ 'is-collapsed': sidebarCollapsed }"
              />
            </button>
            <div
              v-if="selectedFile"
              class="file-toolbar-copy"
            >
              <span class="save-hint"><EditOutlined />{{ t('workflows.task.progress.saveHint') }}</span>
              <strong>
                <FileImageOutlined v-if="selectedContent?.encoding === 'base64'" />
                <FileTextOutlined v-else />
                {{ selectedFile.name }}
              </strong>
            </div>
            <div v-if="selectedFile" class="file-toolbar-actions">
            <a-button
              v-if="selectedContent?.encoding !== 'base64'"
              type="primary"
              :loading="saving"
              :disabled="contentLoading || Boolean(contentError) || !dirty"
              @click="saveFile"
            >
              {{ t('common.actions.save') }}
            </a-button>
            </div>
          </div>

          <div class="file-content">
            <div
              v-if="contentLoading"
              class="content-state"
            >
              <a-spin />
            </div>
            <div
              v-else-if="contentError"
              class="content-state content-error"
            >
              <a-alert
                type="error"
                show-icon
                :message="contentError"
              />
              <a-button @click="retrySelectedFile">{{ t('common.actions.retry') }}</a-button>
            </div>
            <a-empty
              v-else-if="!selectedFile || !selectedContent"
              :description="t('workflows.task.progress.chooseDocument')"
            />
            <div
              v-else-if="selectedContent.encoding === 'base64'"
              class="image-preview"
            >
              <img
                :src="imageDataUrl"
                :alt="selectedFile.name"
              />
            </div>
            <MarkdownEditor
              v-else-if="canPreviewMarkdown"
              :key="selectedFile.path"
              v-model="draftContent"
              class="file-markdown-editor"
              :cache-id="`task-document-${taskUuid}-${selectedFile.path}`"
              :placeholder="t('workflows.task.progress.editFile', { name: selectedFile.name })"
              :disabled="saving"
            />
            <a-textarea
              v-else
              v-model:value="draftContent"
              class="file-editor"
              :disabled="saving"
              :aria-label="t('workflows.task.progress.editFile', { name: selectedFile.name })"
            />
          </div>
        </main>
      </div>
    </div>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  CloseOutlined,
  EditOutlined,
  FileImageOutlined,
  FileMarkdownOutlined,
  FileOutlined,
  FileTextOutlined,
  FolderOutlined,
  LinkOutlined,
  LoadingOutlined,
  ReloadOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()
import openDirectoryIcon from '@/assets/icons/task-documents-open-directory.svg'
import toggleDirectoryIcon from '@/assets/icons/task-documents-toggle-directory.svg'
import { isDesktopRuntime, openDirectory } from '@/composables/useDesktop'
import { copyText } from '@/utils/clipboard'
import type { TaskFileContentResponse, TaskFileNode, TaskFilesResponse } from '@/types/task-files'
import type { PipelineStep } from '@/types/pipeline'
import { initials } from './utils'

interface FileTreeNode {
  key: string
  title: string
  path: string
  name: string
  isDir: boolean
  fileType: string
  meta: string
  source: TaskFileNode
  children?: FileTreeNode[]
}

const props = defineProps<{
  open: boolean
  taskUuid: string
  currentStep?: PipelineStep
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  /** 把某个产出文件引用进输入框（插入 #[名称] 标记） */
  reference: [file: import('@/types/task-files').MentionableTaskFile]
}>()

const treeData = ref<FileTreeNode[]>([])
const fileMap = ref(new Map<string, FileTreeNode>())
const selectedKeys = ref<string[]>([])
const expandedKeys = ref<string[]>([])
const viewportWidth = ref(typeof window === 'undefined' ? 1097 : window.innerWidth)
const minDrawerWidth = computed(() => Math.min(640, viewportWidth.value))
const drawerWidth = ref(Math.min(1097, viewportWidth.value))
const sidebarCollapsed = ref(false)
let dragging = false
let resizeStartX = 0
let resizeStartWidth = 1097
let liveWidth = 1097
let resizeWrapper: HTMLElement | null = null
let resizeRaf = 0
const selectedFile = ref<TaskFileNode>()
const selectedContent = ref<TaskFileContentResponse>()
const draftContent = ref('')
const canPreviewMarkdown = computed(() => Boolean(selectedContent.value && selectedContent.value.encoding !== 'base64' && isMarkdown(selectedFile.value?.file_type)))
const savedContent = ref('')
const treeLoading = ref(false)
const contentLoading = ref(false)
const saving = ref(false)
const openingDirectory = ref(false)
const taskDirectory = ref('')
const treeError = ref('')
const contentError = ref('')
let treeLoadVersion = 0
let contentLoadVersion = 0
let saveVersion = 0

const dirty = computed(
  () =>
    Boolean(selectedContent.value) &&
    selectedContent.value?.encoding !== 'base64' &&
    draftContent.value !== savedContent.value,
)

const imageDataUrl = computed(() => {
  const content = selectedContent.value
  if (!content || content.encoding !== 'base64') return ''
  const type = normalizedFileType(content.file_type)
  const mime =
    {
      jpg: 'image/jpeg',
      jpeg: 'image/jpeg',
      svg: 'image/svg+xml',
      png: 'image/png',
      gif: 'image/gif',
      webp: 'image/webp',
      bmp: 'image/bmp',
      ico: 'image/x-icon',
    }[type] || `image/${type}`
  return `data:${mime};base64,${content.content}`
})

watch(
  () => [props.open, props.taskUuid] as const,
  ([open]) => {
    if (open) void loadTree()
    else resetState()
  },
)

async function loadTree() {
  if (!props.taskUuid) {
    treeError.value = t('workflows.task.progress.taskMissing')
    return
  }
  const requestVersion = ++treeLoadVersion
  contentLoadVersion += 1
  saveVersion += 1
  saving.value = false
  treeLoading.value = true
  treeError.value = ''
  taskDirectory.value = ''
  clearSelection()
  try {
    const result = await apiClient.get<TaskFilesResponse>(
      `/tasks/${encodeURIComponent(props.taskUuid)}/files`,
    )
    if (requestVersion !== treeLoadVersion) return
    taskDirectory.value = result.task_dir || ''
    const nextMap = new Map<string, FileTreeNode>()
    treeData.value = (result.tree || []).map((node) => mapTreeNode(node, nextMap))
    fileMap.value = nextMap
    expandedKeys.value = [...nextMap.values()].filter((node) => node.isDir).map((node) => node.key)
    const firstFile = findFirstFile(treeData.value)
    if (firstFile) await loadFile(firstFile)
  } catch (error) {
    if (requestVersion !== treeLoadVersion) return
    treeError.value = error instanceof Error ? error.message : t('workflows.task.progress.documentTreeFailed')
  } finally {
    if (requestVersion === treeLoadVersion) treeLoading.value = false
  }
}

function mapTreeNode(node: TaskFileNode, target: Map<string, FileTreeNode>): FileTreeNode {
  const children = (node.children || []).map((child) => mapTreeNode(child, target))
  const mapped: FileTreeNode = {
    key: node.path,
    title: node.name,
    path: node.path,
    name: node.name,
    isDir: node.is_dir,
    fileType: normalizedFileType(node.file_type),
    meta: node.is_dir
      ? children.length
        ? String(children.length)
        : ''
      : formatFileSize(node.size),
    source: node,
    children: node.is_dir ? children : undefined,
  }
  target.set(node.path, mapped)
  return mapped
}

function startResize(event: PointerEvent) {
  if (event.button !== 0 || dragging) return
  event.preventDefault()
  dragging = true
  resizeStartX = event.clientX
  resizeStartWidth = drawerWidth.value
  liveWidth = drawerWidth.value
  resizeWrapper = document.querySelector<HTMLElement>('.task-documents-drawer .ant-drawer-content-wrapper')
  document.querySelector('.task-documents-drawer')?.classList.add('is-resizing')
  if (resizeWrapper) resizeWrapper.style.transition = 'none'
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function moveResize(event: PointerEvent) {
  if (!dragging) return
  liveWidth = Math.max(minDrawerWidth.value, Math.min(viewportWidth.value, resizeStartWidth + resizeStartX - event.clientX))
  if (!resizeRaf) resizeRaf = requestAnimationFrame(flushResizeFrame)
}
function flushResizeFrame() {
  resizeRaf = 0
  if (resizeWrapper) resizeWrapper.style.width = `${liveWidth}px`
}
function finishResize() {
  if (!dragging) return
  if (resizeRaf) {
    cancelAnimationFrame(resizeRaf)
    resizeRaf = 0
    flushResizeFrame()
  }
  dragging = false
  drawerWidth.value = liveWidth
  document.querySelector('.task-documents-drawer')?.classList.remove('is-resizing')
  if (resizeWrapper) resizeWrapper.style.transition = ''
  resizeWrapper = null
}
function resizeWithKeyboard(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const next = event.key === 'Home' ? minDrawerWidth.value : event.key === 'End' ? viewportWidth.value
    : drawerWidth.value + (event.key === 'ArrowLeft' ? 24 : -24)
  drawerWidth.value = Math.max(minDrawerWidth.value, Math.min(viewportWidth.value, next))
}
function updateViewport() {
  viewportWidth.value = window.innerWidth
  drawerWidth.value = Math.max(minDrawerWidth.value, Math.min(viewportWidth.value, drawerWidth.value))
}
onMounted(() => window.addEventListener('resize', updateViewport))
onBeforeUnmount(() => {
  window.removeEventListener('resize', updateViewport)
  if (resizeRaf) cancelAnimationFrame(resizeRaf)
  resetState()
})

function findFirstFile(nodes: FileTreeNode[]): FileTreeNode | undefined {
  for (const node of nodes) {
    if (!node.isDir) return node
    const child = findFirstFile(node.children || [])
    if (child) return child
  }
  return undefined
}

function handleSelect(keys: Array<string | number>) {
  if (saving.value) return
  const path = String(keys[0] || '')
  const node = fileMap.value.get(path)
  if (!node || node.isDir || node.path === selectedFile.value?.path) return
  if (!dirty.value) {
    void loadFile(node)
    return
  }
  Modal.confirm({
    title: t('workflows.task.progress.discardFileTitle'),
    content: t('workflows.task.progress.discardFileContent'),
    okText: t('workflows.task.progress.discardAndSwitch'),
    cancelText: t('workflows.task.progress.continueEditing'),
    onOk: () => loadFile(node),
  })
}

async function loadFile(node: FileTreeNode) {
  if (!props.taskUuid || node.isDir) return
  const requestVersion = ++contentLoadVersion
  selectedKeys.value = [node.path]
  selectedFile.value = node.source
  selectedContent.value = undefined
  draftContent.value = ''
  savedContent.value = ''
  contentLoading.value = true
  contentError.value = ''
  try {
    const result = await apiClient.get<TaskFileContentResponse>(
      `/tasks/${encodeURIComponent(props.taskUuid)}/files/content`,
      { path: node.path },
    )
    if (requestVersion !== contentLoadVersion) return
    selectedContent.value = result
    draftContent.value = result.content || ''
    savedContent.value = draftContent.value
  } catch (error) {
    if (requestVersion !== contentLoadVersion) return
    contentError.value = error instanceof Error ? error.message : t('workflows.task.progress.readFailed', { name: node.name })
  } finally {
    if (requestVersion === contentLoadVersion) contentLoading.value = false
  }
}

function retrySelectedFile() {
  const node = selectedFile.value ? fileMap.value.get(selectedFile.value.path) : undefined
  if (node) void loadFile(node)
}

async function saveFile() {
  const file = selectedFile.value
  if (!file || !props.taskUuid || !dirty.value || saving.value) return
  const requestVersion = ++saveVersion
  const taskUuid = props.taskUuid
  const content = draftContent.value
  saving.value = true
  try {
    await apiClient.post(`/tasks/${encodeURIComponent(taskUuid)}/files/content`, {
      path: file.path,
      content,
    })
    if (requestVersion !== saveVersion) return
    savedContent.value = content
    message.success(t('workflows.task.progress.saved', { name: file.name }))
  } catch (error) {
    if (requestVersion !== saveVersion) return
    message.error(error instanceof Error ? error.message : t('workflows.task.progress.saveFailed', { name: file.name }))
  } finally {
    if (requestVersion === saveVersion) saving.value = false
  }
}

function requestClose() {
  if (saving.value) return
  if (!dirty.value) {
    emit('update:open', false)
    return
  }
  Modal.confirm({
    title: t('workflows.task.progress.discardCloseTitle'),
    content: t('workflows.task.progress.discardCloseContent'),
    okText: t('workflows.task.progress.discardChanges'),
    cancelText: t('workflows.task.progress.continueEditing'),
    onOk: () => emit('update:open', false),
  })
}

function clearSelection() {
  selectedKeys.value = []
  selectedFile.value = undefined
  selectedContent.value = undefined
  draftContent.value = ''
  savedContent.value = ''
  contentLoading.value = false
  contentError.value = ''
}

async function openTaskDirectory() {
  if (!taskDirectory.value || openingDirectory.value) return
  if (!isDesktopRuntime()) {
    try {
      await copyText(taskDirectory.value)
      message.info(t('workflows.task.progress.browserFolderCopied'))
    } catch {
      message.warning(t('workflows.task.progress.browserFolderUnavailable', { path: taskDirectory.value }))
    }
    return
  }

  openingDirectory.value = true
  try {
    await openDirectory(taskDirectory.value)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.progress.openWorkspaceFailed'))
  } finally {
    openingDirectory.value = false
  }
}

function resetState() {
  treeLoadVersion += 1
  contentLoadVersion += 1
  saveVersion += 1
  treeData.value = []
  expandedKeys.value = []
  fileMap.value = new Map()
  treeLoading.value = false
  saving.value = false
  openingDirectory.value = false
  taskDirectory.value = ''
  treeError.value = ''
  clearSelection()
  sidebarCollapsed.value = false
  dragging = false
  if (resizeRaf) {
    cancelAnimationFrame(resizeRaf)
    resizeRaf = 0
  }
  document.querySelector('.task-documents-drawer')?.classList.remove('is-resizing')
  resizeWrapper = null
}

function normalizedFileType(type?: string) {
  return (type || '').replace(/^\./, '').toLowerCase()
}

function isMarkdown(type?: string) {
  return ['md', 'markdown'].includes(normalizedFileType(type))
}

function isImageType(type?: string) {
  return ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'bmp', 'ico'].includes(
    normalizedFileType(type),
  )
}

function isTextType(type?: string) {
  return [
    'txt',
    'json',
    'yaml',
    'yml',
    'xml',
    'html',
    'css',
    'js',
    'ts',
    'tsx',
    'jsx',
    'vue',
    'go',
    'py',
    'java',
    'sh',
    'sql',
    'toml',
    'ini',
    'log',
  ].includes(normalizedFileType(type))
}

function formatFileSize(size?: number) {
  if (size === undefined || size < 0) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}
</script>

<style scoped>
.documents-drawer-shell {
  position: relative;
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  background: #fff;
}
.documents-header {
  display: flex;
  min-height: 93px;
  flex: 0 0 auto;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #d9d9d9;
  padding: 26px 32px;
}
.documents-header-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 12px;
}
.open-directory-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 4px;
  color: #bfbfbf;
  background: transparent;
  cursor: pointer;
}
.open-directory-button img,
.open-directory-button :deep(.anticon) {
  display: block;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
}
.open-directory-button:not(:disabled):hover {
  background: #e4e6eb;
}
.open-directory-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}
.open-directory-button:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.documents-heading {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}
.step-avatar {
  display: inline-flex;
  overflow: hidden;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #3157e2;
  font-size: 16px;
  font-weight: 600;
}
.step-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.documents-heading h2 {
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.close-button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-size: 20px;
}
.close-button:hover,
.close-button:focus-visible {
  background: #f3f4f6;
}
.close-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}
.documents-workspace {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
}
.file-sidebar {
  position: relative;
  display: flex;
  width: 248px;
  min-width: 0;
  flex: 0 0 248px;
  flex-direction: column;
  overflow: auto;
  border-right: 1px solid #d9d9d9;
  padding: 16px;
}
.drawer-resize-handle { position: absolute; top: 0; left: 0; z-index: 3; width: 8px; touch-action: none; height: 100%; cursor: col-resize; }
.drawer-resize-handle:hover,
.drawer-resize-handle:focus-visible,
:global(.task-documents-drawer.is-resizing) .drawer-resize-handle {
  background: rgba(49, 87, 226, 0.18);
  outline: none;
}
:global(.task-documents-drawer.is-resizing),
:global(.task-documents-drawer.is-resizing *) {
  cursor: col-resize !important;
  user-select: none !important;
}
:global(.task-documents-drawer.is-resizing .ant-drawer-content-wrapper) {
  transition: none !important;
}
.sidebar-state {
  display: flex;
  min-height: 180px;
  align-items: center;
  justify-content: center;
}
.sidebar-error {
  flex-direction: column;
  gap: 12px;
  color: #8c8c8c;
  font-size: 12px;
  text-align: center;
}
.file-tree-node {
  display: flex;
  min-width: 0;
  min-height: 32px;
  align-items: center;
  gap: 6px;
}
.file-node-icon {
  flex: 0 0 auto;
  color: #8c8c8c;
  font-size: 16px;
}
.folder-icon {
  color: #262626;
}
.markdown-icon {
  color: #16a6a1;
}
.image-icon {
  color: #7c3aed;
}
.text-icon {
  color: #d97706;
}
.file-node-name {
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
  color: #262626;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-node-meta {
  flex: 0 0 auto;
  color: #8c8c8c;
  font-size: 10px;
  line-height: 16px;
}
/* 一键引用到输入框：默认隐藏，悬停行时出现，避免树里视觉噪音 */
.file-node-reference {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  margin-left: auto;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  color: #595959;
  background: #fff;
  font-size: 10px;
  opacity: 0;
  transition: opacity 120ms ease-out;
}
.file-tree-node:hover .file-node-reference {
  opacity: 1;
}
.file-node-reference:hover {
  color: #3157e2;
  border-color: #3157e2;
}
.file-workspace {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
}
.file-toolbar {
  display: flex;
  min-height: 68px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  padding: 10px 16px 10px 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}
.directory-collapse-btn {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 4px;
  background: transparent;
  cursor: pointer;
}
.directory-collapse-btn img {
  display: block;
  width: 16px;
  height: 16px;
}
.directory-collapse-btn img.is-collapsed {
  transform: rotate(180deg);
}
.directory-collapse-btn:hover,
.directory-collapse-btn:focus-visible {
  background: #f2f4f7;
}
.directory-collapse-btn:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}
.file-toolbar-actions { display: flex; gap: 8px; flex: 0 0 auto; }
.file-toolbar-copy {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 4px;
}
.save-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}
.file-toolbar-copy strong {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  overflow: hidden;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-content {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  overflow: hidden;
  padding: 16px;
}
.file-markdown-editor {
  width: 100%;
  height: 100%;
  min-height: 0;
}
.content-state {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
}
.content-error {
  flex-direction: column;
  gap: 16px;
}
.file-editor {
  height: 100%;
  min-height: 300px;
  resize: none;
  border: 0;
  padding: 0;
  color: #262626;
  font-family: 'PingFang SC', sans-serif;
  font-size: 14px;
  line-height: 26px;
  box-shadow: none;
}
.file-editor:focus {
  box-shadow: none;
}
.image-preview {
  display: flex;
  width: 100%;
  min-height: 0;
  align-items: center;
  justify-content: center;
  overflow: auto;
  background: #f8f9fb;
}
.image-preview img {
  display: block;
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
:deep(.ant-tree) {
  background: transparent;
}
:deep(.ant-tree .ant-tree-node-content-wrapper) {
  min-width: 0;
  border-radius: 6px;
  padding-inline: 6px;
}
:deep(.ant-tree .ant-tree-node-content-wrapper.ant-tree-node-selected) {
  background: #e5efff;
}
:global(.task-documents-drawer .ant-drawer-content) {
  overflow: hidden;
  border-radius: 22px 0 0 22px;
}
@media (max-width: 760px) {
  .documents-header {
    min-height: 76px;
    padding: 18px 16px;
  }
  .documents-heading h2 {
    font-size: 20px;
    line-height: 28px;
  }
  .documents-workspace {
    flex-direction: column;
  }
  .file-sidebar {
    width: auto;
    max-height: 220px;
    flex: 0 0 220px;
    border-right: 0;
    border-bottom: 1px solid #d9d9d9;
  }
}
</style>
