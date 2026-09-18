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
              'user-message': isUserMessage(item),
              flash: item.uuid === highlightUuid,
              collapsed: isCollapsed(item.uuid),
            }"
          >
            <div class="message-card-head">

              <div
                class="message-avatar"
                :class="
                  isUserMessage(item)
                    ? 'user-avatar'
                    : {
                        'has-image': stepForUuid(item.task_step_uuid)?.avatar,
                        'has-logo': !stepForUuid(item.task_step_uuid)?.avatar && fallbackActorLogo,
                      }
                "
              >
                <img
                  v-if="!isUserMessage(item) && stepForUuid(item.task_step_uuid)?.avatar"
                  :src="stepForUuid(item.task_step_uuid)?.avatar"
                  alt=""
                />
                <img
                  v-else-if="!isUserMessage(item) && fallbackActorLogo"
                  :src="fallbackActorLogo"
                  alt=""
                />
                <span v-else>{{
                  isUserMessage(item) ? t('workflows.task.progress.me') : initials(stepName(item.task_step_uuid))
                }}</span>
              </div>
              <div class="message-card-meta">
                <p class="message-card-name">
                  {{ isUserMessage(item) ? t('workflows.task.progress.me') : stepName(item.task_step_uuid) }}
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
                :title="t('workflows.task.progress.copyMessage')"
                :aria-label="t('workflows.task.progress.copyMessage')"
                @click="emit('copy', item)"
              >
                <CopyOutlined />
              </button>
              <button
                type="button"
                class="message-card-toggle"
                :title="isCollapsed(item.uuid) ? t('workflows.task.progress.expandMessage') : t('workflows.task.progress.collapseMessage')"
                :aria-label="isCollapsed(item.uuid) ? t('workflows.task.progress.expandMessage') : t('workflows.task.progress.collapseMessage')"
                :aria-expanded="!isCollapsed(item.uuid)"
                @click="toggleCollapsed(item.uuid)"
              >
                <RightOutlined v-if="isCollapsed(item.uuid)" />
                <DownOutlined v-else />
              </button>
            </div>
            <div class="message-card-detail">
              <!-- 执行过程排在正文之前：先看过程（含实时进行中），再看结论 -->
              <ExecutionProcess
                v-if="!isUserMessage(item) && item.session_uuid"
                :task-uuid="taskUuid"
                :session-uuid="item.session_uuid"
                :status="item.status"
                :cli-type="item.cli_type"
                :model-name="item.model"
                :input-tokens="item.input_tokens"
                :output-tokens="item.output_tokens"
                :total-tokens="item.total_tokens"
              />
              <div
                v-if="isUserMessage(item)"
                class="message-card-body"
              >
                <MarkdownPreview
                  :content="userMessageText(item)"
                  :task-uuid="taskUuid"
                />
              </div>
              <!-- 运行中正文区留空：进展统一由执行过程承载，正文只在拿到最终结论后才出现 -->
              <div
                v-else-if="!isRunningItem(item)"
                class="message-card-body"
              >
                <MarkdownPreview
                  :content="resultText(item)"
                  :task-uuid="taskUuid"
                />
              </div>
              <!-- 需求 2202 评论 3：消息下方的「最新活动」小字与执行过程说的是同一件事，
                   已整体移除；实时进展统一由执行过程承载 -->
              <span
                v-if="showMessageStatus(item)"
                class="message-status"
                :class="`status-${item.status}`"
                >{{ displayStatus(item.status) }}</span
              >
              <p
                v-if="item.dispatch_message"
                class="dispatch-message"
                :class="`dispatch-${item.dispatch_status || 'idle'}`"
              >
                {{ dispatchMessage(item.dispatch_message) }}
              </p>
            </div>
          </article>
        </template>

        <div
          v-if="!items.length"
          class="empty-conversation"
        >
          <div class="empty-agent">{{ initials(selectedStepName || fallbackActorName) }}</div>
          <p>{{ t('workflows.task.progress.noConversation') }}</p>
        </div>
      </div>
    </a-spin>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { CopyOutlined, DownOutlined, RightOutlined } from '@ant-design/icons-vue'
import MarkdownPreview from '@/components/MarkdownPreview.vue'
import ExecutionProcess from './ExecutionProcess.vue'
import type { PipelineStep, TaskProgress } from '@/types/pipeline'
import { initials, isUserMessage, resultText, userMessageText } from './utils'
import { useAppI18n, type MessageKey } from '@/i18n'
import { useLocale } from '@/composables/useLocale'

