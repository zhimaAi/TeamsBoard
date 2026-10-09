<template>
  <div
    class="task-card"
    :class="{ dragging }"
    :draggable="draggable"
    @dragstart="handleDragStart"
    @click="emit('open', task.uuid)"
  >
    <div class="card-title-row">
      <h4 class="card-title" :title="task.title">
        <span
          v-if="task.priority"
          class="priority-tag"
          :class="priorityClass(task.priority)"
        >
          {{ priorityLabel(task.priority) }}
        </span>
        {{ task.title }}
      </h4>
      <a-tooltip
        v-if="task.status === 'blocked'"
        :title="task.blocked_reason || t('workflows.board.blockedFallback')"
      >
        <span class="blocked-badge" aria-hidden="true" />
      </a-tooltip>
    </div>
    <div class="card-footer">
      <span
        v-if="agentName(task)"
        class="card-agent"
      >
        <img
          v-if="agentAvatar"
          :src="agentAvatar"
          alt=""
        />
        <span class="card-agent-name" :title="agentName(task)">
          {{ agentName(task) }}
        </span>
      </span>
      <span
        v-else
        class="card-agent unassigned"
      >
        <span
          class="card-agent-name"
          :title="t('workflows.task.common.unassignedExecution')"
        >
          {{ t('workflows.task.common.unassignedExecution') }}
        </span>
      </span>
      <span
        v-if="task.updated_at"
        class="card-updated"
        :title="t('workflows.board.updatedAt', { time: relativeTime(task.updated_at) })"
        :aria-label="t('workflows.board.updatedAt', { time: relativeTime(task.updated_at) })"
      >
        {{ relativeTime(task.updated_at) }}
      </span>
    </div>
    <a-tooltip
      v-if="task.latest_activity"
      placement="top"
      :trigger="['hover', 'focus']"
      overlay-class-name="task-board-activity-tooltip"
      @open-change="handleActivityTooltipOpen"
    >
      <template #title>
        <span class="activity-tooltip-content">{{ activityTooltipText }}</span>
      </template>
      <div
        class="card-activity"
        :class="`is-${task.latest_activity.status}`"
        :aria-label="activityAriaLabel"
        tabindex="0"
      >
        <SyncOutlined
          v-if="task.latest_activity.status === 'running'"
          class="activity-icon activity-icon-running"
          aria-hidden="true"
        />
        <CheckOutlined
          v-else-if="task.latest_activity.status === 'completed'"
          class="activity-icon activity-icon-completed"
          aria-hidden="true"
        />
        <AlertOutlined
          v-else
          class="activity-icon activity-icon-error"
          aria-hidden="true"
        />
        <span class="activity-preview">{{ activityPreviewText }}</span>
      </div>
    </a-tooltip>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AlertOutlined, CheckOutlined, SyncOutlined } from '@ant-design/icons-vue'
import { apiClient } from '@/api/client'
import type {
  TaskActivityDetail,
  TaskActivityStatus,
  TaskBoardTask,
  TaskLatestActivity,
} from '@/types/task-board'
import { useAppI18n } from '@/i18n'
import { useLocale } from '@/composables/useLocale'
import { useTaskExecutionDisplay } from '@/composables/useTaskExecutionDisplay'
import { formatRelativeTime } from '@/utils/relativeTime'

const { t } = useAppI18n()
const { locale } = useLocale()
const { executionName, executionAvatar } = useTaskExecutionDisplay()

const props = withDefaults(defineProps<{
  task: TaskBoardTask
  dragging?: boolean
  /** 团队工作看板只读展示任务卡片，不参与看板的拖拽改状态。 */
  draggable?: boolean
}>(), {
  dragging: false,
  draggable: true,
})

const emit = defineEmits<{
  open: [taskUuid: string]
  'drag-start': [event: DragEvent, task: TaskBoardTask]
}>()

const activityContent = ref('')
const loadedActivityID = ref('')
const loadingActivityID = ref('')
const activityTooltipOpen = ref(false)

const activityDefaultKeys: Record<string, string> = {
  thinking: 'workflows.board.activityDefault.thinking',
  message: 'workflows.board.activityDefault.message',
  tool_call: 'workflows.board.activityDefault.toolCall',
  tool_result: 'workflows.board.activityDefault.toolResult',
  permission_request: 'workflows.board.activityDefault.permissionRequest',
}

function activityStatusLabel(status: TaskActivityStatus) {
  return t(`workflows.board.activityStatus.${status}`)
}

function displayActivityPreview(activity: TaskLatestActivity) {
  const preview = activity.preview.trim()
  if (preview) return preview
  const defaultKey = activityDefaultKeys[activity.kind]
  return defaultKey ? t(defaultKey) : activityStatusLabel(activity.status)
}

const activityPreviewText = computed(() => {
  const activity = props.task.latest_activity
  return activity ? displayActivityPreview(activity) : ''
})

const activityTooltipText = computed(() => activityContent.value || activityPreviewText.value)

const activityAriaLabel = computed(() => {
  const activity = props.task.latest_activity
  if (!activity) return ''
  return t('workflows.board.activityAriaLabel', {
    status: activityStatusLabel(activity.status),
    content: activityPreviewText.value,
  })
})

