<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'ant-design-vue'
import {
  CheckOutlined,
  DeleteOutlined,
  EditOutlined,
  FileTextOutlined,
  FolderAddOutlined,
  FolderOutlined,
  PlusOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue'
import MarkdownIt from 'markdown-it'
import apiClient, { ApiError } from '@/api/client'
import { useAppI18n } from '@/i18n'

const { t, locale } = useAppI18n()

type FolderNode = {
  id: number
  name: string
  parent_id: number
  created_at: number
  updated_at: number
  children?: FolderNode[]
}

type KnowledgeDocument = {
  uuid: string
  folder_id: number
  title: string
  tags: string[]
  content_hash: string
  word_count: number
  created_at: number
  updated_at: number
  content?: string
}

type SearchResult = {
  uuid: string
  title: string
  snippet: string
  folder_id: number
}

type OutlineItem = {
  id: string
  level: number
  title: string
}

type DirectoryNode = {
  key: string
  title: string
  kind: 'folder' | 'document'
  entityId: number | string
  parentKey: string
  folderID: number
  count?: number
  isLeaf?: boolean
  special?: 'all' | 'uncategorized'
  children?: DirectoryNode[]
}

type DirectorySelectInfo = {
  node: DirectoryNode
}

type DirectoryDropInfo = {
  node: DirectoryNode & { pos?: string }
  dragNode: DirectoryNode
  dropToGap: boolean
  dropPosition: number
}

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
  typographer: true,
})

markdown.renderer.rules.heading_open = (tokens, index, _options, env) => {
  const headingIndex = Number(env.headingIndex || 0)
  env.headingIndex = headingIndex + 1
  const token = tokens[index]
  return `<${token.tag} id="knowledge-heading-${headingIndex}">`
}

const loading = ref(false)
const saving = ref(false)
const folders = ref<FolderNode[]>([])
const documents = ref<KnowledgeDocument[]>([])
const trashDocuments = ref<KnowledgeDocument[]>([])
const searchResults = ref<SearchResult[]>([])
const selectedFolderID = ref<number | 'all'>('all')
const selectedDocumentUUID = ref('')
const selectedTreeKey = ref('')
const searchKeyword = ref('')
const searchLoading = ref(false)
const viewMode = ref<'edit' | 'split' | 'preview'>('edit')
const rightTab = ref('outline')
const trashVisible = ref(false)
const editingTitle = ref(false)
const folderModalVisible = ref(false)
const folderParentID = ref(0)
const folderName = ref('')
const directoryOrder = ref<Record<string, string[]>>({})
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchSequence = 0

const draft = reactive({
  uuid: '',
  title: '',
  folderID: 0,
  tags: [] as string[],
  content: '',
  contentHash: '',
  wordCount: 0,
  createdAt: 0,
  updatedAt: 0,
})

const savedSnapshot = ref('')

const flatFolders = computed(() => {
  const result: Array<FolderNode & { depth: number }> = []
  const walk = (nodes: FolderNode[], depth: number) => {
    for (const node of nodes) {
      result.push({ ...node, depth })
      walk(node.children || [], depth + 1)
    }
  }
  walk(folders.value, 0)
  return result
})

const directoryDocuments = computed(() => {
  if (!searchKeyword.value.trim()) return documents.value
  const matches = new Set(searchResults.value.map((result) => result.uuid))
  return documents.value.filter((document) => matches.has(document.uuid))
})

function orderDirectoryNodes(parentKey: string, nodes: DirectoryNode[]) {
  const order = directoryOrder.value[parentKey] || []
  if (!order.length) return nodes
  const indexes = new Map(order.map((key, index) => [key, index]))
  return [...nodes].sort((left, right) => {
    const leftIndex = indexes.get(left.key)
    const rightIndex = indexes.get(right.key)
    if (leftIndex === undefined && rightIndex === undefined) return 0
    if (leftIndex === undefined) return 1
    if (rightIndex === undefined) return -1
    return leftIndex - rightIndex
  })
}

function persistDirectoryOrder() {
  localStorage.setItem('knowledge-directory-order', JSON.stringify(directoryOrder.value))
}

const directoryTreeData = computed<DirectoryNode[]>(() => {
  const documentNode = (
    document: KnowledgeDocument,
    parentKey: string,
    keyPrefix = 'doc',
  ): DirectoryNode => ({
    key: `${keyPrefix}:${document.uuid}`,
    title: document.title,
    kind: 'document',
    entityId: document.uuid,
    parentKey,
    folderID: document.folder_id,
    isLeaf: true,
  })

  const countDocuments = (folder: FolderNode): number => {
    const direct = documents.value.filter((document) => document.folder_id === folder.id).length
    return direct + (folder.children || []).reduce((sum, child) => sum + countDocuments(child), 0)
  }

  const mapFolder = (folder: FolderNode, parentKey: string): DirectoryNode | null => {
    const key = `folder:${folder.id}`
    const childFolders = (folder.children || [])
      .map((child) => mapFolder(child, key))
      .filter((child): child is DirectoryNode => Boolean(child))
    const childDocuments = directoryDocuments.value
      .filter((document) => document.folder_id === folder.id)
      .map((document) => documentNode(document, key))
    const children = orderDirectoryNodes(key, [...childFolders, ...childDocuments])
    if (searchKeyword.value.trim() && !children.length) return null
    return {
      key,
      title: folder.name,
      kind: 'folder',
      entityId: folder.id,
      parentKey,
      folderID: folder.id,
      count: countDocuments(folder),
      children,
    }
  }

  const allDocuments = orderDirectoryNodes(
    'special:all',
    directoryDocuments.value.map((document) =>
      documentNode(document, 'special:all', 'all-doc'),
    ),
  )
  const uncategorizedDocuments = orderDirectoryNodes(
    'special:uncategorized',
    directoryDocuments.value
      .filter((document) => document.folder_id === 0)
      .map((document) => documentNode(document, 'special:uncategorized')),
  )
  const allNode: DirectoryNode = {
    key: 'special:all',
    title: t('knowledge.allDocuments'),
    kind: 'folder',
    entityId: 'all',
    parentKey: 'root',
    folderID: 0,
    special: 'all',
    count: documents.value.length,
    children: allDocuments,
  }
  const defaultFolderNode: DirectoryNode = {
    key: 'special:uncategorized',
    title: t('knowledge.defaultFolder'),
    kind: 'folder',
    entityId: 0,
    parentKey: 'root',
    folderID: 0,
    special: 'uncategorized',
    count: documents.value.filter((document) => document.folder_id === 0).length,
    children: uncategorizedDocuments,
  }
  const sortableRootNodes = orderDirectoryNodes('root', [
    defaultFolderNode,
    ...folders.value
      .map((folder) => mapFolder(folder, 'root'))
      .filter((folder): folder is DirectoryNode => Boolean(folder)),
  ])

  return [allNode, ...sortableRootNodes]
})

