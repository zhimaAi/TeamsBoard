<template>
  <div class="task-board">
    <button
      v-if="false"
      type="button"
      @click="openSettings"
    >
      列表设置
    </button>

    <ASpin :spinning="loading">
      <div class="lanes scrollbar--subtle">
        <section
          v-for="lane in visibleLanes"
          :key="lane.lane_key"
          class="lane"
          :class="{ 'drag-over': dragOverLane === lane.lane_key }"
          :style="{ '--lane-background': laneBackground(lane) }"
          @dragover="onDragOver"
          @dragenter="onDragEnter(lane.lane_key)"
          @dragleave="onDragLeave($event, lane.lane_key)"
          @drop="onDrop($event, lane.lane_key)"
        >
          <div class="lane-header">
            <span class="lane-title">
              <span class="lane-label">
                <span
                  class="lane-dot"
                  :style="{ background: lane.color }"
                />
                {{ lane.title }}
              </span>
              <span class="lane-count">{{ grouped[lane.lane_key]?.length || 0 }}</span>
            </span>
            <a-popover
              :open="colorPickerLaneKey === lane.lane_key"
              trigger="click"
              placement="bottomRight"
              :arrow="false"
              destroy-tooltip-on-hide
              overlay-class-name="lane-color-popover"
              @open-change="handleColorPickerOpenChange(lane.lane_key, $event)"
            >
              <template #content>
                <div class="lane-color-panel">
                  <p>修改背景颜色</p>
                  <div class="lane-color-presets">
                    <button
                      v-for="color in laneBackgroundPresets"
                      :key="color"
                      type="button"
                      class="lane-color-swatch"
                      :class="{ selected: isLaneBackgroundSelected(lane, color) }"
                      :style="{ backgroundColor: color }"
                      :aria-label="`设置为 ${color} 背景色`"
                      :aria-pressed="isLaneBackgroundSelected(lane, color)"
                      @click="setLaneBackground(lane, color)"
                    />
                    <button type="button" class="lane-color-swatch reset-color" aria-label="恢复默认白色背景" @click="resetLaneBackground(lane)">
                      <img :src="noColorIcon" alt="" />
                    </button>
                  </div>
                  <div class="custom-color-row">
                    <p>自定义颜色</p>
                    <label class="lane-color-swatch custom-color-swatch" :style="{ backgroundColor: laneBackground(lane) }">
                      <input
                        type="color"
                        :value="laneBackground(lane)"
                        :aria-label="`为${lane.title}选择自定义背景色`"
                        @input="setCustomLaneBackground(lane, $event)"
                      />
                    </label>
                  </div>
                </div>
              </template>
              <button type="button" class="lane-color-btn" :aria-label="`设置${lane.title}的背景颜色`">
                <span class="lane-more-icon" aria-hidden="true"><img :src="ellipsisDotIcon" alt="" /><img :src="ellipsisDotIcon" alt="" /><img :src="ellipsisDotIcon" alt="" /></span>
              </button>
            </a-popover>
          </div>

          <div class="task-list scrollbar--subtle">
            <TaskBoardCard
              v-for="task in grouped[lane.lane_key]"
              :key="task.uuid"
              :task="task"
              :dragging="draggingUuid === task.uuid"
              @drag-start="onDragStart"
              @open="openTask"
            />

            <div
              v-if="(grouped[lane.lane_key]?.length || 0) === 0"
              class="lane-empty"
            >
              <EmptyIssueIcon
                class="empty-icon"
                aria-hidden="true"
              />
              <span class="empty-text">暂无数据</span>
            </div>
          </div>
        </section>
      </div>
    </ASpin>

    <TaskBoardSettingsModal
      v-model:open="settingsVisible"
      :lanes="allLanes"
      :task-view-mode="taskViewMode"
      @lanes-change="handleSettingsLanesChange"
      @saved="handleSettingsSaved"
    />

    <ADrawer
      v-model:open="taskDrawerOpen"
      title="任务详情"
      placement="right"
      width="min(1100px, 92vw)"
      :body-style="{ padding: 0, overflow: 'hidden' }"
      :destroy-on-close="true"
      @close="closeTaskPreview"
    >
      <div
        v-if="selectedTaskUuid"
        class="task-preview-content"
      >
        <TaskDetail
          :task-uuid="selectedTaskUuid"
          embedded
          @close="closeTaskPreview"
        />
      </div>
    </ADrawer>

    <AModal
      v-model:open="taskModalOpen"
      title="任务详情"
      width="min(1200px, calc(100vw - 48px))"
      :body-style="{ height: '85vh', padding: 0, overflow: 'hidden' }"
      :footer="null"
      :destroy-on-close="true"
      wrap-class-name="task-preview-modal"
      @cancel="closeTaskPreview"
    >
      <div
        v-if="selectedTaskUuid"
        class="task-preview-content"
      >
        <TaskDetail
          :task-uuid="selectedTaskUuid"
          embedded
          @close="closeTaskPreview"
        />
      </div>
    </AModal>

    <CreateLocalTaskModal
      v-model:open="createModalOpen"
      :initial-status="createStatus"
      @created="onTaskCreated"
    />

    <AssignPipelineModal
      v-model:open="assignModalOpen"
      :task-uuid="pendingAssignTask?.uuid || ''"
      :task-title="pendingAssignTask?.title || ''"
      mode="board"
      @assigned="load"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'
