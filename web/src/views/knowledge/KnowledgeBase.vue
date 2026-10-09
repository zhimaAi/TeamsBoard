<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Modal, message } from 'ant-design-vue'
import {
  CheckOutlined,
  DeleteOutlined,
  EditOutlined,
  FileTextOutlined,
  FolderAddOutlined,
  FolderOpenOutlined,
  FolderOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  RollbackOutlined,
  RobotOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue'
import apiClient, { ApiError } from '@/api/client'
import {
  createKnowledgeDocument,
  deleteKnowledgeFolder,
  getKnowledgeRoot,
  migrateKnowledgeRoot,
  scanKnowledge,
  type KnowledgeMigrateConflictItem,
  type KnowledgeMigrateRollback,
} from '@/api/knowledge'
import { isDesktopRuntime, selectDirectory as pickDirectory } from '@/composables/useDesktop'
import { useAppI18n } from '@/i18n'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import { useKnowledgeReferenceStore } from '@/stores/knowledge-reference'
import type { KnowledgeReferenceFragment } from '@/types/knowledge-reference'
import KnowledgeFolderDetail from '@/views/knowledge/components/KnowledgeFolderDetail.vue'
import KnowledgeAgentReferencePopover from '@/views/knowledge/components/KnowledgeAgentReferencePopover.vue'

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
  file_path: string
  ext: 'md' | 'txt'
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

// 大纲项：`key` 仅用于列表渲染；跳转按序号交给编辑器定位（S-UI-10）
type OutlineItem = {
  key: string
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

// 与 MarkdownEditor 的 selection-change 事件 / getSelectionInfo() 载荷结构一致
type EditorSelection = {
  text: string
  prefix: string
  suffix: string
  rect: DOMRect | null
}

const loading = ref(false)
const saving = ref(false)
// S-AS-01: 自动保存状态机
//   idle     - 无未保存改动
//   pending  - 已有改动，等待防抖窗口结束（1500ms）
//   saving   - PUT 请求进行中
//   saved    - 最近一次保存成功，提示 2 秒后回到 idle
//   error    - 最近一次保存失败（保留 dirty，下次再试）
type AutosaveState = 'idle' | 'pending' | 'saving' | 'saved' | 'error'
const AUTOSAVE_DEBOUNCE_MS = 1500
const AUTOSAVE_SAVED_HINT_MS = 2000
const autosaveState = ref<AutosaveState>('idle')
const autosaveSavedAt = ref(0) // 最近一次保存完成的本地时间戳
const autosaveErrorMessage = ref('') // 上一次失败的错误信息
let autosaveTimer: ReturnType<typeof setTimeout> | undefined
let autosaveSavedHintTimer: ReturnType<typeof setTimeout> | undefined
// saveSeq 用于守护过期响应：自增计数，进入请求前取 mySeq，响应回来时若 mySeq !== saveSeq 表示已被新请求覆盖
const saveSeq = ref(0)
// 标记当前 draft 是否已初始化（刚打开文档/刚创建完时，先不触发自动保存）
let draftReady = false
const folders = ref<FolderNode[]>([])
const documents = ref<KnowledgeDocument[]>([])
const trashDocuments = ref<KnowledgeDocument[]>([])
const searchResults = ref<SearchResult[]>([])
const selectedFolderID = ref<number>(0)
const selectedDocumentUUID = ref('')
const selectedTreeKey = ref('')
const searchKeyword = ref('')
const searchLoading = ref(false)
const rightTab = ref('outline')
const trashVisible = ref(false)
// 重命名弹窗（取代原 editor-header 内的标题就地编辑）
const renameModalVisible = ref(false)
const renameInputValue = ref('')
// 通用「新建文档 / 新建文件夹」弹窗：type 区分两种语义，parentID 区分父级位置
type CreateModalType = 'document' | 'folder'
const createModalVisible = ref(false)
const createModalType = ref<CreateModalType>('folder')
const createModalName = ref('')
const createModalParentID = ref(0)
const createModalLoading = ref(false)
// 大纲 / 属性抽屉显隐（独立于媒体查询，避免窄屏下点击无响应）
const inspectorOpen = ref(false)

// 左侧目录栏（knowledge-sidebar）：可折叠、可拖拽改宽度。
// 默认宽度为当前宽度的一半，向上不超过当前宽度，向下不低于当前宽度的一半。
const SIDEBAR_MAX_WIDTH = 380
const SIDEBAR_MIN_WIDTH = 260
const SIDEBAR_DEFAULT_WIDTH = 260
const sidebarCollapsed = ref(false)
const sidebarWidth = ref(SIDEBAR_DEFAULT_WIDTH)
const directoryOrder = ref<Record<string, string[]>>({})
// 目录树展开状态：记录当前展开的文件夹 key
const expandedKeys = ref<string[]>([])
// 记录之前展开的 keys，用于恢复（当 sidebar 收起时）
let previousExpandedKeys: string[] = []
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchSequence = 0

function toggleSidebar() {
  if (sidebarCollapsed.value) {
    // 从收起状态展开时，恢复之前的展开状态
    expandedKeys.value = [...previousExpandedKeys]
  } else {
    // 收起前保存当前展开状态
    previousExpandedKeys = [...expandedKeys.value]
    expandedKeys.value = []
  }
  sidebarCollapsed.value = !sidebarCollapsed.value
}

// 拖拽改宽度：在 mousedown 时记录起始位置和宽度，mousemove 时实时计算并夹紧。
// 监听挂到 document 上以保证鼠标拖出 sidebar 也能继续调整；mouseup 时统一清理。
let sidebarResizeCleanup: (() => void) | null = null
function startSidebarResize(event: MouseEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const startX = event.clientX
  const startWidth = sidebarWidth.value
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'

  const onMove = (moveEvent: MouseEvent) => {
    const next = startWidth + (moveEvent.clientX - startX)
    sidebarWidth.value = Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, next))
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    sidebarResizeCleanup = null
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
  sidebarResizeCleanup = onUp
}

const isSidebarNarrow = computed(
  () => !sidebarCollapsed.value && sidebarWidth.value < 240,
)
const knowledgePageStyle = computed(() => {
  const sidebar = sidebarCollapsed.value ? '0px' : `${sidebarWidth.value}px`
  return { gridTemplateColumns: `${sidebar} minmax(480px, 1fr) 300px` }
})

// S-UI-01: 知识库根目录（KR）设置相关状态
const knowledgeRoot = ref('')
const knowledgeDefaultRoot = ref('')
const rootModalVisible = ref(false)
const rootModalLoading = ref(false)
const pendingRootDir = ref('')

// S-UI-03: 删除文件夹双选项弹窗状态
const deleteFolderModalVisible = ref(false)
const deleteFolderModalLoading = ref(false)
const deleteFolderTarget = ref<FolderNode | null>(null)

// S-UI-14: 文件夹详情列表页（第四个主区工作区）当前文件夹；0 表示根目录视图
const activeFolderID = ref(0)

// S-UI-09 / S-UI-11: vditor 即时渲染编辑器句柄（供大纲跳转与选区读取）
const editorRef = ref<{
  getValue: () => string
  getSelectionInfo: () => EditorSelection | null
  scrollToHeading: (index: number) => void
}>()

// S-UI-13: 编辑器选中内容 → 对话输入区的「添加给 Agent」
const knowledgeReferenceStore = useKnowledgeReferenceStore()
const editorSelectionText = ref('')
const agentReferenceOpen = ref(false)
const agentReferenceFragment = ref<KnowledgeReferenceFragment | null>(null)

const scanning = ref(false)

const draft = reactive({
  uuid: '',
  title: '',
  folderID: 0,
  tags: [] as string[],
  content: '',
  filePath: '',
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

// S-UI-07: 知识库为空时不自动创建任何文件夹或文档，只展示引导与创建入口
const knowledgeEmpty = computed(() => !folders.value.length && !documents.value.length)

// S-UI-14: 当前打开的文件夹（详情页数据源，仅取其直接子项）；null 表示根目录视图
const activeFolder = computed(
  () => flatFolders.value.find((folder) => folder.id === activeFolderID.value) || null,
)

// 根目录视图数据：展示 parent_id=0 的文件夹和 folder_id=0 的文档
const rootViewFolders = computed(() =>
  folders.value.filter((folder) => folder.parent_id === 0),
)
const rootViewDocuments = computed(() =>
  documents.value.filter((document) => document.folder_id === 0),
)

const activeFolderChildren = computed(() => ({
  folders: activeFolder.value?.children || [],
  documents: documents.value.filter((document) => document.folder_id === activeFolderID.value),
}))

// S-UI-14: 聚合视图文档列表：全部文档 或 未分类文档（已移除，根目录视图直接使用 documents）

// 从根到目标文件夹的祖先链（含目标自身）；S-UI-14 的面包屑与 S-UI-13 的引用来源路径共用
function folderAncestorChain(folderID: number): FolderNode[] {
  const current = flatFolders.value.find((folder) => folder.id === folderID)
  if (!current) return []
  const chain: FolderNode[] = [current]
  let parentID = current.parent_id
  // 后端已有环检测（S-BE-04），此处上限仅作死循环兜底
  let guard = 0
  while (parentID > 0 && guard < 256) {
    const parent = flatFolders.value.find((folder) => folder.id === parentID)
    if (!parent) break
    chain.unshift(parent)
    parentID = parent.parent_id
    guard += 1
  }
  return chain
}

// S-UI-14: 面包屑「上级文件夹 / 当前文件夹」；首项为根节点 teamsboard（id=0），末项为当前文件夹（只读）
const activeFolderBreadcrumbs = computed(() => {
  const chain = folderAncestorChain(activeFolderID.value)
  if (!chain.length) return []
  return [
    { id: 0, name: t('knowledge.breadcrumbRoot') },
    ...chain.map((folder) => ({ id: folder.id, name: folder.name })),
  ]
})

// 编辑器头部面包屑：根 → 当前文档所在文件夹链 → 当前文档；超过 3 层显示 .../ + 末两段
const BREADCRUMB_MAX_LENGTH = 20
const BREADCRUMB_OVERFLOW_THRESHOLD = 3

type BreadcrumbLeaf = { id: number | string; kind: 'root' | 'folder' | 'document'; name: string }

const documentBreadcrumbs = computed<BreadcrumbLeaf[]>(() => {
  if (!draft.uuid) return []
  const chain = folderAncestorChain(draft.folderID)
  const fallbackName = draft.title || draft.filePath.split(/[\\/]+/).pop() || ''
  const currentDoc = documents.value.find((item) => item.uuid === draft.uuid)
  const ext = currentDoc?.ext
  const displayName = ext ? `${fallbackName}.${ext}` : fallbackName
  return [
    { id: 0, kind: 'root', name: t('knowledge.breadcrumbRoot') },
    ...chain.map((folder) => ({ id: folder.id, kind: 'folder' as const, name: folder.name })),
    { id: draft.uuid, kind: 'document' as const, name: displayName },
  ]
})

function truncateBreadcrumbName(rawName: string): string {
  const name = rawName || ''
  if (name.length <= BREADCRUMB_MAX_LENGTH) return name
  return `${name.slice(0, BREADCRUMB_MAX_LENGTH)}…`
}

// 文件名特殊处理：保留 .md/.txt 扩展名，仅截断主名部分
function truncateBreadcrumbDocumentName(rawName: string): string {
  const name = rawName || ''
  const lastDot = name.lastIndexOf('.')
  const slashIndex = Math.max(name.lastIndexOf('/'), name.lastIndexOf('\\'))
  if (lastDot > slashIndex && lastDot > 0 && lastDot < name.length - 1) {
    const ext = name.slice(lastDot)
    const stem = name.slice(0, lastDot)
    if (stem.length <= BREADCRUMB_MAX_LENGTH) return name
    return `${stem.slice(0, BREADCRUMB_MAX_LENGTH)}…${ext}`
  }
  return truncateBreadcrumbName(name)
}

function breadcrumbDisplayName(item: BreadcrumbLeaf): string {
  return item.kind === 'document'
    ? truncateBreadcrumbDocumentName(item.name)
    : truncateBreadcrumbName(item.name)
}

function fullBreadcrumbName(item: BreadcrumbLeaf): string {
  return item.name
}

// 通用面包屑点击：folder/root 跳转到对应目录（编辑态先确认放弃）
async function navigateBreadcrumb(item: BreadcrumbLeaf) {
  if (item.kind === 'document') return
  if (item.kind === 'root') {
    if (!(await confirmDiscardChanges())) return
    clearDraft()
    selectedFolderID.value = 0
    selectedTreeKey.value = 'root'
    activeFolderID.value = 0
    return
  }
  if (!(await confirmDiscardChanges())) return
  selectedFolderID.value = item.id as number
  selectedTreeKey.value = `folder:${item.id}`
  activeFolderID.value = item.id as number
  clearDraft()
}

// 弹窗中根据输入预览完整路径（文件名无后缀则默认 .md）
function previewCreateName(raw: string, type: CreateModalType): string {
  if (!raw.trim()) return ''
  if (type === 'folder') return raw.trim()
  const { title, ext } = normalizedDocumentTitle(raw)
  return `${title}.${ext}`
}

const createModalParentPath = computed(() => {
  if (createModalParentID.value === 0) {
    return [t('knowledge.breadcrumbRoot')]
  }
  const chain = folderAncestorChain(createModalParentID.value)
  return [t('knowledge.breadcrumbRoot'), ...chain.map((folder) => folder.name)]
})

const createModalTargetPath = computed(() => {
  const name = previewCreateName(createModalName.value, createModalType.value)
  if (!name) return createModalParentPath.value.join(' / ')
  return [...createModalParentPath.value, name].join(' / ')
})

function toggleInspector() {
  inspectorOpen.value = !inspectorOpen.value
}

// 面包屑省略状态：默认折叠（超过3层时），点击 "…" 按钮展开全部
const breadcrumbExpanded = ref(false)

const visibleBreadcrumbs = computed(() => {
  const items = documentBreadcrumbs.value
  if (items.length <= BREADCRUMB_OVERFLOW_THRESHOLD || breadcrumbExpanded.value) return items
  return [
    { id: 'overflow' as const, kind: 'overflow' as const, name: t('knowledge.breadcrumbOverflow') },
    ...items.slice(items.length - 2),
  ]
})

function showFullBreadcrumb() {
  breadcrumbExpanded.value = true
}

// S-UI-01: 面包屑取 KR 路径最后一段作为当前目录名，根节点用固定标签
const rootBreadcrumbSegments = computed(() => {
  const segments = knowledgeRoot.value.split(/[\\/]+/).filter(Boolean)
  const leaf = segments.pop() || ''
  return [t('knowledge.rootBreadcrumbRoot'), leaf].filter(Boolean)
})

// S-UI-01: 设置弹窗内显示待确认的目录路径
const modalRootBreadcrumbSegments = computed(() => {
  const path = pendingRootDir.value || knowledgeRoot.value
  const segments = path.split(/[\\/]+/).filter(Boolean)
  const leaf = segments.pop() || ''
  return [t('knowledge.rootBreadcrumbRoot'), leaf].filter(Boolean)
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
    title: document.ext ? `${document.title}.${document.ext}` : document.title,
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

  const sortableRootNodes = orderDirectoryNodes('root', [
    ...folders.value
      .map((folder) => mapFolder(folder, 'root'))
      .filter((folder): folder is DirectoryNode => Boolean(folder)),
    // 根目录文件（folder_id === 0）与根目录同级展示，支持搜索过滤、选中、拖拽到文件夹等已有交互
    ...directoryDocuments.value
      .filter((document) => document.folder_id === 0)
      .map((document) => documentNode(document, 'root')),
  ])

  return sortableRootNodes
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

const outline = computed<OutlineItem[]>(() => {
  const items: OutlineItem[] = []
  for (const line of draft.content.split(/\r?\n/)) {
    const match = /^(#{1,6})\s+(.+?)\s*#*$/.exec(line)
    if (!match) continue
    items.push({
      key: `${items.length}:${match[2].trim()}`,
      level: match[1].length,
      title: match[2].trim(),
    })
  }
  return items
})

// 自动保存指示器文案：根据状态机返回不同模板；initial drafts 不显示
const autosaveHint = computed(() => {
  if (!draft.uuid || !draftReady) return ''
  if (autosaveState.value === 'pending') return t('knowledge.autosavePending')
  if (autosaveState.value === 'saving') return t('knowledge.autosaveSaving')
  if (autosaveState.value === 'saved' && autosaveSavedAt.value) {
    const time = new Date(autosaveSavedAt.value).toLocaleTimeString(locale.value, {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    })
    return t('knowledge.autosaveSavedAt', { time })
  }
  if (autosaveState.value === 'error') return t('knowledge.autosaveFailed')
  return ''
})

// 切换文档或新建/清空时，重置自动保存相关的本地状态（在 openDocument / clearDraft 内统一调用）
function resetAutosaveStateOnDraftChange() {
  draftReady = false
  autosaveState.value = 'idle'
  autosaveSavedAt.value = 0
  autosaveErrorMessage.value = ''
  if (autosaveTimer) {
    clearTimeout(autosaveTimer)
    autosaveTimer = undefined
  }
  if (autosaveSavedHintTimer) {
    clearTimeout(autosaveSavedHintTimer)
    autosaveSavedHintTimer = undefined
  }
  // 提序列号，避免上一次文档的 in-flight 请求响应被错误地应用到当前状态
  saveSeq.value++
}

function currentSnapshot() {
  savedSnapshot.value = snapshot.value
}

// S-AS-03: 防抖自动保存核心
// 1. 监听 snapshot（已包含 title/folderID/tags/content）；变化时把状态打到 pending 并重置防抖定时器
// 2. draftReady 守护：避免打开文档/新建文档时立刻触发一次保存（首屏的 dirty=true 是历史遗留而非用户改动）
// 3. dirty 守护：仅在确有未保存改动时启动定时器；保存成功后 currentSnapshot() 会让 dirty 回 false，
//    之后 savedSnapshot 变回匹配值，watch 会再次触发但被 dirty=false 过滤掉
// 4. 右上的「保存失败，点击重试」走 retryAutosave() 重新发起一次
watch(
  () => snapshot.value,
  () => {
    if (!draftReady || !draft.uuid) return
    if (!dirty.value) return
    autosaveState.value = 'pending'
    if (autosaveTimer) clearTimeout(autosaveTimer)
    autosaveTimer = setTimeout(() => {
      autosaveTimer = undefined
      void performSave('auto')
    }, AUTOSAVE_DEBOUNCE_MS)
  },
)

function clearDraft() {
  Object.assign(draft, {
    uuid: '',
    title: '',
    folderID: 0,
    tags: [],
    content: '',
    filePath: '',
    contentHash: '',
    wordCount: 0,
    createdAt: 0,
    updatedAt: 0,
  })
  savedSnapshot.value = ''
  selectedDocumentUUID.value = ''
  selectedTreeKey.value = ''
  renameModalVisible.value = false
  editorSelectionText.value = ''
  resetAutosaveStateOnDraftChange()
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

async function loadWorkspace(selectFirst = false, scan = false) {
  loading.value = true
  try {
    await Promise.all([loadFolders(), loadDocuments(), loadTrash()])
    if (scan) {
      await scanKnowledgeBase()
    }
    if (selectFirst && !draft.uuid && documents.value.length) {
      await openDocument(documents.value[0].uuid, true)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.loadFailed'))
  } finally {
    loading.value = false
  }
}

// S-UI-05: 进入页面与手动刷新时扫描 KR，索引外部 .md/.txt
async function scanKnowledgeBase() {
  scanning.value = true
  try {
    const result = await scanKnowledge()
    if (result.added_folders || result.added_documents) {
      await Promise.all([loadFolders(), loadDocuments()])
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.scanFailed'))
  } finally {
    scanning.value = false
  }
}

async function refreshWorkspace() {
  if (loading.value || scanning.value) return
  await loadWorkspace(false, true)
}

// S-UI-01: 加载当前 KR 与默认 KR
async function loadKnowledgeRoot() {
  try {
    const result = await getKnowledgeRoot()
    knowledgeRoot.value = result.root
    knowledgeDefaultRoot.value = result.default
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.rootLoadFailed'))
  }
}

function openRootModal() {
  pendingRootDir.value = knowledgeRoot.value
  rootModalVisible.value = true
}

async function chooseRootDirectory() {
  if (!isDesktopRuntime()) return
  const defaultPath = pendingRootDir.value || knowledgeDefaultRoot.value
  try {
    const selected = await pickDirectory(defaultPath)
    if (selected) {
      pendingRootDir.value = selected
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.selectDirectoryFailed'))
  }
}

// S-UI-17: 迁移属破坏性操作，确认前先二次确认，迁移期间锁定弹窗防重复提交
async function confirmSetKnowledgeRoot() {
  const dir = pendingRootDir.value.trim()
  if (!dir || rootModalLoading.value) return

  const confirmed = await new Promise<boolean>((resolve) => {
    Modal.confirm({
      title: t('knowledge.migrateConfirmTitle'),
      content: t('knowledge.migrateConfirmDescription'),
      okText: t('knowledge.confirm'),
      okType: 'danger',
      cancelText: t('knowledge.cancel'),
      onOk: () => resolve(true),
      onCancel: () => resolve(false),
    })
  })
  if (!confirmed) return

  rootModalLoading.value = true
  try {
    const result = await migrateKnowledgeRoot(dir)
    knowledgeRoot.value = result.root
    // 迁移后旧 draft 的上下文已失效，回到根目录视图并整体刷新目录树与文档列表
    clearDraft()
    activeFolderID.value = 0
    await loadKnowledgeRoot()
    await loadWorkspace(false, false)
    rootModalVisible.value = false
    pendingRootDir.value = ''
    message.success(
      t('knowledge.migrateSuccess', {
        folders: result.migrated.folders,
        documents: result.migrated.documents,
      }),
    )
  } catch (error) {
    handleMigrateFailure(error)
  } finally {
    rootModalLoading.value = false
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isMigrateConflictItem(value: unknown): value is KnowledgeMigrateConflictItem {
  return (
    isRecord(value) &&
    typeof value.path === 'string' &&
    (value.type === 'file' || value.type === 'dir')
  )
}

function readMigrateConflicts(payload: unknown): KnowledgeMigrateConflictItem[] {
  if (!isRecord(payload) || !Array.isArray(payload.conflicts)) return []
  return payload.conflicts.filter(isMigrateConflictItem)
}

function readMigrateRollback(payload: unknown): KnowledgeMigrateRollback | null {
  if (!isRecord(payload) || !isRecord(payload.rollback)) return null
  const { completed, failed } = payload.rollback
  if (typeof completed !== 'boolean') return null
  return {
    completed,
    failed: Array.isArray(failed)
      ? failed.filter((item): item is string => typeof item === 'string')
      : [],
  }
}

function migrateFailureFallback(code: unknown) {
  if (code === 'knowledge_root_target_invalid') return t('knowledge.migrateTargetInvalid')
  if (code === 'knowledge_root_migrate_conflict') return t('knowledge.migrateConflictTitle')
  if (code === 'knowledge_root_migrate_failed') return t('knowledge.migrateFailed')
  return t('knowledge.knowledgeRootSetFailed')
}

// S-IN-07: 冲突清单直接渲染后端返回的 path/type，配置与目录树保持不变
function showMigrateConflicts(conflicts: KnowledgeMigrateConflictItem[]) {
  Modal.error({
    title: t('knowledge.migrateConflictTitle'),
    okText: t('knowledge.confirm'),
    content: () =>
      h('div', [
        h(
          'p',
          { style: { margin: '0 0 8px', color: '#8C8C8C', fontSize: '13px' } },
          t('knowledge.migrateConflictListTitle'),
        ),
        h(
          'ul',
          { style: { margin: 0, paddingLeft: '18px', maxHeight: '240px', overflowY: 'auto' } },
          conflicts.map((item) =>
            h(
              'li',
              { key: item.path },
              `${item.type === 'dir' ? t('knowledge.migrateConflictDir') : t('knowledge.migrateConflictFile')}：${item.path}`,
            ),
          ),
        ),
      ]),
  })
}

function handleMigrateFailure(error: unknown) {
  const apiError = error instanceof ApiError ? error : null
  const code = apiError?.code
  const conflicts = readMigrateConflicts(apiError?.payload)
  if (apiError?.status === 409 && conflicts.length) {
    showMigrateConflicts(conflicts)
    return
  }
  // 优先展示后端真实文案，缺失时按错误码回退到词条
  const detail = error instanceof Error && error.message ? error.message : ''
  message.error(detail || migrateFailureFallback(code))

  if (code === 'knowledge_root_migrate_failed') {
    const rollback = readMigrateRollback(apiError?.payload)
    if (rollback && !rollback.completed && rollback.failed.length) {
      message.error(t('knowledge.migrateRollbackFailed', { items: rollback.failed.join('、') }))
    }
  }
}

function cancelRootModal() {
  if (rootModalLoading.value) {
    message.info(t('knowledge.migrateBlocked'))
    return
  }
  rootModalVisible.value = false
  pendingRootDir.value = ''
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
      filePath: document.file_path,
      contentHash: document.content_hash,
      wordCount: document.word_count,
      createdAt: document.created_at,
      updatedAt: document.updated_at,
    })
    selectedDocumentUUID.value = uuid
    selectedTreeKey.value = treeKey || `doc:${uuid}`
    trashVisible.value = false
    editorSelectionText.value = ''
    currentSnapshot()
    // 切到新文档：清掉上一文档的自动保存状态/错误信息；序列号提升到下一次，避免过期 in-flight 响应污染
    resetAutosaveStateOnDraftChange()
    // 文档已加载好，可以开始自动保存；任何修改都会触发 snapshot watcher
    draftReady = true
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.documentLoadFailed'))
  }
}

// S-UI-14: 详情页「快速添加」传入目标文件夹；不传则落在当前选中目录（左侧面板「+」号）
async function createDocument(targetFolderId?: number) {
  if (!(await confirmDiscardChanges())) return
  const folderId = targetFolderId ?? selectedFolderID.value
  try {
    // 默认「未命名文档」也预判重名 (n)，避免同一文件夹下出现同名文件；正文首行同步保持一致
    const title = uniqueTitleInFolder(folderId, t('knowledge.untitled'))
    const created = await apiClient.post<KnowledgeDocument>('/knowledge/documents', {
      title,
      folder_id: folderId,
      content: buildNewDocumentContent(title),
      tags: [],
      ext: 'md',
    })
    await loadDocuments()
    // 创建完成后自动展开目标目录
    if (folderId > 0) {
      const folderKey = `folder:${folderId}`
      if (!expandedKeys.value.includes(folderKey)) {
        expandedKeys.value = [...expandedKeys.value, folderKey]
      }
    }
    if (targetFolderId !== undefined) {
      // 保留在详情页，列表随 loadDocuments 实时刷新
      selectedFolderID.value = folderId
      message.success(t('knowledge.documentCreated'))
      return
    }
    await openDocument(created.uuid, true)
    message.success(t('knowledge.documentCreated'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.documentCreateFailed'))
  }
}

// S-UI-02: 新建 txt 文档（能力保留，入口在「更多」子菜单 —— S-UI-08）
async function createTxtDocument(targetFolderId?: number) {
  if (!(await confirmDiscardChanges())) return
  const folderId = targetFolderId ?? selectedFolderID.value
  try {
    const created = await createKnowledgeDocument({
      title: t('knowledge.untitled'),
      folder_id: folderId,
      content: '',
      tags: [],
      ext: 'txt',
    })
    await loadDocuments()
    // 创建完成后自动展开目标目录
    if (folderId > 0) {
      const folderKey = `folder:${folderId}`
      if (!expandedKeys.value.includes(folderKey)) {
        expandedKeys.value = [...expandedKeys.value, folderKey]
      }
    }
    if (targetFolderId !== undefined) {
      selectedFolderID.value = folderId
      message.success(t('knowledge.txtDocumentCreated'))
      return
    }
    await openDocument(created.uuid, true)
    message.success(t('knowledge.txtDocumentCreated'))
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
    const copyTitle = t('knowledge.conflictCopy', { title: draft.title.trim() || t('knowledge.untitled') })
    // 同目录预判重名 (n)，正文首行同步换成副本标题，保持首行与文件名一致
    const finalTitle = uniqueTitleInFolder(draft.folderID, copyTitle)
    const heading = extractFirstHeading(draft.content)
    const finalContent = heading !== null ? replaceFirstHeading(draft.content, finalTitle) : draft.content
    const created = await apiClient.post<KnowledgeDocument>('/knowledge/documents', {
      title: finalTitle,
      folder_id: draft.folderID,
      content: finalContent,
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

// S-AS-02: 共享保存逻辑，拆分手动（Ctrl+S / 保留旧调用）与自动（防抖触发）两条路径。
// 区别仅在反馈：手动保存弹「文档已保存」toast，自动保存只更新右上指示器，409 仍走原冲突弹窗。
async function performSave(source: 'manual' | 'auto') {
  if (!draft.uuid || saving.value) return
  // 正文首行是 `# xxx` 时，文件名取首行；否则沿用 draft.title；并预判同目录下重名，提前追加 (n) 同步正文首行
  const heading = extractFirstHeading(draft.content)
  const titleSource = heading ?? draft.title.trim()
  const unique = uniqueTitleInFolder(draft.folderID, titleSource, draft.uuid)
  const nextTitle = unique
  const nextContent = heading !== null && unique !== heading
    ? replaceFirstHeading(draft.content, unique)
    : draft.content
  if (!nextTitle.trim()) {
    if (source === 'manual') {
      message.error(t('knowledge.titleRequired'))
    } else {
      autosaveState.value = 'idle'
    }
    return
  }
  // 同步到 draft：仅在变化时写回，避免无谓触发 watcher 再次排自动保存
  if (nextTitle !== draft.title) draft.title = nextTitle
  if (nextContent !== draft.content) draft.content = nextContent
  // 手动保存时取消挂起的自动保存定时器，避免在手动保存完成后再被自动保存覆盖
  if (source === 'manual' && autosaveTimer) {
    clearTimeout(autosaveTimer)
    autosaveTimer = undefined
  }
  const mySeq = ++saveSeq.value
  autosaveState.value = 'saving'
  saving.value = true
  try {
    const updated = await apiClient.put<KnowledgeDocument>(
      `/knowledge/documents/${draft.uuid}`,
      {
        title: nextTitle,
        folder_id: draft.folderID,
        content: nextContent,
        tags: draft.tags.map((tag) => tag.trim()).filter(Boolean),
        base_hash: draft.contentHash,
      },
    )
    // 已被更新的请求覆盖，本响应丢弃
    if (mySeq !== saveSeq.value) return
    Object.assign(draft, {
      title: updated.title,
      folderID: updated.folder_id,
      tags: [...updated.tags],
      content: updated.content || '',
      filePath: updated.file_path,
      contentHash: updated.content_hash,
      wordCount: updated.word_count,
      updatedAt: updated.updated_at,
    })
    currentSnapshot()
    updateDocumentInList(updated)
    autosaveSavedAt.value = Date.now()
    autosaveErrorMessage.value = ''
    autosaveState.value = 'saved'
    if (autosaveSavedHintTimer) clearTimeout(autosaveSavedHintTimer)
    autosaveSavedHintTimer = setTimeout(() => {
      if (autosaveState.value === 'saved') autosaveState.value = 'idle'
    }, AUTOSAVE_SAVED_HINT_MS)
    if (source === 'manual') {
      message.success(t('knowledge.documentSaved'))
    }
  } catch (error) {
    if (mySeq !== saveSeq.value) return
    autosaveErrorMessage.value =
      error instanceof Error ? error.message : t('knowledge.saveFailed')
    autosaveState.value = 'error'
    if (error instanceof ApiError && error.status === 409) {
      handleSaveConflict()
    } else if (source === 'manual') {
      message.error(autosaveErrorMessage.value)
    }
    // 自动保存失败时仅显示右上错误指示器，不弹 toast，避免频繁打扰
  } finally {
    if (mySeq === saveSeq.value) saving.value = false
  }
}

function saveDocument() {
  return performSave('manual')
}

// 用户点击「保存失败」提示时，由自动保存路径立即重试一次（不重置防抖）
async function retryAutosave() {
  if (saving.value || !draft.uuid || !draft.title.trim()) return
  if (autosaveTimer) {
    clearTimeout(autosaveTimer)
    autosaveTimer = undefined
  }
  await performSave('auto')
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

// 目录树文档节点三点菜单「重命名」→ 打开顶部重命名弹窗
function handleRenameDocument(uuid: string) {
  if (selectedDocumentUUID.value !== uuid) return
  startRename()
}

// 重命名入口：把当前标题作为弹窗初值。弹窗提交后只更新 draft.title，由自动保存负责落盘
function startRename() {
  if (!draft.uuid) return
  renameInputValue.value = draft.title
  renameModalVisible.value = true
}

function cancelRename() {
  renameModalVisible.value = false
}

async function submitRename() {
  const next = renameInputValue.value.trim()
  if (!next) {
    message.error(t('knowledge.titleRequired'))
    return
  }
  // 提前按文件名规则清洗，避免标题里含非法字符导致后端落盘失败
  const sanitized = sanitizeTitle(next)
  if (sanitized === draft.title.trim()) {
    renameModalVisible.value = false
    return
  }
  renameModalVisible.value = false
  // 直接改 draft.title，watch 到 snapshot 变化后会自动保存；触发表单 → dirty 的过渡
  draft.title = sanitized
  // 正文首行是文件名的体现，同步更新首行保持一致，避免被 performSave 的「首行为准」逻辑覆盖回旧值
  if (extractFirstHeading(draft.content) !== null) {
    draft.content = replaceFirstHeading(draft.content, sanitized)
  }
}

// 目录树文档节点三点菜单「移到回收站」
function handleDeleteDocument(uuid: string) {
  if (selectedDocumentUUID.value !== uuid) {
    // 如果不是当前打开的文档，直接调接口
    Modal.confirm({
      title: t('knowledge.deleteTitle', { title: documents.value.find((d) => d.uuid === uuid)?.title || '' }),
      content: t('knowledge.deleteDescription'),
      okText: t('knowledge.moveToTrash'),
      okType: 'danger',
      cancelText: t('knowledge.cancel'),
      async onOk() {
        try {
          await apiClient.delete(`/knowledge/documents/${uuid}`)
          await Promise.all([loadDocuments(), loadTrash()])
          if (draft.uuid === uuid) clearDraft()
          message.success(t('knowledge.movedToTrash'))
        } catch (error) {
          message.error(error instanceof Error ? error.message : t('knowledge.deleteFailed'))
        }
      },
    })
    return
  }
  deleteDocument()
}

async function openTrash() {
  if (!(await confirmDiscardChanges())) return
  clearDraft()
  activeFolderID.value = 0
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
  trashVisible.value = false

  // 点击文件夹时：如果 sidebar 收起则自动展开，并展开该目录节点
  if (sidebarCollapsed.value) {
    previousExpandedKeys = [...expandedKeys.value]
    sidebarCollapsed.value = false
  }

  // 文件夹进入详情页
  selectedFolderID.value = node.folderID
  activeFolderID.value = node.folderID
  // 自动展开该目录节点
  if (!expandedKeys.value.includes(node.key)) {
    expandedKeys.value = [...expandedKeys.value, node.key]
  }
}

async function moveDirectoryNode(info: DirectoryDropInfo) {
  const dragged = info.dragNode
  const target = info.node
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
      return directoryTreeData.value
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
    targetParentKey = target.key
  } else if (info.dropToGap) {
    targetParentKey = target.parentKey
  } else {
    targetParentKey = target.key
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
    if (dragged.parentKey !== targetParentKey) {
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

// 通用「新建文档 / 新建文件夹」弹窗：编辑态先做放弃/保存确认，再弹出名称输入框
async function openCreateModal(type: CreateModalType, parentId?: number) {
  if (!(await confirmDiscardChanges())) return
  const resolvedParent = parentId ?? selectedFolderID.value
  createModalType.value = type
  createModalParentID.value = resolvedParent
  createModalName.value = ''
  createModalVisible.value = true
}

function cancelCreateModal() {
  if (createModalLoading.value) return
  createModalVisible.value = false
  createModalName.value = ''
}

// 文件名无扩展名时默认补 .md，避免后端校验拒绝
function normalizedDocumentTitle(raw: string): { title: string; ext: 'md' | 'txt' } {
  const title = raw.trim()
  const lastDot = title.lastIndexOf('.')
  const slashIndex = Math.max(title.lastIndexOf('/'), title.lastIndexOf('\\'))
  if (lastDot > slashIndex && lastDot > 0 && lastDot < title.length - 1) {
    const ext = title.slice(lastDot + 1).toLowerCase()
    if (ext === 'md' || ext === 'txt') {
      return { title: title.slice(0, lastDot), ext }
    }
  }
  return { title, ext: 'md' }
}

// 提取正文首行的 `# xxx` 标题；首行必须是 `# ` 开头（单 #）才返回；返回值为去掉两端空白与可选结尾 # 的标题文本
function extractFirstHeading(content: string): string | null {
  if (!content) return null
  const firstLine = content.split(/\r?\n/, 1)[0]
  const match = /^#\s+(.+?)\s*#*\s*$/.exec(firstLine)
  if (!match) return null
  const heading = match[1].trim()
  return heading || null
}

// 把标题清洗为合法文件名，与后端 internal/knowledge/store.go 的 SanitizeFileName 保持一致：
// 替换控制字符与 / \ : * ? " < > | 为下划线；去掉首尾 . 与空白；空结果回退到「未命名文档」。
// 创建/保存前调用一次，避免标题里含这些字符导致后端文件落盘失败。
function sanitizeTitle(title: string): string {
  const trimmed = (title || '').trim()
  let result = ''
  for (const ch of trimmed) {
    const code = ch.codePointAt(0) ?? 0
    const isControl = (code >= 0x00 && code <= 0x1f) || (code >= 0x7f && code <= 0x9f)
    if (
      isControl ||
      ch === '/' || ch === '\\' || ch === ':' || ch === '*' ||
      ch === '?' || ch === '"' || ch === '<' || ch === '>' || ch === '|'
    ) {
      result += '_'
    } else {
      result += ch
    }
  }
  result = result.replace(/^[. ]+|[. ]+$/g, '')
  return result || t('knowledge.untitled')
}

// 用新标题替换正文首行；保留其余内容；如果首行后没有空行则补一行，与默认 newContent 模板结构保持一致
function replaceFirstHeading(content: string, heading: string): string {
  const lines = content.split(/\r?\n/)
  if (!lines.length) return `# ${heading}\n`
  lines[0] = `# ${heading}`
  if (lines.length === 1 || lines[1].trim() !== '') {
    lines.splice(1, 0, '')
  }
  return lines.join('\n')
}

// 同目录下不冲突的标题：与后端 uniqueNewRelPath 的 (n) 后缀行为一致；
// 用于在创建/保存前预判重名，把 (n) 同步反映到正文首行，避免后端改 file_path 而前端 title/content 不一致。
// 同时对输入和已有文档 title 做 sanitize，确保含非法字符的脏数据不会漏判冲突。
function uniqueTitleInFolder(folderId: number, baseTitle: string, excludeUUID = ''): string {
  const normalized = sanitizeTitle(baseTitle)
  const exists = (candidate: string) =>
    documents.value.some(
      (doc) =>
        doc.folder_id === folderId &&
        doc.uuid !== excludeUUID &&
        sanitizeTitle(doc.title) === candidate,
    )
  if (!exists(normalized)) return normalized
  for (let i = 1; i < 10000; i++) {
    const candidate = `${normalized}(${i})`
    if (!exists(candidate)) return candidate
  }
  return normalized
}

// 用默认 newContent 模板构造新文档正文，只把首行标题换成传入的 title，保留「首行 + 空行 + 正文提示」结构
function buildNewDocumentContent(title: string): string {
  const template = t('knowledge.newContent')
  const lines = template.split(/\r?\n/)
  lines[0] = `# ${title}`
  return lines.join('\n')
}

async function submitCreateModal() {
  const raw = createModalName.value.trim()
  if (!raw || createModalLoading.value) return
  createModalLoading.value = true
  const folderId = createModalParentID.value
  try {
    if (createModalType.value === 'folder') {
      await apiClient.post('/knowledge/folders', {
        name: raw,
        parent_id: folderId,
      })
      createModalVisible.value = false
      createModalName.value = ''
      await loadFolders()
      // 创建完成后自动展开父目录
      if (folderId > 0) {
        const folderKey = `folder:${folderId}`
        if (!expandedKeys.value.includes(folderKey)) {
          expandedKeys.value = [...expandedKeys.value, folderKey]
        }
      }
      message.success(t('knowledge.folderCreated'))
      return
    }
    const { title, ext } = normalizedDocumentTitle(raw)
    // md 文档：标题在同目录下预判重名并追加 (n)，正文首行同步换成同一标题，避免后端再加 (n) 造成 title 与内容不一致
    const finalTitle = ext === 'md' ? uniqueTitleInFolder(folderId, title) : title
    const finalContent = ext === 'md' ? buildNewDocumentContent(finalTitle) : ''
    const created = await apiClient.post<KnowledgeDocument>('/knowledge/documents', {
      title: finalTitle,
      folder_id: folderId,
      content: finalContent,
      tags: [],
      ext,
    })
    createModalVisible.value = false
    createModalName.value = ''
    await loadDocuments()
    // 创建完成后自动展开目标目录
    if (folderId > 0) {
      const folderKey = `folder:${folderId}`
      if (!expandedKeys.value.includes(folderKey)) {
        expandedKeys.value = [...expandedKeys.value, folderKey]
      }
    }
    await openDocument(created.uuid, true)
    message.success(t('knowledge.documentCreatedInPath', { path: created.file_path }))
  } catch (error) {
    message.error(
      error instanceof Error
        ? error.message
        : createModalType.value === 'folder'
          ? t('knowledge.folderCreateFailed')
          : t('knowledge.documentCreateFailed'),
    )
  } finally {
    createModalLoading.value = false
  }
}

async function renameSelectedFolder() {
  if (!selectedFolderID.value || selectedFolderID.value === 0) return
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

// S-UI-03: 删除文件夹双选项弹窗
function deleteSelectedFolder() {
  if (!selectedFolderID.value || selectedFolderID.value === 0) return
  const folder = flatFolders.value.find((item) => item.id === selectedFolderID.value)
  if (!folder) return
  deleteFolderTarget.value = folder
  deleteFolderModalVisible.value = true
}

async function confirmDeleteFolder(mode: 'keep' | 'purge') {
  const folder = deleteFolderTarget.value
  if (!folder) return
  deleteFolderModalLoading.value = true
  try {
    await deleteKnowledgeFolder(folder.id, mode)
    deleteFolderModalVisible.value = false
    deleteFolderTarget.value = null
    // S-UI-14: 删除成功后回到上级文件夹详情页（有上级）或根目录视图（folder_id=0）
    const parentID = folder.parent_id
    selectedFolderID.value = parentID
    selectedTreeKey.value = parentID > 0 ? `folder:${parentID}` : 'root'
    activeFolderID.value = parentID
    if (mode === 'purge') {
      await Promise.all([loadFolders(), loadDocuments(), loadTrash()])
    } else {
      await Promise.all([loadFolders(), loadDocuments()])
    }
    message.success(mode === 'purge' ? t('knowledge.folderDeleted') : t('knowledge.folderKept'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.folderDeleteFailed'))
  } finally {
    deleteFolderModalLoading.value = false
  }
}

// S-UI-14: 详情页逐层下钻与面包屑返回
function handleDetailOpenFolder(folderId: number) {
  activeFolderID.value = folderId
  selectedFolderID.value = folderId
  selectedTreeKey.value = `folder:${folderId}`
  trashVisible.value = false
}

function handleDetailNavigate(folderId: number) {
  activeFolderID.value = folderId
  selectedFolderID.value = folderId
  selectedTreeKey.value = folderId > 0 ? `folder:${folderId}` : 'root'
}

async function handleDetailOpenDocument(uuid: string) {
  await openDocument(uuid)
}

// 详情页「快速添加 → 新建文件夹」：直接创建，不需要弹窗（与 createDocument 语义一致）
async function createFolderFromDetail(folderId: number) {
  try {
    await apiClient.post('/knowledge/folders', {
      name: t('knowledge.untitled'),
      parent_id: folderId,
    })
    await loadFolders()
    message.success(t('knowledge.folderCreated'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('knowledge.folderCreateFailed'))
  }
}

// 详情页删除当前文件夹：复用既有 keep/purge 双选项弹窗（S-UI-14 / CK-3）
function handleDetailDeleteFolder(folderId: number) {
  selectedFolderID.value = folderId
  deleteSelectedFolder()
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

// S-UI-10: 大纲跳转改由编辑器定位第 index 个标题元素（vditor 即时渲染 DOM）
function scrollToHeading(index: number) {
  editorRef.value?.scrollToHeading(index)
}

// S-UI-13: 编辑器选区变化（空文本表示选区已离开编辑器或已折叠）
function handleEditorSelection(info: EditorSelection) {
  editorSelectionText.value = info.text
}

// S-UI-13: 打开「添加给 Agent」，以点击瞬间的选区快照作为引用片段（含前后文定位）
function openAgentReference() {
  const info = editorRef.value?.getSelectionInfo() ?? null
  const text = (info?.text || editorSelectionText.value).trim()
  if (!text) return
  agentReferenceFragment.value = {
    text,
    prefix: info?.prefix ?? '',
    suffix: info?.suffix ?? '',
  }
  agentReferenceOpen.value = true
}

// S-UI-13: 发送到对话输入区（跨页投递，由 ChatComposer 消费；T5 实现消费端）
function handleAgentReferenceSend(instruction: string) {
  const fragment = agentReferenceFragment.value
  if (!fragment || !draft.uuid) return
  knowledgeReferenceStore.enqueue({
    uuid: draft.uuid,
    title: documentFileName(),
    filePath: draft.filePath,
    folderPath: folderPathOf(draft.folderID),
    fragment,
    instruction,
  })
  agentReferenceOpen.value = false
  agentReferenceFragment.value = null
  message.success(t('knowledge.agentReferenceSent'))
}

// 引用标题取真实文件名（统一补 .md/.txt 扩展名），与 file_path 保持一致；
// 当 file_path 已含 md/txt 后缀时直接复用，否则回退到当前文档的 ext
function documentFileName() {
  const segments = draft.filePath.split(/[\\/]+/).filter(Boolean)
  const baseName = segments.pop() || draft.title
  const lastDot = baseName.lastIndexOf('.')
  const slashIndex = Math.max(baseName.lastIndexOf('/'), baseName.lastIndexOf('\\'))
  if (lastDot > slashIndex && lastDot > 0 && lastDot < baseName.length - 1) {
    const ext = baseName.slice(lastDot + 1).toLowerCase()
    if (ext === 'md' || ext === 'txt') return baseName
  }
  const currentDoc = documents.value.find((item) => item.uuid === draft.uuid)
  const docExt = currentDoc?.ext
  return docExt ? `${baseName}.${docExt}` : baseName
}

// 引用来源路径：根为 teamsboard；folder_id = 0 视为「根目录」
function folderPathOf(folderID: number) {
  const root = t('knowledge.breadcrumbRoot')
  if (!folderID) return `${root} / `
  const chain = folderAncestorChain(folderID)
  if (!chain.length) return root
  return [root, ...chain.map((folder) => folder.name)].join(' / ')
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
  await loadKnowledgeRoot()
  await loadWorkspace(false, true)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeyboard)
  if (searchTimer) clearTimeout(searchTimer)
  // 防止组件卸载时仍残留拖拽监听与光标样式
  sidebarResizeCleanup?.()
  // 清掉自动保存相关的残余定时器，避免在 setup 之外跑逻辑
  if (autosaveTimer) {
    clearTimeout(autosaveTimer)
    autosaveTimer = undefined
  }
  if (autosaveSavedHintTimer) {
    clearTimeout(autosaveSavedHintTimer)
    autosaveSavedHintTimer = undefined
  }
})
</script>

<template>
  <div class="knowledge-shell">
    <header class="knowledge-titlebar">{{ t('knowledge.title') }}</header>

    <div
      class="knowledge-page"
      :class="{ 'is-loading': loading, 'is-sidebar-collapsed': sidebarCollapsed, 'is-sidebar-narrow': isSidebarNarrow }"
      :style="knowledgePageStyle"
    >
      <aside v-show="!sidebarCollapsed" class="knowledge-sidebar">
        <!-- 目录栏右侧的可拖拽改宽度把手（仅在展开时可见，折叠后自动隐藏） -->
        <button
          v-show="!sidebarCollapsed"
          class="sidebar-resize-handle"
          type="button"
          :aria-label="t('knowledge.resizeSidebar')"
          :title="t('knowledge.resizeSidebar')"
          @mousedown="startSidebarResize"
        />
        <div class="directory-heading">
          <div class="directory-title">
            <strong>{{ t('knowledge.catalog') }}</strong>
            <span class="directory-divider" />
          </div>
          <div class="directory-actions">
            <a-tooltip :title="t('knowledge.refreshDirectory')">
              <button
                class="icon-button refresh-button"
                type="button"
                :aria-label="t('knowledge.refreshDirectory')"
                :disabled="scanning"
                @click="refreshWorkspace"
              >
                <ReloadOutlined :class="{ spinning: scanning }" />
              </button>
            </a-tooltip>
            <a-tooltip :title="t('knowledge.new')">
              <button
                class="icon-button"
                type="button"
                :aria-label="t('knowledge.new')"
                @click="openCreateModal('document',0)"
              >
                <FileTextOutlined />
              </button>
            </a-tooltip>
            <a-tooltip :title="t('knowledge.newFolder')">
              <button
                class="icon-button"
                type="button"
                :aria-label="t('knowledge.newFolder')"
                @click="openCreateModal('folder',0)"
              >
                <FolderAddOutlined />
              </button>
            </a-tooltip>
          </div>
        </div>

        <!-- S-UI-07: 空知识库不自动创建内容，仅给创建入口与引导提示 -->
        <div v-if="knowledgeEmpty && !searchKeyword.trim()" class="sidebar-empty-action">
          <button class="create-first-button" type="button" @click="createDocument()">
            <PlusOutlined />
            <span>{{ t('knowledge.emptyCreateFirst') }}</span>
          </button>
        </div>
        <div v-else class="sidebar-search">
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
                :draggable="true"
                :tree-data="directoryTreeData"
                :selected-keys="trashVisible || !selectedTreeKey ? [] : [selectedTreeKey]"
                :expanded-keys="expandedKeys"
                @select="selectDirectory"
                @drop="moveDirectoryNode"
                @expand="(keys: string[]) => { expandedKeys = keys }"
              >
                <template #title="{ title, key, kind, entityId, count }">
                  <div class="directory-node" :class="`node-${kind}`">
                    <FolderOutlined v-if="kind === 'folder'" class="node-icon" />
                    <FileTextOutlined v-else class="node-icon document-icon" />
                    <span class="node-title">{{ title }}</span>
                    <span v-if="kind === 'folder'" class="node-count">{{ count }}</span>
                    <!-- 文件夹 / 文档 hover 弹出三点按钮：所有操作统一收纳到 dropdown -->
                    <a-dropdown
                      v-if="kind === 'folder' || kind === 'document'"
                      :trigger="['click']"
                      placement="bottomRight"
                    >
                      <span
                        class="node-actions"
                        role="button"
                        tabindex="0"
                        :aria-label="t('knowledge.moreActionsTip')"
                        @click.stop
                      >
                        <MoreOutlined />
                      </span>
                      <template #overlay>
                        <a-menu @click.stop>
                          <template v-if="kind === 'folder'">
                            <a-menu-item
                              key="new-doc"
                              @click="openCreateModal('document', Number(entityId))"
                            >
                              <FileTextOutlined /> {{ t('knowledge.createDocumentIn') }}
                            </a-menu-item>
                            <a-menu-item
                              key="new-folder"
                              @click="openCreateModal('folder', Number(entityId))"
                            >
                              <FolderAddOutlined /> {{ t('knowledge.createFolderIn') }}
                            </a-menu-item>
                            <a-menu-item
                              key="rename"
                              @click="selectedFolderID = Number(entityId); renameSelectedFolder()"
                            >
                              <EditOutlined /> {{ t('knowledge.rename') }}
                            </a-menu-item>
                            <a-menu-item
                              v-if="String(key).startsWith('folder:')"
                              key="delete"
                              danger
                              @click="selectedFolderID = Number(entityId); deleteSelectedFolder()"
                            >
                              <DeleteOutlined /> {{ t('knowledge.deleteFolder') }}
                            </a-menu-item>
                          </template>
                          <template v-else>
                            <a-menu-item
                              key="rename-doc"
                              @click="handleRenameDocument(String(entityId))"
                            >
                              <EditOutlined /> {{ t('knowledge.rename') }}
                            </a-menu-item>
                            <a-menu-item
                              key="delete-doc"
                              danger
                              @click="handleDeleteDocument(String(entityId))"
                            >
                              <DeleteOutlined /> {{ t('knowledge.moveToTrash') }}
                            </a-menu-item>
                          </template>
                        </a-menu>
                      </template>
                    </a-dropdown>
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
          class="root-entry"
          type="button"
          :aria-label="t('knowledge.knowledgeRootLabel')"
          @click="openRootModal"
        >
          <span class="root-entry-label">{{ t('knowledge.knowledgeRootLabel') }}：</span>
          <span class="root-breadcrumb">
            <template v-for="(segment, index) in rootBreadcrumbSegments" :key="index">
              <span class="root-breadcrumb-segment">{{ segment }}</span>
              <span
                v-if="index < rootBreadcrumbSegments.length - 1"
                class="root-breadcrumb-separator"
              > > </span>
            </template>
          </span>
        </button>

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

      <main v-else-if="draft.uuid" class="editor-workspace" :class="{ 'is-inspector-collapsed': !inspectorOpen }">
        <!-- 面包屑导航行 -->
        <nav class="editor-breadcrumb-row" aria-label="文件路径导航">
          <div class="editor-breadcrumb-leading">
            <!-- 左侧目录栏折叠 / 展开切换（固定在 nav 最左侧） -->
            <a-tooltip
              :title="sidebarCollapsed ? t('knowledge.expandSidebar') : t('knowledge.collapseSidebar')"
            >
              <button
                class="icon-button sidebar-nav-toggle"
                type="button"
                :aria-label="sidebarCollapsed ? t('knowledge.expandSidebar') : t('knowledge.collapseSidebar')"
                @click="toggleSidebar"
              >
                <component :is="sidebarCollapsed ? MenuUnfoldOutlined : MenuFoldOutlined" />
              </button>
            </a-tooltip>
            <div class="editor-breadcrumb">
              <template v-for="(item, index) in visibleBreadcrumbs" :key="item.id">
                <!-- 溢出省略号：显示省略号按钮，点击展开全部路径 -->
                <span v-if="item.kind === 'overflow'" class="breadcrumb-overflow">
                    <a-tooltip :title="documentBreadcrumbs.map(i => fullBreadcrumbName(i)).join(' / ')">
                      <button
                        type="button"
                        class="crumb-overflow-btn"
                        :aria-label="t('knowledge.breadcrumbOverflow')"
                        @click="showFullBreadcrumb"
                      >{{ t('knowledge.breadcrumbOverflow') }}</button>
                    </a-tooltip>
                  </span>
                <button
                  v-else-if="item.kind !== 'document'"
                  type="button"
                  class="crumb crumb-link"
                  :title="fullBreadcrumbName(item)"
                  @click="navigateBreadcrumb(item)"
                >{{ breadcrumbDisplayName(item) }}</button>
                <span v-else class="crumb crumb-current" :title="fullBreadcrumbName(item)">{{ breadcrumbDisplayName(item) }}</span>
                <span v-if="index < visibleBreadcrumbs.length - 1" class="crumb-separator"> / </span>
              </template>
            </div>
          </div>

          <div class="editor-header-actions">
            <!-- 自动保存状态指示：pending/saving 时短暂展示文案，saved 后 2 秒回 idle，error 时点击重试 -->
            <button
              v-if="autosaveState === 'error'"
              type="button"
              class="autosave-indicator autosave-indicator--error"
              :title="autosaveErrorMessage"
              :aria-label="t('knowledge.autosaveFailed')"
              @click="retryAutosave"
            >
              <ReloadOutlined />
              <span>{{ t('knowledge.autosaveFailed') }}</span>
            </button>
            <span
              v-else-if="autosaveHint"
              class="autosave-indicator"
              :class="{
                'autosave-indicator--pending': autosaveState === 'pending',
                'autosave-indicator--saving': autosaveState === 'saving',
                'autosave-indicator--saved': autosaveState === 'saved',
              }"
              aria-live="polite"
            >
              <span
                v-if="autosaveState === 'saving'"
                class="autosave-spinner"
                aria-hidden="true"
              />
              <span>{{ autosaveHint }}</span>
            </span>
            <!-- 更多操作：重命名 / 移到回收站（替代原 editor-header 中的入口） -->
            <a-dropdown :trigger="['click']" placement="bottomRight">
              <button class="icon-button" type="button" :aria-label="t('knowledge.moreActions')">
                <MoreOutlined />
              </button>
              <template #overlay>
                <a-menu>
                  <a-menu-item key="rename" @click="startRename">
                    <EditOutlined /> {{ t('knowledge.rename') }}
                  </a-menu-item>
                  <a-menu-item key="delete" danger @click="deleteDocument">
                    <DeleteOutlined /> {{ t('knowledge.moveToTrash') }}
                  </a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
            <!-- 抽屉展开 / 折叠切换 -->
            <a-tooltip :title="inspectorOpen ? t('knowledge.inspectorCollapse') : t('knowledge.inspectorExpand')">
              <button
                class="icon-button"
                type="button"
                :aria-label="inspectorOpen ? t('knowledge.inspectorCollapse') : t('knowledge.inspectorExpand')"
                @click="toggleInspector"
              >
                <component :is="inspectorOpen ? MenuFoldOutlined : MenuUnfoldOutlined" />
              </button>
            </a-tooltip>
          </div>
        </nav>


        <!--
          S-UI-09: md 与 txt 统一使用 vditor 即时渲染（边写边渲染），移除原「编辑/分栏/预览」三态；
          S-UI-11/S-UI-13: 浮动工具栏由编辑器内部维护，此处注入「添加给 Agent」入口。
        -->
        <section class="editor-body">
          <MarkdownEditor
            ref="editorRef"
            v-model="draft.content"
            floating-toolbar
            :placeholder="t('knowledge.editorPlaceholder')"
            @selection-change="handleEditorSelection"
          >
            <template #floating-actions>
              <!-- mousedown.prevent 保留编辑器选区；Enter/Space 覆盖键盘触发（S-UI-13） -->
              <button
                v-if="editorSelectionText"
                type="button"
                class="floating-agent-button"
                @mousedown.prevent="openAgentReference"
                @keydown.enter.prevent="openAgentReference"
                @keydown.space.prevent="openAgentReference"
              >
                <RobotOutlined />
                <span>{{ t('knowledge.agentReferenceAdd') }}</span>
              </button>
            </template>
          </MarkdownEditor>
        </section>
      </main>

      <!-- 根目录视图：展示 folder_id=0 的文档和 parent_id=0 的文件夹 -->
      <KnowledgeFolderDetail
        v-else-if="activeFolderID === 0"
        :folder-id="0"
        :folder-name="t('knowledge.breadcrumbRoot')"
        :breadcrumbs="[{ id: 0, name: t('knowledge.breadcrumbRoot') }]"
        :child-folders="rootViewFolders"
        :child-documents="rootViewDocuments"
        :loading="loading"
        :sidebar-collapsed="sidebarCollapsed"
        @open-folder="handleDetailOpenFolder"
        @open-document="handleDetailOpenDocument"
        @create-document="createDocument"
        @create-txt-document="createTxtDocument"
        @create-folder="createFolderFromDetail"
        @open-create-modal="(type, folderId) => openCreateModal(type, folderId)"
        @toggle-inspector="toggleInspector"
        @toggle-sidebar="toggleSidebar"
        @delete-folder="handleDetailDeleteFolder"
        @navigate="handleDetailNavigate"
      />

      <!-- S-UI-14: 文件夹详情列表页（独立工作区，仅直接子项、逐层下钻） -->
      <KnowledgeFolderDetail
        v-else-if="activeFolder"
        :folder-id="activeFolderID"
        :folder-name="activeFolder.name"
        :breadcrumbs="activeFolderBreadcrumbs"
        :child-folders="activeFolderChildren.folders"
        :child-documents="activeFolderChildren.documents"
        :loading="loading"
        :sidebar-collapsed="sidebarCollapsed"
        @open-folder="handleDetailOpenFolder"
        @open-document="handleDetailOpenDocument"
        @create-document="createDocument"
        @create-txt-document="createTxtDocument"
        @create-folder="createFolderFromDetail"
        @open-create-modal="(type, folderId) => openCreateModal(type, folderId)"
        @toggle-inspector="toggleInspector"
        @toggle-sidebar="toggleSidebar"
        @delete-folder="handleDetailDeleteFolder"
        @navigate="handleDetailNavigate"
      />

      <main v-else class="knowledge-empty">
        <div class="empty-state">
          <div class="empty-illustration" aria-hidden="true">
            <FolderOutlined class="empty-folder" />
            <FileTextOutlined class="empty-document" />
            <span class="empty-check"><CheckOutlined /></span>
          </div>
          <h2>{{ knowledgeEmpty ? t('knowledge.emptyNoContent') : t('knowledge.emptyTitle') }}</h2>
          <p v-if="!knowledgeEmpty">{{ t('knowledge.emptyDescription') }}</p>
        </div>
      </main>

      <aside v-if="!trashVisible && draft.uuid && inspectorOpen" class="knowledge-inspector">
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
              v-for="(item, index) in outline"
              :key="item.key"
              class="outline-item"
              :style="{ paddingLeft: `${12 + (item.level - 1) * 12}px` }"
              @click="scrollToHeading(index)"
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

    <!-- 通用「新建文档 / 新建文件夹」弹窗：输入名称（无后缀默认 .md），显示完整路径，Enter 确认 -->
    <a-modal
      v-model:open="createModalVisible"
      :title="createModalType === 'folder'
        ? t('knowledge.createFolderDialogTitle')
        : t('knowledge.createDocumentDialogTitle')"
      :ok-text="t('knowledge.create')"
      :cancel-text="t('knowledge.cancel')"
      :confirm-loading="createModalLoading"
      :ok-button-props="{ disabled: !createModalName.trim() || createModalLoading }"
      :cancel-button-props="{ disabled: createModalLoading }"
      @ok="submitCreateModal"
      @cancel="cancelCreateModal"
    >
      <a-form layout="vertical">
        <a-form-item
          :label="createModalType === 'folder'
            ? t('knowledge.folderName')
            : t('knowledge.documentName')"
        >
          <a-input
            v-model:value="createModalName"
            autofocus
            :placeholder="createModalType === 'folder'
              ? t('knowledge.folderNamePlaceholder')
              : t('knowledge.documentNamePlaceholder')"
            @press-enter="submitCreateModal"
          />
        </a-form-item>
        <a-form-item
          v-if="createModalType === 'document'"
          :style="{ marginBottom: '8px' }"
        >
          <span class="create-modal-ext-hint">{{ t('knowledge.defaultExtensionHint') }}</span>
        </a-form-item>
        <a-form-item :label="t('knowledge.targetPathLabel')">
          <div class="create-modal-path-preview">
            {{ createModalTargetPath }}
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- S-UI-01: 设置知识库目录弹窗（S-UI-17: 迁移语义 + 二次确认 + 全流程反馈） -->
    <a-modal
      v-model:open="rootModalVisible"
      :title="t('knowledge.knowledgeRootDialogTitle')"
      :confirm-loading="rootModalLoading"
      :closable="!rootModalLoading"
      :mask-closable="false"
      :keyboard="!rootModalLoading"
      :ok-text="t('knowledge.confirm')"
      :cancel-text="t('knowledge.cancel')"
      :ok-button-props="{ disabled: !pendingRootDir.trim() || rootModalLoading }"
      :cancel-button-props="{ disabled: rootModalLoading }"
      @ok="confirmSetKnowledgeRoot"
      @cancel="cancelRootModal"
    >
      <p class="root-modal-description">{{ t('knowledge.knowledgeRootDialogDescription') }}</p>
      <div class="root-current-path">
        <div class="root-current-path-label">{{ t('knowledge.knowledgeRootCurrent') }}</div>
        <div class="root-breadcrumb root-breadcrumb-modal">
          <template v-for="(segment, index) in modalRootBreadcrumbSegments" :key="index">
            <span class="root-breadcrumb-segment">{{ segment }}</span>
            <span
              v-if="index < modalRootBreadcrumbSegments.length - 1"
              class="root-breadcrumb-separator"
            >></span>
          </template>
        </div>
      </div>
      <div v-if="rootModalLoading" class="root-migrate-state">
        <a-spin size="small" />
        <span>{{ t('knowledge.migrateRunning') }}</span>
      </div>
      <button
        v-if="isDesktopRuntime()"
        class="root-select-button"
        type="button"
        :disabled="rootModalLoading"
        @click="chooseRootDirectory"
      >
        <FolderOpenOutlined /> {{ t('knowledge.knowledgeRootSelectFolder') }}
      </button>
      <a-form v-else layout="vertical">
        <a-form-item :label="t('knowledge.knowledgeRootManualPath')">
          <a-input
            v-model:value="pendingRootDir"
            :placeholder="t('knowledge.knowledgeRootPathPlaceholder')"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- S-UI-03: 删除文件夹双选项弹窗 -->
    <a-modal
      v-model:open="deleteFolderModalVisible"
      :title="t('knowledge.deleteFolderTitle', { name: deleteFolderTarget?.name || '' })"
      :confirm-loading="deleteFolderModalLoading"
      :footer="null"
      @cancel="deleteFolderModalVisible = false"
    >
      <div class="delete-folder-options">
        <button
          class="delete-folder-option keep"
          type="button"
          :disabled="deleteFolderModalLoading"
          @click="confirmDeleteFolder('keep')"
        >
          <strong>{{ t('knowledge.deleteFolderKeep') }}</strong>
          <span>{{ t('knowledge.deleteFolderKeepDescription') }}</span>
        </button>
        <button
          class="delete-folder-option purge"
          type="button"
          :disabled="deleteFolderModalLoading"
          @click="confirmDeleteFolder('purge')"
        >
          <strong>{{ t('knowledge.deleteFolderPurge') }}</strong>
          <span>{{ t('knowledge.deleteFolderPurgeDescription') }}</span>
        </button>
      </div>
      <div class="delete-folder-modal-footer">
        <a-button :disabled="deleteFolderModalLoading" @click="deleteFolderModalVisible = false">
          {{ t('knowledge.cancel') }}
        </a-button>
      </div>
    </a-modal>

    <!-- S-UI-13: 选中内容 → 「添加给 Agent」→ 投递到对话输入区 -->
    <KnowledgeAgentReferencePopover
      v-model:open="agentReferenceOpen"
      :source-name="documentFileName()"
      :fragment="agentReferenceFragment"
      @confirm="handleAgentReferenceSend"
    />

    <!-- 文档重命名弹窗（取代原 editor-header 内的标题就地编辑）。提交只更新 draft.title，
         由 snapshot 防抖自动保存落盘；与新建/编辑文档共用 title 校验。 -->
    <a-modal
      v-model:open="renameModalVisible"
      :title="t('knowledge.renameDialogTitle')"
      :ok-text="t('knowledge.renameConfirm')"
      :cancel-text="t('knowledge.cancel')"
      :mask-closable="false"
      @ok="submitRename"
      @cancel="cancelRename"
    >
      <a-form layout="vertical">
        <a-form-item>
          <a-input
            v-model:value="renameInputValue"
            :placeholder="t('knowledge.renameDialogPlaceholder')"
            autofocus
            @press-enter="submitRename"
          />
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
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  padding: 0 24px 20px;
  border-right: 1px solid #e6e8eb;
  background: #fff;
}

/* 可拖拽调整宽度的把手 */
.sidebar-resize-handle {
  position: absolute;
  top: 0;
  right: -2px;
  width: 4px;
  height: 100%;
  cursor: col-resize;
  background: transparent;
  border: 0;
  padding: 0;
  z-index: 10;
  transition: background 0.18s ease;
}

.sidebar-resize-handle:hover,
.sidebar-resize-handle:focus-visible {
  background: rgba(49, 87, 226, 0.35);
}

.sidebar-resize-handle:focus-visible {
  outline: none;
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

.sidebar-empty-action {
  flex: 0 0 auto;
  padding-bottom: 14px;
}

/* S-UI-07: 空知识库的创建入口（不自动创建任何内容，仅给按钮与引导） */
.create-first-button {
  display: flex;
  width: 100%;
  height: 40px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid #d9d9d9;
  border-radius: 12px;
  color: #595959;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  transition: color 0.18s ease, border-color 0.18s ease, background 0.18s ease;
}

.create-first-button:hover {
  border-color: #3157e2;
  color: #3157e2;
  background: #f9fafb;
}

.create-first-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
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
.folder-detail,
.knowledge-empty {
  min-width: 0;
  min-height: 0;
}

/* S-UI-14: 详情页与回收站一致占满主区与右侧列（属性面板仅编辑器工作区展示） */
.folder-detail,
.aggregate-view {
  grid-column: 2 / 4;
}

.editor-workspace {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  /* 明确占据中间列，由 inspectorOpen 控制 inspector aside 是否显示 */
  grid-column: 2;
}

/* S-UI-14: 右侧属性面板收起时，让编辑器工作区横跨中间与右侧两列，
   避免出现 300px 的空白列。展开属性面板时恢复到默认单列宽度。 */
.editor-workspace.is-inspector-collapsed {
  grid-column: 2 / 4;
}

/* S-AS-04: 顶部 nav 右侧的自动保存指示器 */
.autosave-indicator {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  max-width: 220px;
  padding: 0 10px;
  border: 0;
  border-radius: 6px;
  color: #8e939b;
  font-size: 12px;
  line-height: 26px;
  white-space: nowrap;
}

.autosave-indicator > span:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
}

.autosave-indicator--pending {
  color: #6c7178;
}

.autosave-indicator--saving {
  color: #3157e2;
}

.autosave-indicator--saved {
  color: #5d6573;
}

.autosave-indicator--error {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  max-width: 220px;
  padding: 0 10px;
  border: 0;
  border-radius: 6px;
  color: #ff6b4a;
  background: transparent;
  font-size: 12px;
  line-height: 26px;
  cursor: pointer;
  white-space: nowrap;
}

.autosave-indicator--error:hover {
  color: #d9482b;
  background: #fff1ec;
}

.autosave-spinner {
  display: inline-block;
  width: 12px;
  height: 12px;
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: autosave-spin 0.8s linear infinite;
}

@keyframes autosave-spin {
  to {
    transform: rotate(360deg);
  }
}

/* S-UI-09: 编辑器工作区由 vditor 即时渲染承载（移除原「编辑/分栏/预览」三态与对应样式） */
.editor-body {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

/* S-UI-13: 浮动工具栏右端的「添加给 Agent」入口 */
.floating-agent-button {
  display: inline-flex;
  height: 24px;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  color: #3157e2;
  background: #e5efff;
  cursor: pointer;
  font-size: 12px;
  white-space: nowrap;
  transition: color 0.18s ease, background 0.18s ease;
}

.floating-agent-button:hover {
  color: #fff;
  background: #3157e2;
}

.floating-agent-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
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

/* 通用创建弹窗 */
.create-modal-ext-hint {
  color: #8c8c8c;
  font-size: 12px;
}

.create-modal-path-preview {
  padding: 8px 12px;
  border-radius: 6px;
  color: #555a61;
  background: #f5f6f8;
  font-size: 13px;
  word-break: break-all;
}

/* 编辑器头部面包屑行：与 .detail-breadcrumb 排版保持一致，避免点击文件/文件夹切换时高度跳动 */
.editor-breadcrumb-row {
  display: flex;
  height: 48px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  font-size: 14px;
}

/* 面包屑行左侧容器：包含 sidebar 折叠按钮 + 路径导航 */
.editor-breadcrumb-leading {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  overflow: hidden;
}

.sidebar-nav-toggle {
  flex: 0 0 auto;
}

/* 路径部分样式：参考 detail-breadcrumb-path */
.editor-breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow: hidden;
  font-size: 14px;
}

/* crumb 基础样式：参考 .crumb，颜色与字号集中到基础类 */
.editor-breadcrumb .crumb {
  flex: 0 0 auto;
  padding: 0;
  border: 0;
  background: transparent;
  white-space: nowrap;
  color: #8c8c8c;
}

/* 溢出省略号按钮：编辑器特有，用于长路径折叠 */
.editor-breadcrumb .crumb-overflow-btn {
  flex: 0 0 auto;
  padding: 2px 6px;
  border: 0;
  border-radius: 4px;
  color: #8c8c8c;
  background: #f0f1f3;
  cursor: pointer;
  font-size: 12px;
}

.editor-breadcrumb .crumb-overflow-btn:hover {
  color: #3157e2;
  background: #e5efff;
}

/* 链接样式：参考 .crumb-link，光标和颜色变体由链接单独提供 */
.editor-breadcrumb .crumb-link {
  cursor: pointer;
}

.editor-breadcrumb .crumb-link:hover {
  color: #3157e2;
}

.editor-breadcrumb .crumb-link:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

/* 当前页样式：参考 .crumb-current，使用更深的强调色和加粗 */
.editor-breadcrumb .crumb-current {
  overflow: hidden;
  text-overflow: ellipsis;
  color: #262626;
  font-weight: 600;
}

/* 分隔符样式：参考 .crumb-separator，保留 flex 防止溢出时被压缩 */
.editor-breadcrumb .crumb-separator {
  color: #bfbfbf;
  flex: 0 0 auto;
}

.breadcrumb-overflow {
  flex: 0 0 auto;
}

.editor-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}

/* 目录树三点按钮 */
.directory-node .node-actions {
  display: none;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 4px;
  color: #797e87;
  cursor: pointer;
  font-size: 14px;
  user-select: none;
}

.directory-node:hover .node-actions {
  display: flex;
}

.directory-node:hover .node-count {
  display: none;
}

.directory-node .node-actions:hover {
  color: #3157e2;
  background: #f1f4fa;
}

.directory-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.refresh-button {
  width: 28px;
  height: 28px;
  color: #5f6368;
  font-size: 15px;
}

.refresh-button:disabled {
  opacity: 0.5;
}

.spinning {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.root-entry {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  margin: 0 0 8px;
  padding: 8px 12px;
  border: 0;
  border-radius: 8px;
  color: #595959;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  text-align: left;
  transition: background 0.16s ease;
}

.root-entry:hover {
  background: #f3f6fb;
}

.root-entry-label {
  flex: 0 0 auto;
  color: #8c8c8c;
}

.root-breadcrumb {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.root-breadcrumb-segment {
  color: #3157e2;
  white-space: nowrap;
}

.root-breadcrumb-segment:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
}

.root-breadcrumb-separator {
  flex: 0 0 auto;
  color: #bfbfbf;
}

.root-modal-description {
  margin: 0 0 16px;
  color: #595959;
  font-size: 14px;
  line-height: 1.6;
}

.root-current-path {
  margin-bottom: 16px;
  padding: 12px 14px;
  border-radius: 8px;
  background: #f5f6f8;
}

.root-current-path-label {
  margin-bottom: 6px;
  color: #8c8c8c;
  font-size: 12px;
}

/* S-UI-17: 迁移进行中的加载反馈（同时由确认按钮 loading 与弹窗禁用兜底防重复提交） */
.root-migrate-state {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  color: #8c8c8c;
  font-size: 13px;
}

.root-breadcrumb-modal .root-breadcrumb-segment {
  color: #262626;
  font-size: 14px;
}

.root-select-button {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px;
  border: 1px dashed #d9d9d9;
  border-radius: 8px;
  color: #3157e2;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.16s ease, border-color 0.16s ease;
}

.root-select-button:hover {
  border-color: #3157e2;
  background: #f5f6f8;
}

.root-select-button:disabled {
  cursor: default;
  opacity: 0.55;
}

.delete-folder-options {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 20px;
}

.delete-folder-option {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border: 1px solid #d9d9d9;
  border-radius: 8px;
  color: #262626;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  text-align: left;
  transition: border-color 0.16s ease, background 0.16s ease;
}

.delete-folder-option:hover {
  border-color: #3157e2;
  background: #f5f6f8;
}

.delete-folder-option:disabled {
  cursor: default;
  opacity: 0.55;
}

.delete-folder-option strong {
  font-weight: 600;
}

.delete-folder-option span {
  color: #8c8c8c;
  font-size: 13px;
  line-height: 1.5;
}

.delete-folder-option.purge:hover {
  border-color: #ff4d4f;
  background: #fff2f0;
}

.delete-folder-modal-footer {
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 1320px) {
  .knowledge-page {
    grid-template-columns: 320px minmax(420px, 1fr);
  }

  .knowledge-inspector {
    display: none;
  }

  .trash-workspace,
  .folder-detail,
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

/* 窄屏时（宽度 < 240px）隐藏目录标题文字，仅保留图标区 */
.knowledge-page.is-sidebar-narrow .directory-title span:not(.directory-divider) {
  display: none;
}

/* 折叠状态：移除 sidebar 可见边框，防止 0px 时仍有边框线残留 */
.knowledge-page.is-sidebar-collapsed .knowledge-sidebar {
  border-right-color: transparent;
}
</style>
