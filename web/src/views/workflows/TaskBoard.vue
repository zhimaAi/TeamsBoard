<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { message, notification } from 'ant-design-vue'
import { AppstoreOutlined, ColumnWidthOutlined, ExportOutlined } from '@ant-design/icons-vue'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'
import MarkdownIt from 'markdown-it'
import CreateTaskModal from '@/components/CreateTaskModal.vue'
import TaskDetail from '@/views/workflows/TaskDetail.vue'

const md = new MarkdownIt({ html: true, breaks: true, linkify: true })

interface Lane {
  id: number
  lane_key: string
  title: string
  color: string
  sort_order: number
  is_hidden: number
  task_count: number
}

interface Task {
  uuid: string
  title: string
  status: string
  execution_status: string
  agent_id: string
  work_item_type: string
  work_item_id: string
  created_at: number
  updated_at: number
  content_snapshot?: string
  agent_name_snapshot?: string
}

type TaskViewMode = 'drawer' | 'modal' | 'new-window'

const TASK_VIEW_MODE_STORAGE_KEY = 'goteams.workflows.task-view-mode'
const taskViewModeOptions = [
  {
    label: h('span', { class: 'view-mode-option' }, [h(ColumnWidthOutlined), h('span', '侧滑')]),
    title: '侧滑',
    value: 'drawer' as TaskViewMode,
  },
  {
    label: h('span', { class: 'view-mode-option' }, [h(AppstoreOutlined), h('span', '弹窗')]),
    title: '弹窗',
    value: 'modal' as TaskViewMode,
  },
  {
    label: h('span', { class: 'view-mode-option' }, [h(ExportOutlined), h('span', '新窗口')]),
    title: '新窗口',
    value: 'new-window' as TaskViewMode,
  },
]

function readTaskViewMode(): TaskViewMode {
  try {
    const storedMode = window.localStorage.getItem(TASK_VIEW_MODE_STORAGE_KEY)
    if (storedMode === 'drawer' || storedMode === 'modal' || storedMode === 'new-window') {
      return storedMode
    }
  } catch {
    // 浏览器禁用本地存储时，仍使用默认查看模式。
  }
  return 'new-window'
}

const router = useRouter()
const loading = ref(false)
const tasks = ref<Task[]>([])
const updating = ref<string>()
const allLanes = ref<Lane[]>([])
const taskViewMode = ref<TaskViewMode>(readTaskViewMode())
const settingsTaskViewMode = ref<TaskViewMode>(taskViewMode.value)
const selectedTaskUuid = ref('')
const taskDrawerOpen = ref(false)
const taskModalOpen = ref(false)

/** UUID of the task being dragged */
const draggingUuid = ref<string | null>(null)
/** Target column hovered during drag */
const dragOverLane = ref<string | null>(null)

/** Columns shown on the board (not hidden) */
const visibleLanes = computed(() => allLanes.value.filter((l) => !l.is_hidden))

const grouped = computed(() => Object.fromEntries(
  visibleLanes.value.map((lane) => [lane.lane_key, tasks.value.filter((task) => task.status === lane.lane_key)]),
))

async function load() {
  loading.value = true
  try {
    const [taskRes, laneRes] = await Promise.all([
      apiClient.get<{ items: Task[] }>('/tasks', { page_size: 100 }),
      apiClient.get<{ items: Lane[] }>('/tasks/task-lanes')
    ])
    tasks.value = taskRes.items || []
    allLanes.value = laneRes.items || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载失败')
  } finally {
    loading.value = false
  }
}

/** Drag start */
function onDragStart(e: DragEvent, task: Task) {
  draggingUuid.value = task.uuid
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', task.uuid)
  }
}

/** Drag enter column */
function onDragEnter(laneKey: string) {
  dragOverLane.value = laneKey
}

/** Drag leave column */
function onDragLeave(e: DragEvent, laneKey: string) {
  const related = e.relatedTarget as Node | null
  const currentTarget = e.currentTarget as HTMLElement
  if (!related || !currentTarget.contains(related)) {
    if (dragOverLane.value === laneKey) {
      dragOverLane.value = null
    }
  }
}

/** Drop into column */
async function onDrop(e: DragEvent, targetStatus: string) {
  e.preventDefault()
  dragOverLane.value = null
  const uuid = e.dataTransfer?.getData('text/plain') || draggingUuid.value
  draggingUuid.value = null
  if (!uuid) return

  const task = tasks.value.find((t) => t.uuid === uuid)
  if (!task || task.status === targetStatus) return

  await changeStatus(task, targetStatus)
}

/** Allow drop */
function onDragOver(e: DragEvent) {
  e.preventDefault()
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'move'
  }
}

