<template>
  <AModal
    :open="open"
    :title="t('workflows.board.listSettings')"
    :width="480"
    :footer="null"
    @update:open="handleOpenChange"
    @cancel="closeSettings"
  >
    <section class="view-mode-settings">
      <h3 class="settings-section-title">{{ t('workflows.board.viewMode') }}</h3>
      <ASegmented
        v-model:value="settingsTaskViewMode"
        class="view-mode-segmented"
        :options="taskViewModeOptions"
        block
      />
    </section>

    <h3 class="settings-section-title settings-section-title--lanes">{{ t('workflows.board.statusSettings') }}</h3>
    <div class="settings-lane-list">
      <div
        v-for="lane in settingsLanes"
        :key="lane.clientKey"
        class="settings-lane-item"
        :class="{ dragging: settingsDraggingKey === lane.clientKey }"
        draggable="true"
        @dragstart="onSettingsDragStart($event, lane)"
        @dragover="onSettingsDragOver"
        @drop="onSettingsDrop($event, lane)"
      >
        <span class="drag-handle">⋮</span>
        <input
          v-model="lane.title"
          class="lane-name-input"
          :placeholder="t('workflows.board.statusName')"
          maxlength="20"
          @blur="onLaneTitleBlur(lane)"
        />
        <span class="lane-task-count">{{ lane.task_count }}</span>
        <button
          type="button"
          class="visibility-btn"
          :class="{ hidden: lane.is_hidden }"
          :title="lane.is_hidden ? t('workflows.board.show') : t('workflows.board.hide')"
          @click="settingsToggleHidden(lane)"
        >
          <svg
            viewBox="0 0 16 16"
            fill="none"
            width="14"
            height="14"
          >
            <template v-if="!lane.is_hidden">
              <path
                d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5Z"
                stroke="currentColor"
                stroke-width="1.2"
              />
              <circle
                cx="8"
                cy="8"
                r="2"
                stroke="currentColor"
                stroke-width="1.2"
              />
            </template>
            <template v-else>
              <path
                d="M1 8s2.5-5 7-5 7 5 7 5-2.5 5-7 5-7-5-7-5Z"
                stroke="currentColor"
                stroke-width="1.2"
                opacity="0.35"
              />
              <circle
                cx="8"
                cy="8"
                r="2"
                stroke="currentColor"
                stroke-width="1.2"
                opacity="0.35"
              />
              <line
                x1="2"
                y1="2"
                x2="14"
                y2="14"
                stroke="currentColor"
                stroke-width="1.3"
                stroke-linecap="round"
              />
            </template>
          </svg>
        </button>
        <button
          type="button"
          class="delete-btn"
          :title="t('common.actions.delete')"
          @click="settingsRemoveLane(lane.clientKey)"
        >
          <svg
            viewBox="0 0 16 16"
            fill="none"
            width="14"
            height="14"
          >
            <path
              d="M3 4h10M6 4V3a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1M5 4v9a1 1 0 0 0 1 1h4a1 1 0 0 0 1-1V4"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>
      </div>
    </div>

    <div class="settings-add-row">
      <input
        v-model="newLaneTitle"
        class="lane-name-input"
        :placeholder="t('workflows.board.enterStatusName')"
        maxlength="20"
        @keydown.enter="settingsAddLane"
      />
      <span class="lane-task-count">0</span>
      <button
        type="button"
        class="delete-btn disabled"
        disabled
      >
        <svg
          viewBox="0 0 16 16"
          fill="none"
          width="14"
          height="14"
        >
          <path
            d="M3 4h10M6 4V3a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1M5 4v9a1 1 0 0 0 1 1h4a1 1 0 0 0 1-1V4"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
    </div>

    <div class="settings-footer">
      <button
        type="button"
        class="btn-cancel"
        @click="closeSettings"
      >
        {{ t('common.actions.cancel') }}
      </button>
      <button
        type="button"
        class="btn-add-lane"
        @click="settingsAddLane"
      >
        {{ t('workflows.board.addStatus') }}
      </button>
      <button
        type="button"
        class="btn-confirm"
        :disabled="settingsSaving"
        @click="saveSettings"
      >
        {{ settingsSaving ? t('workflows.board.saving') : t('common.actions.confirm') }}
      </button>
    </div>
  </AModal>
