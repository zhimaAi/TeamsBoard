<template>
  <div
    class="task-card"
    :class="{ dragging }"
    draggable="true"
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
        <span class="blocked-badge">!</span>
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TaskBoardTask } from '@/types/task-board'
import { useAppI18n } from '@/i18n'
import { useLocale } from '@/composables/useLocale'
import { useCliNames } from '@/composables/useCliNames'
import cliExecutionLogo from '@/assets/icons/task-composer-cli.svg'

const { t } = useAppI18n()
const { locale } = useLocale()
const { cliDisplayName, ensureLoaded } = useCliNames()
void ensureLoaded()

const props = withDefaults(defineProps<{
  task: TaskBoardTask
  dragging?: boolean
}>(), {
  dragging: false,
})

const emit = defineEmits<{
  open: [taskUuid: string]
  'drag-start': [event: DragEvent, task: TaskBoardTask]
}>()

function handleDragStart(event: DragEvent) {
  emit('drag-start', event, props.task)
}

function agentName(task: TaskBoardTask) {
  if (task.execution_mode === 'vibe_coding' && task.execution_tool === 'codex') return 'Codex'
	if (task.execution_mode === 'expert_group') return task.expert_group_name_snapshot || ''
  // 需求 2204：CLI 直接执行的任务在看板卡片上显示 CLI 名称，与其他执行方式保持一致，
  // 不再落到「未指派执行方式」。看板列表接口不透出模型，卡片也不展示模型。
  if (task.execution_mode === 'cli') return cliDisplayName(task.execution_tool || '')
  return task.pipeline_name_snapshot || task.agent_name_snapshot || ''
}

/** 卡片头像：CLI 用固定 logo，其余沿用各自快照。 */
const agentAvatar = computed(() => {
  const task = props.task
  if (task.execution_mode === 'cli') return cliExecutionLogo
  if (task.execution_mode === 'expert_group') return task.expert_group_avatar_snapshot
  return task.pipeline_avatar_snapshot
})

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
  const value = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const elapsed = Math.max(0, Date.now() - value)
  const minutes = Math.floor(elapsed / 60000)
  if (minutes < 1) return t('workflows.board.justNow')
  if (minutes < 60) return t('workflows.board.minutesAgo', { count: minutes })

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('workflows.board.hoursAgo', { count: hours })

  const days = Math.floor(hours / 24)
  if (days < 30) return t('workflows.board.daysAgo', { count: days })
  return new Date(value).toLocaleDateString(locale.value)
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
</style>
