import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import apiClient from '@/api/client'
import type { TaskNotification } from '@/types/pipeline'
import type { LocalProject } from '@/types/project'
import { t } from '@/i18n'

export interface TaskConversation {
  task_uuid: string
  title: string
  work_dir: string
  project_uuid: string
  items: TaskNotification[]
  latest: TaskNotification
  unread: boolean
  is_archived: boolean
}

export interface ConversationGroup {
  key: string
  workDir: string
  displayName: string
  fullPath: string
  projectName?: string
  items: TaskConversation[]
}

const ARCHIVED_COLLAPSED_KEY = 'goteams.conversation.archivedCollapsed'
const WORK_DIR_HISTORY_KEY = 'goteams.workDirectories.history'

function normalizePath(p: string): string {
  let value = (p || '').trim()
  // \\?\ 与 \\?\UNC\ 只是 Windows 扩展路径前缀，指向的仍是同一目录。
  if (/^\\\\\?\\unc\\/i.test(value)) {
    value = `\\\\${value.slice(8)}`
  } else if (/^\\\\\?\\/i.test(value)) {
    value = value.slice(4)
  }
  return value.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()
}

export function getLastDirSegment(p: string): string {
  const clean = (p || '').replace(/\\/g, '/').replace(/\/+$/, '').trim()
  if (!clean) return ''
  const parts = clean.split('/')
  return parts[parts.length - 1] || clean
}