async function changeStatus(task: Task, status: string) {
  if (task.status === status) return
  updating.value = task.uuid
  try {
    await apiClient.put(`/tasks/${task.uuid}/status`, { status })
    task.status = status
    message.success('任务状态已更新')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
  } finally {
    updating.value = undefined
  }
}

function formatTime(timestamp: number) {
  if (!timestamp) return '—'
  const d = new Date(timestamp)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** Render requirement content as HTML (supports markdown + embedded HTML) */
function renderContent(task: Task): string {
  const text = task.content_snapshot
  if (!text) return '<span class="card-desc-placeholder">暂无描述</span>'
  // Detect whether it is already HTML
  if (/<[a-z][\s\S]*>/i.test(text)) {
    return text
  }
  return md.render(text)
}

/** Agent name */
function agentName(task: Task) {
  return task.agent_name_snapshot || `Agent #${task.agent_id}`
}

onMounted(load)

/** New task dialog */
const createModalOpen = ref(false)
/** Initial status carried when creating from the column header "+" (empty string = default pending) */
const createStatus = ref('')

function openCreate(status?: string) {
  createStatus.value = status || ''
  createModalOpen.value = true
}

function onTaskCreated(uuid: string) {
  load()
  router.push(`/workflows/task/${uuid}`)
}

function openTask(taskUuid: string) {
  if (taskViewMode.value === 'new-window') {
    router.push(`/workflows/task/${taskUuid}`)
    return
  }

  selectedTaskUuid.value = taskUuid
  if (taskViewMode.value === 'drawer') {
    taskDrawerOpen.value = true
  } else {
    taskModalOpen.value = true
  }
}

function closeTaskPreview() {
  taskDrawerOpen.value = false
  taskModalOpen.value = false
  selectedTaskUuid.value = ''
  void load()
}

// ====== Status settings dialog ======
const settingsVisible = ref(false)
const settingsLanes = ref<Lane[]>([])
const newLaneTitle = ref('')
const settingsSaving = ref(false)
/** Lane id being drag-sorted */
const settingsDraggingId = ref<number | null>(null)

async function openSettings() {
  settingsVisible.value = true
  settingsTaskViewMode.value = taskViewMode.value
  newLaneTitle.value = ''
  // Always fetch the latest lanes data to ensure the dialog shows the latest state
  try {
    const res = await apiClient.get<{ items: Lane[] }>('/tasks/task-lanes')
    allLanes.value = res.items || []
  } catch { /* ignore */ }
  settingsLanes.value = allLanes.value.map((l) => ({ ...l }))
}

function closeSettings() {
  settingsVisible.value = false
}

function settingsAddLane() {
  const title = newLaneTitle.value.trim()
  if (!title) return
  settingsLanes.value.push({
    id: 0,
    lane_key: '',
    title,
    color: '#8c8c8c',
    sort_order: settingsLanes.value.length,
    is_hidden: 0,
    task_count: 0,
  })
  newLaneTitle.value = ''
}

function settingsRemoveLane(index: number) {
  settingsLanes.value.splice(index, 1)
}

function settingsToggleHidden(lane: Lane) {
  lane.is_hidden = lane.is_hidden ? 0 : 1
  // Sync to allLanes
  const idx = allLanes.value.findIndex((l) => l.id === lane.id)
  if (idx >= 0) allLanes.value[idx].is_hidden = lane.is_hidden
  if (lane.id > 0) {
    apiClient.put(`/tasks/task-lanes/${lane.id}`, { is_hidden: lane.is_hidden }).then(() => {
      notification.success({ message: '保存成功', placement: 'topRight', duration: 2 })
    }).catch(() => {
      notification.error({ message: '保存失败', placement: 'topRight', duration: 3 })
    })
  }
}

/** Save a single lane in real time on blur */
async function onLaneTitleBlur(lane: Lane) {
  const title = lane.title.trim()
  if (!title) return
  if (lane.id === 0) {
    // New lane, create via POST
    try {
      const res = await apiClient.post<{ id: number; lane_key: string }>('/tasks/task-lanes', {
        title,
        color: lane.color,
      })
      lane.id = res.id
      lane.lane_key = res.lane_key
      // Sync to allLanes so the board shows the new column immediately
      allLanes.value.push({ ...lane })
      notification.success({ message: '状态已创建', placement: 'topRight', duration: 2 })
    } catch {
      notification.error({ message: '创建状态失败', placement: 'topRight', duration: 3 })
    }
  } else {
    // Existing lane, update via PUT
    try {
      await apiClient.put(`/tasks/task-lanes/${lane.id}`, { title })
      // Sync to allLanes
      const idx = allLanes.value.findIndex((l) => l.id === lane.id)
      if (idx >= 0) allLanes.value[idx].title = title
      notification.success({ message: '保存成功', placement: 'topRight', duration: 2 })
    } catch {
      notification.error({ message: '保存失败', placement: 'topRight', duration: 3 })
    }
  }
}

function onSettingsDragStart(e: DragEvent, lane: Lane) {
  settingsDraggingId.value = lane.id
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(lane.id))
  }
}

