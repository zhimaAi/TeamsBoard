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
        :title="task.blocked_reason || '执行异常导致任务阻塞'"
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
          v-if="task.pipeline_avatar_snapshot"
          :src="task.pipeline_avatar_snapshot"
          alt=""
        />
        {{ agentName(task) }}
      </span>
      <span
        v-else
        class="card-agent unassigned"
      >
        未指派流水线
      </span>
      <span
        v-if="task.updated_at"
        class="card-updated"
      >
        更新于 {{ relativeTime(task.updated_at) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { TaskBoardTask } from '@/types/task-board'

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
  return task.pipeline_name_snapshot || task.agent_name_snapshot || ''
}

function priorityClass(priority?: string): 'high' | 'medium' | 'low' {
  if (priority === '高' || priority === 'high') return 'high'
  if (priority === '中' || priority === 'medium') return 'medium'
  return 'low'
}

function priorityLabel(priority?: string) {
  if (priority === '高' || priority === 'high') return '高'
  if (priority === '中' || priority === 'medium') return '中'
  return '低'
}

function relativeTime(timestamp: number) {
  const value = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const elapsed = Math.max(0, Date.now() - value)
  const minutes = Math.floor(elapsed / 60000)
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`

  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}天前`
  return new Date(value).toLocaleDateString('zh-CN')
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  line-height: 20px;
}

.card-agent {
  display: flex;
  max-width: 72%;
  overflow: hidden;
  align-items: center;
  gap: 7px;
  color: #8c8c8c;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-agent img {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  object-fit: cover;
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
  flex-shrink: 0;
  color: #8c8c8c;
  white-space: nowrap;
}
</style>