export const useConversationStore = defineStore('conversation', () => {
  const rawNotifications = ref<TaskNotification[]>([])
  const projects = ref<LocalProject[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const searchQuery = ref('')
  const archivedCollapsed = ref(
    typeof window !== 'undefined' ? localStorage.getItem(ARCHIVED_COLLAPSED_KEY) !== 'false' : true,
  )
  const collapsedGroupKeys = ref<Set<string>>(new Set())
  const dirHistory = ref<string[]>([])
  let loadVersion = 0

  function loadDirHistory() {
    try {
      const raw = localStorage.getItem(WORK_DIR_HISTORY_KEY)
      if (raw) {
        dirHistory.value = JSON.parse(raw) as string[]
      }
    } catch {
      dirHistory.value = []
    }
  }

  function recordWorkingDir(dir: string) {
    if (!dir || !dir.trim()) return
    const trimmed = dir.trim()
    const normalized = normalizePath(trimmed)
    const next = [trimmed, ...dirHistory.value.filter((d) => normalizePath(d) !== normalized)].slice(0, 50)
    dirHistory.value = next
    try {
      localStorage.setItem(WORK_DIR_HISTORY_KEY, JSON.stringify(next))
    } catch {
      // 忽略存储失败
    }
  }

  loadDirHistory()

  const allKnownWorkDirs = computed(() => {
    const set = new Set<string>()
    for (const d of dirHistory.value) {
      if (d) set.add(d)
    }
    for (const p of projects.value) {
      if (p.local_dir) set.add(p.local_dir)
    }
    for (const n of rawNotifications.value) {
      if (n.work_dir) set.add(n.work_dir)
    }
    return Array.from(set)
  })

  // 解析工作目录展示名称与匹配的项目
  function resolveWorkDirInfo(workDir: string): { displayName: string; fullPath: string; projectName?: string } {
    const trimmed = (workDir || '').trim()
    if (!trimmed) {
      return {
        displayName: t('workflows.task.common.defaultDirectory') || '默认目录',
        fullPath: '',
      }
    }
    const norm = normalizePath(trimmed)
    const matchedProject = projects.value.find((p) => p.local_dir && normalizePath(p.local_dir) === norm)
    if (matchedProject) {
      return {
        displayName: matchedProject.name,
        fullPath: trimmed,
        projectName: matchedProject.name,
      }
    }
    return {
      displayName: getLastDirSegment(trimmed),
      fullPath: trimmed,
    }
  }

  const allConversations = computed<TaskConversation[]>(() => {
    const grouped = new Map<string, TaskNotification[]>()
    for (const item of rawNotifications.value) {
      const items = grouped.get(item.task_uuid) || []
      items.push(item)
      grouped.set(item.task_uuid, items)
    }

    return [...grouped.entries()]
      .map(([taskUuid, items]) => {
        items.sort((a, b) => b.created_at - a.created_at)
        const latest = items[0]
        const workDir = items.find((i) => i.work_dir)?.work_dir || ''
        const projectUuid = items.find((i) => i.project_uuid)?.project_uuid || ''
        const isArchived = items.some((i) => i.is_archived)

        return {
          task_uuid: taskUuid,
          title:
            latest.task_title ||
            latest.title ||
            t('workflows.task.feedback.taskFallback', { id: taskUuid.slice(0, 8) }),
          work_dir: workDir,
          project_uuid: projectUuid,
          items,
          latest,
          unread: items.some((i) => !i.is_read),
          is_archived: isArchived,
        }
      })
      .sort((a, b) => b.latest.created_at - a.latest.created_at)
  })

  function filterBySearch(conv: TaskConversation): boolean {
    const q = searchQuery.value.trim().toLowerCase()
    if (!q) return true
    return (
      conv.title.toLowerCase().includes(q) ||
      conv.work_dir.toLowerCase().includes(q)
    )
  }

  const activeConversations = computed(() =>
    allConversations.value.filter((c) => !c.is_archived && filterBySearch(c)),
  )

  const archivedConversations = computed(() =>
    allConversations.value.filter((c) => c.is_archived && filterBySearch(c)),
  )

  const unreadCount = computed(
    () => allConversations.value.filter((c) => !c.is_archived && c.unread).length,
  )

  function groupList(convs: TaskConversation[]): ConversationGroup[] {
    const groupsMap = new Map<string, TaskConversation[]>()
    for (const conv of convs) {
      // 斜杠、盘符大小写、尾部分隔符和 \\?\ 前缀不同，仍是同一目录。
      const dirKey = normalizePath(conv.work_dir)
      const list = groupsMap.get(dirKey) || []
      list.push(conv)
      groupsMap.set(dirKey, list)
    }

    const groups: ConversationGroup[] = []
    for (const [dirKey, items] of groupsMap.entries()) {
      const project = dirKey
        ? projects.value.find((item) => item.local_dir && normalizePath(item.local_dir) === dirKey)
        : undefined
      const rawDir = project?.local_dir
        || items.map((item) => item.work_dir.trim()).find(Boolean)
        || ''
      const info = resolveWorkDirInfo(rawDir)
      groups.push({
        key: dirKey || '__empty__',
        workDir: rawDir,
        displayName: info.displayName,
        fullPath: info.fullPath,
        projectName: info.projectName,
        items,
      })
    }

    return groups
  }

  const activeGroups = computed(() => groupList(activeConversations.value))
  const archivedGroups = computed(() => groupList(archivedConversations.value))

  async function loadProjects() {
    try {
      const res = await apiClient.get<{ items: LocalProject[] }>('/projects')
      projects.value = res.items || []
    } catch {
      // 容错保留空数组
    }
  }

  async function loadConversations(force = false) {
    if (loading.value && !force) return
    const requestVersion = ++loadVersion
    loading.value = true
    error.value = ''
    try {
      const [notifRes] = await Promise.all([
        apiClient.get<{ items: TaskNotification[] }>('/notifications', {
          include_archived: 1,
        }),
        loadProjects(),
      ])
      if (requestVersion !== loadVersion) return
      rawNotifications.value = notifRes.items || []
      loaded.value = true
    } catch (err) {
      if (requestVersion === loadVersion) {
        error.value = err instanceof Error ? err.message : '加载失败'
      }
    } finally {
      if (requestVersion === loadVersion) loading.value = false
    }
  }

  async function setTaskRead(taskUuid: string, isRead: boolean) {
    await apiClient.put(`/notifications/tasks/${encodeURIComponent(taskUuid)}/read`, {
      is_read: isRead,
    })
    for (const item of rawNotifications.value) {
      if (item.task_uuid === taskUuid) {
        item.is_read = isRead
      }
    }
  }

  async function archiveTask(taskUuid: string, isArchived: boolean) {
    await apiClient.put(`/notifications/tasks/${encodeURIComponent(taskUuid)}/archive`, {
      is_archived: isArchived,
    })
    for (const item of rawNotifications.value) {
      if (item.task_uuid === taskUuid) {
        item.is_archived = isArchived
      }
    }
  }

  function toggleGroupCollapse(key: string) {
    if (collapsedGroupKeys.value.has(key)) {
      collapsedGroupKeys.value.delete(key)
    } else {
      collapsedGroupKeys.value.add(key)
    }
  }

  function isGroupCollapsed(key: string): boolean {
    return collapsedGroupKeys.value.has(key)
  }

  function toggleArchivedCollapse() {
    archivedCollapsed.value = !archivedCollapsed.value
    try {
      localStorage.setItem(ARCHIVED_COLLAPSED_KEY, String(archivedCollapsed.value))
    } catch {
      // 忽略
    }
  }

  function setSearchQuery(q: string) {
    searchQuery.value = q
  }

  function upsertNotification(item: TaskNotification) {
    const idx = rawNotifications.value.findIndex((n) => n.uuid === item.uuid)
    if (idx >= 0) {
      rawNotifications.value[idx] = { ...rawNotifications.value[idx], ...item }
    } else {
      rawNotifications.value.unshift(item)
    }
  }

  return {
    rawNotifications,
    projects,
    loading,
    loaded,
    error,
    searchQuery,
    archivedCollapsed,
    dirHistory,
    allKnownWorkDirs,
    allConversations,
    activeConversations,
    archivedConversations,
    activeGroups,
    archivedGroups,
    unreadCount,
    resolveWorkDirInfo,
    loadConversations,
    loadProjects,
    setTaskRead,
    archiveTask,
    toggleGroupCollapse,
    isGroupCollapsed,
    toggleArchivedCollapse,
    setSearchQuery,
    recordWorkingDir,
    upsertNotification,
  }
})