</template>

<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { notification } from 'ant-design-vue'
import { AppstoreOutlined, ColumnWidthOutlined, ExportOutlined } from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import type { TaskBoardLane, TaskViewMode } from '@/types/task-board'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

interface EditableLane extends TaskBoardLane {
  clientKey: string
}

interface TaskViewModeOption {
  label: ReturnType<typeof h>
  title: string
  value: TaskViewMode
}

const props = defineProps<{
  open: boolean
  lanes: TaskBoardLane[]
  taskViewMode: TaskViewMode
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'lanes-change': [lanes: TaskBoardLane[]]
  saved: [taskViewMode: TaskViewMode]
}>()

const taskViewModeOptions = computed<TaskViewModeOption[]>(() => [
  {
    label: h('span', { class: 'view-mode-option' }, [h(ColumnWidthOutlined), h('span', t('workflows.board.drawer'))]),
    title: t('workflows.board.drawer'),
    value: 'drawer',
  },
  {
    label: h('span', { class: 'view-mode-option' }, [h(AppstoreOutlined), h('span', t('workflows.board.modal'))]),
    title: t('workflows.board.modal'),
    value: 'modal',
  },
  {
    label: h('span', { class: 'view-mode-option' }, [h(ExportOutlined), h('span', t('workflows.board.newWindow'))]),
    title: t('workflows.board.newWindow'),
    value: 'new-window',
  },
])

const settingsTaskViewMode = ref<TaskViewMode>(props.taskViewMode)
const settingsLanes = ref<EditableLane[]>([])
const originalLanes = ref<TaskBoardLane[]>([])
const newLaneTitle = ref('')
const settingsSaving = ref(false)
const settingsDraggingKey = ref<string | null>(null)
let draftLaneSequence = 0

watch(() => props.open, (open) => {
  if (open) void initializeSettings()
})

function createEditableLane(lane: TaskBoardLane): EditableLane {
  return {
    ...lane,
    clientKey: lane.id > 0 ? `lane-${lane.id}` : `draft-${++draftLaneSequence}`,
  }
}

function toLane(lane: EditableLane): TaskBoardLane {
  return {
    id: lane.id,
    lane_key: lane.lane_key,
    title: lane.title,
    color: lane.color,
    sort_order: lane.sort_order,
    is_hidden: lane.is_hidden,
    task_count: lane.task_count,
  }
}

function emitLanesChange() {
  emit('lanes-change', settingsLanes.value.map(toLane))
}

async function initializeSettings() {
  settingsTaskViewMode.value = props.taskViewMode
  newLaneTitle.value = ''
  settingsDraggingKey.value = null

  let latestLanes = props.lanes.map((lane) => ({ ...lane }))
  try {
    const response = await apiClient.get<{ items: TaskBoardLane[] }>('/tasks/task-lanes')
    latestLanes = response.items || []
  } catch {
    // 获取最新状态失败时沿用父组件已有数据，避免阻断设置弹窗。
  }

  originalLanes.value = latestLanes.map((lane) => ({ ...lane }))
  settingsLanes.value = latestLanes.map(createEditableLane)
  emitLanesChange()
}

function handleOpenChange(open: boolean) {
  emit('update:open', open)
}

function closeSettings() {
  emit('update:open', false)
}

function settingsAddLane() {
  const title = newLaneTitle.value.trim()
  if (!title) return

  settingsLanes.value.push(createEditableLane({
    id: 0,
    lane_key: '',
    title,
    color: '#8c8c8c',
    sort_order: settingsLanes.value.length,
    is_hidden: 0,
    task_count: 0,
  }))
  newLaneTitle.value = ''
}

