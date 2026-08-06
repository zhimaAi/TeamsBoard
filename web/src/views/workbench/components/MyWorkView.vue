<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CalendarOutlined,
  DownOutlined,
  LogoutOutlined,
  ReloadOutlined,
  RightOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import apiClient, { ApiError } from '@/api/client'
import CreateTaskModal from '@/components/CreateTaskModal.vue'
import { useDocumentTitle } from '@/composables/useDocumentTitle'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { MyWorkItem } from '@/types/workitem'

type WorkTab = 'todo' | 'done' | 'today' | 'yesterday' | 'tomorrow'

interface WorkGroup {
  id: string
  name: string
  color?: string
  items: MyWorkItem[]
}

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const loading = ref(false)
const logoutLoading = ref(false)
const errorText = ref('')
const items = ref<MyWorkItem[]>([])
const activeTab = ref<WorkTab>('todo')
const keyword = ref('')
const selectedPriority = ref<string>()
const collapsedGroups = ref(new Set<string>())
const userMenuOpen = ref(false)
const assignModalOpen = ref(false)
const selectedWorkItem = ref<MyWorkItem>()

const displayName = computed(() =>
  authStore.cloudUser?.displayName || authStore.cloudUser?.username || '用户',
)
const username = computed(() => authStore.cloudUser?.username || '')
const avatarText = computed(() => displayName.value.trim().charAt(0) || '用')
const avatarSource = computed(() => authStore.cloudUser?.avatar?.trim() || undefined)

function boolValue(value: MyWorkItem['is_done']) {
  return value === true || value === 1 || value === '1' || value === 'true'
}

function numberValue(value: number | string | undefined, fallback = 0) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function localDayTimestamp(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime() / 1000
}

const today = localDayTimestamp(new Date())
const yesterday = localDayTimestamp(new Date(Date.now() - 24 * 60 * 60 * 1000))
const tomorrow = localDayTimestamp(new Date(Date.now() + 24 * 60 * 60 * 1000))

function plannedDateCoversDay(item: MyWorkItem, target: number) {
  const start = numberValue(item.planned_start_date)
  const end = numberValue(item.planned_end_date)
  if (!start && !end) return false

  const startDay = start ? localDayTimestamp(new Date(start * 1000)) : 0
  const endDay = end ? localDayTimestamp(new Date(end * 1000)) : 0
  return (!startDay || startDay <= target) && (!endDay || target <= endDay)
}

const todoItems = computed(() => items.value.filter((item) => !boolValue(item.is_done)))
const doneItems = computed(() => items.value.filter((item) => boolValue(item.is_done)))

function dateCount(target: number) {
  return items.value.filter((item) => plannedDateCoversDay(item, target)).length
}

const tabs = computed(() => [
  { key: 'todo' as const, label: '全部待办', count: todoItems.value.length },
  { key: 'done' as const, label: '已办', count: doneItems.value.length },
  { key: 'today' as const, label: '今日', count: dateCount(today) },
  { key: 'yesterday' as const, label: '昨日', count: dateCount(yesterday) },
  { key: 'tomorrow' as const, label: '明日', count: dateCount(tomorrow) },
])

const activeTabTitle = computed(() =>
  tabs.value.find((tab) => tab.key === activeTab.value)?.label,
)

useDocumentTitle(activeTabTitle)

const priorityOptions = computed(() => {
  const priorities = new Map<string, number>()
  for (const item of items.value) {
    const name = item.priority_name?.trim()
    if (!name) continue
    const sortOrder = numberValue(item.priority_sort_order, Number.MAX_SAFE_INTEGER)
    priorities.set(name, Math.min(priorities.get(name) ?? Number.MAX_SAFE_INTEGER, sortOrder))
  }
  return [...priorities.entries()]
    .sort((a, b) => a[1] - b[1] || a[0].localeCompare(b[0], 'zh-CN'))
    .map(([value]) => ({ value, label: value }))
})

const tabItems = computed(() => {
  if (activeTab.value === 'done') return doneItems.value
  if (activeTab.value === 'today') {
    return items.value.filter((item) => plannedDateCoversDay(item, today))
  }
  if (activeTab.value === 'yesterday') {
    return items.value.filter((item) => plannedDateCoversDay(item, yesterday))
  }
  if (activeTab.value === 'tomorrow') {
    return items.value.filter((item) => plannedDateCoversDay(item, tomorrow))
  }
  return todoItems.value
})