function onSettingsDragOver(e: DragEvent) {
  e.preventDefault()
}

function onSettingsDrop(e: DragEvent, targetLane: Lane) {
  e.preventDefault()
  const dragId = settingsDraggingId.value
  if (dragId == null || dragId === targetLane.id) return
  const lanes = settingsLanes.value
  const fromIdx = lanes.findIndex((l) => l.id === dragId)
  const toIdx = lanes.findIndex((l) => l.id === targetLane.id)
  if (fromIdx < 0 || toIdx < 0) return
  const [moved] = lanes.splice(fromIdx, 1)
  lanes.splice(toIdx, 0, moved)
  settingsDraggingId.value = null
}

async function saveSettings() {
  settingsSaving.value = true
  try {
    // Filter out rows with empty titles
    const lanes = settingsLanes.value.filter((l) => l.title.trim() !== '')
    // 1. Handle newly added (id=0)
    for (const lane of lanes) {
      if (lane.id === 0) {
        const res = await apiClient.post<{ id: number; lane_key: string }>('/tasks/task-lanes', {
          title: lane.title.trim(),
          color: lane.color,
        })
        lane.id = res.id
        lane.lane_key = res.lane_key
      }
    }
    // 2. Update each existing lane's title / is_hidden
    for (const lane of lanes) {
      await apiClient.put(`/tasks/task-lanes/${lane.id}`, {
        title: lane.title,
        is_hidden: lane.is_hidden,
      })
    }
    // 3. Delete the removed ones (in allLanes but not in settingsLanes)
    const keepIds = new Set(lanes.map((l) => l.id))
    for (const old of allLanes.value) {
      if (!keepIds.has(old.id)) {
        try {
          await apiClient.delete(`/tasks/task-lanes/${old.id}`)
        } catch {
          // If deletion fails (task reference exists), skip
        }
      }
    }
    // 4. Re-sort
    await apiClient.put('/tasks/task-lanes/reorder', { ids: lanes.map((l) => l.id) })

    taskViewMode.value = settingsTaskViewMode.value
    try {
      window.localStorage.setItem(TASK_VIEW_MODE_STORAGE_KEY, taskViewMode.value)
    } catch {
      // 本地存储不可用时，当前页面内的设置仍然生效。
    }

    closeSettings()
    // Full refresh: lanes + task list
    await load()
    notification.success({ message: '列表设置已保存', placement: 'topRight', duration: 2 })
  } catch (error) {
    notification.error({ message: error instanceof Error ? error.message : '保存失败', placement: 'topRight', duration: 3 })
  } finally {
    settingsSaving.value = false
  }
}
</script>

