<template>
  <div class="message-list-card">
    <a-spin :spinning="loading">
      <div class="message-list">
        <template
          v-for="item in items"
          :key="item.uuid"
        >
          <article
            :id="`progress-${item.uuid}`"
            class="message-card"
            :class="{
              failed: item.status === 'failed',
              flash: item.uuid === highlightUuid,
              collapsed: isCollapsed(item.uuid),
            }"
          >
            <div class="message-card-head">
              <button
                type="button"
                class="message-card-toggle"
                :title="isCollapsed(item.uuid) ? '展开消息' : '收起消息'"
                :aria-label="isCollapsed(item.uuid) ? '展开消息' : '收起消息'"
                :aria-expanded="!isCollapsed(item.uuid)"
                @click="toggleCollapsed(item.uuid)"
              >
                <RightOutlined v-if="isCollapsed(item.uuid)" />
                <DownOutlined v-else />
              </button>
              <div
                class="message-avatar"
                :class="
                  isUserMessage(item)
                    ? 'user-avatar'
                    : { 'has-image': stepForUuid(item.task_step_uuid)?.avatar }
                "
              >
                <img
                  v-if="!isUserMessage(item) && stepForUuid(item.task_step_uuid)?.avatar"
                  :src="stepForUuid(item.task_step_uuid)?.avatar"
                  alt=""
                />
                <span v-else>{{
                  isUserMessage(item) ? '我' : initials(stepName(item.task_step_uuid))
                }}</span>
              </div>
              <div class="message-card-meta">
                <p class="message-card-name">
                  {{ isUserMessage(item) ? '我' : stepName(item.task_step_uuid) }}
                </p>
                <p class="message-card-time">
                  {{
                    formatTime(
                      isUserMessage(item)
                        ? item.created_at
                        : item.finished_at || item.started_at || item.created_at,
                    )
                  }}
                </p>
              </div>
              <button
                type="button"
                class="message-card-copy"
                title="复制消息"
                @click="emit('copy', item)"
              >
                <CopyOutlined />
              </button>
            </div>
            <div class="message-card-detail">
              <div
                v-if="isUserMessage(item)"
                class="message-card-body"
              >
                {{ item.user_prompt || item.question }}
              </div>
              <div
                v-else
                class="message-card-body"
              >
                <div
                  v-if="item.status === 'created' || item.status === 'running'"
                  class="running-line"
                >
                  <LoadingOutlined spin /> {{ resultText(item) }}
                </div>
                <MarkdownPreview
                  v-else
                  :content="resultText(item)"
                />
              </div>
              <div
                v-if="
                  !isUserMessage(item) &&
                  (item.status === 'created' || item.status === 'running') &&
                  latestEventOneLine(item)
                "
                class="running-latest"
                :title="latestEventOneLine(item)"
              >
                <span class="running-latest-label">{{
                  latestEventLabel(item.latest_event_type)
                }}</span>
                <span class="running-latest-content">{{
                  (item.latest_event_content || '').replace(/\s+/g, ' ').trim()
                }}</span>
                <time>{{ formatTime(item.latest_event_at) }}</time>
              </div>
              <span
                v-if="!isUserMessage(item) && item.status !== 'success'"
                class="message-status"
                :class="`status-${item.status}`"
                >{{ displayStatus(item.status) }}</span
              >
            </div>
          </article>
        </template>

        <div
          v-if="!items.length"
          class="empty-conversation"
        >
          <div class="empty-agent">{{ initials(selectedStepName) }}</div>
          <p>暂无对话记录</p>
        </div>
      </div>
    </a-spin>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { CopyOutlined, DownOutlined, LoadingOutlined, RightOutlined } from '@ant-design/icons-vue'
import MarkdownPreview from '@/components/MarkdownPreview.vue'
import type { PipelineStep, TaskProgress } from '@/types/pipeline'
import { initials, isUserMessage, resultText } from './utils'

const props = withDefaults(
  defineProps<{
    items: TaskProgress[]
    steps: PipelineStep[]
    highlightUuid?: string
    loading?: boolean
    selectedStepName?: string
  }>(),
  { highlightUuid: '', loading: false, selectedStepName: '' },
)

const emit = defineEmits<{
  copy: [item: TaskProgress]
}>()

const collapsedUuids = ref(new Set<string>())
const stepMap = computed(() => new Map(props.steps.map((step) => [step.uuid, step])))

function isCollapsed(uuid: string) {
  return collapsedUuids.value.has(uuid)
}

function toggleCollapsed(uuid: string) {
  const nextCollapsedUuids = new Set(collapsedUuids.value)
  if (nextCollapsedUuids.has(uuid)) nextCollapsedUuids.delete(uuid)
  else nextCollapsedUuids.add(uuid)
  collapsedUuids.value = nextCollapsedUuids
}

function stepForUuid(uuid?: string) {
  return stepMap.value.get(uuid || '')
}
function stepName(uuid?: string) {
  return stepForUuid(uuid)?.name || 'Agent'
}

