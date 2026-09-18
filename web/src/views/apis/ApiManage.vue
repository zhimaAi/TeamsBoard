<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'ant-design-vue'
import {
  CaretRightOutlined,
  CheckOutlined,
  CloseCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  FileTextOutlined,
  FolderOutlined,
  LinkOutlined,
  MoreOutlined,
  PlusOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import { useDocumentTitle } from '@/composables/useDocumentTitle'
import moveIcon from '@/assets/api-move.svg'
import responseEmptyIcon from '@/assets/api-response-empty.svg'
import saveIcon from '@/assets/api-save.svg'
import { useAppI18n } from '@/i18n'

const { t, locale } = useAppI18n()
const variableSyntaxExample = '{{name}}'

type Collection = {
  id: number
  name: string
  description: string
}

type Folder = {
  id: number
  collection_id: number
  parent_id: number
  name: string
  description: string
  sort_order: number
}

type KeyValue = {
  key: string
  value: string
  enabled: boolean
  type?: string
}

type AuthConfig = {
  type: 'none' | 'bearer' | 'basic' | 'api_key'
  token?: string
  username?: string
  password?: string
  key?: string
  value?: string
  add_to?: 'header' | 'query'
  has_secret?: boolean
}

type APIRequest = {
  id: number
  collection_id: number
  folder_id: number
  name: string
  method: string
  url: string
  headers: Record<string, string>
  query: KeyValue[]
  auth: AuthConfig
  body: string
  body_type: 'none' | 'json' | 'text' | 'form' | 'multipart'
  body_form: KeyValue[]
  environment_id: number
  description: string
  created_at: number
  updated_at: number
}

type Environment = {
  id: number
  collection_id: number
  name: string
  description: string
  variables: Record<string, string>
}

type Execution = {
  id?: number
  run_uuid?: string
  status: number
  headers: Record<string, string[]>
  body: string
  duration_ms: number
  body_truncated: boolean
  error_summary?: string
  started_at?: number
  method?: string
  url?: string
}

type RunHistory = {
  id: number
  run_uuid: string
  response_status: number
  response_headers: Record<string, string[]>
  response_body: string
  response_body_truncated: boolean
  duration_ms: number
  error_summary: string
  started_at: number
  method: string
  url: string
}

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const collapsedFoldersStorageKey = 'goteams.api-manager.collapsed-folders'
const loading = ref(false)
const saving = ref(false)
const executing = ref(false)
const collections = ref<Collection[]>([])
const folders = ref<Folder[]>([])
const requests = ref<APIRequest[]>([])
const environments = ref<Environment[]>([])
const histories = ref<RunHistory[]>([])
const selectedCollectionID = ref<number>()
const selectedRequestID = ref<number>()
const selectedFolderID = ref(0)
const collapsedFolderIDs = ref<number[]>(readCollapsedFolderIDs())
const draggedFolderID = ref<number>()
const dragOverFolderID = ref<number>()
const folderDropPosition = ref<'before' | 'after'>('before')
const folderSortSaving = ref(false)
const searchKeyword = ref('')
const environmentKeyword = ref('')
const workspaceTab = ref<'apis' | 'environments'>('apis')
const responseTab = ref('response')
const requestTab = ref('params')
const execution = ref<Execution | null>(null)

const workspaceTabTitles = computed(() => ({
  apis: t('apis.title'),
  environments: t('apis.environments'),
}))
const workspaceTabTitle = computed(() =>
  selectedCollectionID.value ? workspaceTabTitles.value[workspaceTab.value] : undefined,
)

useDocumentTitle(workspaceTabTitle)

const collectionModal = ref(false)
const collectionModalMode = ref<'create' | 'edit'>('create')
const folderModal = ref(false)
const renameFolderModal = ref(false)
const requestModal = ref(false)
const curlModal = ref(false)
const moveModal = ref(false)
const collectionModalName = ref('')
const collectionModalDescription = ref('')
const newFolderName = ref('')
const renameFolderName = ref('')
const renameFolderTarget = ref<Folder>()
const newRequestName = ref('')
const curlText = ref('')
const moveFolderID = ref(0)

const editingEnvironmentID = ref<number>()
const envNameInputRef = ref<{ $el?: HTMLElement }>()
const environmentForm = reactive({
  name: '',
  description: '',
  variables: [] as KeyValue[],
})

const form = reactive({
  name: '',
  method: 'GET',
  url: '',
  headers: [] as KeyValue[],
  query: [] as KeyValue[],
  auth: { type: 'none' } as AuthConfig,
  body: '',
  bodyType: 'none' as APIRequest['body_type'],
  bodyForm: [] as KeyValue[],
  environmentID: 0,
  description: '',
})

const visibleEnvironments = computed(() => {
  const keyword = environmentKeyword.value.trim().toLowerCase()
  if (!keyword) return environments.value
  return environments.value.filter((item) =>
    [item.name, item.description].some((value) => String(value || '').toLowerCase().includes(keyword)),
  )
})

const visibleRequests = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return requests.value
  return requests.value.filter((item) =>
    [item.name, item.url, item.method, item.description].some((value) =>
      String(value || '').toLowerCase().includes(keyword),
    ),
  )
})

const rootRequests = computed(() => visibleRequests.value.filter((item) => !item.folder_id))

const prettyResponseBody = computed(() => {
  const body = execution.value?.body || ''
  if (!body) return t('apis.emptyResponse')
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
})

const responseHeaderRows = computed(() =>
  Object.entries(execution.value?.headers || {}).map(([key, value]) => ({
    key,
    value: Array.isArray(value) ? value.join('\n') : String(value),
  })),
)

function requestsInFolder(folderID: number) {
  return visibleRequests.value.filter((item) => item.folder_id === folderID)
}

function folderRequestCount(folderID: number) {
  return requests.value.filter((item) => item.folder_id === folderID).length
}

function readCollapsedFolderIDs(): number[] {
  if (typeof window === 'undefined') return []
  try {
    const value = JSON.parse(window.localStorage.getItem(collapsedFoldersStorageKey) || '[]')
    return Array.isArray(value) ? value.filter((id): id is number => Number.isInteger(id) && id > 0) : []
  } catch {
    return []
  }
}

function saveCollapsedFolderIDs() {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(collapsedFoldersStorageKey, JSON.stringify(collapsedFolderIDs.value))
}

function isFolderExpanded(folderID: number) {
  return !collapsedFolderIDs.value.includes(folderID)
}

function toggleFolder(folderID: number) {
  collapsedFolderIDs.value = isFolderExpanded(folderID)
    ? [...collapsedFolderIDs.value, folderID]
    : collapsedFolderIDs.value.filter((id) => id !== folderID)
  saveCollapsedFolderIDs()
}

function startFolderDrag(event: DragEvent, folderID: number) {
  if (folderSortSaving.value) {
    event.preventDefault()
    return
  }
  draggedFolderID.value = folderID
  dragOverFolderID.value = undefined
  event.dataTransfer?.setData('text/plain', String(folderID))
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}

function updateFolderDropTarget(event: DragEvent, folderID: number) {
  if (!draggedFolderID.value || draggedFolderID.value === folderID) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  dragOverFolderID.value = folderID
  folderDropPosition.value = event.clientY < rect.top + rect.height / 2 ? 'before' : 'after'
}

function clearFolderDragState() {
  draggedFolderID.value = undefined
  dragOverFolderID.value = undefined
  folderDropPosition.value = 'before'
}