function settingsRemoveLane(clientKey: string) {
  settingsLanes.value = settingsLanes.value.filter((lane) => lane.clientKey !== clientKey)
}

async function settingsToggleHidden(lane: EditableLane) {
  lane.is_hidden = lane.is_hidden ? 0 : 1
  emitLanesChange()
  if (lane.id <= 0) return

  try {
    await apiClient.put(`/tasks/task-lanes/${lane.id}`, { is_hidden: lane.is_hidden })
    notification.success({ message: t('components.resourceTable.saveSuccess'), placement: 'topRight', duration: 2 })
  } catch {
    notification.error({ message: t('components.resourceTable.saveFailed'), placement: 'topRight', duration: 3 })
  }
}

async function onLaneTitleBlur(lane: EditableLane) {
  const title = lane.title.trim()
  if (!title) return

  if (lane.id === 0) {
    try {
      const response = await apiClient.post<{ id: number; lane_key: string }>('/tasks/task-lanes', {
        title,
        color: lane.color,
      })
      lane.id = response.id
      lane.lane_key = response.lane_key
      originalLanes.value.push(toLane(lane))
      emitLanesChange()
      notification.success({ message: t('workflows.board.statusCreated'), placement: 'topRight', duration: 2 })
    } catch {
      notification.error({ message: t('workflows.board.statusCreateFailed'), placement: 'topRight', duration: 3 })
    }
    return
  }

  try {
    await apiClient.put(`/tasks/task-lanes/${lane.id}`, { title })
    emitLanesChange()
    notification.success({ message: t('components.resourceTable.saveSuccess'), placement: 'topRight', duration: 2 })
  } catch {
    notification.error({ message: t('components.resourceTable.saveFailed'), placement: 'topRight', duration: 3 })
  }
}

function onSettingsDragStart(event: DragEvent, lane: EditableLane) {
  settingsDraggingKey.value = lane.clientKey
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', lane.clientKey)
  }
}

function onSettingsDragOver(event: DragEvent) {
  event.preventDefault()
}

function onSettingsDrop(event: DragEvent, targetLane: EditableLane) {
  event.preventDefault()
  const dragKey = settingsDraggingKey.value
  if (!dragKey || dragKey === targetLane.clientKey) return

  const fromIndex = settingsLanes.value.findIndex((lane) => lane.clientKey === dragKey)
  const toIndex = settingsLanes.value.findIndex((lane) => lane.clientKey === targetLane.clientKey)
  if (fromIndex < 0 || toIndex < 0) return

  const [movedLane] = settingsLanes.value.splice(fromIndex, 1)
  settingsLanes.value.splice(toIndex, 0, movedLane)
  settingsDraggingKey.value = null
}

async function saveSettings() {
  settingsSaving.value = true
  try {
    const lanes = settingsLanes.value.filter((lane) => lane.title.trim() !== '')
    for (const lane of lanes) {
      if (lane.id !== 0) continue
      const response = await apiClient.post<{ id: number; lane_key: string }>('/tasks/task-lanes', {
        title: lane.title.trim(),
        color: lane.color,
      })
      lane.id = response.id
      lane.lane_key = response.lane_key
    }

    for (const lane of lanes) {
      await apiClient.put(`/tasks/task-lanes/${lane.id}`, {
        title: lane.title,
        is_hidden: lane.is_hidden,
      })
    }

    const keepIds = new Set(lanes.map((lane) => lane.id))
    for (const originalLane of originalLanes.value) {
      if (keepIds.has(originalLane.id)) continue
      try {
        await apiClient.delete(`/tasks/task-lanes/${originalLane.id}`)
      } catch {
        // 状态仍被任务引用时后端会拒绝删除，继续保存其他设置。
      }
    }

    await apiClient.put('/tasks/task-lanes/reorder', { ids: lanes.map((lane) => lane.id) })
    emitLanesChange()
    emit('saved', settingsTaskViewMode.value)
    closeSettings()
    notification.success({ message: t('workflows.board.settingsSaved'), placement: 'topRight', duration: 2 })
  } catch (error) {
    notification.error({
      message: error instanceof Error ? error.message : t('components.resourceTable.saveFailed'),
      placement: 'topRight',
      duration: 3,
    })
  } finally {
    settingsSaving.value = false
  }
}
</script>