async function loadActivityContent() {
  const activity = props.task.latest_activity
  if (!activity || loadedActivityID.value === activity.id || loadingActivityID.value === activity.id) return
  const requestedID = activity.id
  loadingActivityID.value = requestedID
  try {
    const detail = await apiClient.get<TaskActivityDetail>(
      `/tasks/${encodeURIComponent(props.task.uuid)}/activities/${encodeURIComponent(requestedID)}`,
    )
    if (props.task.latest_activity?.id !== requestedID) return
    activityContent.value = detail.content
    loadedActivityID.value = requestedID
  } catch {
    if (props.task.latest_activity?.id === requestedID) {
      activityContent.value = ''
      loadedActivityID.value = requestedID
    }
  } finally {
    if (loadingActivityID.value === requestedID) loadingActivityID.value = ''
  }
}

function handleActivityTooltipOpen(open: boolean) {
  activityTooltipOpen.value = open
  if (open) void loadActivityContent()
}

watch(() => props.task.latest_activity?.id, () => {
  activityContent.value = ''
  loadedActivityID.value = ''
  if (activityTooltipOpen.value) void loadActivityContent()
})

function handleDragStart(event: DragEvent) {
  // 只读卡片（团队工作看板）不参与拖拽改状态，即便事件被其它途径触发也直接忽略。
  if (!props.draggable) {
    event.preventDefault()
    return
  }
  emit('drag-start', event, props.task)
}

function agentName(task: TaskBoardTask) {
  return executionName(task)
}

/** 卡片头像：CLI 用固定 logo，其余沿用各自快照。 */
const agentAvatar = computed(() => executionAvatar(props.task))

function priorityClass(priority?: string): 'high' | 'medium' | 'low' {
  if (priority === '高' || priority === 'high') return 'high'
  if (priority === '中' || priority === 'medium') return 'medium'
  return 'low'
}

function priorityLabel(priority?: string) {
  if (priority === '高' || priority === 'high') return t('workflows.task.priority.high')
  if (priority === '中' || priority === 'medium') return t('workflows.task.priority.medium')
  return t('workflows.task.priority.low')
}

function relativeTime(timestamp: number) {
  return formatRelativeTime(timestamp, locale.value)
}
</script>

<style scoped>
.task-card {
  display: flex;
  width: 100%;
  box-sizing: border-box;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  border: 1px solid #dfe5ee;
  border-radius: 13px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(16,24,40,.03), 0 2px 4px rgba(34,52,79,.03);
  cursor: pointer;
  transition: box-shadow 0.2s, border-color 0.2s;
}

.task-card:hover {
  border-color: #c5d4f0;
  box-shadow: 0 4px 12px rgba(34, 52, 79, 0.07);
}

.task-card.dragging {
  opacity: 0.4;
}

.card-title-row {
  display: flex;
  min-height: 48px;
  align-items: flex-start;
  gap: 6px;
}

.card-title {
  display: -webkit-box;
  min-width: 0;
  overflow: hidden;
  flex: 1;
  margin: 0;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.blocked-badge {
  display: flex;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #ef4444;
  background: #fef2f2;
  font-size: 9px;
  font-weight: 700;
}

.card-footer {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  line-height: 20px;
}

.card-agent {
  display: flex;
  min-width: 0;
  overflow: hidden;
  align-items: center;
  gap: 7px;
  color: #8c8c8c;
  white-space: nowrap;
}

.card-agent img {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  border-radius: 50%;
  object-fit: cover;
}

.card-agent-name {
  min-width: 0;
  overflow: hidden;
  flex: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-agent.unassigned {
  color: #8c8c8c;
}

.priority-tag {
  display: inline-flex;
  align-items: center;
  padding: 0 3px;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  vertical-align: 1px;
}

.priority-tag.high {
  color: #ed744a;
  background: #fff5e5;
}

.priority-tag.medium {
  color: #3157e2;
  background: #e5efff;
}

.priority-tag.low {
  color: #16a34a;
  background: #f0fdf4;
}

.card-updated {
  color: #8c8c8c;
  white-space: nowrap;
}

.blocked-badge::before {
  content: '!';
}

.card-activity {
  display: flex;
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  align-items: center;
  gap: 6px;
  padding: 5px 8px 6px;
  border-radius: 10px;
  outline: none;
  background: #f3f5f8;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}

.card-activity.is-completed,
.card-activity.is-error {
  min-height: 34px;
}

.card-activity:focus-visible {
  box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.22);
}

.activity-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  font-size: 16px;
  line-height: 16px;
}

.activity-icon-running {
  animation: task-activity-spin 1s linear infinite;
  color: #40acef;
}

.activity-icon-completed {
  color: #47cb8a;
}

.activity-icon-error {
  color: #fc8f57;
}

.activity-preview {
  min-width: 0;
  overflow: hidden;
  flex: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@keyframes task-activity-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .activity-icon-running {
    animation: none;
  }
}

:global(.task-board-activity-tooltip) {
  max-width: 205px;
}

:global(.task-board-activity-tooltip .ant-tooltip-inner) {
  width: 205px;
  max-height: min(320px, 40vh);
  box-sizing: border-box;
  overflow: auto;
  padding: 6px 8px;
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.75);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  color: #fff;
  font-size: 14px;
  line-height: 22px;
  white-space: pre-wrap;
  word-break: break-word;
}

:global(.task-board-activity-tooltip .ant-tooltip-arrow::before) {
  background: rgba(0, 0, 0, 0.75);
}
</style>