async function dropFolder(event: DragEvent, targetFolderID: number) {
  event.preventDefault()
  const sourceFolderID = draggedFolderID.value
  if (!sourceFolderID || sourceFolderID === targetFolderID || !selectedCollectionID.value) {
    clearFolderDragState()
    return
  }

  const previous = [...folders.value]
  const reordered = folders.value.filter((folder) => folder.id !== sourceFolderID)
  const targetIndex = reordered.findIndex((folder) => folder.id === targetFolderID)
  if (targetIndex < 0) {
    clearFolderDragState()
    return
  }
  const insertIndex = targetIndex + (folderDropPosition.value === 'after' ? 1 : 0)
  const source = previous.find((folder) => folder.id === sourceFolderID)
  if (!source) {
    clearFolderDragState()
    return
  }
  reordered.splice(insertIndex, 0, source)
  reordered.forEach((folder, index) => { folder.sort_order = index })
  folders.value = reordered
  clearFolderDragState()

  folderSortSaving.value = true
  try {
    await apiClient.put('/apis/folders/reorder', {
      collection_id: selectedCollectionID.value,
      folder_ids: reordered.map((folder) => folder.id),
    })
  } catch (error) {
    folders.value = previous
    message.error(error instanceof Error ? error.message : t('apis.folderSortFailed'))
  } finally {
    folderSortSaving.value = false
  }
}

function emptyRow(): KeyValue {
  return { key: '', value: '', enabled: true }
}

function mapToRows(value: Record<string, string> | undefined): KeyValue[] {
  const rows = Object.entries(value || {}).map(([key, item]) => ({ key, value: item, enabled: true }))
  return rows.length ? rows : [emptyRow()]
}

function rowsToMap(rows: KeyValue[]): Record<string, string> {
  return Object.fromEntries(
    rows
      .filter((item) => item.enabled && item.key.trim())
      .map((item) => [item.key.trim(), item.value]),
  )
}

function normalizeRows(rows: KeyValue[] | undefined): KeyValue[] {
  return rows?.length ? rows.map((item) => ({ ...item, enabled: item.enabled !== false })) : [emptyRow()]
}

function addRow(rows: KeyValue[]) {
  rows.push(emptyRow())
}

function removeRow(rows: KeyValue[], index: number) {
  rows.splice(index, 1)
  if (!rows.length) rows.push(emptyRow())
}

async function loadCollections() {
  const result = await apiClient.get<{ data: Collection[] }>('/apis/collections')
  collections.value = result.data || []
  if (!selectedCollectionID.value && collections.value.length) {
    await selectCollection(collections.value[0].id)
  }
}

async function loadCollectionData(collectionID: number) {
  const [folderResult, requestResult, environmentResult] = await Promise.all([
    apiClient.get<{ data: Folder[] }>('/apis/folders', { collection_id: collectionID }),
    apiClient.get<{ data: APIRequest[] }>('/apis/requests', { collection_id: collectionID }),
    apiClient.get<{ data: Environment[] }>('/apis/environments', { collection_id: collectionID }),
  ])
  folders.value = folderResult.data || []
  requests.value = requestResult.data || []
  environments.value = environmentResult.data || []
}

async function selectCollection(id: number) {
  selectedCollectionID.value = id
  selectedFolderID.value = 0
  selectedRequestID.value = undefined
  execution.value = null
  histories.value = []
  await loadCollectionData(id)
  if (environments.value.length) {
    editEnvironment(environments.value[0])
  } else {
    editingEnvironmentID.value = undefined
    environmentForm.name = ''
    environmentForm.description = ''
    environmentForm.variables = [emptyRow()]
  }
}

function switchWorkspaceTab(tab: 'apis' | 'environments') {
  workspaceTab.value = tab
  if (tab === 'environments' && !editingEnvironmentID.value) {
    const firstEnvironment = environments.value[0]
    if (firstEnvironment) editEnvironment(firstEnvironment)
  }
}

async function selectRequest(request: APIRequest) {
  selectedRequestID.value = request.id
  selectedFolderID.value = request.folder_id || 0
  requestTab.value = 'params'
  form.name = request.name
  form.method = request.method
  form.url = request.url
  form.headers = mapToRows(request.headers)
  form.query = normalizeRows(request.query)
  form.auth = { ...(request.auth || {}), type: request.auth?.type || 'none' }
  form.body = request.body || ''
  form.bodyType = request.body_type || 'none'
  form.bodyForm = normalizeRows(request.body_form)
  form.environmentID = request.environment_id || 0
  form.description = request.description || ''
  execution.value = null
  responseTab.value = 'response'
  await loadHistory()
}

function openCreateCollection() {
  collectionModalMode.value = 'create'
  collectionModalName.value = ''
  collectionModalDescription.value = ''
  collectionModal.value = true
}

function openEditCollection() {
  const target = collections.value.find((item) => item.id === selectedCollectionID.value)
  if (!target) return
  collectionModalMode.value = 'edit'
  collectionModalName.value = target.name
  collectionModalDescription.value = target.description || ''
  collectionModal.value = true
}

async function createCollection() {
  const name = collectionModalName.value.trim()
  if (!name) return
  const created = await apiClient.post<Collection>('/apis/collections', {
    name,
    description: collectionModalDescription.value,
    parent_id: 0,
  })
  collectionModal.value = false
  await loadCollections()
  await selectCollection(created.id)
}

async function saveCollection() {
  if (!selectedCollectionID.value) return
  const name = collectionModalName.value.trim()
  if (!name) {
    message.error(t('apis.nameRequired'))
    return
  }
  saving.value = true
  try {
    const updated = await apiClient.put<Collection>(
      `/apis/collections/${selectedCollectionID.value}`,
      { name, description: collectionModalDescription.value },
    )
    const index = collections.value.findIndex((item) => item.id === updated.id)
    if (index >= 0) collections.value[index] = updated
    collectionModal.value = false
    message.success(t('apis.collectionSaved'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('apis.saveFailed'))
  } finally {
    saving.value = false
  }
}

function deleteCollection() {
  if (!selectedCollectionID.value) return
  const target = collections.value.find((item) => item.id === selectedCollectionID.value)
  Modal.confirm({
    title: t('apis.deleteCollectionTitle', { name: target?.name || '' }),
    content: t('apis.deleteCollectionDescription'),
    okText: t('apis.delete'),
    cancelText: t('apis.cancel'),
    okType: 'danger',
    async onOk() {
      try {
        await apiClient.delete(`/apis/collections/${selectedCollectionID.value}`)
        selectedCollectionID.value = undefined
        selectedRequestID.value = undefined
        await loadCollections()
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('apis.delete'))
        return Promise.reject(error)
      }
    },
  })
}

async function createFolder() {
  const name = newFolderName.value.trim()
  if (!selectedCollectionID.value || !name) return
  await apiClient.post('/apis/folders', {
    collection_id: selectedCollectionID.value,
    parent_id: 0,
    name,
    description: '',
  })
  folderModal.value = false
  newFolderName.value = ''
  await loadCollectionData(selectedCollectionID.value)
}

function openRenameFolder(folder: Folder) {
  renameFolderTarget.value = folder
  renameFolderName.value = folder.name
  renameFolderModal.value = true
}

async function confirmRenameFolder() {
  const target = renameFolderTarget.value
  const name = renameFolderName.value.trim()
  if (!target || !name) return
  await apiClient.put(`/apis/folders/${target.id}`, { name, description: target.description || '' })
  target.name = name
  renameFolderModal.value = false
  renameFolderTarget.value = undefined
}

function deleteFolder(folder: Folder) {
  Modal.confirm({
    title: t('apis.deleteFolderTitle', { name: folder.name }),
    content: t('apis.deleteFolderDescription'),
    okText: t('apis.delete'),
    cancelText: t('apis.cancel'),
    okType: 'danger',
    async onOk() {
      await apiClient.delete(`/apis/folders/${folder.id}`)
      collapsedFolderIDs.value = collapsedFolderIDs.value.filter((id) => id !== folder.id)
      saveCollapsedFolderIDs()
      await loadCollectionData(selectedCollectionID.value!)
    },
  })
}

function openCreateRequest(folderID = 0) {
  selectedFolderID.value = folderID
  newRequestName.value = ''
  requestModal.value = true
}