const filteredItems = computed(() => {
  const text = keyword.value.trim().toLocaleLowerCase('zh-CN')
  return tabItems.value.filter((item) => {
    const matchesTitle = !text || item.title.toLocaleLowerCase('zh-CN').includes(text)
    const matchesPriority = !selectedPriority.value
      || item.priority_name === selectedPriority.value
    return matchesTitle && matchesPriority
  })
})

function compareItems(left: MyWorkItem, right: MyWorkItem) {
  if (activeTab.value === 'todo') return numberValue(right.id) - numberValue(left.id)
  if (activeTab.value === 'done') {
    return numberValue(right.updated_at) - numberValue(left.updated_at)
  }
  return numberValue(left.planned_start_date, Number.MAX_SAFE_INTEGER)
    - numberValue(right.planned_start_date, Number.MAX_SAFE_INTEGER)
    || numberValue(left.priority_sort_order, Number.MAX_SAFE_INTEGER)
      - numberValue(right.priority_sort_order, Number.MAX_SAFE_INTEGER)
    || numberValue(right.id) - numberValue(left.id)
}

const groups = computed<WorkGroup[]>(() => {
  const grouped = new Map<string, WorkGroup>()
  for (const item of filteredItems.value) {
    const id = String(item.workspace_id)
    const group = grouped.get(id) || {
      id,
      name: item.workspace_name || `工作区 #${id}`,
      color: item.workspace_color,
      items: [],
    }
    group.items.push(item)
    grouped.set(id, group)
  }
  return [...grouped.values()]
    .map((group) => ({ ...group, items: [...group.items].sort(compareItems) }))
    .sort((left, right) => {
      const itemComparison = compareItems(left.items[0], right.items[0])
      return itemComparison || left.name.localeCompare(right.name, 'zh-CN')
    })
})

const columns = [
  { title: '操作', key: 'action', width: 104 },
  { title: '标题', key: 'title', width: 520 },
  { title: '状态', key: 'status', width: 120 },
  { title: '优先级', key: 'priority', width: 110 },
  { title: '预计开始', key: 'planned_start_date', width: 126 },
  { title: '预计结束', key: 'planned_end_date', width: 126 },
  { title: '处理人', key: 'assignee', width: 220 },
]