<style scoped>
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
  max-height: 360px;
  overflow-y: auto;
  flex-direction: column;
  gap: 8px;
  padding: 4px 0;
}

.settings-lane-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid #e5e6eb;
  border-radius: 8px;
  background: #f7f8fa;
  transition: background 0.15s, border-color 0.15s, opacity 0.15s;
}

.settings-lane-item:hover {
  border-color: #c5d4f0;
  background: #eef2ff;
}

.settings-lane-item.dragging {
  opacity: 0.4;
}

.drag-handle {
  padding: 0 2px;
  color: #c9cdd4;
  font-size: 16px;
  line-height: 1;
  cursor: grab;
  user-select: none;
}

.drag-handle:active {
  cursor: grabbing;
}

.lane-name-input {
  min-width: 0;
  height: 32px;
  flex: 1;
  padding: 0 10px;
  border: 1px solid #e5e6eb;
  border-radius: 6px;
  outline: none;
  color: #1d2129;
  background: #fff;
  font-size: 14px;
  transition: border-color 0.2s;
}

.lane-name-input:focus {
  border-color: #3157e2;
}

.lane-name-input::placeholder {
  color: #c9cdd4;
}

.lane-task-count {
  display: inline-flex;
  min-width: 24px;
  height: 22px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  padding: 0 6px;
  border-radius: 11px;
  color: #4e5969;
  background: #e5e6eb;
  font-size: 12px;
  font-weight: 600;
}

.visibility-btn,
.delete-btn {
  display: inline-flex;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 6px;
  color: #8c8c8c;
  background: transparent;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.visibility-btn:hover {
  color: #4e5969;
  background: #e5e6eb;
}

.visibility-btn.hidden {
  color: #c9cdd4;
}

.delete-btn:hover {
  color: #f53f3f;
  background: #fff1f0;
}

.delete-btn.disabled {
  color: #d9d9d9;
  cursor: not-allowed;
}

.delete-btn.disabled:hover {
  color: #d9d9d9;
  background: transparent;
}

.settings-add-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding: 10px 0 0;
  border-top: 1px solid #eeeff2;
}

.settings-add-row .lane-name-input {
  flex: 1;
}

.settings-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #eeeff2;
}

.btn-cancel,
.btn-add-lane,
.btn-confirm {
  height: 32px;
  padding: 0 16px;
  border-radius: 6px;
  background: #fff;
  font-size: 14px;
  cursor: pointer;
}

.btn-cancel {
  border: 1px solid #e5e6eb;
  color: #4e5969;
  transition: all 0.2s;
}

.btn-cancel:hover {
  border-color: #3157e2;
  color: #3157e2;
}

.btn-add-lane {
  border: 1px dashed #3157e2;
  color: #3157e2;
  transition: all 0.2s;
}

.btn-add-lane:hover {
  background: #eef2ff;
}

.btn-confirm {
  padding: 0 20px;
  border: none;
  color: #fff;
  background: #3157e2;
  transition: background 0.2s;
}

.btn-confirm:hover {
  background: #2745b8;
}

.btn-confirm:disabled {
  background: #a0b4f0;
  cursor: not-allowed;
}

@media (max-width: 640px) {
  .view-mode-segmented :deep(.ant-segmented-item-label) {
    padding: 0 6px;
  }

  .view-mode-segmented :deep(.view-mode-option) {
    gap: 5px;
    font-size: 13px;
  }
}
</style>