<template>
  <div class="task-board">
    <!-- Top action bar -->
    <div class="board-topbar">
      <button class="btn-add" @click="openCreate()">
        <svg viewBox="0 0 16 16" fill="none" class="btn-icon">
          <path d="M8 2v12M2 8h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
        新增任务
      </button>
      <div class="topbar-right">
        <button class="btn-icon-only" @click="openSettings" title="列表设置">
          <svg viewBox="0 0 14 14" fill="none" class="btn-icon">
            <path d="M13.4492 8.77659L12.4258 7.90159C12.4742 7.60471 12.4992 7.30159 12.4992 6.99846C12.4992 6.69534 12.4742 6.39221 12.4258 6.09534L13.4492 5.22034C13.5264 5.15425 13.5817 5.06624 13.6076 4.96799C13.6336 4.86975 13.629 4.76592 13.5945 4.67034L13.5805 4.62971C13.2987 3.84225 12.8768 3.11227 12.3352 2.47502L12.307 2.44221C12.2413 2.36495 12.1537 2.30941 12.0558 2.28291C11.9579 2.25641 11.8543 2.2602 11.7586 2.29377L10.4883 2.74534C10.0195 2.36096 9.4961 2.05784 8.93047 1.84534L8.68516 0.517212C8.66665 0.417278 8.61817 0.325341 8.54617 0.253615C8.47416 0.181889 8.38204 0.133769 8.28203 0.115649L8.23985 0.107837C7.42578 -0.0390381 6.56953 -0.0390381 5.75547 0.107837L5.71328 0.115649C5.61328 0.133769 5.52115 0.181889 5.44915 0.253615C5.37714 0.325341 5.32867 0.417278 5.31016 0.517212L5.06328 1.85159C4.50217 2.06413 3.97964 2.3671 3.51641 2.74846L2.23672 2.29377C2.14104 2.25993 2.03733 2.25601 1.93937 2.28252C1.84141 2.30904 1.75383 2.36473 1.68828 2.44221L1.66016 2.47502C1.11914 3.11272 0.697309 3.84258 0.414847 4.62971L0.400784 4.67034C0.330472 4.86565 0.388284 5.0844 0.546097 5.22034L1.58203 6.10471C1.5336 6.39846 1.51016 6.69846 1.51016 6.9969C1.51016 7.2969 1.5336 7.5969 1.58203 7.88909L0.546097 8.77346C0.468903 8.83955 0.413654 8.92756 0.387697 9.02581C0.36174 9.12405 0.366305 9.22787 0.400784 9.32346L0.414847 9.36409C0.697659 10.1516 1.11641 10.8781 1.66016 11.5188L1.68828 11.5516C1.75399 11.6288 1.84157 11.6844 1.93948 11.7109C2.03738 11.7374 2.14101 11.7336 2.23672 11.7L3.51641 11.2453C3.98203 11.6281 4.50235 11.9313 5.06328 12.1422L5.31016 13.4766C5.32867 13.5765 5.37714 13.6685 5.44915 13.7402C5.52115 13.8119 5.61328 13.86 5.71328 13.8781L5.75547 13.886C6.57701 14.0336 7.4183 14.0336 8.23985 13.886L8.28203 13.8781C8.38204 13.86 8.47416 13.8119 8.54617 13.7402C8.61817 13.6685 8.66665 13.5765 8.68516 13.4766L8.93047 12.1485C9.49587 11.9365 10.0223 11.6324 10.4883 11.2485L11.7586 11.7C11.8543 11.7339 11.958 11.7378 12.0559 11.7113C12.1539 11.6848 12.2415 11.6291 12.307 11.5516L12.3352 11.5188C12.8789 10.8766 13.2977 10.1516 13.5805 9.36409L13.5945 9.32346C13.6648 9.13128 13.607 8.91253 13.4492 8.77659ZM11.3164 6.27971C11.3555 6.51565 11.3758 6.75784 11.3758 7.00002C11.3758 7.24221 11.3555 7.4844 11.3164 7.72034L11.2133 8.3469L12.3805 9.34534C12.2035 9.75298 11.9802 10.1389 11.7148 10.4953L10.2648 9.98127L9.77422 10.3844C9.40078 10.6906 8.98516 10.9313 8.53516 11.1L7.93985 11.3235L7.66016 12.8391C7.21886 12.8891 6.77333 12.8891 6.33203 12.8391L6.05235 11.3203L5.46172 11.0938C5.01641 10.925 4.60235 10.6844 4.23203 10.3797L3.74141 9.97503L2.28203 10.4938C2.01641 10.136 1.79453 9.75002 1.61641 9.34377L2.7961 8.33596L2.69453 7.71096C2.65703 7.47815 2.63672 7.23752 2.63672 7.00002C2.63672 6.76096 2.65547 6.5219 2.69453 6.28909L2.7961 5.66409L1.61641 4.65627C1.79297 4.24846 2.01641 3.86409 2.28203 3.50627L3.74141 4.02502L4.23203 3.62034C4.60235 3.31565 5.01641 3.07502 5.46172 2.90627L6.05391 2.68284L6.3336 1.16409C6.77266 1.11409 7.2211 1.11409 7.66172 1.16409L7.94141 2.67971L8.53672 2.90315C8.98516 3.0719 9.40235 3.31252 9.77578 3.61877L10.2664 4.0219L11.7164 3.50784C11.982 3.86565 12.2039 4.25159 12.382 4.65784L11.2148 5.65627L11.3164 6.27971ZM6.99922 4.09377C5.48047 4.09377 4.24922 5.32502 4.24922 6.84377C4.24922 8.36252 5.48047 9.59377 6.99922 9.59377C8.51797 9.59377 9.74922 8.36252 9.74922 6.84377C9.74922 5.32502 8.51797 4.09377 6.99922 4.09377ZM8.23672 8.08127C8.07441 8.24405 7.88152 8.37313 7.66914 8.46108C7.45676 8.54904 7.22909 8.59413 6.99922 8.59377C6.53203 8.59377 6.09297 8.41096 5.76172 8.08127C5.59895 7.91897 5.46987 7.72607 5.38191 7.5137C5.29396 7.30132 5.24886 7.07364 5.24922 6.84377C5.24922 6.37659 5.43203 5.93752 5.76172 5.60627C6.09297 5.27502 6.53203 5.09377 6.99922 5.09377C7.46641 5.09377 7.90547 5.27502 8.23672 5.60627C8.3995 5.76858 8.52858 5.96148 8.61653 6.17385C8.70448 6.38623 8.74958 6.6139 8.74922 6.84377C8.74922 7.31096 8.56641 7.75002 8.23672 8.08127Z" fill="currentColor" fill-opacity="0.88"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- Board -->
    <a-spin :spinning="loading">
      <div class="lanes">
        <section
          v-for="lane in visibleLanes"
          :key="lane.lane_key"
          class="lane"
          :class="{ 'drag-over': dragOverLane === lane.lane_key }"
          @dragover="onDragOver"
          @dragenter="onDragEnter(lane.lane_key)"
          @dragleave="onDragLeave($event, lane.lane_key)"
          @drop="onDrop($event, lane.lane_key)"
        >
          <!-- Column header -->
          <div class="lane-header">
            <span class="lane-title">
              <span class="lane-dot" :style="{ background: lane.color }" />
              {{ lane.title }}
              <span class="lane-count">{{ grouped[lane.lane_key]?.length || 0 }}</span>
            </span>
            <button class="lane-add-btn" title="新建任务" @click="openCreate(lane.lane_key)">+</button>
          </div>

          <!-- Task card list -->
          <div class="task-list">
            <div
              v-for="task in grouped[lane.lane_key]"
              :key="task.uuid"
              class="task-card"
              :class="{ dragging: draggingUuid === task.uuid }"
              draggable="true"
              @dragstart="onDragStart($event, task)"
              @click="openTask(task.uuid)"
            >
              <!-- Top fixed tab: GoTeams icon + text -->
              <div class="card-brand">
                <span class="brand-icon">
                  <svg viewBox="0 0 16 16" fill="none">
                    <defs>
                      <linearGradient id="gtg" x1="0" y1="0" x2="16" y2="16" gradientUnits="userSpaceOnUse">
                        <stop stop-color="#8B5CF6"/>
                        <stop offset="1" stop-color="#3B82F6"/>
                      </linearGradient>
                    </defs>
                    <circle cx="5.5" cy="4.2" r="1.85" fill="url(#gtg)"/>
                    <circle cx="10.4" cy="4.2" r="1.85" fill="url(#gtg)"/>
                    <circle cx="3.7" cy="8" r="1.85" fill="url(#gtg)"/>
                    <circle cx="12.3" cy="8" r="1.85" fill="url(#gtg)"/>
                    <circle cx="5.95" cy="12.05" r="1.85" fill="url(#gtg)"/>
                    <circle cx="10.05" cy="12.05" r="1.85" fill="url(#gtg)"/>
                    <circle cx="8" cy="8" r="2.6" fill="url(#gtg)"/>
                    <path d="M6.4 10.1h.9A2.1 2.1 0 1 1 8.7 10.1h.9" stroke="#fff" stroke-width="1.1" stroke-linecap="round" fill="none"/>
                  </svg>
                </span>
                <span class="brand-text">GoTeams</span>
              </div>
              <!-- Title -->
              <h4 class="card-title">{{ task.title }}</h4>
              <!-- Description (requirement content) -->
              <div class="card-desc" v-html="renderContent(task)"></div>
              <!-- Divider + bottom info -->
              <div class="card-footer">
                <span class="card-agent">{{ agentName(task) }}</span>
                <span class="card-time">{{ formatTime(task.created_at) }}</span>
              </div>
            </div>

            <!-- Empty state -->
            <div v-if="(grouped[lane.lane_key]?.length || 0) === 0" class="lane-empty">
              <svg viewBox="0 0 64 64" fill="none" class="empty-icon">
                <!-- Rearmost card (faintest) -->
                <rect x="8" y="18" width="34" height="40" rx="5" stroke="currentColor" stroke-width="1.4" opacity="0.25"/>
                <!-- Middle card -->
                <rect x="15" y="12" width="34" height="40" rx="5" stroke="currentColor" stroke-width="1.4" opacity="0.4"/>
                <!-- Frontmost card (white background fill) -->
                <rect x="22" y="6" width="34" height="40" rx="5" fill="#f7f8fa" stroke="currentColor" stroke-width="1.4"/>
                <!-- Four-point star burst -->
                <path d="M39 20v-4M39 32v-4M33 26h-4M45 26h-4M35.2 22.2l-2.4-2.4M42.8 29.8l2.4 2.4M42.8 22.2l2.4-2.4M35.2 29.8l-2.4 2.4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
                <!-- Small sparkle -->
                <circle cx="45.5" cy="17.5" r="1.2" fill="currentColor" opacity="0.6"/>
              </svg>
              <span class="empty-text">无 Issue</span>
            </div>
          </div>
        </section>
      </div>
    </a-spin>

    <!-- Status settings dialog -->
    <a-modal
      v-model:open="settingsVisible"
      title="列表设置"
      :width="480"
      :footer="null"
      @cancel="closeSettings"
    >
      <section class="view-mode-settings">
        <h3 class="settings-section-title">查看模式</h3>
        <a-segmented
          v-model:value="settingsTaskViewMode"
          class="view-mode-segmented"
          :options="taskViewModeOptions"
          block
        />
      </section>

      <h3 class="settings-section-title settings-section-title--lanes">状态设置</h3>
      <div class="settings-lane-list">
        <div
          v-for="(lane, idx) in settingsLanes"
          :key="lane.id || 'new-' + idx"
          class="settings-lane-item"
          :class="{ dragging: settingsDraggingId === lane.id }"
          draggable="true"
          @dragstart="onSettingsDragStart($event, lane)"
          @dragover="onSettingsDragOver"
          @drop="onSettingsDrop($event, lane)"
        >
          <span class="drag-handle">⋮</span>
          <input
            v-model="lane.title"
            class="lane-name-input"
            placeholder="状态名称"
            maxlength="20"
            @blur="onLaneTitleBlur(lane)"
          />
          <span class="lane-task-count">{{ lane.task_count }}</span>
          <button
            class="visibility-btn"
            :class="{ hidden: lane.is_hidden }"
            :title="lane.is_hidden ? '点击显示' : '点击隐藏'"
            @click="settingsToggleHidden(lane)"
          >
            <svg viewBox="0 0 16 16" fill="none" width="14" height="14">
              <template v-if="!lane.is_hidden">
                <path d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5Z" stroke="currentColor" stroke-width="1.2"/>
                <circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.2"/>
              </template>
              <template v-else>
                <path d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5Z" stroke="currentColor" stroke-width="1.2" opacity="0.35"/>
                <circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.2" opacity="0.35"/>
                <line x1="2" y1="2" x2="14" y2="14" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
              </template>
            </svg>
          </button>
          <button class="delete-btn" title="删除" @click="settingsRemoveLane(idx)">
            <svg viewBox="0 0 16 16" fill="none" width="14" height="14">
              <path d="M3 4h10M6 4V3a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1M5 4v9a1 1 0 0 0 1 1h4a1 1 0 0 0 1-1V4" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- Add new status -->
      <div class="settings-add-row">
        <input
          v-model="newLaneTitle"
          class="lane-name-input"
          placeholder="输入状态名称"
          maxlength="20"
          @keydown.enter="settingsAddLane"
        />
        <span class="lane-task-count">0</span>
        <button class="delete-btn disabled" disabled>
          <svg viewBox="0 0 16 16" fill="none" width="14" height="14">
            <path d="M3 4h10M6 4V3a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1M5 4v9a1 1 0 0 0 1 1h4a1 1 0 0 0 1-1V4" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
      </div>

      <!-- Bottom button -->
      <div class="settings-footer">
        <button class="btn-cancel" @click="closeSettings">取消</button>
        <button class="btn-add-lane" @click="settingsAddLane">+ 添加状态</button>
        <button class="btn-confirm" :disabled="settingsSaving" @click="saveSettings">
          {{ settingsSaving ? '保存中...' : '确定' }}
        </button>
      </div>
    </a-modal>

    <a-drawer
      v-model:open="taskDrawerOpen"
      title="任务详情"
      placement="right"
      width="min(1100px, 92vw)"
      :body-style="{ padding: 0, overflow: 'hidden' }"
      :destroy-on-close="true"
      @close="closeTaskPreview"
    >
      <div v-if="selectedTaskUuid" class="task-preview-content">
        <TaskDetail :task-uuid="selectedTaskUuid" embedded @close="closeTaskPreview" />
      </div>
    </a-drawer>

    <a-modal
      v-model:open="taskModalOpen"
      title="任务详情"
      width="min(1200px, calc(100vw - 48px))"
      :body-style="{ height: '85vh', padding: 0, overflow: 'hidden' }"
      :footer="null"
      :destroy-on-close="true"
      wrap-class-name="task-preview-modal"
      @cancel="closeTaskPreview"
    >
      <div v-if="selectedTaskUuid" class="task-preview-content">
        <TaskDetail :task-uuid="selectedTaskUuid" embedded @close="closeTaskPreview" />
      </div>
    </a-modal>

    <!-- New task (three-step wizard) -->
    <CreateTaskModal
      v-model:open="createModalOpen"
      :initial-status="createStatus"
      @created="onTaskCreated"
    />
  </div>
