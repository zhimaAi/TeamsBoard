import type { TaskNotification, TaskProgress } from '@/types/pipeline'
import type { TaskWithDetails } from '@/types/task-detail'

const DATABASE_NAME = 'goteams-task-conversations'
const DATABASE_VERSION = 1
const CACHE_SCHEMA_VERSION = 1
const NOTIFICATION_STORE = 'notificationSnapshots'
const TASK_VIEW_STORE = 'taskViews'
const VIEW_STATE_STORE = 'viewStates'
const NOTIFICATION_SNAPSHOT_KEY = 'unarchived'
const VIEW_STATE_KEY = 'task-notifications'

interface NotificationSnapshotRecord {
  key: typeof NOTIFICATION_SNAPSHOT_KEY
  schemaVersion: number
  updatedAt: number
  items: TaskNotification[]
}

interface TaskViewRecord {
  taskUuid: string
  schemaVersion: number
  updatedAt: number
  task: TaskWithDetails
  progress: TaskProgress[]
}

interface ViewStateRecord {
  key: typeof VIEW_STATE_KEY
  schemaVersion: number
  updatedAt: number
  lastSelectedTaskUuid: string
  pipelineExpanded?: boolean
}

export interface PersistentTaskConversationCache {
  notifications: TaskNotification[]
  taskViews: Array<{
    taskUuid: string
    task: TaskWithDetails
    progress: TaskProgress[]
  }>
  lastSelectedTaskUuid: string
  pipelineExpanded: boolean
}

let databasePromise: Promise<IDBDatabase> | undefined

function requestResult<T>(request: IDBRequest<T>) {
  return new Promise<T>((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error || new Error('IndexedDB 请求失败'))
  })
}

function transactionCompleted(transaction: IDBTransaction) {
  return new Promise<void>((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onabort = () => reject(transaction.error || new Error('IndexedDB 事务已取消'))
    transaction.onerror = () => reject(transaction.error || new Error('IndexedDB 事务失败'))
  })
}

function openDatabase() {
  if (databasePromise) return databasePromise
  databasePromise = new Promise<IDBDatabase>((resolve, reject) => {
    if (typeof indexedDB === 'undefined') {
      reject(new Error('当前环境不支持 IndexedDB'))
      return
    }
    const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(NOTIFICATION_STORE)) {
        database.createObjectStore(NOTIFICATION_STORE, { keyPath: 'key' })
      }
      if (!database.objectStoreNames.contains(TASK_VIEW_STORE)) {
        database.createObjectStore(TASK_VIEW_STORE, { keyPath: 'taskUuid' })
      }
      if (!database.objectStoreNames.contains(VIEW_STATE_STORE)) {
        database.createObjectStore(VIEW_STATE_STORE, { keyPath: 'key' })
      }
    }
    request.onsuccess = () => {
      const database = request.result
      database.onversionchange = () => {
        database.close()
        databasePromise = undefined
      }
      resolve(database)
    }
    request.onerror = () => {
      databasePromise = undefined
      reject(request.error || new Error('无法打开 IndexedDB'))
    }
    request.onblocked = () => {
      databasePromise = undefined
      reject(new Error('IndexedDB 升级被其他页面阻塞'))
    }
  })
  return databasePromise
}

