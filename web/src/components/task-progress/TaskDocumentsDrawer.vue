<template>
  <a-drawer
    :open="open"
    width="min(1097px, 100vw)"
    :closable="false"
    :mask-closable="false"
    :body-style="{ padding: 0, overflow: 'hidden' }"
    root-class-name="task-documents-drawer"
    @close="requestClose"
  >
    <div class="documents-drawer-shell">
      <header class="documents-header">
        <div class="documents-heading">
          <span class="step-avatar">
            <img
              v-if="currentStep?.avatar"
              :src="currentStep.avatar"
              :alt="`${currentStep.name}头像`"
            />
            <span v-else>{{ initials(currentStep?.name) }}</span>
          </span>
          <h2>{{ currentStep?.name || '当前步骤' }}</h2>
          <button
            type="button"
            class="open-directory-button"
            :disabled="!taskDirectory || openingDirectory"
            :aria-label="openingDirectory ? '正在打开文件夹' : '在文件夹中打开'"
            :title="openingDirectory ? '正在打开文件夹' : '在文件夹中打开'"
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
            aria-label="关闭产出文档"
            @click="requestClose"
          >
            <CloseOutlined />
          </button>
        </div>
      </header>

      <div class="documents-workspace">
        <aside class="file-sidebar">
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
              <ReloadOutlined />重试
            </a-button>
          </div>
          <a-empty
            v-else-if="!treeData.length"
            :image="false"
            description="暂无产出文档"
          />
          <a-tree
            v-else
            block-node
            :tree-data="treeData"
            :selected-keys="selectedKeys"
            @select="handleSelect"
          >
            <template #title="{ title, isDir, fileType, meta }">
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
              </div>
            </template>
          </a-tree>
        </aside>

        <main class="file-workspace">
          <div
            v-if="selectedFile"
            class="file-toolbar"
          >
            <div class="file-toolbar-copy">
              <span class="save-hint"><EditOutlined />修改内容保存后同步更新该产出文档</span>
              <strong>
                <FileImageOutlined v-if="selectedContent?.encoding === 'base64'" />
                <FileTextOutlined v-else />
                {{ selectedFile.name }}
              </strong>
            </div>
            <a-button
              v-if="selectedContent?.encoding !== 'base64'"
              type="primary"
              :loading="saving"
              :disabled="contentLoading || Boolean(contentError) || !dirty"
              @click="saveFile"
            >
              保存
            </a-button>
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
              <a-button @click="retrySelectedFile">重试</a-button>
            </div>
            <a-empty
              v-else-if="!selectedFile || !selectedContent"
              description="请选择产出文档"
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
            <a-textarea
              v-else
              v-model:value="draftContent"
              class="file-editor"
              :disabled="saving"
              :aria-label="`编辑文件 ${selectedFile.name}`"
            />
          </div>
        </main>
      </div>
    </div>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  CloseOutlined,
  EditOutlined,
  FileImageOutlined,
  FileMarkdownOutlined,
  FileOutlined,
  FileTextOutlined,
  FolderOutlined,
  LoadingOutlined,
  ReloadOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import openDirectoryIcon from '@/assets/icons/task-documents-open-directory.svg'
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
}>()

const treeData = ref<FileTreeNode[]>([])
const fileMap = ref(new Map<string, FileTreeNode>())
const selectedKeys = ref<string[]>([])
const selectedFile = ref<TaskFileNode>()
const selectedContent = ref<TaskFileContentResponse>()
const draftContent = ref('')
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
    treeError.value = '缺少任务信息，无法加载产出文档'
    return
  }
  const requestVersion = ++treeLoadVersion
  contentLoadVersion += 1
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
    const firstFile = findFirstFile(treeData.value)
    if (firstFile) await loadFile(firstFile)
  } catch (error) {
    if (requestVersion !== treeLoadVersion) return
    treeError.value = error instanceof Error ? error.message : '产出文档目录加载失败'
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

function findFirstFile(nodes: FileTreeNode[]): FileTreeNode | undefined {
  for (const node of nodes) {
    if (!node.isDir) return node
    const child = findFirstFile(node.children || [])
    if (child) return child
  }
  return undefined
}

function handleSelect(keys: Array<string | number>) {
  const path = String(keys[0] || '')
  const node = fileMap.value.get(path)
  if (!node || node.isDir || node.path === selectedFile.value?.path) return
  if (!dirty.value) {
    void loadFile(node)
    return
  }
  Modal.confirm({
    title: '放弃当前文件的未保存修改？',
    content: '切换文件后，当前尚未保存的内容将丢失。',
    okText: '放弃并切换',
    cancelText: '继续编辑',
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
    contentError.value = error instanceof Error ? error.message : `${node.name}读取失败`
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
  saving.value = true
  try {
    await apiClient.post(`/tasks/${encodeURIComponent(props.taskUuid)}/files/content`, {
      path: file.path,
      content: draftContent.value,
    })
    savedContent.value = draftContent.value
    message.success(`${file.name}已保存`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : `${file.name}保存失败`)
  } finally {
    saving.value = false
  }
}

function requestClose() {
  if (saving.value) return
  if (!dirty.value) {
    emit('update:open', false)
    return
  }
  Modal.confirm({
    title: '放弃未保存的文档修改？',
    content: '关闭后，当前文件尚未保存的内容将丢失。',
    okText: '放弃修改',
    cancelText: '继续编辑',
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
      message.info('浏览器环境无法直接打开文件夹，任务工作空间路径已复制')
    } catch {
      message.warning(`浏览器环境无法直接打开文件夹：${taskDirectory.value}`)
    }
    return
  }

  openingDirectory.value = true
  try {
    await openDirectory(taskDirectory.value)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '打开任务工作空间失败')
  } finally {
    openingDirectory.value = false
  }
}

function resetState() {
  treeLoadVersion += 1
  contentLoadVersion += 1
  treeData.value = []
  fileMap.value = new Map()
  treeLoading.value = false
  saving.value = false
  openingDirectory.value = false
  taskDirectory.value = ''
  treeError.value = ''
  clearSelection()
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
  width: 248px;
  min-width: 0;
  flex: 0 0 248px;
  overflow: auto;
  border-right: 1px solid #d9d9d9;
  padding: 16px;
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
.file-workspace {
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
  justify-content: space-between;
  gap: 16px;
  padding: 10px 16px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}
.file-toolbar-copy {
  display: flex;
  min-width: 0;
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
  overflow: hidden;
  padding: 16px;
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
