<template>
  <div class="task-list-view">
    <div
      class="status-filters"
      role="tablist"
      :aria-label="t('workflows.board.statusFilter')"
    >
      <button
        v-for="filter in filterOptions"
        :key="filter.key"
        type="button"
        role="tab"
        :class="{ active: selectedStatus === filter.key }"
        :aria-selected="selectedStatus === filter.key"
        @click="selectStatus(filter.key)"
      >
        {{ t('workflows.board.statusFilterCount', { label: filter.label, count: filter.count }) }}
      </button>
    </div>

    <ASpin
      class="list-spinner"
      :spinning="loading"
    >
      <div class="table-scroll scrollbar--subtle">
        <div class="task-table">
          <div class="table-header" role="row">
            <span>{{ t('workflows.board.columns.task') }}</span>
            <span>{{ t('workflows.board.columns.status') }}</span>
            <span>{{ t('workflows.board.columns.executor') }}</span>
            <span>{{ t('workflows.board.columns.time') }}</span>
            <span class="sr-only">{{ t('workflows.board.columns.actions') }}</span>
          </div>

          <div
            v-for="row in filteredRows"
            :key="row.key"
            class="task-row"
            :class="{ 'is-clickable': rowClickable }"
            role="row"
            :tabindex="rowClickable ? 0 : undefined"
            :aria-label="rowClickable ? row.title : undefined"
            @click="handleRowClick(row)"
            @keydown.enter.self.prevent="handleRowClick(row)"
            @keydown.space.self.prevent="handleRowClick(row)"
          >
            <div class="task-cell">
              <div class="task-title" :title="row.title">
                <span
                  v-if="priorityLabel(row.priority)"
                  class="priority-tag"
                  :class="priorityClass(row.priority)"
                >
                  {{ priorityLabel(row.priority) }}
                </span>
                <span class="task-title-text">{{ row.title }}</span>
              </div>
              <div
                v-if="row.activity"
                class="task-activity"
                :class="`is-${row.activity.status}`"
              >
                <SyncOutlined
                  v-if="row.activity.status === 'running'"
                  class="activity-icon running"
                  spin
                  aria-hidden="true"
                />
                <CheckOutlined
                  v-else-if="row.activity.status === 'completed'"
                  class="activity-icon completed"
                  aria-hidden="true"
                />
                <AlertOutlined
                  v-else
                  class="activity-icon error"
                  aria-hidden="true"
                />
                <span :title="activityText(row)">{{ activityText(row) }}</span>
              </div>
            </div>

            <div class="status-cell">
              <span class="status-dot" :style="{ backgroundColor: row.statusColor }" />
              <span>{{ row.statusLabel }}</span>
            </div>

            <div class="executor-cell">
              <img
                v-if="row.executorAvatar"
                :src="row.executorAvatar"
                alt=""
              />
              <span v-else-if="row.executorName" class="executor-fallback" aria-hidden="true">
                {{ row.executorName.slice(0, 1).toUpperCase() }}
              </span>
              <span class="executor-name" :title="row.executorName">
                {{ row.executorName || t('workflows.task.common.unassignedExecution') }}
              </span>
            </div>

            <time class="time-cell" :datetime="dateTimeValue(row.updatedAt)">
              {{ relativeTime(row.updatedAt) }}
            </time>

            <div class="action-cell">
              <a-tooltip
                v-for="action in row.actions"
                :key="action.kind"
                :title="action.label"
              >
                <button
                  type="button"
                  class="row-action"
                  :class="`is-${action.kind}`"
                  :aria-label="action.label"
                  @click.stop="emit('action', row, action.kind)"
                >
                  <SettingOutlined v-if="action.kind === 'configure'" aria-hidden="true" />
                  <RightOutlined v-else aria-hidden="true" />
                </button>
              </a-tooltip>
            </div>
          </div>

          <div v-if="!loading && filteredRows.length === 0" class="empty-state">
            <EmptyIssueIcon class="empty-icon" aria-hidden="true" />
            <span>{{ t('common.states.noData') }}</span>
          </div>
        </div>
      </div>
    </ASpin>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  AlertOutlined,
  CheckOutlined,
  RightOutlined,
  SettingOutlined,
  SyncOutlined,
} from '@ant-design/icons-vue'
import { useLocale } from '@/composables/useLocale'
import { useAppI18n } from '@/i18n'
import type {
  BoardListActionKind,
  BoardListFilter,
  BoardListRow,
  TaskLatestActivity,
} from '@/types/task-board'
import { formatRelativeTime } from '@/utils/relativeTime'
import EmptyIssueIcon from './EmptyIssueIcon.vue'