async function createRequest() {
  if (!selectedCollectionID.value || !newRequestName.value.trim()) return
  const created = await apiClient.post<APIRequest>('/apis/requests', {
    collection_id: selectedCollectionID.value,
    folder_id: selectedFolderID.value,
    name: newRequestName.value.trim(),
    method: 'GET',
    url: '',
    headers: {},
    query: [],
    auth: { type: 'none' },
    body: '',
    body_type: 'none',
    body_form: [],
    environment_id: 0,
    description: '',
  })
  requestModal.value = false
  await loadCollectionData(selectedCollectionID.value)
  const item = requests.value.find((request) => request.id === created.id)
  if (item) await selectRequest(item)
}

function currentPayload() {
  return {
    name: form.name.trim(),
    method: form.method,
    url: form.url.trim(),
    headers: rowsToMap(form.headers),
    query: form.query.filter((item) => item.key.trim()).map((item) => ({ ...item })),
    auth: { ...form.auth },
    body: form.body,
    body_type: form.bodyType,
    body_form: form.bodyForm.filter((item) => item.key.trim()).map((item) => ({ ...item })),
    environment_id: form.environmentID || 0,
    description: form.description,
  }
}

async function saveRequest(showSuccess = true) {
  if (!selectedRequestID.value) return false
  if (!form.name.trim()) {
    message.error(t('apis.requestNameRequired'))
    return false
  }
  if (form.bodyType === 'json' && form.body.trim()) {
    try {
      JSON.parse(form.body)
    } catch {
      message.error(t('apis.invalidJson'))
      requestTab.value = 'body'
      return false
    }
  }
  saving.value = true
  try {
    const updated = await apiClient.put<APIRequest>(
      `/apis/requests/${selectedRequestID.value}`,
      currentPayload(),
    )
    const index = requests.value.findIndex((item) => item.id === updated.id)
    if (index >= 0) requests.value[index] = updated
    if (showSuccess) message.success(t('apis.requestSaved'))
    return true
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('apis.saveFailed'))
    return false
  } finally {
    saving.value = false
  }
}

async function executeRequest() {
  if (!selectedRequestID.value || !(await saveRequest(false))) return
  executing.value = true
  execution.value = null
  responseTab.value = 'response'
  try {
    execution.value = await apiClient.post<Execution>(
      `/apis/requests/${selectedRequestID.value}/execute`,
      { environment_id: form.environmentID || 0 },
    )
    await loadHistory()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('apis.requestFailed'))
    await loadHistory()
  } finally {
    executing.value = false
  }
}

async function copyRequest() {
  if (!selectedCollectionID.value) return
  const created = await apiClient.post<APIRequest>('/apis/requests', {
    collection_id: selectedCollectionID.value,
    folder_id: selectedFolderID.value,
    ...currentPayload(),
    name: t('apis.copySuffix', { name: form.name }),
  })
  await loadCollectionData(selectedCollectionID.value)
  const item = requests.value.find((request) => request.id === created.id)
  if (item) await selectRequest(item)
  message.success(t('apis.requestCopied'))
}

async function moveRequest() {
  if (!selectedRequestID.value) return
  await apiClient.put(`/apis/requests/${selectedRequestID.value}`, { folder_id: moveFolderID.value })
  moveModal.value = false
  selectedFolderID.value = moveFolderID.value
  await loadCollectionData(selectedCollectionID.value!)
}

function deleteRequest() {
  if (!selectedRequestID.value) return
  Modal.confirm({
    title: t('apis.deleteRequestTitle', { name: form.name }),
    okText: t('apis.delete'),
    cancelText: t('apis.cancel'),
    okType: 'danger',
    async onOk() {
      await apiClient.delete(`/apis/requests/${selectedRequestID.value}`)
      selectedRequestID.value = undefined
      execution.value = null
      await loadCollectionData(selectedCollectionID.value!)
      if (requests.value.length) await selectRequest(requests.value[0])
    },
  })
}

async function importCurl() {
  if (!selectedCollectionID.value || !curlText.value.trim()) return
  try {
    const draft = await apiClient.post<APIRequest>('/apis/requests/parse-curl', { curl: curlText.value })
    const created = await apiClient.post<APIRequest>('/apis/requests', {
      ...draft,
      collection_id: selectedCollectionID.value,
      folder_id: selectedFolderID.value,
    })
    curlModal.value = false
    curlText.value = ''
    await loadCollectionData(selectedCollectionID.value)
    const item = requests.value.find((request) => request.id === created.id)
    if (item) await selectRequest(item)
    message.success(t('apis.curlImported'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('apis.curlImportFailed'))
  }
}

async function loadHistory() {
  if (!selectedRequestID.value) {
    histories.value = []
    return
  }
  const result = await apiClient.get<{ data: RunHistory[] }>(
    `/apis/requests/${selectedRequestID.value}/history`,
    { limit: 50 },
  )
  histories.value = result.data || []
}

function showHistory(item: RunHistory) {
  execution.value = {
    id: item.id,
    run_uuid: item.run_uuid,
    status: item.response_status,
    headers: item.response_headers || {},
    body: item.response_body || '',
    duration_ms: item.duration_ms,
    body_truncated: item.response_body_truncated,
    error_summary: item.error_summary,
    started_at: item.started_at,
    method: item.method,
    url: item.url,
  }
  responseTab.value = 'response'
}

function clearHistory() {
  if (!selectedRequestID.value || !histories.value.length) return
  Modal.confirm({
    title: t('apis.clearHistoryTitle'),
    okText: t('apis.clear'),
    cancelText: t('apis.cancel'),
    okType: 'danger',
    async onOk() {
      await apiClient.delete(`/apis/requests/${selectedRequestID.value}/history`)
      histories.value = []
      execution.value = null
    },
  })
}

function editEnvironment(environment: Environment) {
  editingEnvironmentID.value = environment.id
  environmentForm.name = environment.name
  environmentForm.description = environment.description || ''
  environmentForm.variables = mapToRows(environment.variables)
}

function newEnvironment() {
  editingEnvironmentID.value = undefined
  environmentForm.name = ''
  environmentForm.description = ''
  environmentForm.variables = [emptyRow()]
  nextTick(() => focusEnvNameInput())
}

function focusEnvNameInput() {
  const el = envNameInputRef.value?.$el
  if (!el) return
  const input = el instanceof HTMLInputElement ? el : el.querySelector('input')
  input?.focus()
}

function resetEnvironmentForm() {
  if (editingEnvironmentID.value) {
    const target = environments.value.find((item) => item.id === editingEnvironmentID.value)
    if (target) {
      editEnvironment(target)
      return
    }
  }
  newEnvironment()
}

async function saveEnvironment() {
  if (!selectedCollectionID.value || !environmentForm.name.trim()) {
    message.error(t('apis.environmentNameRequired'))
    return
  }
  const payload = {
    collection_id: selectedCollectionID.value,
    name: environmentForm.name.trim(),
    description: environmentForm.description,
    variables: rowsToMap(environmentForm.variables),
  }
  let savedID = editingEnvironmentID.value
  if (editingEnvironmentID.value) {
    await apiClient.put(`/apis/environments/${editingEnvironmentID.value}`, payload)
  } else {
    const created = await apiClient.post<Environment>('/apis/environments', payload)
    savedID = created.id
  }
  await loadCollectionData(selectedCollectionID.value)
  const saved = environments.value.find((item) => item.id === savedID)
  if (saved) {
    editEnvironment(saved)
  } else {
    newEnvironment()
  }
  message.success(t('apis.environmentSaved'))
}

function deleteEnvironment(environment: Environment) {
  Modal.confirm({
    title: t('apis.deleteEnvironmentTitle', { name: environment.name }),
    okText: t('apis.delete'),
    cancelText: t('apis.cancel'),
    okType: 'danger',
    async onOk() {
      await apiClient.delete(`/apis/environments/${environment.id}`)
      if (form.environmentID === environment.id) form.environmentID = 0
      await loadCollectionData(selectedCollectionID.value!)
      if (environments.value.length) {
        editEnvironment(environments.value[0])
      } else {
        newEnvironment()
      }
    },
  })
}

function isSensitiveKey(key: string) {
  return /token|secret|password|authorization|cookie|api[-_]?key/i.test(key)
}

function methodColor(method: string) {
  return {
    GET: 'green',
    POST: 'blue',
    PUT: 'orange',
    PATCH: 'purple',
    DELETE: 'red',
  }[method] || 'default'
}

function statusColor(status: number) {
  if (!status) return 'default'
  if (status < 300) return 'success'
  if (status < 400) return 'processing'
  return 'error'
}

function formatTime(timestamp?: number) {
  return timestamp ? new Date(timestamp).toLocaleString(locale.value) : ''
}

function handleShortcut(event: KeyboardEvent) {
  if (workspaceTab.value !== 'apis') return
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    void saveRequest()
  }
  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
    event.preventDefault()
    void executeRequest()
  }
}