import AssignPipelineModal from '@/components/AssignPipelineModal.vue'
import CreateLocalTaskModal from '@/components/CreateLocalTaskModal.vue'
import type { TaskBoardLane, TaskBoardTask, TaskViewMode } from '@/types/task-board'
import TaskDetail from '@/views/workflows/TaskDetail.vue'
import ellipsisDotIcon from '@/assets/ellipsis-dot.svg'
import noColorIcon from '@/assets/no-color.svg'
import EmptyIssueIcon from './EmptyIssueIcon.vue'
import TaskBoardCard from './TaskBoardCard.vue'
import TaskBoardSettingsModal from './TaskBoardSettingsModal.vue'

const TASK_VIEW_MODE_STORAGE_KEY = 'goteams.workflows.task-view-mode'
const LANE_BACKGROUND_STORAGE_KEY = 'goteams.workflows.board-lane-backgrounds.v1'
const DEFAULT_LANE_BACKGROUND = '#fbfbfc'
const laneBackgroundPresets = ['#fff7cd', '#e7f0ff', '#e8f9ef', '#f1f2f4', DEFAULT_LANE_BACKGROUND]

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

function readLaneBackgrounds(): Record<string, string> {
  try {
    const storedBackgrounds = JSON.parse(window.localStorage.getItem(LANE_BACKGROUND_STORAGE_KEY) || '{}') as Record<string, unknown>
    return Object.fromEntries(Object.entries(storedBackgrounds).filter((entry): entry is [string, string] => (
      typeof entry[1] === 'string' && /^#[0-9a-f]{6}$/i.test(entry[1])
    )))
  } catch {
    return {}
  }
}

const router = useRouter()
const loading = ref(false)
const tasks = ref<TaskBoardTask[]>([])
const updating = ref<string>()
const allLanes = ref<TaskBoardLane[]>([])
const taskViewMode = ref<TaskViewMode>(readTaskViewMode())
const selectedTaskUuid = ref('')
const taskDrawerOpen = ref(false)
const taskModalOpen = ref(false)
const assignModalOpen = ref(false)
const pendingAssignTask = ref<TaskBoardTask>()
const draggingUuid = ref<string | null>(null)
const dragOverLane = ref<string | null>(null)
const createModalOpen = ref(false)
const createStatus = ref('')
const settingsVisible = ref(false)
const colorPickerLaneKey = ref<string | null>(null)
const laneBackgrounds = ref<Record<string, string>>(readLaneBackgrounds())

const visibleLanes = computed(() => allLanes.value.filter((lane) => !lane.is_hidden))

const grouped = computed(() => Object.fromEntries(
  visibleLanes.value.map((lane) => [
    lane.lane_key,
    tasks.value.filter((task) => laneStatus(task.status) === lane.lane_key),
  ]),
))

onMounted(load)

function laneStatus(status: string) {
  if (status === 'todo') return 'pending'
  if (['in_progress', 'running', 'developing', 'developed'].includes(status)) return 'active'
  if (status === 'completed') return 'done'
  return status
}