const props = withDefaults(defineProps<{
  rows: BoardListRow[]
  filters: BoardListFilter[]
  loading?: boolean
  defaultStatus?: string
  rowClickable?: boolean
}>(), {
  loading: false,
  defaultStatus: 'active',
  rowClickable: false,
})

const emit = defineEmits<{
  action: [row: BoardListRow, action: BoardListActionKind]
  'row-click': [row: BoardListRow]
}>()

const { t } = useAppI18n()
const { locale } = useLocale()
const selectedStatus = ref(props.defaultStatus)
const statusSelectedByUser = ref(false)

const filterOptions = computed(() => [
  {
    key: 'all',
    label: t('workflows.board.allStatuses'),
    count: props.rows.length,
  },
  ...props.filters.map((filter) => ({
    key: filter.key,
    label: filter.label,
    count: props.rows.filter((row) => row.status === filter.key).length,
  })),
])

const filteredRows = computed(() => (
  selectedStatus.value === 'all'
    ? props.rows
    : props.rows.filter((row) => row.status === selectedStatus.value)
))

watch(
  () => props.filters.map((filter) => filter.key),
  (keys) => {
    if (keys.length === 0) return
    if (!statusSelectedByUser.value && keys.includes(props.defaultStatus)) {
      selectedStatus.value = props.defaultStatus
      return
    }
    if (selectedStatus.value !== 'all' && !keys.includes(selectedStatus.value)) {
      selectedStatus.value = keys.includes(props.defaultStatus) ? props.defaultStatus : 'all'
    }
  },
  { immediate: true },
)

function selectStatus(status: string) {
  statusSelectedByUser.value = true
  selectedStatus.value = status
}

function handleRowClick(row: BoardListRow) {
  if (props.rowClickable) emit('row-click', row)
}

const activityDefaultKeys: Record<string, string> = {
  thinking: 'workflows.board.activityDefault.thinking',
  message: 'workflows.board.activityDefault.message',
  tool_call: 'workflows.board.activityDefault.toolCall',
  tool_result: 'workflows.board.activityDefault.toolResult',
  permission_request: 'workflows.board.activityDefault.permissionRequest',
}

function activityText(row: BoardListRow) {
  const activity = row.activity
  if (!activity) return ''
  const preview = activity.preview.trim()
  if (preview) return preview
  const defaultKey = activityDefaultKeys[activity.kind]
  return defaultKey ? t(defaultKey) : activityStatusLabel(activity)
}

function activityStatusLabel(activity: TaskLatestActivity) {
  return t(`workflows.board.activityStatus.${activity.status}`)
}

function priorityClass(priority?: string | number) {
  const value = String(priority || '').trim().toLowerCase()
  if (value === '紧急' || value === 'urgent') return 'urgent'
  if (value === '高' || value === 'high') return 'high'
  if (value === '中' || value === 'medium') return 'medium'
  if (value === '低' || value === 'low') return 'low'
  return 'custom'
}

function priorityLabel(priority?: string | number) {
  if (typeof priority !== 'string') return ''
  const kind = priorityClass(priority)
  return kind === 'custom' ? priority.trim() : t(`workflows.task.priority.${kind}`)
}