onMounted(async () => {
  window.addEventListener('keydown', handleShortcut)
  loading.value = true
  try {
    await loadCollections()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('apis.loadFailed'))
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => window.removeEventListener('keydown', handleShortcut))
</script>

<template>
  <a-spin :spinning="loading">
    <div class="api-workbench">
      <div class="collection-bar">
        <h1 class="api-title">{{ t('apis.title') }}</h1>
        <button
          v-for="collection in collections"
          :key="collection.id"
          class="collection-tab"
          :class="{ active: collection.id === selectedCollectionID }"
          @click="selectCollection(collection.id)"
        >
          <span class="ellipsis">{{ collection.name }}</span>
        </button>
        <button class="collection-tab collection-tab-add" :title="t('apis.newCollection')" @click="openCreateCollection()">
          <PlusOutlined />
        </button>
      </div>

      <div v-if="selectedCollectionID" class="workspace-tab-bar">
        <div class="workspace-tabs" role="tablist" :aria-label="t('apis.workspace')">
          <button
            type="button"
            role="tab"
            class="workspace-tab"
            :class="{ active: workspaceTab === 'apis' }"
            :aria-selected="workspaceTab === 'apis'"
            @click="switchWorkspaceTab('apis')"
          >
            {{ t('apis.title') }}
          </button>
          <button
            type="button"
            role="tab"
            class="workspace-tab"
            :class="{ active: workspaceTab === 'environments' }"
            :aria-selected="workspaceTab === 'environments'"
            @click="switchWorkspaceTab('environments')"
          >
            {{ t('apis.environments') }}
          </button>
        </div>
        <div class="workspace-tab-actions">
          <a-tooltip :title="t('apis.editCollection')">
            <a-button type="text" class="collection-action" @click="openEditCollection">
              <SettingOutlined />
            </a-button>
          </a-tooltip>
          <a-tooltip :title="t('apis.deleteCollection')">
            <a-button type="text" class="collection-action danger" @click="deleteCollection">
              <DeleteOutlined />
            </a-button>
          </a-tooltip>
        </div>
      </div>

      <aside v-if="selectedCollectionID && workspaceTab === 'apis'" class="tree-panel">
        <template v-if="selectedCollectionID">
          <div class="tree-toolbar">
            <a-input v-model:value="searchKeyword" allow-clear :placeholder="t('apis.search')" />
          <a-dropdown :trigger="['click']">
            <a class="tree-add" @click.stop><PlusOutlined /></a>
            <template #overlay>
              <a-menu>
                <a-menu-item @click="openCreateRequest(0)">{{ t('apis.newRequest') }}</a-menu-item>
                <a-menu-item @click="folderModal = true">{{ t('apis.newFolder') }}</a-menu-item>
                <a-menu-item @click="selectedFolderID = 0; curlModal = true">{{ t('apis.importCurl') }}</a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
          </div>

          <div class="tree-content">
            <div
              v-for="folder in folders"
              :key="folder.id"
              class="tree-group folder-group"
              :class="{
                dragging: draggedFolderID === folder.id,
                'drag-over-before': dragOverFolderID === folder.id && folderDropPosition === 'before',
                'drag-over-after': dragOverFolderID === folder.id && folderDropPosition === 'after',
              }"
            >
              <div
                class="tree-group-title folder-title"
                :draggable="!folderSortSaving"
                @dragstart="startFolderDrag($event, folder.id)"
                @dragend="clearFolderDragState"
                @dragover="updateFolderDropTarget($event, folder.id)"
                @drop="dropFolder($event, folder.id)"
              >
                <button
                  type="button"
                  class="folder-toggle"
                  :aria-expanded="isFolderExpanded(folder.id)"
                  @click="toggleFolder(folder.id)"
                  @keydown.enter.prevent="toggleFolder(folder.id)"
                  @keydown.space.prevent="toggleFolder(folder.id)"
                >
                  <span class="folder-name ellipsis">
                    <span class="tree-caret" :class="{ collapsed: !isFolderExpanded(folder.id) }"><CaretRightOutlined /></span>
                    <FolderOutlined class="folder-icon" />
                    <span class="ellipsis">{{ folder.name }}</span>
                  </span>
                </button>
                <span class="tree-group-meta">
                  <span class="count">{{ folderRequestCount(folder.id) }}</span>
                  <a-dropdown>
                    <a class="tree-more" :aria-label="t('apis.more')" @click.stop><MoreOutlined /></a>
                    <template #overlay>
                      <a-menu>
                        <a-menu-item @click="openCreateRequest(folder.id)">{{ t('apis.newRequest') }}</a-menu-item>
                        <a-menu-item @click="selectedFolderID = folder.id; curlModal = true">{{ t('apis.importCurl') }}</a-menu-item>
                        <a-menu-item @click="openRenameFolder(folder)">{{ t('apis.rename') }}</a-menu-item>
                        <a-menu-item danger @click="deleteFolder(folder)">{{ t('apis.deleteFolder') }}</a-menu-item>
                      </a-menu>
                    </template>
                  </a-dropdown>
                </span>
              </div>
              <template v-if="isFolderExpanded(folder.id)">
                <button
                  v-for="request in requestsInFolder(folder.id)"
                  :key="request.id"
                  class="request-node"
                  :class="{ active: request.id === selectedRequestID }"
                  @click="selectRequest(request)"
                >
                  <LinkOutlined class="request-icon" :title="request.method" />
                  <span class="ellipsis">{{ request.name }}</span>
                </button>
              </template>
            </div>
            <button
              v-for="request in rootRequests"
              :key="request.id"
              class="request-node root-request"
              :class="{ active: request.id === selectedRequestID }"
              @click="selectRequest(request)"
            >
              <LinkOutlined class="request-icon" :title="request.method" />
              <span class="ellipsis">{{ request.name }}</span>
            </button>
          </div>
        </template>
      </aside>

      <main v-if="workspaceTab === 'apis' && selectedRequestID" class="editor-panel">
        <div class="editor-header">
          <div class="request-heading">
            <a-tag :color="methodColor(form.method)">{{ form.method }}</a-tag>
            <a-input v-model:value="form.name" class="name-input" :placeholder="t('apis.requestName')" />
          </div>
          <div class="editor-actions">
            <a-tooltip :title="t('apis.copyRequest')">
              <a-button type="text" class="action-icon" @click="copyRequest">
                <CopyOutlined />
              </a-button>
            </a-tooltip>
            <a-tooltip :title="t('apis.moveRequest')">
              <a-button
                type="text"
                class="action-icon"
                @click="moveFolderID = selectedFolderID; moveModal = true"
              >
                <img class="action-icon-image" :src="moveIcon" alt="" />
              </a-button>
            </a-tooltip>
            <a-tooltip :title="t('apis.deleteRequest')">
              <a-button type="text" class="action-icon danger" @click="deleteRequest">
                <DeleteOutlined />
              </a-button>
            </a-tooltip>
            <a-tooltip :title="t('apis.saveShortcut')">
              <a-button type="text" class="action-icon save" :loading="saving" @click="saveRequest()">
                <img v-if="!saving" class="action-icon-image" :src="saveIcon" alt="" />
              </a-button>
            </a-tooltip>
          </div>
        </div>

        <div class="request-line">
          <a-select v-model:value="form.method" class="method-select">
            <a-select-option v-for="method in methods" :key="method" :value="method">
              {{ method }}
            </a-select-option>
          </a-select>
          <a-input
            v-model:value="form.url"
            class="url-input"
            placeholder="https://api.example.com/users/{{userId}}"
          />
          <a-select
            v-model:value="form.environmentID"
            class="env-select"
          >
            <a-select-option :value="0">{{ t('apis.noEnvironment') }}</a-select-option>
            <a-select-option v-for="env in environments" :key="env.id" :value="env.id">
              {{ env.name }}
            </a-select-option>
          </a-select>
          <a-button type="primary" ghost class="send-btn" :loading="executing" @click="executeRequest">
            {{ t('apis.send') }}
          </a-button>
        </div>

        <div class="shortcut-tip">{{ t('apis.shortcutTip', { syntax: variableSyntaxExample }) }}</div>

        <a-tabs v-model:active-key="requestTab" class="request-tabs">
          <a-tab-pane key="params" :tab="`Params (${form.query.filter((item) => item.key).length})`">
            <div class="kv-table">
              <div class="kv-row kv-head">
                <span>{{ t('apis.parameterName') }}</span><span>{{ t('apis.parameterValue') }}</span><span>{{ t('apis.enabled') }}</span>
              </div>
              <div v-for="(item, index) in form.query" :key="index" class="kv-row">
                <a-input v-model:value="item.key" placeholder="key" />
                <a-input v-model:value="item.value" :placeholder="`${t('apis.parameterValue')} / {{variable}}`" />
                <div class="kv-enable">
                  <a-switch v-model:checked="item.enabled" size="small" />
                  <a-button type="text" class="remove-row" size="small" @click="removeRow(form.query, index)">
                    <CloseCircleOutlined />
                  </a-button>
                </div>
              </div>
              <a-button type="dashed" block class="kv-add" @click="addRow(form.query)">{{ t('apis.addParams') }}</a-button>
            </div>
          </a-tab-pane>

          <a-tab-pane key="headers" :tab="`Headers (${form.headers.filter((item) => item.key).length})`">
            <div class="kv-table">
              <div class="kv-row kv-head">
                <span>Header</span><span>{{ t('apis.value') }}</span><span>{{ t('apis.enabled') }}</span>
              </div>
              <div v-for="(item, index) in form.headers" :key="index" class="kv-row">
                <a-input v-model:value="item.key" placeholder="Content-Type" />
                <a-input v-model:value="item.value" placeholder="application/json" />
                <div class="kv-enable">
                  <a-switch v-model:checked="item.enabled" size="small" />
                  <a-button type="text" class="remove-row" size="small" @click="removeRow(form.headers, index)">
                    <CloseCircleOutlined />
                  </a-button>
                </div>
              </div>
              <a-button type="dashed" block class="kv-add" @click="addRow(form.headers)">{{ t('apis.addHeaders') }}</a-button>
            </div>
          </a-tab-pane>

          <a-tab-pane key="body" tab="Body">
            <a-radio-group v-model:value="form.bodyType" button-style="solid" class="body-type">
              <a-radio-button value="none">none</a-radio-button>
              <a-radio-button value="json">JSON</a-radio-button>
              <a-radio-button value="text">Text</a-radio-button>
              <a-radio-button value="form">x-www-form-urlencoded</a-radio-button>
              <a-radio-button value="multipart">multipart/form-data</a-radio-button>
            </a-radio-group>
            <a-textarea
              v-if="['json', 'text'].includes(form.bodyType)"
              v-model:value="form.body"
              :rows="18"
              class="code-input"
              :placeholder="form.bodyType === 'json' ? '{\n  &quot;name&quot;: &quot;{{name}}&quot;\n}' : t('apis.rawBody')"
            />
            <div v-else-if="['form', 'multipart'].includes(form.bodyType)" class="kv-table body-form">
              <div class="kv-row kv-head">
                <span>{{ t('apis.fieldName') }}</span><span>{{ t('apis.fieldValue') }}</span><span>{{ t('apis.enabled') }}</span>
              </div>
              <div v-for="(item, index) in form.bodyForm" :key="index" class="kv-row">
                <a-input v-model:value="item.key" placeholder="field" />
                <a-input v-model:value="item.value" placeholder="value" />
                <div class="kv-enable">
                  <a-switch v-model:checked="item.enabled" size="small" />
                  <a-button type="text" class="remove-row" size="small" @click="removeRow(form.bodyForm, index)">
                    <CloseCircleOutlined />
                  </a-button>
                </div>
              </div>
              <a-button type="dashed" block class="kv-add" @click="addRow(form.bodyForm)">{{ t('apis.addField') }}</a-button>
            </div>
            <a-empty v-else :image="false" :description="t('apis.noBody')" />
          </a-tab-pane>

          <a-tab-pane key="description" :tab="t('apis.description')">
            <a-textarea
              v-model:value="form.description"
              :rows="16"
              :placeholder="t('apis.descriptionPlaceholder')"
            />
          </a-tab-pane>
        </a-tabs>
      </main>

      <section v-if="workspaceTab === 'apis' && selectedRequestID" class="result-panel">
        <a-tabs v-model:active-key="responseTab" class="result-tabs">
          <a-tab-pane key="response" :tab="t('apis.response')">
            <div v-if="execution" class="response-content">
              <div class="response-summary">
                <a-tag :color="statusColor(execution.status)">
                  {{ execution.status || 'ERR' }}
                </a-tag>
                <span>{{ execution.duration_ms }} ms</span>
                <span v-if="execution.started_at">{{ formatTime(execution.started_at) }}</span>
              </div>
              <a-alert
                v-if="execution.error_summary"
                type="error"
                :message="execution.error_summary"
                show-icon
              />
              <a-alert
                v-if="execution.body_truncated"
                type="warning"
                :message="t('apis.responseTruncated')"
                show-icon
              />
              <a-tabs size="small">
                <a-tab-pane key="body" tab="Body">
                  <pre class="response-body">{{ prettyResponseBody }}</pre>
                </a-tab-pane>
                <a-tab-pane key="headers" :tab="`Headers (${responseHeaderRows.length})`">
                  <div class="response-headers">
                    <div v-for="item in responseHeaderRows" :key="item.key" class="header-row">
                      <strong>{{ item.key }}</strong>
                      <span>{{ item.value }}</span>
                    </div>
                  </div>
                </a-tab-pane>
              </a-tabs>
            </div>
            <div v-else class="response-empty">
              <div class="response-empty-icon">
                <img :src="responseEmptyIcon" alt="" />
              </div>
              <p>{{ t('apis.responseHint') }}</p>
            </div>
          </a-tab-pane>

          <a-tab-pane key="history" :tab="t('apis.history', { count: histories.length })">
            <div class="history-toolbar">
              <span>{{ t('apis.recentRuns') }}</span>
              <a v-if="histories.length" class="danger-link" @click="clearHistory">{{ t('apis.clear') }}</a>
            </div>
            <button
              v-for="item in histories"
              :key="item.id"
              class="history-item"
              @click="showHistory(item)"
            >
              <div>
                <a-tag :color="statusColor(item.response_status)">
                  {{ item.response_status || 'ERR' }}
                </a-tag>
                <strong>{{ item.method }}</strong>
                <span>{{ item.duration_ms }} ms</span>
              </div>
              <time>{{ formatTime(item.started_at) }}</time>
              <span class="history-url">{{ item.url }}</span>
            </button>
            <a-empty v-if="!histories.length" :image="false" :description="t('apis.emptyHistory')" />
          </a-tab-pane>
        </a-tabs>
      </section>

      <section v-if="selectedCollectionID && workspaceTab === 'environments'" class="environment-list-panel">
        <div class="environment-list-toolbar">
          <a-input v-model:value="environmentKeyword" allow-clear :placeholder="t('apis.searchEnvironment')" />
          <a-tooltip :title="t('apis.newEnvironment')">
            <a-button type="text" class="tree-add" @click="newEnvironment"><PlusOutlined /></a-button>
          </a-tooltip>
        </div>
        <div class="environment-list">
          <button
            v-for="environment in visibleEnvironments"
            :key="environment.id"
            class="environment-list-item"
            :class="{ active: environment.id === editingEnvironmentID }"
            @click="editEnvironment(environment)"
          >
            <FolderOutlined />
            <span class="ellipsis">{{ environment.name }}</span>
            <a-dropdown :trigger="['click']">
              <span class="environment-more" :aria-label="t('apis.more')" @click.stop><MoreOutlined /></span>
              <template #overlay>
                <a-menu>
                  <a-menu-item danger @click="deleteEnvironment(environment)">{{ t('apis.delete') }}</a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
          </button>
          <button v-if="!editingEnvironmentID" class="environment-list-item active draft-environment" @click="focusEnvNameInput">
            <FolderOutlined />
            <span class="ellipsis">{{ environmentForm.name || t('apis.newEnvironment') }}</span>
          </button>
          <a-empty v-if="editingEnvironmentID && !visibleEnvironments.length" :image="false" :description="t('apis.noEnvironmentFound')" />
        </div>
      </section>

      <section v-if="selectedCollectionID && workspaceTab === 'environments'" class="environment-workspace">
        <div class="environment-editor">
          <a-form layout="vertical">
            <a-form-item :label="t('apis.environmentName')">
              <a-input
                ref="envNameInputRef"
                v-model:value="environmentForm.name"
                class="env-name-input"
                :placeholder="t('apis.enter')"
              />
            </a-form-item>
            <a-form-item :label="t('apis.variables')">
              <div class="env-kv-table">
                <div v-for="(item, index) in environmentForm.variables" :key="index" class="env-kv-row">
                  <a-input v-model:value="item.key" :placeholder="t('apis.variableName')" />
                  <a-input-password
                    v-if="isSensitiveKey(item.key)"
                    v-model:value="item.value"
                    :placeholder="t('apis.secretVariableValue')"
                  />
                  <a-input v-else v-model:value="item.value" :placeholder="t('apis.variableValue')" />
                  <a-button type="text" class="remove-row" @click="removeRow(environmentForm.variables, index)">
                    <CloseCircleOutlined />
                  </a-button>
                </div>
                <a-button type="dashed" block class="kv-add" @click="addRow(environmentForm.variables)">
                  {{ t('apis.addVariable') }}
                </a-button>
              </div>
            </a-form-item>
            <a-space>
              <a-button type="primary" @click="saveEnvironment">{{ t('apis.saveEnvironment') }}</a-button>
              <a-button @click="resetEnvironmentForm">{{ t('apis.reset') }}</a-button>
            </a-space>
          </a-form>
        </div>
      </section>
      <div
        v-if="selectedCollectionID && workspaceTab === 'apis' && !selectedRequestID"
        class="request-workspace-empty"
      >
        <div class="empty-state">
          <div class="empty-illustration" aria-hidden="true">
            <FolderOutlined class="empty-folder" />
            <FileTextOutlined class="empty-document" />
            <span class="empty-check"><CheckOutlined /></span>
          </div>
          <h2>{{ t('apis.emptySelectionTitle') }}</h2>
          <p>{{ t('apis.emptySelectionDescription') }}</p>
        </div>
      </div>
      <a-empty
        v-if="!selectedCollectionID"
        class="workspace-empty"
        :description="t('apis.emptyCollection')"
      >
        <a-button type="primary" @click="openCreateCollection()">{{ t('apis.createCollection') }}</a-button>
      </a-empty>
    </div>
  </a-spin>

  <a-modal
    v-model:open="collectionModal"
    :title="collectionModalMode === 'edit' ? t('apis.editApiCollection') : t('apis.newApiCollection')"
    :ok-text="collectionModalMode === 'edit' ? t('apis.save') : t('apis.create')"
    :cancel-text="t('apis.cancel')"
    @ok="collectionModalMode === 'edit' ? saveCollection() : createCollection()"
  >
    <a-form layout="vertical">
      <a-form-item :label="t('apis.collectionName')">
        <a-input
          v-model:value="collectionModalName"
          autofocus
          :placeholder="t('apis.collectionName')"
          @press-enter="collectionModalMode === 'edit' ? saveCollection() : createCollection()"
        />
      </a-form-item>
      <a-form-item :label="t('apis.description')">
        <a-textarea
          v-model:value="collectionModalDescription"
          :rows="4"
          :placeholder="t('apis.collectionDescriptionPlaceholder')"
        />
      </a-form-item>
    </a-form>
  </a-modal>

  <a-modal v-model:open="folderModal" :title="t('apis.newFolder')" :ok-text="t('apis.create')" :cancel-text="t('apis.cancel')" @ok="createFolder">
    <a-form layout="vertical">
      <a-form-item :label="t('apis.folderName')">
        <a-input v-model:value="newFolderName" autofocus @press-enter="createFolder" />
      </a-form-item>
    </a-form>
  </a-modal>

  <a-modal v-model:open="renameFolderModal" :title="t('apis.renameFolder')" :ok-text="t('apis.save')" :cancel-text="t('apis.cancel')" @ok="confirmRenameFolder">
    <a-form layout="vertical">
      <a-form-item :label="t('apis.folderName')">
        <a-input v-model:value="renameFolderName" autofocus @press-enter="confirmRenameFolder" />
      </a-form-item>
    </a-form>
  </a-modal>

  <a-modal v-model:open="requestModal" :title="t('apis.newRequest')" :ok-text="t('apis.create')" :cancel-text="t('apis.cancel')" @ok="createRequest">
    <a-form layout="vertical">
      <a-form-item :label="t('apis.requestName')">
        <a-input v-model:value="newRequestName" autofocus @press-enter="createRequest" />
      </a-form-item>
      <a-form-item :label="t('apis.targetFolder')">
        <a-select v-model:value="selectedFolderID">
          <a-select-option :value="0">{{ t('apis.collectionRoot') }}</a-select-option>
          <a-select-option v-for="folder in folders" :key="folder.id" :value="folder.id">
            {{ folder.name }}
          </a-select-option>
        </a-select>
      </a-form-item>
    </a-form>
  </a-modal>

  <a-modal v-model:open="curlModal" :title="t('apis.curlModalTitle')" width="760px" :ok-text="t('apis.import')" :cancel-text="t('apis.cancel')" @ok="importCurl">
    <a-alert
      type="info"
      :message="t('apis.curlSupport')"
      class="modal-alert"
    />
    <a-textarea
      v-model:value="curlText"
      :rows="16"
      class="code-input"
      placeholder="curl 'https://api.example.com/users?page=1' -H 'Authorization: Bearer {{token}}'"
    />
  </a-modal>

  <a-modal v-model:open="moveModal" :title="t('apis.moveRequest')" :ok-text="t('apis.move')" :cancel-text="t('apis.cancel')" @ok="moveRequest">
    <a-form layout="vertical">
      <a-form-item :label="t('apis.moveTarget')">
        <a-select v-model:value="moveFolderID">
          <a-select-option :value="0">{{ t('apis.collectionRoot') }}</a-select-option>
          <a-select-option v-for="folder in folders" :key="folder.id" :value="folder.id">
            {{ folder.name }}
          </a-select-option>
        </a-select>
      </a-form-item>
    </a-form>
  </a-modal>