function toggleGroup(id: string) {
  const next = new Set(collapsedGroups.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsedGroups.value = next
}

function typeLabel(type: MyWorkItem['type']) {
  if (type === 'defect') return '缺陷'
  if (type === 'task') return '任务'
  return '需求'
}

function formatDate(value: number | string | undefined) {
  const timestamp = numberValue(value)
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function pillStyle(color: string | undefined, fallback: string) {
  const value = color?.trim() || fallback
  const match = /^#([0-9a-f]{6})$/i.exec(value)
  const background = match
    ? `${value}14`
    : '#f5f7fa'
  return {
    '--pill-color': value,
    '--pill-background': background,
  }
}

function assigneeInitial(item: MyWorkItem, index: number) {
  return (item.assignee_names?.[index] || item.assignee_usernames?.[index] || '?')
    .trim()
    .charAt(0) || '?'
}

function assigneeTitle(item: MyWorkItem, index: number) {
  return item.assignee_names?.[index] || item.assignee_usernames?.[index] || '未知用户'
}

function assigneeAvatar(item: MyWorkItem, index: number) {
  return item.assignee_avatars?.[index]?.trim() || undefined
}

function assigneeNames(item: MyWorkItem) {
  return (item.assignee_ids || [])
    .map((_, index) => assigneeTitle(item, index))
    .join('; ')
}

function openAssignment(item: MyWorkItem) {
  if (item.local_task_uuid) return
  selectedWorkItem.value = item
  assignModalOpen.value = true
}

function handleTaskCreated(uuid: string) {
  if (selectedWorkItem.value) selectedWorkItem.value.local_task_uuid = uuid
  router.push(`/workflows/task/${uuid}`)
}

async function loadMyWork() {
  loading.value = true
  errorText.value = ''
  try {
    const result = await apiClient.get<{ items: MyWorkItem[] }>('/tasks/my-work')
    items.value = (result.items || []).map((item) => ({
      ...item,
      id: numberValue(item.id),
      workspace_id: numberValue(item.workspace_id),
    }))
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      authStore.clearCloudAuth()
      appStore.setCloudStatus('auth-expired')
      message.error('登录已失效，请重新登录')
      return
    }
    errorText.value = error instanceof Error ? error.message : '我的工作加载失败'
  } finally {
    loading.value = false
  }
}

async function handleLogout() {
  if (logoutLoading.value) return
  logoutLoading.value = true
  try {
    await apiClient.post('/auth/logout')
  } catch {
    // Local state must still be cleared to avoid the UI retaining a stale account when the backend errors.
  } finally {
    userMenuOpen.value = false
    authStore.clearCloudAuth()
    authStore.setLocalSession(false)
    appStore.setCloudStatus('offline')
    logoutLoading.value = false
    message.success('已退出登录')
  }
}

void loadMyWork()
</script>

<template>
  <div class="my-work-page">
    <header class="workbench-header">
      <div class="header-title">
        <span class="header-icon"><CalendarOutlined /></span>
        <h1>我的工作</h1>
      </div>

      <nav class="work-tabs" aria-label="我的工作筛选">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key"
        >
          {{ tab.label }} <span class="tab-count">{{ tab.count }}</span>
        </button>
      </nav>

      <a-popover
        v-model:open="userMenuOpen"
        trigger="click"
        placement="bottomRight"
        overlay-class-name="workbench-user-popover"
      >
        <template #content>
          <div class="account-menu">
            <div class="account-info">
              <strong>{{ displayName }}</strong>
              <span v-if="username && username !== displayName">{{ username }}</span>
              <small>已登录</small>
            </div>
            <button type="button" class="logout-button" :disabled="logoutLoading" @click="handleLogout">
              <LogoutOutlined />
              <span>{{ logoutLoading ? '退出中...' : '退出登录' }}</span>
            </button>
          </div>
        </template>
        <button type="button" class="user-trigger" aria-label="账号信息">
          <a-avatar v-if="avatarSource" :src="avatarSource" :size="36" />
          <a-avatar v-else :size="36" class="fallback-avatar">{{ avatarText }}</a-avatar>
        </button>
      </a-popover>
    </header>

    <main class="workbench-body">
      <div class="filter-row">
        <a-input v-model:value="keyword" allow-clear placeholder="搜索标题" class="title-search">
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <a-select
          v-model:value="selectedPriority"
          allow-clear
          placeholder="优先级"
          :options="priorityOptions"
          class="priority-filter"
        />
        <a-tooltip title="刷新">
          <a-button class="refresh-button" :loading="loading" aria-label="刷新" @click="loadMyWork">
            <template #icon><ReloadOutlined /></template>
          </a-button>
        </a-tooltip>
      </div>

      <a-alert
        v-if="errorText"
        type="error"
        show-icon
        :message="errorText"
        class="load-error"
      >
        <template #action>
          <a-button size="small" @click="loadMyWork">重试</a-button>
        </template>
      </a-alert>

      <div v-if="loading && !items.length" class="loading-state">
        <a-spin size="large" />
      </div>

      <div v-else-if="groups.length" class="group-list">
        <section v-for="group in groups" :key="group.id" class="work-group">
          <button type="button" class="group-header" @click="toggleGroup(group.id)">
            <DownOutlined v-if="!collapsedGroups.has(group.id)" />
            <RightOutlined v-else />
            <span class="workspace-mark" :style="{ color: group.color || '#1677ff' }">
              {{ group.name.trim().charAt(0) || 'G' }}
            </span>
            <strong>{{ group.name }}</strong>
            <small>{{ group.items.length }} 项</small>
          </button>

          <a-table
            v-if="!collapsedGroups.has(group.id)"
            :columns="columns"
            :data-source="group.items"
            :pagination="false"
            :scroll="{ x: 1280 }"
            :row-key="(record: MyWorkItem) => `${record.type}-${record.id}`"
            size="small"
            table-layout="fixed"
            class="work-table"
          >
            <template #bodyCell="{ column, record }: { column: { key: string }, record: MyWorkItem }">
              <template v-if="column.key === 'action'">
                <button
                  v-if="!record.local_task_uuid"
                  type="button"
                  class="assign-button"
                  @click="openAssignment(record)"
                >
                  分配 Agent
                </button>
                <span v-else class="assigned-text">已分配</span>
              </template>

              <template v-else-if="column.key === 'title'">
                <div class="title-cell">
                  <span class="type-tag" :class="`type-${record.type}`">{{ typeLabel(record.type) }}</span>
                  <span :title="record.title" class="work-title">{{ record.title }}</span>
                </div>
              </template>

              <template v-else-if="column.key === 'status'">
                <span
                  v-if="record.status_name"
                  class="field-pill"
                  :style="pillStyle(record.status_color, '#1677ff')"
                >
                  {{ record.status_name }}
                </span>
                <span v-else>-</span>
              </template>

              <template v-else-if="column.key === 'priority'">
                <span
                  v-if="record.priority_name"
                  class="field-pill"
                  :style="pillStyle(record.priority_color, '#8c8c8c')"
                >
                  {{ record.priority_name }}
                </span>
                <span v-else>-</span>
              </template>

              <template v-else-if="column.key === 'planned_start_date'">
                <span class="date-text">{{ formatDate(record.planned_start_date) }}</span>
              </template>

              <template v-else-if="column.key === 'planned_end_date'">
                <span class="date-text">{{ formatDate(record.planned_end_date) }}</span>
              </template>

              <template v-else-if="column.key === 'assignee'">
                <div v-if="record.assignee_ids?.length" class="assignee-list">
                  <a-tooltip :title="assigneeNames(record)">
                    <span class="assignee-item">
                      <a-avatar
                        v-if="assigneeAvatar(record, 0)"
                        :size="20"
                        :src="assigneeAvatar(record, 0)"
                      />
                      <a-avatar v-else :size="20" class="assignee-fallback-avatar">
                        {{ assigneeInitial(record, 0) }}
                      </a-avatar>
                      <span class="assignee-names">{{ assigneeNames(record) }}</span>
                    </span>
                  </a-tooltip>
                </div>
                <span v-else>-</span>
              </template>
            </template>
          </a-table>
        </section>
      </div>

      <a-empty v-else-if="!errorText" description="暂无工作项" class="empty-state" />
    </main>

    <CreateTaskModal
      v-model:open="assignModalOpen"
      :initial-work-item="selectedWorkItem"
      @created="handleTaskCreated"
    />
  </div>
</template>

<style scoped>
.my-work-page {
  display: flex;
  width: calc(100% + 48px);
  height: calc(100% + 48px);
  min-width: 0;
  margin: -24px;
  flex-direction: column;
  overflow: hidden;
  color: #172033;
  background: #fff;
}

.workbench-header {
  display: flex;
  height: 56px;
  flex: 0 0 56px;
  align-items: center;
  padding: 0 28px 0 30px;
  border-bottom: 1px solid #e3e8f0;
  background: #fff;
}

.header-title {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 10px;
  margin-right: 88px;
}

.header-icon {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  color: #fff;
  background: #1677ff;
  font-size: 14px;
}

.header-title h1 {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  letter-spacing: 0;
  white-space: nowrap;
}

.user-trigger {
  display: inline-flex;
  flex: 0 0 auto;
  margin-left: auto;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  box-shadow: 0 3px 9px rgba(15, 23, 42, 0.16);
  cursor: pointer;
}

.fallback-avatar {
  color: #fff;
  background: #1677ff;
  font-weight: 600;
}

.workbench-body {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 26px 30px 32px;
}

.work-tabs {
  display: flex;
  min-width: 0;
  height: 100%;
  flex: 1 1 auto;
  align-items: stretch;
  gap: 34px;
  overflow-x: auto;
  scrollbar-width: none;
}

.work-tabs::-webkit-scrollbar {
  display: none;
}

.work-tabs button {
  position: relative;
  display: inline-flex;
  height: 100%;
  flex: 0 0 auto;
  align-items: center;
  padding: 0;
  border: 0;
  color: #172033;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  font-weight: 400;
  white-space: nowrap;
}

.work-tabs button::after {
  position: absolute;
  right: 0;
  bottom: -1px;
  left: 0;
  height: 2px;
  background: transparent;
  content: '';
}

.work-tabs button.active {
  color: #1677ff;
  font-weight: 500;
}

.work-tabs button.active::after {
  background: #1677ff;
}

.tab-count {
  display: inline-flex;
  min-width: 18px;
  height: 18px;
  align-items: center;
  justify-content: center;
  margin-left: 5px;
  padding: 0 5px;
  border-radius: 9px;
  color: #8793a5;
  background: #f1f3f6;
  font-size: 11px;
  line-height: 18px;
}

.work-tabs button.active .tab-count {
  color: #fff;
  background: #1677ff;
}

.filter-row {
  display: flex;
  align-items: center;
  gap: 14px;
  margin: 18px 0 20px;
  margin-top: 0;
}

.title-search {
  width: min(400px, 45vw);
}

.priority-filter {
  width: 180px;
}

.title-search :deep(.ant-input-affix-wrapper),
.priority-filter :deep(.ant-select-selector),
.refresh-button {
  border-radius: 6px;
}

.load-error {
  margin-bottom: 16px;
}

.loading-state,
.empty-state {
  display: flex;
  min-height: 360px;
  align-items: center;
  justify-content: center;
}

.group-list {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 12px;
}

.work-group {
  overflow: hidden;
  border: 1px solid #e5e8ee;
  border-radius: 8px;
  background: #fff;
}

.group-header {
  display: flex;
  width: 100%;
  height: 36px;
  align-items: center;
  gap: 7px;
  padding: 0 12px;
  border: 0;
  color: #162033;
  background: #fff;
  cursor: pointer;
  text-align: left;
}

.group-header > :first-child {
  color: #27364e;
  font-size: 10px;
}

.workspace-mark {
  display: inline-flex;
  width: 22px;
  height: 22px;
  flex: 0 0 22px;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  background: #eaf2ff;
  font-size: 12px;
  font-weight: 600;
}

.group-header strong {
  overflow: hidden;
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-header small {
  flex: 0 0 auto;
  color: #7f8ba0;
  font-size: 12px;
  font-weight: 400;
}

.work-table :deep(.ant-table) {
  border-top: 1px solid #f0f2f5;
  color: #243047;
}

.work-table :deep(.ant-table-thead > tr > th) {
  height: 38px;
  padding: 8px 10px;
  color: #7b8797;
  background: #fafafa;
  font-size: 13px;
  font-weight: 400;
}

.work-table :deep(.ant-table-tbody > tr > td) {
  height: 42px;
  padding: 7px 10px;
  border-bottom-color: #edf0f3;
  font-size: 13px;
}

.assign-button {
  padding: 0;
  border: 0;
  color: #1677ff;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
}

.assign-button:hover {
  color: #0958d9;
}

.assigned-text {
  color: #a2aab8;
}

.title-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.type-tag {
  flex: 0 0 auto;
  min-width: 24px;
  padding: 1px 3px;
  border-radius: 2px;
  color: #fff;
  font-size: 11px;
  line-height: 16px;
  text-align: center;
}

.type-requirement {
  color: #fff;
  background: #3b8df5;
}

.type-defect {
  color: #fff;
  background: #ff6868;
}

.type-task {
  color: #fff;
  background: #8b5cf6;
}

.work-title {
  overflow: hidden;
  color: #102a52;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.field-pill {
  display: inline-flex;
  min-height: 22px;
  align-items: center;
  padding: 1px 9px;
  border: 1px solid var(--pill-color);
  border-radius: 12px;
  color: var(--pill-color);
  /* background: var(--pill-background); */
  font-size: 12px;
  line-height: 18px;
  white-space: nowrap;
}

.date-text {
  color: #17345e;
  white-space: nowrap;
}

.assignee-list {
  display: flex;
  align-items: center;
  gap: 10px;
  overflow: hidden;
}

.assignee-item {
  display: inline-flex;
  max-width: 100%;
  min-width: 0;
  align-items: center;
  gap: 5px;
  color: #17345e;
  white-space: nowrap;
}

.assignee-item :deep(.ant-avatar) {
  flex: 0 0 20px;
  color: #fff;
  font-size: 11px;
  font-weight: 500;
}

.assignee-names {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assignee-fallback-avatar {
  background: #1677ff;
}

:global(.workbench-user-popover .ant-popover-inner) {
  min-width: 220px;
  padding: 0;
  border-radius: 8px;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.16);
}

.account-info {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 16px 20px 12px;
}

.account-info strong {
  color: #172033;
  font-size: 16px;
}

.account-info span,
.account-info small {
  color: #8995a8;
  font-size: 13px;
}

.logout-button {
  display: flex;
  width: 100%;
  height: 48px;
  align-items: center;
  gap: 10px;
  padding: 0 20px;
  border: 0;
  border-top: 1px solid #edf0f4;
  border-radius: 0 0 8px 8px;
  color: #ff4d4f;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
}

.logout-button:hover {
  background: #fff7f7;
}

.logout-button:disabled {
  cursor: wait;
  opacity: 0.65;
}

@media (max-width: 760px) {
  .workbench-header {
    height: 104px;
    flex: 0 0 104px;
    flex-wrap: wrap;
    align-content: flex-start;
    padding: 0 12px;
  }

  .header-title {
    height: 56px;
    gap: 8px;
    margin-right: 0;
  }

  .user-trigger :deep(.ant-avatar) {
    width: 32px !important;
    height: 32px !important;
    line-height: 32px !important;
  }

  .workbench-body {
    padding: 18px 16px 24px;
  }

  .work-tabs {
    order: 3;
    width: 100%;
    height: 48px;
    flex: 0 0 100%;
    gap: 24px;
  }

  .filter-row {
    flex-wrap: wrap;
  }

  .title-search {
    width: 100%;
  }

  .priority-filter {
    min-width: 0;
    flex: 1;
  }

  .group-header {
    gap: 5px;
    padding: 0 8px;
  }

  .group-header strong {
    overflow: hidden;
    font-size: 13px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
</style>