const snapshot = computed(() =>
  JSON.stringify({
    title: draft.title,
    folderID: draft.folderID,
    tags: [...draft.tags].sort(),
    content: draft.content,
  }),
)

const dirty = computed(() => Boolean(draft.uuid) && snapshot.value !== savedSnapshot.value)

const renderedMarkdown = computed(() =>
  markdown.render(draft.content || '', { headingIndex: 0 }),
)

const outline = computed<OutlineItem[]>(() => {
  const items: OutlineItem[] = []
  for (const line of draft.content.split(/\r?\n/)) {
    const match = /^(#{1,6})\s+(.+?)\s*#*$/.exec(line)
    if (!match) continue
    items.push({
      id: `knowledge-heading-${items.length}`,
      level: match[1].length,
      title: match[2].trim(),
    })
  }
  return items
})

function currentSnapshot() {
  savedSnapshot.value = snapshot.value
}

function clearDraft() {
  Object.assign(draft, {
    uuid: '',
    title: '',
    folderID: 0,
    tags: [],
    content: '',
    contentHash: '',
    wordCount: 0,
    createdAt: 0,
    updatedAt: 0,
  })
  savedSnapshot.value = ''
  selectedDocumentUUID.value = ''
  selectedTreeKey.value = ''
  editingTitle.value = false
}

function formatTime(value: number) {
  if (!value) return '—'
  return new Date(value).toLocaleString(locale.value, { hour12: false })
}

async function loadFolders() {
  const result = await apiClient.get<{ data: FolderNode[] }>('/knowledge/folders')
  folders.value = result.data || []
}

async function loadDocuments() {
  const result = await apiClient.get<{ data: KnowledgeDocument[] }>('/knowledge/documents')
  documents.value = result.data || []
}

async function loadTrash() {
  const result = await apiClient.get<{ data: KnowledgeDocument[] }>('/knowledge/trash')
  trashDocuments.value = result.data || []
}

async function refreshTrash() {
  try {
    await loadTrash()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.trashLoadFailed'))
  }
}

async function loadWorkspace(selectFirst = false) {
  loading.value = true
  try {
    await Promise.all([loadFolders(), loadDocuments(), loadTrash()])
    if (selectFirst && !draft.uuid && documents.value.length) {
      await openDocument(documents.value[0].uuid, true)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.loadFailed'))
  } finally {
    loading.value = false
  }
}

function confirmDiscardChanges() {
  if (!dirty.value) return Promise.resolve(true)
  return new Promise<boolean>((resolve) => {
    Modal.confirm({
      title: t('knowledge.unsavedTitle'),
      content: t('knowledge.unsavedDescription'),
      okText: t('knowledge.discard'),
      okType: 'danger',
      cancelText: t('knowledge.continueEditing'),
      onOk: () => resolve(true),
      onCancel: () => resolve(false),
    })
  })
}

async function openDocument(uuid: string, force = false, treeKey = '') {
  if (!force && uuid !== draft.uuid && !(await confirmDiscardChanges())) return
  try {
    const document = await apiClient.get<KnowledgeDocument>(
      `/knowledge/documents/${uuid}`,
    )
    Object.assign(draft, {
      uuid: document.uuid,
      title: document.title,
      folderID: document.folder_id,
      tags: [...(document.tags || [])],
      content: document.content || '',
      contentHash: document.content_hash,
      wordCount: document.word_count,
      createdAt: document.created_at,
      updatedAt: document.updated_at,
    })
    selectedDocumentUUID.value = uuid
    selectedTreeKey.value = treeKey || `doc:${uuid}`
    trashVisible.value = false
    currentSnapshot()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.documentLoadFailed'))
  }
}

async function createDocument() {
  if (!(await confirmDiscardChanges())) return
  try {
    const created = await apiClient.post<KnowledgeDocument>('/knowledge/documents', {
      title: t('knowledge.untitled'),
      folder_id: selectedFolderID.value === 'all' ? 0 : selectedFolderID.value,
      content: t('knowledge.newContent'),
      tags: [],
    })
    await loadDocuments()
    await openDocument(created.uuid, true)
    viewMode.value = 'edit'
    message.success(t('knowledge.documentCreated'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.documentCreateFailed'))
  }
}

function updateDocumentInList(document: KnowledgeDocument) {
  const index = documents.value.findIndex((item) => item.uuid === document.uuid)
  if (index >= 0) {
    documents.value[index] = { ...documents.value[index], ...document, content: undefined }
  } else {
    documents.value.unshift({ ...document, content: undefined })
  }
}

async function saveAsCopy() {
  try {
    const created = await apiClient.post<KnowledgeDocument>('/knowledge/documents', {
      title: t('knowledge.conflictCopy', { title: draft.title.trim() || t('knowledge.untitled') }),
      folder_id: draft.folderID,
      content: draft.content,
      tags: draft.tags,
    })
    await loadDocuments()
    await openDocument(created.uuid, true)
    message.success(t('knowledge.savedAsCopy'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.saveAsFailed'))
  }
}

function handleSaveConflict() {
  Modal.confirm({
    title: t('knowledge.conflictTitle'),
    content: t('knowledge.conflictDescription'),
    okText: t('knowledge.reload'),
    cancelText: t('knowledge.saveAsCopy'),
    async onOk() {
      await openDocument(draft.uuid, true)
    },
    async onCancel() {
      await saveAsCopy()
    },
  })
}

async function saveDocument() {
  if (!draft.uuid || saving.value) return
  if (!draft.title.trim()) {
    message.error(t('knowledge.titleRequired'))
    return
  }
  saving.value = true
  try {
    const updated = await apiClient.put<KnowledgeDocument>(
      `/knowledge/documents/${draft.uuid}`,
      {
        title: draft.title.trim(),
        folder_id: draft.folderID,
        content: draft.content,
        tags: draft.tags.map((tag) => tag.trim()).filter(Boolean),
        base_hash: draft.contentHash,
      },
    )
    Object.assign(draft, {
      title: updated.title,
      folderID: updated.folder_id,
      tags: [...updated.tags],
      content: updated.content || '',
      contentHash: updated.content_hash,
      wordCount: updated.word_count,
      updatedAt: updated.updated_at,
    })
    currentSnapshot()
    updateDocumentInList(updated)
    message.success(t('knowledge.documentSaved'))
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      handleSaveConflict()
    } else {
      message.error(error instanceof Error ? error.message : t('knowledge.saveFailed'))
    }
  } finally {
    saving.value = false
  }
}

function deleteDocument() {
  if (!draft.uuid) return
  Modal.confirm({
    title: t('knowledge.deleteTitle', { title: draft.title }),
    content: t('knowledge.deleteDescription'),
    okText: t('knowledge.moveToTrash'),
    okType: 'danger',
    cancelText: t('knowledge.cancel'),
    async onOk() {
      try {
        await apiClient.delete(`/knowledge/documents/${draft.uuid}`)
        clearDraft()
        await Promise.all([loadDocuments(), loadTrash()])
        message.success(t('knowledge.movedToTrash'))
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('knowledge.deleteFailed'))
      }
    },
  })
}

async function openTrash() {
  if (!(await confirmDiscardChanges())) return
  clearDraft()
  trashVisible.value = true
  await refreshTrash()
}

async function restoreDocument(document: KnowledgeDocument) {
  try {
    await apiClient.post(`/knowledge/documents/${document.uuid}/restore`)
    await Promise.all([loadTrash(), loadDocuments()])
    message.success(t('knowledge.restored'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.restoreFailed'))
  }
}

function hardDeleteDocument(document: KnowledgeDocument) {
  Modal.confirm({
    title: t('knowledge.hardDeleteTitle', { title: document.title }),
    content: t('knowledge.hardDeleteDescription'),
    okText: t('knowledge.deletePermanently'),
    okType: 'danger',
    cancelText: t('knowledge.cancel'),
    async onOk() {
      try {
        await apiClient.delete(`/knowledge/documents/${document.uuid}/permanent`)
        await loadTrash()
        message.success(t('knowledge.hardDeleted'))
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('knowledge.hardDeleteFailed'))
      }
    },
  })
}

async function selectDirectory(keys: Array<string | number>, info: DirectorySelectInfo) {
  if (!keys.length) return
  const node = info.node
  if (node.kind === 'document') {
    selectedFolderID.value = node.folderID
    await openDocument(String(node.entityId), false, node.key)
    return
  }
  if (!(await confirmDiscardChanges())) return
  clearDraft()
  selectedTreeKey.value = node.key
  selectedFolderID.value = node.special === 'all' ? 'all' : node.folderID
  trashVisible.value = false
}

function canDragDirectoryNode(node: DirectoryNode) {
  return node.special !== 'all'
}

async function moveDirectoryNode(info: DirectoryDropInfo) {
  const dragged = info.dragNode
  const target = info.node
  if (dragged.special === 'all') {
    message.info(t('knowledge.allFixed'))
    return
  }
  if (dragged.special === 'uncategorized' && !info.dropToGap) {
    message.info(t('knowledge.defaultTopLevel'))
    return
  }
  if (target.special === 'all') {
    message.info(t('knowledge.allAggregate'))
    return
  }
  if (!info.dropToGap && target.kind !== 'folder') return

  const findTreeNode = (nodes: DirectoryNode[], key: string): DirectoryNode | undefined => {
    for (const node of nodes) {
      if (node.key === key) return node
      const found = findTreeNode(node.children || [], key)
      if (found) return found
    }
    return undefined
  }
  const childrenOf = (parentKey: string) => {
    if (parentKey === 'root') {
      return directoryTreeData.value.filter((node) => node.special !== 'all')
    }
    return findTreeNode(directoryTreeData.value, parentKey)?.children || []
  }
  const findFolder = (nodes: FolderNode[], id: number): FolderNode | undefined => {
    for (const folder of nodes) {
      if (folder.id === id) return folder
      const found = findFolder(folder.children || [], id)
      if (found) return found
    }
    return undefined
  }

  const dropDocumentIntoFolder = dragged.kind === 'document' && target.kind === 'folder'
  let targetParentKey: string
  if (dropDocumentIntoFolder) {
    targetParentKey = target.special === 'uncategorized' ? 'special:uncategorized' : target.key
  } else if (info.dropToGap) {
    targetParentKey = target.parentKey
  } else if (target.special === 'uncategorized') {
    targetParentKey = dragged.kind === 'folder' ? 'root' : 'special:uncategorized'
  } else {
    targetParentKey = target.key
  }

  if (dragged.kind === 'document' && targetParentKey === 'special:all') {
    message.info(t('knowledge.allAggregate'))
    return
  }

  if (dragged.kind === 'document' && targetParentKey === 'root') {
    message.info(t('knowledge.moveDocumentHint'))
    return
  }

  const targetFolderID = targetParentKey.startsWith('folder:')
    ? Number(targetParentKey.slice('folder:'.length))
    : 0

  if (dragged.kind === 'folder' && Number(dragged.entityId) === targetFolderID) return

  if (dragged.kind === 'folder' && targetFolderID > 0) {
    const draggedFolderID = Number(dragged.entityId)
    const containsFolder = (folder: FolderNode, id: number): boolean =>
      (folder.children || []).some(
        (child) => child.id === id || containsFolder(child, id),
      )
    const draggedFolder = findFolder(folders.value, draggedFolderID)
    if (draggedFolder && containsFolder(draggedFolder, targetFolderID)) {
      message.error(t('knowledge.folderCycle'))
      return
    }
  }

  const sourceFolderID =
    dragged.kind === 'folder'
      ? findFolder(folders.value, Number(dragged.entityId))?.parent_id || 0
      : dragged.folderID
  const draggedOrderKey =
    dragged.kind === 'document' ? `doc:${dragged.entityId}` : dragged.key
  const targetSiblingKeys = childrenOf(targetParentKey)
    .map((node) => node.key)
    .filter((key) => key !== dragged.key && key !== draggedOrderKey)
  if (info.dropToGap && !dropDocumentIntoFolder) {
    const targetIndex = targetSiblingKeys.indexOf(target.key)
    const positionIndex = Number(target.pos?.split('-').at(-1) || 0)
    const insertAfter = info.dropPosition - positionIndex > 0
    targetSiblingKeys.splice(
      Math.max(0, targetIndex + (insertAfter ? 1 : 0)),
      0,
      draggedOrderKey,
    )
  } else {
    targetSiblingKeys.push(draggedOrderKey)
  }

  try {
    if (dragged.kind === 'folder' && sourceFolderID !== targetFolderID) {
      await apiClient.put(`/knowledge/folders/${dragged.entityId}`, {
        parent_id: targetFolderID,
      })
      await loadFolders()
    } else if (dragged.kind === 'document' && sourceFolderID !== targetFolderID) {
      const updated = await apiClient.put<KnowledgeDocument>(
        `/knowledge/documents/${dragged.entityId}`,
        { folder_id: targetFolderID },
      )
      updateDocumentInList(updated)
      if (draft.uuid === dragged.entityId) {
        draft.folderID = targetFolderID
        selectedTreeKey.value = draggedOrderKey
        currentSnapshot()
      }
    }

    const nextDirectoryOrder = {
      ...directoryOrder.value,
      [targetParentKey]: targetSiblingKeys,
    }
    if (dragged.parentKey !== 'special:all' && dragged.parentKey !== targetParentKey) {
      nextDirectoryOrder[dragged.parentKey] = (
        directoryOrder.value[dragged.parentKey] ||
        childrenOf(dragged.parentKey).map((node) => node.key)
      ).filter((key) => key !== dragged.key && key !== draggedOrderKey)
    }
    directoryOrder.value = nextDirectoryOrder
    persistDirectoryOrder()
    selectedFolderID.value = targetFolderID
    if (dragged.kind === 'document') {
      message.success(sourceFolderID === targetFolderID ? t('knowledge.documentOrderUpdated') : t('knowledge.documentMoved'))
    } else {
      message.success(sourceFolderID === targetFolderID ? t('knowledge.folderOrderUpdated') : t('knowledge.folderMoved'))
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.moveFailed'))
  }
}

function openFolderModal() {
  folderName.value = ''
  folderParentID.value = typeof selectedFolderID.value === 'number' ? selectedFolderID.value : 0
  folderModalVisible.value = true
}

async function createFolder() {
  const name = folderName.value.trim()
  if (!name) return
  try {
    await apiClient.post('/knowledge/folders', {
      name,
      parent_id: folderParentID.value,
    })
    folderModalVisible.value = false
    await loadFolders()
    message.success(t('knowledge.folderCreated'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.folderCreateFailed'))
  }
}

async function renameSelectedFolder() {
  if (typeof selectedFolderID.value !== 'number' || selectedFolderID.value === 0) return
  const folder = flatFolders.value.find((item) => item.id === selectedFolderID.value)
  if (!folder) return
  const name = window.prompt(t('knowledge.folderName'), folder.name)?.trim()
  if (!name) return
  try {
    await apiClient.put(`/knowledge/folders/${folder.id}`, { name })
    await loadFolders()
    message.success(t('knowledge.folderRenamed'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.renameFailed'))
  }
}

function deleteSelectedFolder() {
  if (typeof selectedFolderID.value !== 'number' || selectedFolderID.value === 0) return
  const folder = flatFolders.value.find((item) => item.id === selectedFolderID.value)
  if (!folder) return
  Modal.confirm({
    title: t('knowledge.deleteFolderTitle', { name: folder.name }),
    content: t('knowledge.deleteFolderDescription'),
    okText: t('knowledge.deleteFolder'),
    okType: 'danger',
    cancelText: t('knowledge.cancel'),
    async onOk() {
      try {
        await apiClient.delete(`/knowledge/folders/${folder.id}`)
        selectedFolderID.value = 'all'
        await Promise.all([loadFolders(), loadDocuments()])
        message.success(t('knowledge.folderDeleted'))
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('knowledge.folderDeleteFailed'))
      }
    },
  })
}

async function performSearch() {
  const keyword = searchKeyword.value.trim()
  const sequence = ++searchSequence
  if (!keyword) {
    searchResults.value = []
    searchLoading.value = false
    return
  }
  searchLoading.value = true
  try {
    const result = await apiClient.get<{ items: SearchResult[]; total: number }>(
      '/knowledge/search',
      { q: keyword },
    )
    if (sequence === searchSequence) searchResults.value = result.items || []
  } catch (error) {
    if (sequence === searchSequence) {
      message.error(error instanceof Error ? error.message : t('knowledge.searchFailed'))
    }
  } finally {
    if (sequence === searchSequence) searchLoading.value = false
  }
}

function scheduleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => void performSearch(), 260)
}

function scrollToHeading(item: OutlineItem) {
  nextTick(() => {
    document.getElementById(item.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

function handleKeyboard(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    void saveDocument()
  }
}

onMounted(async () => {
  window.addEventListener('keydown', handleKeyboard)
  try {
    directoryOrder.value = JSON.parse(
      localStorage.getItem('knowledge-directory-order') || '{}',
    )
  } catch {
    directoryOrder.value = {}
  }
  await loadWorkspace(false)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeyboard)
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<template>
  <div class="knowledge-shell">
    <header class="knowledge-titlebar">{{ t('knowledge.title') }}</header>

    <div class="knowledge-page" :class="{ 'is-loading': loading }">
      <aside class="knowledge-sidebar">
        <div class="directory-heading">
          <div class="directory-title">
            <strong>{{ t('knowledge.catalog') }}</strong>
            <span class="directory-divider" />
            <span>{{ t('knowledge.dragHint') }}</span>
          </div>
          <a-dropdown :trigger="['click']">
            <button class="icon-button add-button" type="button" :aria-label="t('knowledge.new')">
              <PlusOutlined />
            </button>
            <template #overlay>
              <a-menu>
                <a-menu-item key="document" @click="createDocument">
                  <FileTextOutlined /> {{ t('knowledge.newDocument') }}
                </a-menu-item>
                <a-menu-item key="folder" @click="openFolderModal">
                  <FolderAddOutlined /> {{ t('knowledge.newFolder') }}
                </a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
        </div>

        <div class="sidebar-search">
          <a-input
            v-model:value="searchKeyword"
            allow-clear
            :placeholder="t('knowledge.search')"
            @input="scheduleSearch"
            @press-enter="performSearch"
          >
            <template #suffix><SearchOutlined /></template>
          </a-input>
        </div>

        <div class="directory-tree-area">
          <a-spin :spinning="searchLoading" class="directory-spin">
            <div class="directory-scroll">
              <a-tree
                block-node
                :draggable="canDragDirectoryNode"
                :tree-data="directoryTreeData"
                :selected-keys="trashVisible || !selectedTreeKey ? [] : [selectedTreeKey]"
                @select="selectDirectory"
                @drop="moveDirectoryNode"
              >
                <template #title="{ title, key, kind, entityId, count }">
                  <div class="directory-node" :class="`node-${kind}`">
                    <FolderOutlined v-if="kind === 'folder'" class="node-icon" />
                    <FileTextOutlined v-else class="node-icon document-icon" />
                    <span class="node-title">{{ title }}</span>
                    <span v-if="kind === 'folder'" class="node-count">{{ count }}</span>
                    <span
                      v-if="kind === 'folder' && String(key).startsWith('folder:')"
                      class="node-actions"
                    >
                      <a-tooltip :title="t('knowledge.rename')">
                        <button
                          type="button"
                          :aria-label="t('knowledge.renameFolder')"
                          @click.stop="selectedFolderID = Number(entityId); renameSelectedFolder()"
                        >
                          <EditOutlined />
                        </button>
                      </a-tooltip>
                      <a-tooltip :title="t('knowledge.deleteFolder')">
                        <button
                          type="button"
                          :aria-label="t('knowledge.deleteFolder')"
                          @click.stop="selectedFolderID = Number(entityId); deleteSelectedFolder()"
                        >
                          <DeleteOutlined />
                        </button>
                      </a-tooltip>
                    </span>
                  </div>
                </template>
              </a-tree>
              <div v-if="searchKeyword && !directoryDocuments.length" class="directory-empty">
                {{ t('knowledge.noMatches') }}
              </div>
            </div>
          </a-spin>
        </div>

        <button
          class="trash-entry"
          :class="{ active: trashVisible }"
          type="button"
          @click="openTrash"
        >
          <DeleteOutlined />
          <span>{{ t('knowledge.trash') }}</span>
          <span class="trash-count">{{ trashDocuments.length }}</span>
        </button>
      </aside>

      <main v-if="trashVisible" class="trash-workspace">
        <div class="trash-header">
          <h2><DeleteOutlined /> {{ t('knowledge.trash') }}</h2>
          <a-tooltip :title="t('common.actions.refresh')">
            <button class="icon-button" type="button" :aria-label="t('knowledge.refreshTrash')" @click="refreshTrash">
              <ReloadOutlined />
            </button>
          </a-tooltip>
        </div>
        <div v-if="trashDocuments.length" class="trash-list">
          <div v-for="document in trashDocuments" :key="document.uuid" class="trash-card">
            <div class="trash-document-icon"><FileTextOutlined /></div>
            <div class="trash-card-main">
              <div class="trash-card-title">{{ document.title }}</div>
              <div class="trash-card-meta">{{ t('knowledge.deletedAt', { time: formatTime(document.updated_at) }) }}</div>
            </div>
            <div class="trash-card-actions">
              <a-tooltip :title="t('knowledge.restore')">
                <button
                  class="icon-button"
                  type="button"
                  :aria-label="t('knowledge.restore')"
                  @click="restoreDocument(document)"
                >
                  <RollbackOutlined />
                </button>
              </a-tooltip>
              <a-tooltip :title="t('knowledge.deletePermanently')">
                <button
                  class="icon-button danger-button"
                  type="button"
                  :aria-label="t('knowledge.deletePermanently')"
                  @click="hardDeleteDocument(document)"
                >
                  <DeleteOutlined />
                </button>
              </a-tooltip>
            </div>
          </div>
        </div>
        <div v-else class="trash-empty">{{ t('knowledge.trashEmpty') }}</div>
      </main>

      <main v-else-if="draft.uuid" class="editor-workspace">
        <header class="editor-header">
          <div class="editor-title-row">
            <div class="editor-document-title">
              <FileTextOutlined />
              <a-input
                v-if="editingTitle"
                v-model:value="draft.title"
                class="title-input"
                :placeholder="t('knowledge.documentTitle')"
                autofocus
                @press-enter="editingTitle = false"
                @blur="editingTitle = false"
              />
              <h2 v-else>{{ draft.title }}</h2>
            </div>
            <div class="editor-actions">
              <a-tooltip :title="t('knowledge.rename')">
                <button class="icon-button" type="button" :aria-label="t('knowledge.rename')" @click="editingTitle = true">
                  <EditOutlined />
                </button>
              </a-tooltip>
              <a-tooltip :title="t('knowledge.moveToTrash')">
                <button class="icon-button" type="button" :aria-label="t('knowledge.moveToTrash')" @click="deleteDocument">
                  <DeleteOutlined />
                </button>
              </a-tooltip>
              <a-tooltip :title="t('common.actions.save')">
                <button
                  class="icon-button save-button"
                  type="button"
                  :aria-label="t('knowledge.saveDocument')"
                  :disabled="saving"
                  @click="saveDocument"
                >
                  <CheckOutlined />
                </button>
              </a-tooltip>
            </div>
          </div>
          <div class="editor-meta-row">
            <div class="save-state" :class="{ dirty }">
              {{ dirty ? t('knowledge.unsaved') : t('knowledge.saved') }}
            </div>
            <span class="meta-divider" />
            <span class="markdown-support">{{ t('knowledge.markdown') }}</span>
            <a-segmented
              v-model:value="viewMode"
              class="view-switcher"
              :options="[
                { value: 'edit', label: t('knowledge.edit') },
                { value: 'split', label: t('knowledge.split') },
                { value: 'preview', label: t('knowledge.preview') },
              ]"
            />
          </div>
        </header>

        <section class="editor-body" :class="`mode-${viewMode}`">
          <div v-if="viewMode !== 'preview'" class="source-pane">
            <a-textarea
              v-model:value="draft.content"
              class="markdown-input"
              :placeholder="t('knowledge.editorPlaceholder')"
              :spellcheck="false"
            />
          </div>
          <div v-if="viewMode !== 'edit'" class="preview-pane">
            <article class="markdown-preview" v-html="renderedMarkdown"></article>
          </div>
        </section>
      </main>

      <main v-else class="knowledge-empty">
        <div class="empty-state">
          <div class="empty-illustration" aria-hidden="true">
            <FolderOutlined class="empty-folder" />
            <FileTextOutlined class="empty-document" />
            <span class="empty-check"><CheckOutlined /></span>
          </div>
          <h2>{{ t('knowledge.emptyTitle') }}</h2>
          <p>{{ t('knowledge.emptyDescription') }}</p>
        </div>
      </main>

      <aside v-if="!trashVisible && draft.uuid" class="knowledge-inspector">
        <a-segmented
          v-model:value="rightTab"
          class="inspector-switcher"
          :options="[
            { value: 'outline', label: t('knowledge.outline') },
            { value: 'properties', label: t('knowledge.properties') },
          ]"
        />

        <div v-if="rightTab === 'outline'" class="inspector-content">
          <div v-if="outline.length" class="outline-list">
            <button
              v-for="item in outline"
              :key="item.id"
              class="outline-item"
              :style="{ paddingLeft: `${12 + (item.level - 1) * 12}px` }"
              @click="scrollToHeading(item)"
            >
              {{ item.title }}
            </button>
          </div>
          <div v-else class="inspector-empty">{{ t('knowledge.noOutline') }}</div>
        </div>

        <div v-else class="inspector-content">
          <div class="property-editor">
            <label>{{ t('knowledge.folder') }}</label>
            <a-select v-model:value="draft.folderID" class="folder-select">
              <a-select-option :value="0">{{ t('knowledge.defaultFolder') }}</a-select-option>
              <a-select-option v-for="folder in flatFolders" :key="folder.id" :value="folder.id">
                {{ `${'　'.repeat(folder.depth)}${folder.name}` }}
              </a-select-option>
            </a-select>
          </div>
          <dl class="property-list">
            <div><dt>{{ t('knowledge.wordCount') }}</dt><dd>{{ draft.wordCount }}</dd></div>
            <div><dt>{{ t('knowledge.createdAt') }}</dt><dd>{{ formatTime(draft.createdAt) }}</dd></div>
            <div><dt>{{ t('knowledge.updatedAt') }}</dt><dd>{{ formatTime(draft.updatedAt) }}</dd></div>
          </dl>
        </div>
      </aside>

    <a-modal
      v-model:open="folderModalVisible"
      :title="t('knowledge.newFolder')"
      :ok-text="t('knowledge.create')"
      :cancel-text="t('knowledge.cancel')"
      :ok-button-props="{ disabled: !folderName.trim() }"
      @ok="createFolder"
    >
      <a-form layout="vertical">
        <a-form-item :label="t('knowledge.folderName')">
          <a-input v-model:value="folderName" autofocus :placeholder="t('knowledge.folderExample')" @press-enter="createFolder" />
        </a-form-item>
        <a-form-item :label="t('knowledge.parentFolder')">
          <a-select v-model:value="folderParentID" class="modal-select">
            <a-select-option :value="0">{{ t('knowledge.root') }}</a-select-option>
            <a-select-option v-for="folder in flatFolders" :key="folder.id" :value="folder.id">
              {{ `${'　'.repeat(folder.depth)}${folder.name}` }}
            </a-select-option>
          </a-select>
        </a-form-item>
      </a-form>
    </a-modal>

    </div>
  </div>
</template>

<style scoped>
.knowledge-shell {
  height: 100vh;
  overflow: hidden;
  color: #202124;
  background: #fff;
}

.knowledge-titlebar {
  display: flex;
  height: 44px;
  align-items: center;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.knowledge-page {
  display: grid;
  grid-template-columns: 360px minmax(480px, 1fr) 300px;
  height: calc(100vh - 44px);
  min-height: 0;
  overflow: hidden;
  background: #fff;
}

.knowledge-sidebar {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  padding: 0 24px 20px;
  border-right: 1px solid #e6e8eb;
  background: #fff;
}

.directory-heading {
  display: flex;
  height: 72px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
}

.directory-title {
  display: flex;
  align-items: center;
  gap: 14px;
  color: #a2a5ab;
  font-size: 14px;
  white-space: nowrap;
}

.directory-title strong {
  color: #25272b;
  font-size: 16px;
  font-weight: 650;
}

.directory-divider,
.meta-divider {
  width: 1px;
  height: 18px;
  background: #e2e4e8;
}

.icon-button {
  display: inline-flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 7px;
  color: #5f6368;
  background: transparent;
  cursor: pointer;
  font-size: 16px;
  transition: background 0.16s ease, color 0.16s ease;
}

.icon-button:hover {
  color: #3157e2;
  background: #f1f4fa;
}

.icon-button:disabled {
  cursor: default;
  opacity: 0.55;
}

.add-button {
  width: 24px;
  height: 24px;
  color: #2f6bff;
  font-size: 16px;
}

.sidebar-search {
  flex: 0 0 auto;
  padding-bottom: 14px;
}

.sidebar-search :deep(.ant-input-affix-wrapper) {
  height: 32px;
  padding: 4px 11px;
  border-color: #dadde2;
  border-radius: 6px;
  box-shadow: none;
  font-size: 14px;
}

.sidebar-search :deep(.ant-input-suffix) {
  color: #555a61;
  font-size: 16px;
}

.directory-tree-area {
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.directory-tree-area :deep(.ant-spin-nested-loading),
.directory-tree-area :deep(.ant-spin-container) {
  display: block;
  width: 100%;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

.directory-scroll {
  min-height: 0;
  height: 100%;
  max-height: 100%;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0 0 16px;
  scrollbar-gutter: stable;
  scrollbar-width: thin;
  scrollbar-color: #c9cdd4 transparent;
}

.directory-scroll::-webkit-scrollbar {
  width: 6px;
}

.directory-scroll::-webkit-scrollbar-thumb {
  border-radius: 6px;
  background: #c9cdd4;
}

.directory-scroll :deep(.ant-tree) {
  color: #292b2f;
  background: transparent;
  font-size: 14px;
}

.directory-scroll :deep(.ant-tree-treenode) {
  width: 100%;
  min-height: 32px;
  align-items: center;
  padding: 0 0 2px;
}

.directory-scroll :deep(.ant-tree-switcher) {
  display: flex;
  width: 22px;
  height: 32px;
  align-items: center;
  justify-content: center;
  color: #303236;
}

.directory-scroll :deep(.ant-tree-node-content-wrapper) {
  min-width: 0;
  height: 32px;
  flex: 1;
  padding: 0 10px 0 4px;
  border-radius: 8px;
  line-height: 32px;
}

.directory-scroll :deep(.ant-tree-node-content-wrapper:hover) {
  background: #f3f6fb;
}

.directory-scroll :deep(.ant-tree-node-content-wrapper.ant-tree-node-selected) {
  background: #dfeaff;
}

.directory-scroll :deep(.ant-tree-treenode.drag-over .ant-tree-node-content-wrapper),
.directory-scroll :deep(.ant-tree-treenode.drag-over-gap-top .ant-tree-node-content-wrapper),
.directory-scroll :deep(.ant-tree-treenode.drag-over-gap-bottom .ant-tree-node-content-wrapper) {
  outline: 1px solid #7aa3ff;
  color: #2458d3;
  background: #e9f1ff;
}

.directory-node {
  display: flex;
  min-width: 0;
  height: 32px;
  align-items: center;
  gap: 8px;
}

.node-icon {
  flex: 0 0 auto;
  color: #272a2f;
  font-size: 16px;
}

.document-icon {
  color: #6b7078;
  font-size: 15px;
}

.node-title {
  overflow: hidden;
  flex: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-count,
.trash-count {
  flex: 0 0 auto;
  color: #9ca0a7;
  font-size: 11px;
}

.node-actions {
  display: none;
  flex: 0 0 auto;
  align-items: center;
}

.directory-node:hover .node-actions {
  display: flex;
}

.directory-node:hover .node-count {
  display: none;
}

.node-actions button {
  display: inline-flex;
  width: 25px;
  height: 25px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  color: #797e87;
  background: transparent;
  cursor: pointer;
}

.node-actions button:hover {
  color: #3157e2;
}

.directory-empty {
  padding: 24px 12px;
  color: #999da4;
  font-size: 14px;
  text-align: center;
}

.trash-entry {
  display: grid;
  height: 32px;
  flex: 0 0 auto;
  grid-template-columns: 20px 1fr auto;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border: 0;
  border-radius: 8px;
  color: #33363b;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  text-align: left;
}

.trash-entry:hover,
.trash-entry.active {
  background: #dfeaff;
}

.trash-entry.active .trash-count {
  color: #2f6bff;
}

.editor-workspace,
.trash-workspace,
.knowledge-empty {
  min-width: 0;
  min-height: 0;
}

.editor-workspace {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.editor-header {
  flex: 0 0 auto;
  padding: 28px 30px 18px;
}

.editor-title-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.editor-document-title {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 12px;
}

.editor-document-title > .anticon {
  color: #4c5159;
  font-size: 16px;
}

.editor-document-title h2 {
  overflow: hidden;
  margin: 0;
  color: #292b2f;
  font-size: 18px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title-input {
  max-width: 520px;
  border: 0;
  border-bottom: 1px solid #8aa7ff;
  border-radius: 0;
  box-shadow: none !important;
  font-size: 18px;
  font-weight: 650;
}

.editor-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.save-button {
  color: #fff;
  background: #3157e2;
}

.save-button:hover {
  color: #fff;
  background: #2448c8;
}

.editor-meta-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 28px;
  color: #95999f;
  font-size: 13px;
}

.save-state {
  color: #8e939b;
}

.save-state.dirty {
  color: #ff6b4a;
}

.markdown-support {
  white-space: nowrap;
}

.view-switcher {
  margin-left: auto;
}

.view-switcher :deep(.ant-segmented) {
  background: #f4f5f7;
}

.view-switcher :deep(.ant-segmented-item-selected) {
  color: #2f6bff;
}

.editor-body {
  display: grid;
  min-height: 0;
  flex: 1;
}

.editor-body.mode-split {
  grid-template-columns: 1fr 1fr;
  border-top: 1px solid #eceef1;
}

.editor-body.mode-edit,
.editor-body.mode-preview {
  grid-template-columns: 1fr;
}

.source-pane,
.preview-pane {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.source-pane {
  border-right: 1px solid #e6e8eb;
}

.markdown-input {
  min-height: 0;
  flex: 1;
  height: 100% !important;
  padding: 22px 30px 56px;
  resize: none;
  border: 0;
  border-radius: 0;
  outline: none;
  box-shadow: none !important;
  color: #4b4f55;
  font-family: ui-monospace, "SFMono-Regular", Consolas, monospace;
  font-size: 14px;
  line-height: 1.8;
}

.markdown-preview {
  overflow-wrap: anywhere;
  color: #555a61;
  font-size: 14px;
  line-height: 1.8;
}

.preview-pane > .markdown-preview {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 22px 30px 56px;
}

.markdown-preview :deep(h1),
.markdown-preview :deep(h2),
.markdown-preview :deep(h3),
.markdown-preview :deep(h4) {
  scroll-margin-top: 20px;
  color: #1f2430;
  font-weight: 650;
  line-height: 1.35;
}

.markdown-preview :deep(h1) {
  font-size: 2em;
}

.markdown-preview :deep(h2) {
  font-size: 1.5em;
}

.markdown-preview :deep(h3) {
  font-size: 1.24em;
}

.markdown-preview :deep(p),
.markdown-preview :deep(ul),
.markdown-preview :deep(ol),
.markdown-preview :deep(blockquote),
.markdown-preview :deep(pre),
.markdown-preview :deep(table) {
  margin: 0.8em 0;
}

.markdown-preview :deep(code) {
  padding: 0.15em 0.35em;
  border-radius: 4px;
  color: #c7254e;
  background: #f6f7f9;
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 0.88em;
}

.markdown-preview :deep(pre) {
  overflow-x: auto;
  padding: 14px 16px;
  border-radius: 7px;
  background: #1f2430;
}

.markdown-preview :deep(pre code) {
  padding: 0;
  color: #e8eaf0;
  background: transparent;
}

.markdown-preview :deep(blockquote) {
  margin-left: 0;
  padding-left: 14px;
  border-left: 3px solid #aebcff;
  color: #686f7c;
}

.markdown-preview :deep(table) {
  width: 100%;
  border-collapse: collapse;
}

.markdown-preview :deep(th),
.markdown-preview :deep(td) {
  padding: 7px 10px;
  border: 1px solid #dfe3ea;
  text-align: left;
}

.markdown-preview :deep(img) {
  max-width: 100%;
}

.knowledge-inspector {
  min-width: 0;
  overflow-y: auto;
  padding: 30px 28px;
  border-left: 1px solid #e6e8eb;
  background: #fff;
}

.inspector-switcher {
  width: 100%;
}

.inspector-switcher :deep(.ant-segmented-group) {
  display: grid;
  grid-template-columns: 1fr 1fr;
}

.inspector-switcher :deep(.ant-segmented-item-selected) {
  color: #2f6bff;
}

.inspector-content {
  margin-top: 24px;
}

.outline-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.outline-item {
  overflow: hidden;
  width: 100%;
  padding-top: 6px;
  padding-bottom: 6px;
  border: 0;
  border-radius: 5px;
  color: #59606d;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.outline-item:hover {
  color: #3157e2;
  background: #eef2ff;
}

.property-list {
  margin: 22px 0 0;
}

.property-list > div {
  padding: 9px 0;
  border-bottom: 1px solid #edf0f5;
}

.property-list dt {
  margin-bottom: 4px;
  color: #9298a4;
  font-size: 11px;
}

.property-list dd {
  margin: 0;
  color: #3e4450;
  font-size: 12px;
}

.property-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.property-editor label {
  margin-top: 8px;
  color: #8e939b;
  font-size: 12px;
}

.folder-select {
  width: 100%;
}

.inspector-empty {
  padding: 38px 12px;
  color: #a0a4ab;
  font-size: 13px;
  text-align: center;
}


.trash-workspace {
  grid-column: 2 / 4;
  overflow-y: auto;
  padding: 34px 38px;
}

.trash-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}

.trash-header h2 {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
  font-size: 18px;
  font-weight: 650;
}

.trash-list {
  display: flex;
  flex-direction: column;
}

.trash-card {
  display: flex;
  min-height: 108px;
  align-items: center;
  gap: 28px;
  padding: 18px 20px;
  border-bottom: 1px solid #eceef1;
  border-radius: 12px;
  transition: background 0.16s ease;
}

.trash-card:hover {
  background: #f4f7fc;
}

.trash-document-icon {
  display: flex;
  width: 56px;
  height: 56px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 18px;
  color: #2f6bff;
  background: #dbe8ff;
  font-size: 22px;
}

.trash-card-main {
  min-width: 0;
  flex: 1;
}

.trash-card-title {
  color: #303641;
  font-size: 16px;
  font-weight: 600;
}

.trash-card-meta {
  margin-top: 8px;
  color: #969ca7;
  font-size: 13px;
}

.trash-card-actions {
  display: flex;
  flex: 0 0 auto;
  gap: 14px;
}

.danger-button {
  color: #ff4d4f;
}

.trash-empty {
  padding: 120px 20px;
  color: #a0a4ab;
  text-align: center;
}

.knowledge-empty {
  grid-column: 2 / 4;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-state {
  transform: translateY(-20px);
  text-align: center;
}

.empty-illustration {
  position: relative;
  width: 82px;
  height: 70px;
  margin: 0 auto 20px;
}

.empty-folder {
  position: absolute;
  left: 1px;
  bottom: 0;
  color: #e5efff;
  font-size: 70px;
}

.empty-document {
  position: absolute;
  right: 8px;
  bottom: 0;
  z-index: 1;
  color: #3157e2;
  background: #fff;
  font-size: 43px;
}

.empty-check {
  position: absolute;
  top: 1px;
  right: 0;
  z-index: 2;
  display: flex;
  width: 19px;
  height: 19px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #3157e2;
  font-size: 11px;
}

.empty-state h2 {
  margin: 0;
  color: #2c2e32;
  font-size: 16px;
  font-weight: 650;
}

.empty-state p {
  margin: 8px 0 0;
  color: #9b9fa6;
  font-size: 14px;
}

.modal-select {
  width: 100%;
}

@media (max-width: 1320px) {
  .knowledge-page {
    grid-template-columns: 320px minmax(420px, 1fr);
  }

  .knowledge-inspector {
    display: none;
  }

  .trash-workspace,
  .knowledge-empty {
    grid-column: 2;
  }
}

@media (max-width: 860px) {
  .knowledge-page {
    grid-template-columns: 270px minmax(360px, 1fr);
  }

  .knowledge-sidebar {
    padding-inline: 18px;
  }

  .directory-title span:not(.directory-divider) {
    display: none;
  }
}
</style>