</template>

<style scoped>
.api-workbench {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr) 360px;
  grid-template-rows: auto auto auto minmax(0, 1fr);
  height: 100vh;
  min-height: 600px;
  background: #fff;
  overflow: hidden;
  font-family: 'PingFang SC', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Microsoft YaHei', sans-serif;
}

.collection-bar {
  grid-column: 1 / -1;
  grid-row: 1;
  display: flex;
  align-items: stretch;
  gap: 32px;
  min-height: 44px;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  overflow-x: auto;
}

.workspace-tab-bar {
  grid-column: 1 / -1;
  grid-row: 3;
  display: flex;
  align-items: center;
  min-height: 56px;
  padding: 0 24px;
  border-bottom: 1px solid #d9d9d9;
}

.workspace-tabs {
  display: grid;
  grid-template-columns: repeat(2, 128px);
  overflow: hidden;
  border: 1px solid #8c8c8c;
  border-radius: 6px;
}

.workspace-tab {
  height: 34px;
  padding: 0 12px;
  border: 0;
  background: #fff;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
  white-space: nowrap;
  cursor: pointer;
}

.workspace-tab + .workspace-tab {
  border-left: 1px solid #8c8c8c;
}

.workspace-tab:hover {
  color: #1677ff;
}

.workspace-tab.active {
  border-color: #3157e2;
  background: #3157e2;
  color: #fff;
}