const { t } = useAppI18n()
const { locale } = useLocale()

const props = withDefaults(
  defineProps<{
    items: TaskProgress[]
    steps: PipelineStep[]
    highlightUuid?: string
    loading?: boolean
    selectedStepName?: string
    fallbackActorName?: string
    fallbackActorLogo?: string
    taskUuid?: string
  }>(),
  {
    highlightUuid: '',
    loading: false,
    selectedStepName: '',
    fallbackActorName: '',
    fallbackActorLogo: '',
    taskUuid: '',
  },
)

const emit = defineEmits<{
  copy: [item: TaskProgress]
}>()

const collapsedUuids = ref(new Set<string>())
const stepMap = computed(() => new Map(props.steps.map((step) => [step.uuid, step])))
const dispatchMessageKeys: Record<string, MessageKey> = {
  'expert_routing.routing_pending': 'expertGroups.dispatch.routingPending',
  'expert_routing.context_load_failed': 'expertGroups.dispatch.contextLoadFailed',
  'expert_routing.leader_failed': 'expertGroups.dispatch.leaderFailed',
  'expert_routing.no_delegation': 'expertGroups.dispatch.noDelegation',
  'expert_routing.multiple_delegations': 'expertGroups.dispatch.multipleDelegations',
  'expert_routing.invalid_member': 'expertGroups.dispatch.invalidMember',
  'expert_routing.member_activation_failed': 'expertGroups.dispatch.memberActivationFailed',
  'expert_routing.member_start_failed': 'expertGroups.dispatch.memberStartFailed',
  'expert_routing.dispatched': 'expertGroups.dispatch.dispatched',
  'expert_routing.leader_missing': 'expertGroups.dispatch.leaderMissing',
  'expert_routing.leader_activation_failed': 'expertGroups.dispatch.leaderActivationFailed',
  'expert_routing.leader_start_failed': 'expertGroups.dispatch.leaderStartFailed',
  'expert_routing.returned': 'expertGroups.dispatch.returned',
  'expert_routing.round_limit': 'expertGroups.dispatch.roundLimit',
}

/** 执行过程承载了这条会话的运行状态（CLI 会话才有执行过程面板） */
function processShowsRunning(item: TaskProgress) {
  return Boolean(item.cli_type && item.session_uuid)
}

/** 卡片底部状态文字：运行中被执行过程覆盖，其余状态（失败、中断等）照常展示 */
function showMessageStatus(item: TaskProgress) {
  if (isUserMessage(item) || item.status === 'success') return false
  return !(isRunningItem(item) && processShowsRunning(item))
}

function dispatchMessage(value: string) {
  const key = dispatchMessageKeys[value]
  return key ? t(key) : value
}

// 运行中的消息正文区不渲染内容：结论未产出，且进展已由执行过程承载
function isRunningItem(item: TaskProgress) {
  return item.status === 'created' || item.status === 'running'
}

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
  return stepForUuid(uuid)?.name || props.fallbackActorName || t('workflows.task.common.agentOrchestration')
}

function displayStatus(status?: string) {
  return (
    {
      created: t('workflows.task.execution.created'),
      running: t('workflows.task.execution.running'),
      idle: t('workflows.task.status.pending'),
      failed: t('workflows.task.execution.failed'),
      stopped: t('workflows.task.execution.stopped'),
      interrupted: t('workflows.task.execution.interrupted'),
    }[status || ''] ||
    status ||
    t('workflows.task.status.pending')
  )
}

// 设计稿消息时间为 HH:mm；非当天消息补月-日
function formatTime(timestamp?: number) {
  if (!timestamp) return ''
  const milliseconds = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const date = new Date(milliseconds)
  const sameDay = date.toDateString() === new Date().toDateString()
  return date.toLocaleString(
    locale.value,
    sameDay
      ? { hour: '2-digit', minute: '2-digit', hour12: false }
      : { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false },
  )
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
  transition: background-color 0.15s;
}
.message-card.user-message { margin-inline: -16px; padding-inline: 16px; background: #f5f9ff; }
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
.message-avatar.has-logo {
  width: 16px;
  height: 16px;
  background: transparent;
}
.message-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.message-avatar.has-logo img {
  object-fit: contain;
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
.dispatch-message { margin:10px 0 0; padding:8px 10px; border-radius:8px; color:#595959; background:#f5f6f8; font-size:12px; }
.dispatch-failed,.dispatch-blocked { color:#b42318; background:#fef2f2; }
.dispatch-dispatched,.dispatch-returned { color:#3157e2; background:#e5efff; }
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