</template>

<style scoped>
.task-board {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.task-board :deep(.ant-spin-nested-loading) {
  flex: 1;
  min-height: 0;
}

.task-board :deep(.ant-spin-container) {
  height: 100%;
}

/* ===== Top action bar ===== */
.board-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.btn-add {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #3157e2;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 7px 16px;
  font-size: 14px;
  cursor: pointer;
  transition: background 0.2s;
}

.btn-add:hover {
  background: #2745b8;
}

.btn-icon {
  width: 14px;
  height: 14px;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.btn-icon-only {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  color: #4e5969;
  transition: all 0.2s;
}

.btn-icon-only:hover {
  color: #3157e2;
  border-color: #3157e2;
}

.btn-icon-only .btn-icon {
  width: 16px;
  height: 16px;
}

.spin .btn-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* ===== Board column ===== */
.lanes {
  display: flex;
  gap: 12px;
  align-items: stretch;
  height: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 4px;
}

.lane {
  width: 316px;
  min-width: 316px;
  flex-shrink: 0;
  background: #f3f4f6;
  border: 1px solid #e5e6eb;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: border-color 0.2s, background 0.2s;
}

.lane.drag-over {
  border-color: #3157e2;
  background: #eef2ff;
}

.lane-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 14px 10px;
}

.lane-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: #1d2129;
}