.api-title {
  flex: none;
  align-self: center;
  margin: 0;
  color: #262626;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.collection-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  max-width: 220px;
  min-height: 44px;
  padding: 0;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: #595959;
  font-size: 16px;
  white-space: nowrap;
  cursor: pointer;
}

.collection-tab:hover {
  color: #1677ff;
}

.collection-tab.active {
  border-bottom-color: #1677ff;
  color: #1677ff;
  font-weight: 600;
}

.collection-tab-add {
  flex: none;
  color: #3157e2;
  font-size: 16px;
  font-weight: 400;
}

.collection-tab-add:hover {
  color: #1677ff;
}

.tab-more {
  color: #bbb;
  font-size: 12px;
  letter-spacing: 1px;
  padding: 2px;
}

.workspace-tab-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
}

.collection-action {
  width: 36px;
  height: 36px;
  color: #595959;
  font-size: 20px;
}

.collection-action:hover {
  color: #3157e2;
  background: #f0f5ff;
}

.collection-action.danger:hover {
  color: #ff4d4f;
  background: #fff1f0;
}

.tab-more:hover {
  color: #1677ff;
}

.tree-panel,
.editor-panel,
.result-panel {
  min-width: 0;
  background: #fff;
}

.tree-panel {
  grid-column: 1;
  grid-row: 4;
  display: flex;
  flex-direction: column;
  border-right: 1px solid #e8e8e8;
}