function displayStatus(status?: string) {
  return (
    {
      created: '等待执行',
      running: '执行中',
      idle: '等待开始',
      failed: '执行失败',
      stopped: '已停止',
      interrupted: '已中断',
    }[status || ''] ||
    status ||
    '等待开始'
  )
}

// 设计稿消息时间为 HH:mm；非当天消息补月-日
function formatTime(timestamp?: number) {
  if (!timestamp) return ''
  const milliseconds = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const date = new Date(milliseconds)
  const sameDay = date.toDateString() === new Date().toDateString()
  return date.toLocaleString(
    'zh-CN',
    sameDay
      ? { hour: '2-digit', minute: '2-digit', hour12: false }
      : { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false },
  )
}

const latestEventLabels: Record<string, string> = {
  message: '输出',
  tool_call: '工具调用',
  tool_result: '工具结果',
}

function latestEventLabel(type?: string) {
  return latestEventLabels[type || ''] || '活动'
}

// 最新一条 CLI 活动压缩为单行文本（去掉换行/连续空白），用于“正在执行”下方小字
function latestEventOneLine(item: TaskProgress) {
  const content = (item.latest_event_content || '').replace(/\s+/g, ' ').trim()
  return content ? `[${latestEventLabel(item.latest_event_type)}] ${content}` : ''
}

function scrollToItem(uuid: string) {
  document
    .getElementById(`progress-${uuid}`)
    ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
}
defineExpose({ scrollToItem })
</script>

<style scoped>
.message-list-card {
  overflow: hidden;
  border: 1px solid #d9d9d9;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 2px 24px rgba(0, 0, 0, 0.08);
}
.message-list {
  padding: 0 16px;
}
.message-card {
  padding: 16px 0;
}
.message-card + .message-card {
  border-top: 1px solid #f0f0f0;
}
.message-card.failed .message-card-body {
  color: #dc2626;
}
.message-card.flash {
  animation: record-flash 1.2s ease;
}
@keyframes record-flash {
  0% {
    background: #eef1ff;
  }
  60% {
    background: #f5f7ff;
  }
  100% {
    background: transparent;
  }
}
.message-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.message-card-toggle,
.message-card-copy {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  color: #595959;
  background: transparent;
  cursor: pointer;
}
.message-card-toggle:hover,
.message-card-copy:hover {
  color: #262626;
  background: #f3f4f6;
}
.message-avatar {
  display: flex;
  overflow: hidden;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  color: #fff;
  background: #9ca3af;
  font-size: 9px;
  font-weight: 600;
}
.message-avatar.user-avatar {
  color: #64748b;
  background: #e2e8f0;
}
.message-avatar.has-image {
  border: 1px solid #fff;
  box-sizing: border-box;
}
.message-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.message-card-meta {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: baseline;
  gap: 9px;
}
.message-card-name {
  margin: 0;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}
.message-card-time {
  margin: 0;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 16px;
}
.message-card-copy {
  margin-left: auto;
  color: #c4c9d1;
}
.message-card-copy:hover {
  color: #6b7280;
}
.message-card-detail {
  overflow: hidden;
}
.message-card.collapsed .message-card-detail {
  display: none;
}
.message-card-body {
  margin-top: 16px;
  padding-left: 32px;
  color: #262626;
  font-size: 14px;
  line-height: 32px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
.message-card-body :deep(.markdown-body) {
  font-size: 14px;
  line-height: 32px;
}
.message-card-body :deep(.markdown-body > :first-child) {
  margin-top: 0;
}
.message-card-body :deep(.markdown-body > :last-child) {
  margin-bottom: 0;
}
.message-card-body :deep(.markdown-preview-shell) {
  min-height: 0;
  padding: 0;
  background: transparent;
}
.running-line {
  display: flex;
  align-items: center;
  gap: 7px;
  color: #6b7280;
}
.running-latest {
  display: flex;
  max-width: 520px;
  align-items: center;
  gap: 6px;
  min-width: 0;
  margin-top: 8px;
  color: #9ca3af;
  font-size: 11px;
  line-height: 1.4;
}
.running-latest-label {
  flex: 0 0 auto;
  border-radius: 3px;
  padding: 0 5px;
  color: #3157e2;
  background: rgba(49, 87, 226, 0.08);
  font-size: 10px;
}
.running-latest-content {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.running-latest time {
  flex: 0 0 auto;
  color: #c4c9d1;
  font-size: 10px;
}
.message-status {
  display: inline-block;
  margin-top: 6px;
  color: #9ca3af;
  font-size: 11px;
}
.status-failed {
  color: #dc2626;
}
.empty-conversation {
  display: flex;
  height: 210px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  color: #8c8c8c;
}
.empty-agent {
  display: flex;
  width: 42px;
  height: 42px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #cbd5e1;
}
.empty-conversation p {
  margin-top: 10px;
  font-size: 14px;
}
</style>