.lane-dot {
  width: 8px;
  height: 8px;
  min-width: 8px;
  border-radius: 50%;
}

.lane-count {
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: #e5e6eb;
  color: #4e5969;
  font-size: 12px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.lane-add-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #86909c;
  font-size: 16px;
  line-height: 1;
  cursor: pointer;
  transition: all 0.2s;
}

.lane-add-btn:hover {
  color: #fff;
  background: #3157e2;
}

/* ===== Task card ===== */
.task-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.task-card {
  background: #fff;
  border: 1px solid #dfe5ee;
  border-radius: 13px;
  padding: 16px;
  cursor: pointer;
  transition: box-shadow 0.2s, border-color 0.2s;
  box-shadow: 0 2px 4px rgba(34, 52, 79, 0.03);
}

.task-card:hover {
  border-color: #c5d4f0;
  box-shadow: 0 4px 12px rgba(34, 52, 79, 0.07);
}

.task-card.dragging {
  opacity: 0.4;
}

.card-brand {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: #f2f4f7;
  border-radius: 6px;
  padding: 2px 6px;
}

.brand-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
}

.brand-icon svg {
  width: 100%;
  height: 100%;
}

.brand-text {
  font-size: 12px;
  font-weight: 400;
  color: #595959;
  line-height: 20px;
}