.result-panel {
  grid-column: 3;
  grid-row: 4;
  border-left: 1px solid #e8e8e8;
}

.panel-title,
.editor-header,
.request-line,
.tree-toolbar,
.history-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.panel-title {
  min-height: 72px;
  padding: 0 24px;
  border-bottom: 0;
}

.panel-title-text {
  display: flex;
  align-items: center;
  gap: 8px;
}

.panel-title-text strong {
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.panel-title-divider {
  width: 1px;
  height: 12px;
  background: #f0f0f0;
}

.panel-subtitle {
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
  font-weight: 400;
}

.tree-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: #3157e2;
  font-size: 16px;
  border-radius: 6px;
}

.tree-add:hover {
  color: #3157e2;
  background: #f0f5ff;
}

.collection-list {
  max-height: 154px;
  padding: 6px;
  overflow-y: auto;
  border-bottom: 1px solid #f0f0f0;
}

.collection-item,
.request-node,
.history-item {
  width: 100%;
  border: 0;
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.request-node:hover,
.request-node.active {
  background: #e5efff;
  color: #1677ff;
}

.request-node:focus {
  outline: none;
}

.request-node:focus-visible {
  box-shadow: none;
}

.count {
  color: #aaa;
  font-size: 11px;
}

.tree-toolbar {
  padding: 12px 24px 13px;
  border-bottom: 0;
  min-width: 0;
}

.tree-toolbar :deep(.ant-input-affix-wrapper),
.tree-toolbar > :deep(.ant-input) {
  width: 100%;
  min-width: 0;
  height: 32px;
  box-sizing: border-box;
  border-color: #d9d9d9;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

.tree-toolbar :deep(.ant-input-affix-wrapper) {
  display: flex;
  align-items: center;
  padding: 4px 11px;
  overflow: visible;
}

.tree-toolbar :deep(.ant-input-affix-wrapper input.ant-input) {
  width: 100%;
  height: 22px;
  min-height: 22px;
  padding: 0;
  font-size: 14px;
  line-height: 22px;
}

.tree-content {
  flex: 1;
  padding: 1px 12px 12px;
  overflow-y: auto;
}

.tree-group {
  position: relative;
  margin-bottom: 4px;
}

.folder-group.dragging {
  opacity: 0.45;
}

.folder-group.drag-over-before::before,
.folder-group.drag-over-after::after {
  position: absolute;
  right: 4px;
  left: 4px;
  z-index: 2;
  height: 2px;
  border-radius: 2px;
  background: #3157e2;
  content: '';
  pointer-events: none;
}

.folder-group.drag-over-before::before { top: -3px; }
.folder-group.drag-over-after::after { bottom: -3px; }

.tree-group-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  min-width: 0;
  padding: 0 8px;
  color: #595959;
  font-size: 14px;
  font-weight: 400;
  border-radius: 6px;
}

.tree-group-title:hover {
  background: #f5f7fa;
}

.tree-group-meta {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  flex: 0 0 48px;
  width: 48px;
  height: 24px;
  margin-left: 8px;
}

.folder-title {
  color: #595959;
  font-size: 14px;
  font-weight: 400;
  user-select: none;
  cursor: grab;
}

.folder-title:active { cursor: grabbing; }

.folder-toggle {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  height: 32px;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.folder-toggle:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: -2px;
  border-radius: 4px;
}

.folder-name {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  gap: 4px;
}

.tree-caret {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 24px;
  width: 24px;
  height: 24px;
  color: #595959;
  font-size: 16px;
  transform: rotate(90deg);
  transition: transform 0.15s ease;
}

.tree-caret.collapsed {
  transform: rotate(0deg);
}

.folder-icon {
  flex: 0 0 16px;
  font-size: 16px;
}

.tree-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: #999;
  font-size: 16px;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.15s ease;
}

.tree-group-title:hover .tree-more,
.tree-group-title:focus-within .tree-more {
  opacity: 1;
  pointer-events: auto;
}

.tree-group-title:hover .count,
.tree-group-title:focus-within .count {
  opacity: 0;
}

.count {
  min-width: 24px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
  text-align: right;
  transition: opacity 0.15s ease;
}

.request-node {
  display: grid;
  grid-template-columns: 16px 1fr;
  align-items: center;
  gap: 4px;
  min-height: 32px;
  padding: 5px 8px 5px 36px;
  border-radius: 2px;
  color: #595959;
  font-size: 14px;
}

.root-request {
  padding-left: 20px;
}

.request-icon {
  color: #595959;
  font-size: 16px;
}

.editor-panel {
  grid-column: 2;
  grid-row: 4;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 0 24px;
  overflow: hidden;
}

.editor-header {
  min-height: 60px;
  border-bottom: 1px solid #f0f0f0;
}

.editor-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-icon {
  display: inline-flex !important;
  align-items: center;
  justify-content: center;
  flex: 0 0 24px;
  width: 24px;
  min-width: 24px;
  height: 24px;
  margin: 0;
  padding: 4px !important;
  box-sizing: border-box;
  border-radius: 6px;
  color: #666;
  font-size: 16px;
  line-height: 0;
  vertical-align: middle;
}

.action-icon :deep(.anticon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  margin: 0;
  line-height: 1;
  vertical-align: 0;
}

.action-icon :deep(.anticon > svg) {
  display: block;
  width: 16px;
  height: 16px;
}

.action-icon :deep(.ant-btn-loading-icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  margin: 0;
}

.action-icon-image {
  display: block;
  flex: 0 0 16px;
  width: 16px;
  height: 16px;
  object-fit: contain;
  vertical-align: top;
}

.action-icon:hover {
  color: #1677ff;
  background: #f0f5ff;
}

.action-icon.danger:hover {
  color: #ff4d4f;
  background: #fff1f0;
}

.action-icon.save,
.action-icon.save:hover,
.action-icon.save:focus {
  color: #fff;
  background: #3157e2;
}