export async function readTaskConversationCache(): Promise<PersistentTaskConversationCache> {
  const database = await openDatabase()
  const transaction = database.transaction(
    [NOTIFICATION_STORE, TASK_VIEW_STORE, VIEW_STATE_STORE],
    'readonly',
  )
  const completed = transactionCompleted(transaction)
  const notificationRequest = transaction
    .objectStore(NOTIFICATION_STORE)
    .get(NOTIFICATION_SNAPSHOT_KEY) as IDBRequest<NotificationSnapshotRecord | undefined>
  const taskViewsRequest = transaction
    .objectStore(TASK_VIEW_STORE)
    .getAll() as IDBRequest<TaskViewRecord[]>
  const viewStateRequest = transaction
    .objectStore(VIEW_STATE_STORE)
    .get(VIEW_STATE_KEY) as IDBRequest<ViewStateRecord | undefined>
  const [notificationSnapshot, taskViewRecords, viewState] = await Promise.all([
    requestResult(notificationRequest),
    requestResult(taskViewsRequest),
    requestResult(viewStateRequest),
    completed,
  ])

  return {
    notifications:
      notificationSnapshot?.schemaVersion === CACHE_SCHEMA_VERSION &&
      Array.isArray(notificationSnapshot.items)
        ? notificationSnapshot.items
        : [],
    taskViews: taskViewRecords
      .filter((record) => record.schemaVersion === CACHE_SCHEMA_VERSION)
      .map((record) => ({
        taskUuid: record.taskUuid,
        task: record.task,
        progress: record.progress,
      })),
    lastSelectedTaskUuid:
      viewState?.schemaVersion === CACHE_SCHEMA_VERSION
        ? viewState.lastSelectedTaskUuid
        : '',
    pipelineExpanded:
      viewState?.schemaVersion === CACHE_SCHEMA_VERSION &&
      typeof viewState.pipelineExpanded === 'boolean'
        ? viewState.pipelineExpanded
        : true,
  }
}

export async function saveTaskNotificationSnapshot(items: TaskNotification[]) {
  const database = await openDatabase()
  const transaction = database.transaction(NOTIFICATION_STORE, 'readwrite')
  transaction.objectStore(NOTIFICATION_STORE).put({
    key: NOTIFICATION_SNAPSHOT_KEY,
    schemaVersion: CACHE_SCHEMA_VERSION,
    updatedAt: Date.now(),
    items,
  } satisfies NotificationSnapshotRecord)
  await transactionCompleted(transaction)
}

export async function saveTaskView(
  taskUuid: string,
  task: TaskWithDetails,
  progress: TaskProgress[],
) {
  const database = await openDatabase()
  const transaction = database.transaction(TASK_VIEW_STORE, 'readwrite')
  transaction.objectStore(TASK_VIEW_STORE).put({
    taskUuid,
    schemaVersion: CACHE_SCHEMA_VERSION,
    updatedAt: Date.now(),
    task,
    progress,
  } satisfies TaskViewRecord)
  await transactionCompleted(transaction)
}

export async function saveTaskConversationViewState(
  taskUuid: string,
  pipelineExpanded: boolean,
) {
  const database = await openDatabase()
  const transaction = database.transaction(VIEW_STATE_STORE, 'readwrite')
  transaction.objectStore(VIEW_STATE_STORE).put({
    key: VIEW_STATE_KEY,
    schemaVersion: CACHE_SCHEMA_VERSION,
    updatedAt: Date.now(),
    lastSelectedTaskUuid: taskUuid,
    pipelineExpanded,
  } satisfies ViewStateRecord)
  await transactionCompleted(transaction)
}

export async function deleteTaskView(taskUuid: string) {
  const database = await openDatabase()
  const transaction = database.transaction(TASK_VIEW_STORE, 'readwrite')
  transaction.objectStore(TASK_VIEW_STORE).delete(taskUuid)
  await transactionCompleted(transaction)
}

export async function deleteTaskViewsOutside(validTaskUuids: Set<string>) {
  const database = await openDatabase()
  const transaction = database.transaction(TASK_VIEW_STORE, 'readwrite')
  const store = transaction.objectStore(TASK_VIEW_STORE)
  const cursorRequest = store.openCursor()
  cursorRequest.onsuccess = () => {
    const cursor = cursorRequest.result
    if (!cursor) return
    const record = cursor.value as TaskViewRecord
    if (
      record.schemaVersion !== CACHE_SCHEMA_VERSION ||
      !validTaskUuids.has(record.taskUuid)
    ) {
      cursor.delete()
    }
    cursor.continue()
  }
  await transactionCompleted(transaction)
}