.card-title {
  margin: 12px 0 0;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  color: #262626;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
}

.card-desc {
  margin: 7px 0 0;
  min-height: 42px;
  font-size: 14px;
  line-height: 21px;
  color: #595959;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.card-desc :deep(p) {
  margin: 0 0 4px;
}

.card-desc :deep(p:last-child) {
  margin-bottom: 0;
}

.card-desc :deep(h1),
.card-desc :deep(h2),
.card-desc :deep(h3),
.card-desc :deep(h4),
.card-desc :deep(h5),
.card-desc :deep(h6) {
  margin: 0 0 4px;
  font-size: 14px;
  font-weight: 600;
  line-height: 21px;
}

.card-desc :deep(ul),
.card-desc :deep(ol) {
  margin: 0 0 4px;
  padding-left: 18px;
}

.card-desc :deep(li) {
  margin-bottom: 2px;
}

.card-desc :deep(a) {
  color: #3157e2;
  text-decoration: none;
}

.card-desc :deep(code) {
  background: #f2f4f7;
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 12px;
}

.card-desc :deep(pre) {
  margin: 0 0 4px;
  background: #f2f4f7;
  padding: 6px 8px;
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
}

.card-desc :deep(blockquote) {
  margin: 0 0 4px;
  padding-left: 10px;
  border-left: 3px solid #dfe5ee;
  color: #8c8c8c;
}

.card-desc :deep(img) {
  display: none;
}

.card-desc :deep(.card-desc-placeholder) {
  color: #c9cdd4;
  font-style: italic;
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  margin-top: 13px;
  padding-top: 12px;
  border-top: 1px solid #eeeff2;
}

.card-agent {
  color: #595959;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-time {
  color: #8c8c8c;
  white-space: nowrap;
}