.request-heading {
  display: flex;
  align-items: center;
  min-width: 0;
}

.name-input {
  width: min(400px, 34vw);
  border-color: transparent;
  color: #262626;
  font-size: 16px;
  line-height: 24px;
  font-weight: 600;
}

.request-line {
  margin-top: 10px;
}

.method-select { width: 109px; flex: none; }
.url-input { flex: 1; }
.env-select { width: 107px; flex: none; }

.send-btn {
  flex: none;
  min-width: 60px;
  margin-left: 16px;
  border-radius: 6px;
}

.shortcut-tip {
  display: none;
}

.request-tabs :deep(.ant-tabs-content-holder) {
  overflow-y: auto;
  max-height: calc(100vh - 196px);
  padding-bottom: 24px;
}

.request-tabs :deep(.ant-tabs-nav) {
  margin: 12px 0 14px;
}

.request-tabs :deep(.ant-tabs-nav::before) {
  border-bottom: 0;
}

.request-tabs :deep(.ant-tabs-nav-list) {
  gap: 8px;
}

.request-tabs :deep(.ant-tabs-tab) {
  margin: 0 !important;
  padding: 5px 16px;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

.request-tabs :deep(.ant-tabs-tab-active) {
  background: #d6e6ff;
}

.request-tabs :deep(.ant-tabs-ink-bar) {
  display: none;
}

.kv-table {
  overflow: visible;
}

.kv-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 96px;
  align-items: center;
  gap: 16px;
  border-bottom: 0;
}

.kv-row > * {
  margin: 4px 0;
}

.kv-head {
  min-height: 30px;
  background: transparent;
  color: #595959;
  font-size: 14px;
}

.kv-head > span {
  margin: 0;
}

.kv-enable {
  display: flex;
  align-items: center;
  gap: 4px;
}

.kv-add {
  height: 32px;
  margin: 8px 0;
  color: #595959;
  font-size: 14px;
}

.remove-row {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  min-width: 32px;
  height: 32px;
  padding: 6px;
  color: #595959;
  font-size: 20px;
}

.body-type { margin-bottom: 14px; }
.body-form { margin-top: 4px; }
.code-input,
.response-body {
  font-family: Consolas, Monaco, 'Courier New', monospace;
}

.result-tabs {
  padding: 0 23px;
}

.result-tabs :deep(.ant-tabs-content-holder) {
  max-height: calc(100vh - 150px);
  overflow-y: auto;
  overflow-x: hidden;
}

.result-tabs > :deep(.ant-tabs-nav) {
  align-self: flex-start;
  margin: 24px 0 0;
  padding: 2px;
  border-radius: 6px;
  background: #edeff2;
}

.result-tabs > :deep(.ant-tabs-nav::before) {
  border-bottom: 0;
}

.result-tabs > :deep(.ant-tabs-nav .ant-tabs-nav-list) {
  gap: 2px;
}

.result-tabs > :deep(.ant-tabs-nav .ant-tabs-tab) {
  margin: 0;
  padding: 3px 16px;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

.result-tabs > :deep(.ant-tabs-nav .ant-tabs-tab-active) {
  background: #fff;
}

.result-tabs > :deep(.ant-tabs-nav .ant-tabs-ink-bar) {
  display: none;
}

.response-summary {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  color: #777;
  font-size: 12px;
}

.response-content :deep(.ant-alert) {
  margin-bottom: 10px;
}

.response-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  margin-top: 164px;
  color: #595959;
  font-size: 14px;
  line-height: 22px;
  text-align: center;
}

.response-empty-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: #edeff2;
}

.response-empty-icon img {
  display: block;
  width: 20px;
  height: 20px;
}

.response-empty p {
  margin: 0;
  white-space: nowrap;
}

.response-body {
  min-height: 160px;
  max-height: calc(100vh - 270px);
  margin: 0;
  padding: 12px;
  overflow: auto;
  border-radius: 6px;
  background: #f7f8fa;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
}

.header-row {
  display: grid;
  grid-template-columns: 118px 1fr;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px solid #f0f0f0;
  font-size: 12px;
  word-break: break-word;
}

.history-toolbar {
  margin-bottom: 8px;
  color: #999;
  font-size: 12px;
}

.history-item {
  display: block;
  padding: 10px 4px;
  border-bottom: 1px solid #f0f0f0;
}

.history-item:hover {
  background: #fafafa;
}

.history-item > div {
  display: flex;
  align-items: center;
  gap: 7px;
}

.history-item time,
.history-url {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  color: #999;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workspace-empty {
  grid-column: 1 / -1;
  grid-row: 2 / -1;
  align-self: center;
}

.request-workspace-empty {
  grid-column: 2 / 4;
  grid-row: 4;
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

.environment-list-panel {
  grid-column: 1;
  grid-row: 4;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  border-right: 1px solid #e8e8e8;
  background: #fff;
}

.environment-list-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 24px;
}

.environment-list-toolbar :deep(.ant-input-affix-wrapper) {
  flex: 1;
  min-width: 0;
  height: 32px;
}

.environment-list {
  flex: 1;
  padding: 0 24px 16px;
  overflow-y: auto;
}

.environment-list-item {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) 24px;
  align-items: center;
  gap: 6px;
  width: 100%;
  height: 32px;
  padding: 0 8px;
  border: 0;
  border-radius: 4px;
  background: #fff;
  color: #595959;
  font-size: 14px;
  text-align: left;
  cursor: pointer;
}

.environment-list-item:hover,
.environment-list-item.active {
  background: #f0f5ff;
}

.environment-list-item :deep(.anticon) {
  font-size: 16px;
}

.environment-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: #8c8c8c;
  opacity: 0;
}

.environment-list-item:hover .environment-more,
.environment-list-item:focus-within .environment-more {
  opacity: 1;
}

.draft-environment {
  border: 1px dashed #91caff;
}

.environment-workspace {
  grid-column: 2 / 4;
  grid-row: 4;
  padding: 24px;
  overflow-y: auto;
  background: #fff;
}

.environment-editor {
  max-width: 556px;
}

.env-name-input {
  max-width: 556px;
}

.env-kv-table {
  max-width: 556px;
}

.env-kv-row {
  display: grid;
  grid-template-columns: 254px 254px 32px;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.environment-editor :deep(.ant-form-item) {
  margin-bottom: 24px;
}

.environment-editor :deep(.ant-form-item-label) {
  padding-bottom: 4px;
}

.environment-editor :deep(.ant-form-item-label > label) {
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

.environment-editor :deep(.ant-input-affix-wrapper) {
  height: 32px;
  box-sizing: border-box;
  border-color: #d9d9d9;
  border-radius: 6px;
  font-size: 14px;
}

.environment-editor :deep(input.ant-input) {
  height: 32px;
  box-sizing: border-box;
  padding: 4px 12px;
  font-size: 14px;
  line-height: 22px;
}

.environment-editor :deep(.ant-input-affix-wrapper) {
  display: flex;
  align-items: center;
  padding: 4px 12px;
}

.environment-editor :deep(.ant-input-affix-wrapper input.ant-input) {
  height: 22px;
  min-height: 22px;
  padding: 0;
  line-height: 22px;
}

.env-kv-row .remove-row {
  align-self: center;
  justify-self: center;
  margin: 0;
}

.environment-editor .kv-add {
  height: 32px;
  margin: 8px 0 0;
  border-color: #d9d9d9;
  color: #595959;
  font-size: 14px;
}

.environment-editor :deep(.ant-btn-primary),
.environment-editor :deep(.ant-btn-default) {
  height: 32px;
  padding: 0 16px;
  border-radius: 6px;
  font-size: 14px;
}

.compact-empty {
  margin: 10px 0;
}

.danger-link { color: #ff4d4f; }
.ellipsis { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.modal-alert { margin-bottom: 12px; }

@media (max-width: 1399px) {
  .method-select { width: 82px; }
  .env-select { width: 90px; }
  .send-btn { margin-left: 0; }
}

</style>