async function load() {
  loading.value = true
  try {
    const [taskResponse, laneResponse] = await Promise.all([
      apiClient.get<{ items: TaskBoardTask[] }>('/tasks', { page_size: 100 }),
      apiClient.get<{ items: TaskBoardLane[] }>('/tasks/task-lanes'),
    ])
    tasks.value = taskResponse.items || []
    allLanes.value = laneResponse.items || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function onDragStart(event: DragEvent, task: TaskBoardTask) {
  draggingUuid.value = task.uuid
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', task.uuid)
  }
}

function onDragEnter(laneKey: string) {
  dragOverLane.value = laneKey
}

function onDragLeave(event: DragEvent, laneKey: string) {
  const relatedTarget = event.relatedTarget as Node | null
  const currentTarget = event.currentTarget as HTMLElement
  if (!relatedTarget || !currentTarget.contains(relatedTarget)) {
    if (dragOverLane.value === laneKey) dragOverLane.value = null
  }
}

async function onDrop(event: DragEvent, targetStatus: string) {
  event.preventDefault()
  dragOverLane.value = null
  const taskUuid = event.dataTransfer?.getData('text/plain') || draggingUuid.value
  draggingUuid.value = null
  if (!taskUuid) return

  const task = tasks.value.find((item) => item.uuid === taskUuid)
  if (!task || task.status === targetStatus) return
  await changeStatus(task, targetStatus)
}

function onDragOver(event: DragEvent) {
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
}

async function changeStatus(task: TaskBoardTask, status: string) {
  if (task.status === status) return
  const shouldStart = ['active', 'in_progress', 'developing'].includes(status)
  if (shouldStart && !task.pipeline_snapshot_uuid) {
    pendingAssignTask.value = task
    assignModalOpen.value = true
    return
  }
  // 流水线步骤缺少提示词/CLI/模型时，弹窗补全
  if (shouldStart && task.pipeline_snapshot_uuid) {
    try {
      const detail = await apiClient.get<{ steps: Array<{ prompt_snapshot?: string; cli_type?: string; model_name?: string }> }>(`/tasks/${task.uuid}`)
      if (detail.steps?.some(s => !s.prompt_snapshot || !s.cli_type || !s.model_name)) {
        pendingAssignTask.value = task
        assignModalOpen.value = true
        return
      }
    } catch { /* 加载失败时直接调用 API */ }
  }
  updating.value = task.uuid
  try {
    await apiClient.put(`/tasks/${task.uuid}/status`, { status })
    task.status = status
    if (shouldStart) message.success('任务已切换为进行中并自动启动')
    else message.success('任务状态已更新')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
  } finally {
    updating.value = undefined
  }
}

function openCreate(status?: string) {
  createStatus.value = status || ''
  createModalOpen.value = true
}

function laneBackground(lane: TaskBoardLane) {
  return laneBackgrounds.value[lane.lane_key] || DEFAULT_LANE_BACKGROUND
}

function isLaneBackgroundSelected(lane: TaskBoardLane, color: string) {
  return laneBackground(lane).toLowerCase() === color.toLowerCase()
}

function persistLaneBackgrounds() {
  try {
    window.localStorage.setItem(LANE_BACKGROUND_STORAGE_KEY, JSON.stringify(laneBackgrounds.value))
  } catch {
    // 本地存储不可用时，仅保留当前页面会话内的颜色设置。
  }
}

function setLaneBackground(lane: TaskBoardLane, color: string) {
  laneBackgrounds.value = { ...laneBackgrounds.value, [lane.lane_key]: color }
  persistLaneBackgrounds()
}

function resetLaneBackground(lane: TaskBoardLane) {
  const { [lane.lane_key]: _, ...backgrounds } = laneBackgrounds.value
  laneBackgrounds.value = backgrounds
  persistLaneBackgrounds()
}

function setCustomLaneBackground(lane: TaskBoardLane, event: Event) {
  setLaneBackground(lane, (event.target as HTMLInputElement).value)
}

function handleColorPickerOpenChange(laneKey: string, open: boolean) {
  colorPickerLaneKey.value = open ? laneKey : null
}

function onTaskCreated(taskUuid: string) {
  void load()
  void router.push(`/board/task/${taskUuid}`)
}

defineExpose({ openCreate })

function openTask(taskUuid: string) {
  if (taskViewMode.value === 'new-window') {
    void router.push(`/board/task/${taskUuid}`)
    return
  }

  selectedTaskUuid.value = taskUuid
  if (taskViewMode.value === 'drawer') taskDrawerOpen.value = true
  else taskModalOpen.value = true
}

function closeTaskPreview() {
  taskDrawerOpen.value = false
  taskModalOpen.value = false
  selectedTaskUuid.value = ''
  void load()
}

function openSettings() {
  settingsVisible.value = true
}

function handleSettingsLanesChange(lanes: TaskBoardLane[]) {
  allLanes.value = lanes
}

function handleSettingsSaved(viewMode: TaskViewMode) {
  taskViewMode.value = viewMode
  try {
    window.localStorage.setItem(TASK_VIEW_MODE_STORAGE_KEY, taskViewMode.value)
  } catch {
    // 本地存储不可用时，当前页面内的设置仍然生效。
  }
  void load()
}
</script>

<style scoped>
.task-board {
  display: flex;
  height: 100%;
  flex-direction: column;
  background: #fff;
}

.task-board :deep(.ant-spin-nested-loading) {
  min-height: 0;
  flex: 1;
}

.task-board :deep(.ant-spin-container) {
  height: 100%;
}

.lanes {
  display: flex;
  height: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  align-items: stretch;
  gap: 14px;
  padding-bottom: 4px;
}

.lane {
  display: flex;
  width: 280px;
  min-width: 280px;
  box-sizing: border-box;
  overflow: hidden;
  flex-shrink: 0;
  flex-direction: column;
  border: 0;
  border-radius: 16px;
  background: var(--lane-background, #fbfbfc);
  transition: border-color 0.2s, background 0.2s;
}

.lane.drag-over {
  border-color: #3157e2;
  box-shadow: inset 0 0 0 1px #3157e2;
}

.lane-header {
  display: flex;
  height: 50px;
  min-height: 50px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 11px;
}

.lane-title {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.lane-label {
  display: flex;
  align-items: center;
  gap: 9px;
}

.lane-dot {
  width: 9px;
  min-width: 9px;
  height: 9px;
  border-radius: 4.5px;
}

.lane-count {
  display: inline-flex;
  width: 22px;
  min-width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border-radius: 11px;
  color: #8c95a8;
  background: #edeff2;
  font-size: 12px;
  font-weight: 400;
  line-height: 14px;
}

.lane-color-btn {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  transition: background 0.2s;
}

.lane-color-btn:hover,
.lane-color-btn:focus-visible {
  outline: none;
  background: #e4e6eb;
}

.lane-color-btn:focus-visible,
.lane-color-swatch:focus-visible {
  box-shadow: 0 0 0 2px #3157e2;
}

.lane-more-icon {
  display: flex;
  align-items: center;
  gap: 2px;
}

.lane-more-icon img {
  width: 2px;
  height: 2px;
}

.lane-color-panel {
  width: 240px;
  box-sizing: border-box;
  padding: 16px;
}

.lane-color-panel > p,
.custom-color-row > p {
  margin: 0;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

.lane-color-presets {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.lane-color-swatch {
  position: relative;
  display: inline-flex;
  width: 28px;
  height: 28px;
  box-sizing: border-box;
  align-items: center;
  justify-content: center;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  cursor: pointer;
}

.lane-color-swatch.selected::after {
  position: absolute;
  inset: -5px;
  border: 2px solid #3157e2;
  border-radius: 6px;
  content: '';
  pointer-events: none;
}

.reset-color {
  padding: 0;
  background: #fff;
}

.reset-color img {
  width: 16px;
  height: 16px;
}

.custom-color-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 14px;
}

.custom-color-swatch {
  overflow: hidden;
}

.custom-color-swatch input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  opacity: 0;
  cursor: pointer;
}

:global(.lane-color-popover .ant-popover-inner) {
  padding: 0;
  border-radius: 16px;
  box-shadow: 0 6px 30px 5px rgba(0,0,0,.05), 0 16px 24px 2px rgba(0,0,0,.04), 0 8px 10px -5px rgba(0,0,0,.08);
}

.task-list {
  display: flex;
  overflow-y: auto;
  flex: 1;
  flex-direction: column;
  gap: 10px;
  padding: 0 12px 12px;
}

.lane-empty {
  display: flex;
  width: 100%;
  height: 144px;
  box-sizing: border-box;
  flex-direction: column;
  align-items: center;
  padding-top: 60px;
  color: rgba(0,0,0,.25);
}

.empty-icon {
  width: 64px;
  height: 40px;
}

.empty-text {
  margin-top: 8px;
  font-size: 14px;
  line-height: 22px;
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