/* ===== Empty state ===== */
.lane-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #c9cdd4;
}

.empty-icon {
  width: 56px;
  height: 56px;
  color: #b0b5be;
}

.empty-text {
  margin-top: 8px;
  font-size: 13px;
}

/* ===== Status settings dialog ===== */
.view-mode-settings {
  margin-bottom: 18px;
}

.settings-section-title {
  margin: 0 0 10px;
  color: #1d2129;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.settings-section-title--lanes {
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid #eeeff2;
}

.view-mode-segmented {
  width: 100%;
  padding: 4px;
  background: #f2f3f5;
}

.view-mode-segmented :deep(.ant-segmented-item-label) {
  min-height: 38px;
  padding: 0 12px;
  line-height: 38px;
}

.view-mode-segmented :deep(.view-mode-option) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #4e5969;
  font-size: 14px;
}

.view-mode-segmented :deep(.view-mode-option .anticon) {
  font-size: 16px;
}

.settings-lane-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 360px;
  overflow-y: auto;
  padding: 4px 0;
}

.settings-lane-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: #f7f8fa;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  transition: background 0.15s, border-color 0.15s, opacity 0.15s;
}

.settings-lane-item:hover {
  background: #eef2ff;
  border-color: #c5d4f0;
}

.settings-lane-item.dragging {
  opacity: 0.4;
}

.drag-handle {
  cursor: grab;
  color: #c9cdd4;
  font-size: 16px;
  line-height: 1;
  user-select: none;
  padding: 0 2px;
}

.drag-handle:active {
  cursor: grabbing;
}

.lane-name-input {
  flex: 1;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  font-size: 14px;
  color: #1d2129;
  background: #fff;
  outline: none;
  transition: border-color 0.2s;
}

.lane-name-input:focus {
  border-color: #3157e2;
}

.lane-name-input::placeholder {
  color: #c9cdd4;
}

.lane-task-count {
  min-width: 24px;
  height: 22px;
  padding: 0 6px;
  border-radius: 11px;
  background: #e5e6eb;
  color: #4e5969;
  font-size: 12px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.visibility-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #8c8c8c;
  cursor: pointer;
  flex-shrink: 0;
  transition: background 0.15s, color 0.15s;
}

.visibility-btn:hover {
  background: #e5e6eb;
  color: #4e5969;
}

.visibility-btn.hidden {
  color: #c9cdd4;
}

.delete-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #8c8c8c;
  cursor: pointer;
  flex-shrink: 0;
  transition: background 0.15s, color 0.15s;
}

.delete-btn:hover {
  background: #fff1f0;
  color: #f53f3f;
}

.delete-btn.disabled {
  color: #d9d9d9;
  cursor: not-allowed;
}

.delete-btn.disabled:hover {
  background: transparent;
  color: #d9d9d9;
}

.settings-add-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 0 0;
  border-top: 1px solid #eeeff2;
  margin-top: 12px;
}

.settings-add-row .lane-name-input {
  flex: 1;
}

.settings-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 16px;
  margin-top: 16px;
  border-top: 1px solid #eeeff2;
}

.btn-cancel {
  height: 32px;
  padding: 0 16px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  background: #fff;
  color: #4e5969;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-cancel:hover {
  border-color: #3157e2;
  color: #3157e2;
}

.btn-add-lane {
  height: 32px;
  padding: 0 16px;
  border: 1px dashed #3157e2;
  border-radius: 6px;
  background: #fff;
  color: #3157e2;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-add-lane:hover {
  background: #eef2ff;
}

.btn-confirm {
  height: 32px;
  padding: 0 20px;
  border: none;
  border-radius: 6px;
  background: #3157e2;
  color: #fff;
  font-size: 14px;
  cursor: pointer;
  transition: background 0.2s;
}

.btn-confirm:hover {
  background: #2745b8;
}

.btn-confirm:disabled {
  background: #a0b4f0;
  cursor: not-allowed;
}

.task-preview-content {
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

:global(.task-preview-modal .ant-modal) {
  top: 24px;
  max-width: calc(100vw - 48px);
  padding-bottom: 0;
}

@media (max-width: 640px) {
  .view-mode-segmented :deep(.ant-segmented-item-label) {
    padding: 0 6px;
  }

  .view-mode-segmented :deep(.view-mode-option) {
    gap: 5px;
    font-size: 13px;
  }

  :global(.task-preview-modal .ant-modal) {
    top: 12px;
    width: calc(100vw - 24px) !important;
    max-width: calc(100vw - 24px);
  }

  :global(.task-preview-modal .ant-modal-body) {
    height: calc(100vh - 92px) !important;
  }
}
</style>