function relativeTime(value?: number | string) {
  const timestamp = Number(value || 0)
  return timestamp > 0 ? formatRelativeTime(timestamp, locale.value) : '-'
}

function dateTimeValue(value?: number | string) {
  const timestamp = Number(value || 0)
  if (timestamp <= 0) return undefined
  const normalized = timestamp < 1e12 ? timestamp * 1000 : timestamp
  return new Date(normalized).toISOString()
}
</script>

<style scoped>
.task-list-view {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  background: #fff;
}

.status-filters {
  display: flex;
  min-height: 48px;
  flex: 0 0 48px;
  align-items: flex-start;
  gap: 16px;
  overflow-x: auto;
}

.status-filters button {
  min-width: 90px;
  height: 32px;
  padding: 5px 14px;
  border: 0;
  border-radius: 999px;
  color: #595959;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
  white-space: nowrap;
}

.status-filters button.active {
  color: #fff;
  background: #262626;
}

.status-filters button:focus-visible,
.row-action:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.list-spinner {
  min-height: 0;
  flex: 1;
}

.list-spinner :deep(.ant-spin-container),
.list-spinner :deep(.ant-spin-nested-loading) {
  height: 100%;
}

.table-scroll {
  height: 100%;
  overflow: auto;
}

.task-table {
  min-width: 960px;
}

.table-header,
.task-row {
  display: grid;
  grid-template-columns: minmax(360px, 2.4fr) minmax(150px, .8fr) minmax(180px, 1fr) minmax(120px, .72fr) 88px;
  align-items: center;
}

.table-header {
  height: 52px;
  padding: 0 16px;
  color: #8c8c8c;
  background: #f5f5f5;
  font-size: 14px;
  line-height: 22px;
}

.task-row {
  min-height: 70px;
  padding: 0 16px;
  border-bottom: 1px solid #f0f0f0;
  transition: background .2s;
}

.task-row:hover {
  background: #f3f7ff;
}

.task-row.is-clickable {
  cursor: pointer;
}

.task-row.is-clickable:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}

.task-cell {
  min-width: 0;
  padding: 10px 20px 10px 0;
}

.task-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.task-title-text,
.task-activity span,
.executor-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.priority-tag {
  display: inline-flex;
  min-width: 22px;
  height: 22px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 400;
  line-height: 22px;
}

.priority-tag.urgent,
.priority-tag.high { color: #f5222d; background: #fff1f0; }
.priority-tag.medium { color: #fa541c; background: #fff2e8; }
.priority-tag.low,
.priority-tag.custom { color: #8c8c8c; background: #f5f5f5; }

.task-activity {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  margin-top: 2px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}

.activity-icon { flex: 0 0 auto; font-size: 13px; }
.activity-icon.running { color: #1890ff; }
.activity-icon.completed { color: #52c41a; }
.activity-icon.error { color: #f5222d; }

.status-cell,
.executor-cell,
.action-cell {
  display: flex;
  min-width: 0;
  align-items: center;
}

.status-cell {
  gap: 8px;
  color: #262626;
}

.status-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 50%;
}

.executor-cell {
  gap: 8px;
  padding-right: 20px;
  color: #595959;
}

.executor-cell img,
.executor-fallback {
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  border-radius: 50%;
}

.executor-cell img { object-fit: cover; }

.executor-fallback {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #595959;
  background: #edeff2;
  font-size: 11px;
}

.time-cell {
  color: #8c8c8c;
  white-space: nowrap;
}

.action-cell {
  justify-content: flex-end;
  gap: 6px;
}

.row-action {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: #8c8c8c;
  background: transparent;
  cursor: pointer;
  font-size: 16px;
}

.row-action:hover {
  color: #3157e2;
  background: #edf2ff;
}

.row-action.is-configure {
  color: #3157e2;
}

.empty-state {
  display: flex;
  min-height: 180px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: rgba(0, 0, 0, .25);
}

.empty-icon { width: 64px; height: 40px; margin-bottom: 8px; }

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  clip-path: inset(50%);
}
</style>
