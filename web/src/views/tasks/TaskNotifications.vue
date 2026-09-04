<template>
  <div class="task-page">
    <header class="page-titlebar">
      <div class="page-titlebar-copy">
        <strong>对话</strong>
        <p>与 Agent 对话推进任务，Agent 的确认请求会在这里与你沟通</p>
      </div>
      <a-tooltip
        :title="pipelineExpanded ? '隐藏任务概览' : '显示任务概览'"
        placement="bottom"
      >
        <button
          type="button"
          class="overview-toggle-button"
          :aria-label="pipelineExpanded ? '隐藏任务概览' : '显示任务概览'"
          :aria-expanded="pipelineExpanded"
          aria-controls="task-overview"
          @click="togglePipelineExpanded"
        >
          <img
            :src="taskOverviewToggleIcon"
            alt=""
            aria-hidden="true"
          />
        </button>
      </a-tooltip>
    </header>

    <section
      v-if="selectedConversation || newConversationPipeline"
      v-show="pipelineExpanded"
      id="task-overview"
      class="task-overview"
      :aria-busy="taskLoading"
    >
      <template v-if="newConversationPipeline">
        <div class="task-overview-head">
          <div class="task-overview-title">
            <h2 title="新任务">新任务</h2>
            <span class="task-status">待创建</span>
          </div>
        </div>
        <div class="task-step-region">
          <AgentStepStrip
            v-if="newConversationSteps.length"
            :steps="newConversationSteps"
            :current-step-index="-1"
            current-step-uuid=""
            selected-step-uuid=""
            read-only
          />
          <div
            v-else
            class="step-region-empty"
          >
            当前流水线暂无执行步骤
          </div>
        </div>
      </template>
      <template v-else-if="task">
        <div class="task-overview-head">
          <div class="task-overview-title">
            <h2 :title="task.title">{{ task.title }}</h2>
            <span
              class="task-status"
              :class="`status-${taskStatus}`"
              >{{ taskStatusText }}</span
            >
            <span
              v-if="taskPriorityText"
              class="task-priority"
              >{{ taskPriorityText }}</span
            >
          </div>
        </div>
        <div class="task-step-region">
          <AgentStepStrip
            v-if="hasPipelineSnapshot"
            :steps="sortedSteps"
            :current-step-index="currentStepIndex"
            :current-step-uuid="effectiveCurrentStepUuid"
            :selected-step-uuid="selectedStepUuid"
            @select="selectStep"
          />
          <div
            v-else
            class="step-region-empty"
          >
            当前任务未指派流水线
          </div>
        </div>
      </template>
      <div
        v-else
        class="task-overview-skeleton"
        aria-hidden="true"
      >
        <div class="overview-skeleton-head">
          <span class="overview-skeleton-title" />
        </div>
        <div class="overview-skeleton-steps">
          <span
            v-for="index in 3"
            :key="index"
          />
        </div>
      </div>
    </section>

    <div
      ref="taskLayoutRef"
      class="task-layout"
      :class="{ 'is-resizing-sidebar': sidebarDragging }"
    >
      <aside
        ref="conversationSidebarRef"
        class="conversation-sidebar"
        :style="
          sidebarResized
            ? { width: `${sidebarWidth}px`, flexBasis: `${sidebarWidth}px` }
            : undefined
        "
      >
        <div class="sidebar-heading">
          <span>对话</span>
          <small>{{ displayedConversationCount }}</small>
          <a-dropdown
            v-model:open="pipelineMenuOpen"
            :trigger="['click']"
            placement="bottomRight"
            :get-popup-container="getPipelinePopupContainer"
            @open-change="handlePipelineMenuOpenChange"
          >
            <button
              type="button"
              class="sidebar-add-button"
              aria-label="查看执行流水线"
              :aria-expanded="pipelineMenuOpen"
            >
              <PlusOutlined />
            </button>
            <template #overlay>
              <div
                class="pipeline-menu"
                role="menu"
                aria-label="执行流水线"
              >
                <div class="pipeline-menu-heading">
                  <PipelineFlowIcon />
                  <span>执行流水线</span>
                </div>
                <div
                  v-if="pipelinesLoading"
                  class="pipeline-menu-state"
                >
                  <a-spin size="small" />
                </div>
                <div
                  v-else-if="pipelinesError"
                  class="pipeline-menu-state pipeline-menu-error"
                >
                  <span>{{ pipelinesError }}</span>
                  <button
                    type="button"
                    @click.stop="loadPipelines(true)"
                  >
                    <ReloadOutlined />重试
                  </button>
                </div>
                <div
                  v-else-if="!pipelines.length"
                  class="pipeline-menu-state"
                >
                  暂无流水线
                </div>
                <div
                  v-else
                  class="pipeline-menu-list"
                >
                  <button
                    v-for="pipeline in pipelines"
                    :key="pipeline.uuid"
                    type="button"
                    role="menuitem"
                    class="pipeline-menu-item"
                    :title="pipeline.name"
                    @click="startNewConversation(pipeline)"
                  >
                    <img
                      v-if="pipeline.avatar"
                      :src="pipeline.avatar"
                      alt=""
                    />
                    <span
                      v-else
                      class="pipeline-avatar-fallback"
                      >{{ pipelineInitials(pipeline.name) }}</span
                    >
                    <span class="pipeline-menu-name">{{ pipeline.name }}</span>
                  </button>
                </div>
              </div>
            </template>
          </a-dropdown>
        </div>

        <div class="conversation-list scrollbar--subtle">
          <div
            v-if="newConversationPipeline"
            class="conversation-item active new-conversation-item"
            aria-current="true"
          >
            <span class="conversation-title">
              <strong title="新任务">新任务</strong>
            </span>
            <span
              v-if="newConversationSteps[0]"
              class="conversation-subtitle"
              :title="`${newConversationSteps[0].name} · 待执行`"
            >
              {{ newConversationSteps[0].name }} · 待执行
            </span>
          </div>
          <div
            v-if="notificationsLoading && !conversations.length"
            class="sidebar-loading"
          >
            <a-spin size="small" />
          </div>
          <div
            v-else-if="notificationsError && !conversations.length"
            class="sidebar-error"
          >
            <span>{{ notificationsError }}</span>
            <button
              type="button"
              @click="loadNotifications(false)"
            >
              <ReloadOutlined />重试
            </button>
          </div>
          <div
            v-for="conversation in conversations"
            :key="conversation.task_uuid"
            class="conversation-item"
            :class="{ active: selectedTaskUuid === conversation.task_uuid }"
            @contextmenu="openContextMenu($event, conversation)"
          >
            <button
              type="button"
              class="conversation-select-button"
              @click="selectConversation(conversation)"
            >
              <span class="conversation-title">
                <strong :title="conversation.title">{{ conversation.title }}</strong>
                <i
                  v-if="conversation.unread"
                  title="未读"
                />
              </span>
              <span class="conversation-subtitle">
                {{ conversation.latest.step_name || 'Agent 编排' }} ·
                {{
                  terminalLabel(conversation.latest.status || conversation.latest.terminal_status)
                }}
              </span>
            </button>
            <button
              type="button"
              class="conversation-menu-button"
              aria-label="更多操作"
              aria-haspopup="menu"
              aria-controls="conversation-context-menu"
              :aria-expanded="contextTaskUuid === conversation.task_uuid"
              @click.stop="toggleConversationMenu($event, conversation)"
            >
              <img
                :src="conversationMenuIcon"
                alt=""
                aria-hidden="true"
              />
            </button>
          </div>
          <div
            v-if="
              !newConversationPipeline &&
              !conversations.length &&
              !notificationsLoading &&
              !notificationsError
            "
            class="sidebar-empty"
          >
            <FolderOutlined />
            <span>暂无任务会话</span>
          </div>
        </div>
        <div
          class="sidebar-resize-handle"
          role="separator"
          tabindex="0"
          aria-label="调整对话列表宽度"
          aria-orientation="vertical"
          :aria-valuemin="MIN_SIDEBAR_WIDTH"
          :aria-valuemax="sidebarMaxWidth"
          :aria-valuenow="Math.round(sidebarWidth)"
          :aria-valuetext="`${Math.round(sidebarWidth)} 像素`"
          @pointerdown="startSidebarResize"
          @pointermove="handleSidebarResize"
          @pointerup="finishSidebarResize"
          @pointercancel="finishSidebarResize"
          @lostpointercapture="finishSidebarResize"
          @keydown="handleSidebarResizeKeydown"
        />
      </aside>

      <main
        class="conversation-main"
        :aria-busy="initialTaskLoading || taskRefreshing"
      >
        <template v-if="newConversationPipeline">
          <div class="main-state new-conversation-state">
            <PipelineFlowIcon />
            <h3>与「{{ newConversationPipeline.name }}」开始新对话</h3>
            <p>发送首条消息后将自动创建任务并开始执行</p>
          </div>
          <ChatComposer
            ref="newConversationComposerRef"
            v-model="newConversationQuestion"
            :can-ask="true"
            :submitting="submitting"
            placeholder="输入任务需求，发送后自动创建任务…"
            :context-text="`使用「${newConversationPipeline.name}」创建任务`"
            task-uuid=""
            show-work-directory
            :work-directory="newConversationWorkDir"
            @submit="submitNewConversation"
            @select-work-directory="chooseNewConversationDirectory"
          />
        </template>
        <template v-else-if="selectedConversation">
          <div
            v-if="taskError && !task"
            class="main-state main-error"
          >
            <span>{{ taskError }}</span>
            <button
              type="button"
              @click="loadSelectedTask()"
            >
              <ReloadOutlined />重试
            </button>
          </div>
          <div
            v-else-if="initialTaskLoading || !task"
            class="conversation-loading-shell"
            aria-hidden="true"
          >
            <div class="message-scroll message-loading-skeleton scrollbar--subtle">
              <a-skeleton
                active
                :title="false"
                :paragraph="{ rows: 5 }"
              />
            </div>
            <div class="composer-loading-skeleton">
              <div class="composer-loading-body" />
            </div>
          </div>
          <template v-else-if="task && hasPipelineSnapshot">
            <NextStepButton
              v-if="showNextStepCard"
              :next-step-name="nextStepName"
              :is-last-step="isLastStep"
              :disabled="!canComplete"
              :completing="completing"
              @confirm="completeStep"
            />
            <div
              class="message-scroll scrollbar--subtle"
              :aria-busy="taskRefreshing"
            >
              <div
                v-if="taskRefreshing"
                class="message-refresh-indicator"
                role="status"
                aria-label="正在刷新任务"
              >
                <a-spin
                  :spinning="taskRefreshing"
                  :delay="150"
                  size="small"
                />
              </div>
              <StepMessageList
                :items="selectedProgress"
                :steps="sortedSteps"
                :highlight-uuid="highlightUuid"
                :selected-step-name="selectedStep?.name || ''"
                :task-uuid="selectedTaskUuid"
                @copy="copyResult"
              />
            </div>
            <ChatComposer
              ref="composerRef"
              v-model="question"
              :can-ask="canAsk"
              :submitting="submitting"
              :running="Boolean(activeSelectedProgress)"
              :stopping="stoppingSessionUuid === activeSelectedProgress?.session_uuid"
              :cli-type="composerCliType"
              :model-name="composerModelName"
              :placeholder="composerPlaceholder"
              :context-text="composerContextText"
              :task-uuid="selectedTaskUuid"
              :current-step="selectedStep"
              :steps="sortedSteps"
              :executing-step-uuid="effectiveCurrentStepUuid"
              @submit="submitQuestion"
              @stop="stopSelectedConversation"
              @prompt-saved="loadSelectedTask"
            />
          </template>
          <div
            v-else-if="task"
            class="main-state"
            :aria-busy="taskRefreshing"
          >
            <div
              v-if="taskRefreshing"
              class="main-refresh-indicator"
              role="status"
              aria-label="正在刷新任务"
            >
              <a-spin
                :spinning="taskRefreshing"
                :delay="150"
                size="small"
              />
            </div>
            <FolderOutlined />
            <h3>未指派流水线</h3>
            <p>请先为任务分配流水线，动态记录将自动展示</p>
          </div>
        </template>
        <div
          v-else
          class="main-state"
        >
          <CheckCircleOutlined />
          <h3>暂无待处理任务</h3>
          <p>Agent 执行和确认请求会在这里与你沟通</p>
        </div>
      </main>
    </div>

    <div
      v-if="contextTaskUuid"
      id="conversation-context-menu"
      class="context-menu"
      role="menu"
      :style="{ left: `${contextX}px`, top: `${contextY}px` }"
      @click.stop
    >
      <button
        type="button"
        role="menuitem"
        @click="openConversationDetail"
      >
        <img
          :src="contextDetailIcon"
          alt=""
          aria-hidden="true"
        />详情
      </button>
      <button
        type="button"
        role="menuitem"
        @click="toggleRead"
      >
        <img
          :src="contextReadIcon"
          alt=""
          aria-hidden="true"
        />{{ contextConversation?.unread ? '标为已读' : '设为未读' }}
      </button>
      <button
        type="button"
        role="menuitem"
        @click="archiveConversation"
      >
        <img
          :src="contextArchiveIcon"
          alt=""
          aria-hidden="true"
        />归档
      </button>
    </div>

    <TaskDetailInfoModal
      v-model:open="detailModalOpen"
      :task="task"
      :status="taskStatus"
      :rendered-description="renderedDescription"
      @preview-image="showImagePreview"
      @saved="handleTaskSaved"
    />
    <StopExecutionConfirmModal
      :open="stopConfirmOpen"
      :loading="Boolean(stoppingSessionUuid)"
      @close="closeStopConfirm"
      @confirm="confirmStopConversation"
    />
    <TaskImagePreviewModal
      v-model:open="previewImageVisible"
      :image-url="previewImageUrl"
    />
    <AssignPipelineModal
      v-model:open="pipelineConfigModalOpen"
      :preferred-pipeline-uuid="newConversationPipeline?.uuid || ''"
      mode="create"
      @selected="handleCreationPipelineSelected"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRaw, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { message } from 'ant-design-vue'
import {
  CheckCircleOutlined,
  FolderOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons-vue'
import MarkdownIt from 'markdown-it'
import apiClient, { ApiError } from '@/api/client'
import conversationMenuIcon from '@/assets/icons/common-more-actions.svg'
import contextArchiveIcon from '@/assets/icons/task-context-archive.svg'
import contextDetailIcon from '@/assets/icons/task-context-detail.svg'
import contextReadIcon from '@/assets/icons/task-context-read.svg'
import taskOverviewToggleIcon from '@/assets/icons/task-overview-toggle.svg'
import AssignPipelineModal from '@/components/AssignPipelineModal.vue'
import AgentStepStrip from '@/components/task-progress/AgentStepStrip.vue'
import ChatComposer from '@/components/task-progress/ChatComposer.vue'
import NextStepButton from '@/components/task-progress/NextStepButton.vue'
import PipelineFlowIcon from '@/components/task-progress/PipelineFlowIcon.vue'
import StepMessageList from '@/components/task-progress/StepMessageList.vue'
import StopExecutionConfirmModal from '@/components/task-progress/StopExecutionConfirmModal.vue'
import { copyText } from '@/utils/clipboard'
import { isStopConfirmSuppressed, suppressStopConfirm } from '@/utils/stopConfirm'
import { selectDirectory } from '@/composables/useDesktop'
import { useLocalWS, useLocalWSStatus } from '@/composables/useLocalWebSocket'
import { usePipelineStore } from '@/stores/pipeline'
import type {
  CompleteStepResponse,
  Pipeline,
  TaskNotification,
  TaskProgress,
} from '@/types/pipeline'
import type { ChatComposerSubmission } from '@/types/task-attachments'
import { isUserMessage, resultText, userMessageText } from '@/components/task-progress/utils'
import type { TaskWithDetails } from '@/types/task-detail'
import {
  deleteTaskView,
  deleteTaskViewsOutside,
  readTaskConversationCache,
  saveTaskConversationViewState,
  saveTaskNotificationSnapshot,
  saveTaskView,
} from '@/utils/taskConversationCache'
import TaskDetailInfoModal from '@/views/workflows/components/TaskDetailInfoModal.vue'
import TaskImagePreviewModal from '@/views/workflows/components/TaskImagePreviewModal.vue'

interface TaskConversation {
  task_uuid: string
  title: string
  items: TaskNotification[]
  latest: TaskNotification
  unread: boolean
}

interface CreateTaskNotification {
  task_uuid: string
  task_title: string
  step_name: string
  session_uuid: string
  status: 'created'
  summary: string
  created_at: number
}

const pipelineStore = usePipelineStore()
const { pipelines, loading: pipelinesLoading, error: pipelinesError } = storeToRefs(pipelineStore)

type CreatedTaskResponse = TaskWithDetails & {
  notification?: CreateTaskNotification
}

interface TaskViewCache {
  task: TaskWithDetails
  progress: TaskProgress[]
}

const notificationsLoading = ref(false)
const notificationsError = ref('')
const notifications = ref<TaskNotification[]>([])
const temporaryNotifications = ref<TaskNotification[]>([])
const selectedTaskUuid = ref('')
const selectedStepUuid = ref('')
const selectedProgressUuid = ref('')

const taskLoading = ref(false)
const taskError = ref('')
const task = ref<TaskWithDetails>()
const progress = ref<TaskProgress[]>([])
const taskViewCache = new Map<string, TaskViewCache>()
const submitting = ref(false)
const completing = ref(false)
const stoppingSessionUuid = ref('')
const stopConfirmOpen = ref(false)
const stopConfirmSessionUuid = ref('')
const question = ref('')
const composerRef = ref<InstanceType<typeof ChatComposer>>()
const highlightUuid = ref('')
const pipelineExpanded = ref(true)
const taskLayoutRef = ref<HTMLElement>()
const conversationSidebarRef = ref<HTMLElement>()
const sidebarWidth = ref(300)
const sidebarMaxWidth = ref(300)
const sidebarResized = ref(false)
const sidebarDragging = ref(false)

const pipelineMenuOpen = ref(false)
const newConversationPipeline = ref<Pipeline>()
const newConversationQuestion = ref('')
const newConversationWorkDir = ref('')
const newConversationComposerRef = ref<InstanceType<typeof ChatComposer>>()
const newConversationSubmission = ref<ChatComposerSubmission>()
const pipelineConfigModalOpen = ref(false)

const contextTaskUuid = ref('')
const contextX = ref(0)
const contextY = ref(0)
const detailModalOpen = ref(false)
const previewImageUrl = ref('')
const previewImageVisible = ref(false)

let refreshTimer: ReturnType<typeof setTimeout> | undefined
let highlightTimer: ReturnType<typeof setTimeout> | undefined
let taskLoadVersion = 0
let notificationsLoadVersion = 0
let persistentCacheAvailable = true
let sidebarResizeState:
  | {
      pointerId: number
      pointerX: number
      width: number
    }
  | undefined

const MIN_SIDEBAR_WIDTH = 140
const MAX_SIDEBAR_WIDTH = 500
const SIDEBAR_KEYBOARD_STEP = 10
const SIDEBAR_WIDTH_STORAGE_KEY = 'goteams.tasks.conversationSidebarWidth'

const wsConnected = useLocalWSStatus()

const taskStatusLabels: Record<string, string> = {
  pending: '待开始',
  in_progress: '进行中',
  blocked: '已阻塞',
  done: '已完成',
}
const taskPriorityLabels: Record<string, string> = {
  urgent: '紧急',
  high: '高',
  medium: '中',
  low: '低',
}
const terminalLabels: Record<string, string> = {
  created: '等待执行',
  running: '执行中',
  success: '执行完成',
  failed: '执行失败',
  stopped: '执行停止',
  interrupted: '执行中断',
}

const conversations = computed<TaskConversation[]>(() => {
  const grouped = new Map<string, TaskNotification[]>()
  for (const item of [...temporaryNotifications.value, ...notifications.value]) {
    const items = grouped.get(item.task_uuid) || []
    items.push(item)
    grouped.set(item.task_uuid, items)
  }
  return [...grouped.entries()]
    .map(([taskUuid, items]) => {
      items.sort((a, b) => b.created_at - a.created_at)
      const latest = items[0]
      return {
        task_uuid: taskUuid,
        title: latest.task_title || latest.title || `任务 ${taskUuid.slice(0, 8)}`,
        items,
        latest,
        unread: items.some((item) => !item.is_read),
      }
    })
    .sort((a, b) => b.latest.created_at - a.latest.created_at)
})
const displayedConversationCount = computed(
  () => conversations.value.length + (newConversationPipeline.value ? 1 : 0),
)
const newConversationSteps = computed(() =>
  [...(newConversationPipeline.value?.steps || [])].sort((a, b) => a.sort_order - b.sort_order),
)

const selectedConversation = computed(() =>
  conversations.value.find((item) => item.task_uuid === selectedTaskUuid.value),
)
const contextConversation = computed(() =>
  conversations.value.find((item) => item.task_uuid === contextTaskUuid.value),
)
const sortedSteps = computed(() =>
  [...(task.value?.steps || [])].sort((a, b) => a.sort_order - b.sort_order),
)
const hasPipelineSnapshot = computed(() =>
  Boolean(task.value?.pipeline_snapshot_uuid || sortedSteps.value.length),
)
const effectiveCurrentStepUuid = computed(
  () =>
    task.value?.current_step_uuid ||
    sortedSteps.value.find((step) => step.status === 'active')?.uuid ||
    sortedSteps.value[0]?.uuid ||
    '',
)
const currentStep = computed(() =>
  sortedSteps.value.find((step) => step.uuid === effectiveCurrentStepUuid.value),
)
const currentStepIndex = computed(() =>
  sortedSteps.value.findIndex((step) => step.uuid === effectiveCurrentStepUuid.value),
)
const selectedStep = computed(() =>
  sortedSteps.value.find((step) => step.uuid === selectedStepUuid.value),
)
const selectedStepIndex = computed(() =>
  sortedSteps.value.findIndex((step) => step.uuid === selectedStepUuid.value),
)
const selectedProgress = computed(() =>
  progress.value.filter((item) => item.task_step_uuid === selectedStepUuid.value),
)
const latestSelectedAgentProgress = computed(() =>
  [...selectedProgress.value].reverse().find((item) => !isUserMessage(item)),
)
const activeSelectedProgress = computed(() =>
  [...selectedProgress.value]
    .reverse()
    .find(
      (item) => !isUserMessage(item) && (item.status === 'created' || item.status === 'running'),
    ),
)
const composerCliType = computed(
  () => latestSelectedAgentProgress.value?.cli_type || selectedStep.value?.cli_type || '',
)
const composerModelName = computed(
  () =>
    latestSelectedAgentProgress.value?.model ||
    selectedStep.value?.model_name ||
    selectedStep.value?.model ||
    '',
)
const initialTaskLoading = computed(() => taskLoading.value && !task.value)
const taskRefreshing = computed(() => taskLoading.value && Boolean(task.value))
const selectedIsCurrent = computed(() => selectedStepUuid.value === effectiveCurrentStepUuid.value)
const selectedStepReached = computed(() =>
  Boolean(
    selectedStep.value &&
    (selectedIsCurrent.value ||
      selectedStep.value.status === 'completed' ||
      (currentStepIndex.value >= 0 &&
        selectedStepIndex.value >= 0 &&
        selectedStepIndex.value < currentStepIndex.value) ||
      selectedProgress.value.length > 0),
  ),
)
const hasRunningSelectedConversation = computed(() => Boolean(activeSelectedProgress.value))
const currentStepLocked = computed(
  () =>
    !task.value ||
    task.value.status === 'done' ||
    Boolean(task.value.current_step_completed) ||
    currentStep.value?.status === 'completed',
)
const canAsk = computed(
  () => !taskLoading.value && selectedStepReached.value && !hasRunningSelectedConversation.value,
)
const canComplete = computed(
  () =>
    selectedIsCurrent.value &&
    !currentStepLocked.value &&
    canAsk.value &&
    !submitting.value &&
    !completing.value &&
    selectedProgress.value.length > 0,
)
const lastProgressTerminal = computed(() => {
  const last = [...selectedProgress.value].reverse().find((item) => !isUserMessage(item))
  return Boolean(last && !['created', 'running'].includes(last.status))
})
const isLastStep = computed(() =>
  Boolean(currentStep.value && sortedSteps.value.at(-1)?.uuid === currentStep.value.uuid),
)
const nextStepName = computed(() =>
  isLastStep.value ? '' : sortedSteps.value[currentStepIndex.value + 1]?.name || '',
)
const showNextStepCard = computed(
  () => selectedIsCurrent.value && !currentStepLocked.value && lastProgressTerminal.value,
)
const composerPlaceholder = computed(() => {
  if (taskLoading.value) return '正在刷新任务，请稍候'
  if (!selectedStepReached.value) return '执行到当前步骤后才可发起对话'
  if (hasRunningSelectedConversation.value) return 'Agent 正在执行，请等待本轮完成'
  return '输入留言，输入 @选择成员沟通…'
})
const composerContextText = computed(
  () =>
    `正在与「${selectedStep.value?.name || 'Agent'}」沟通 · 步骤 ${Math.max(selectedStepIndex.value + 1, 1)}`,
)
const taskStatus = computed(() => statusValue(task.value?.status))
const taskStatusText = computed(() => taskStatusLabels[taskStatus.value] || taskStatus.value)
const taskPriorityText = computed(
  () => taskPriorityLabels[String(task.value?.priority || '').toLowerCase()] || '',
)

const md = new MarkdownIt({ breaks: true, linkify: true })
const renderedDescription = computed(() => {
  const text = task.value?.description
  if (!text) return ''
  // 兼容接口可能返回的历史 HTML 描述；纯文本和 Markdown 仍统一渲染。
  if (/<[a-z][\s\S]*>/i.test(text)) return text
  return md.render(text)
})

function handleTaskSaved() {
  void loadSelectedTask()
}

function statusValue(status?: string) {
  if (['todo', 'pending'].includes(status || '')) return 'pending'
  if (['active', 'running', 'developing', 'developed', 'in_progress'].includes(status || ''))
    return 'in_progress'
  if (['done', 'completed'].includes(status || '')) return 'done'
  return status || 'pending'
}

function terminalLabel(status?: string) {
  return terminalLabels[status || ''] || '等待处理'
}

function pipelineInitials(name?: string) {
  return (name || '流').trim().slice(0, 1).toUpperCase()
}

function clearRefreshTimer() {
  if (!refreshTimer) return
  clearTimeout(refreshTimer)
  refreshTimer = undefined
}

// WS 连接正常时进度刷新依赖 task.changed 推送，轮询仅在断线期间兜底，避免长任务持续请求 /progress。
function scheduleRefresh() {
  clearRefreshTimer()
  if (
    wsConnected.value ||
    !progress.value.some(
      (item) => !isUserMessage(item) && (item.status === 'created' || item.status === 'running'),
    )
  ) {
    return
  }
  refreshTimer = setTimeout(() => {
    void loadSelectedTask(true)
  }, 2000)
}

function clearSelectedTask() {
  taskLoadVersion += 1
  clearRefreshTimer()
  task.value = undefined
  progress.value = []
  selectedStepUuid.value = ''
  selectedProgressUuid.value = ''
  taskError.value = ''
  taskLoading.value = false
}

function resolveSelectedStepUuid(
  loadedTask: TaskWithDetails,
  loadedProgress: TaskProgress[],
  preferredStepUuid: string,
) {
  if (preferredStepUuid && loadedTask.steps.some((step) => step.uuid === preferredStepUuid)) {
    return preferredStepUuid
  }
  const latestProgressStep = loadedProgress.at(-1)?.task_step_uuid || ''
  const validLatestProgressStep =
    latestProgressStep && loadedTask.steps.some((step) => step.uuid === latestProgressStep)
      ? latestProgressStep
      : ''
  return (
    validLatestProgressStep ||
    loadedTask.current_step_uuid ||
    loadedTask.steps.find((step) => step.status === 'active')?.uuid ||
    loadedTask.steps[0]?.uuid ||
    ''
  )
}

function restoreCachedTaskView(taskUuid: string) {
  const cached = taskViewCache.get(taskUuid)
  if (!cached) {
    task.value = undefined
    progress.value = []
    return false
  }
  task.value = cached.task
  progress.value = cached.progress
  selectedStepUuid.value = resolveSelectedStepUuid(
    cached.task,
    cached.progress,
    selectedStepUuid.value,
  )
  return true
}

function reportCacheFailure(action: string, error: unknown) {
  persistentCacheAvailable = false
  const reason = error instanceof Error ? error.message : String(error)
  console.warn(`[task-conversation-cache] ${action}失败: ${reason}`)
}

function currentNotificationSnapshot() {
  const uniqueItems = new Map<string, TaskNotification>()
  for (const item of [...toRaw(temporaryNotifications.value), ...toRaw(notifications.value)]) {
    uniqueItems.set(item.uuid, item)
  }
  return [...uniqueItems.values()]
}

async function persistNotificationSnapshot() {
  if (!persistentCacheAvailable) return
  try {
    await saveTaskNotificationSnapshot(currentNotificationSnapshot())
  } catch (error) {
    reportCacheFailure('保存通知快照', error)
  }
}

async function persistViewState() {
  if (!persistentCacheAvailable) return
  try {
    await saveTaskConversationViewState(selectedTaskUuid.value, pipelineExpanded.value)
  } catch (error) {
    reportCacheFailure('保存页面状态', error)
  }
}

async function persistTaskView(
  taskUuid: string,
  loadedTask: TaskWithDetails,
  loadedProgress: TaskProgress[],
) {
  if (!persistentCacheAvailable) return
  try {
    await saveTaskView(taskUuid, loadedTask, loadedProgress)
  } catch (error) {
    reportCacheFailure('保存会话详情', error)
  }
}

async function removePersistentTaskView(taskUuid: string) {
  if (!persistentCacheAvailable) return
  try {
    await deleteTaskView(taskUuid)
  } catch (error) {
    reportCacheFailure('删除会话详情', error)
  }
}

async function reconcileTaskViewCache(validTaskUuids: Set<string>) {
  for (const taskUuid of taskViewCache.keys()) {
    if (!validTaskUuids.has(taskUuid)) taskViewCache.delete(taskUuid)
  }
  if (!persistentCacheAvailable) return
  try {
    await deleteTaskViewsOutside(validTaskUuids)
  } catch (error) {
    reportCacheFailure('清理失效会话', error)
  }
}

async function restorePersistentTaskConversationCache() {
  try {
    const cached = await readTaskConversationCache()
    notifications.value = cached.notifications
    temporaryNotifications.value = []
    taskViewCache.clear()
    cached.taskViews.forEach((item) => {
      taskViewCache.set(item.taskUuid, { task: item.task, progress: item.progress })
    })
    pipelineExpanded.value = cached.pipelineExpanded
    if (!conversations.value.length) return false

    const selectedConversationFromCache =
      conversations.value.find((item) => item.task_uuid === cached.lastSelectedTaskUuid) ||
      conversations.value[0]
    selectedTaskUuid.value = selectedConversationFromCache.task_uuid
    selectedStepUuid.value = selectedConversationFromCache.latest.task_step_uuid
    selectedProgressUuid.value = selectedConversationFromCache.latest.progress_uuid || ''
    restoreCachedTaskView(selectedConversationFromCache.task_uuid)
    void persistViewState()
    return true
  } catch (error) {
    reportCacheFailure('读取本地缓存', error)
    return false
  }
}

async function loadNotifications(preserveSelection = true) {
  const requestVersion = ++notificationsLoadVersion
  const hadCachedNotifications = conversations.value.length > 0
  notificationsLoading.value = true
  notificationsError.value = ''
  try {
    const result = await apiClient.get<{ items: TaskNotification[] }>('/notifications')
    if (requestVersion !== notificationsLoadVersion) return
    const loadedNotifications = result.items || []
    notifications.value = loadedNotifications
    temporaryNotifications.value = temporaryNotifications.value.filter(
      (temporary) => !loadedNotifications.some((item) => item.task_uuid === temporary.task_uuid),
    )
    const previousSelectedTaskUuid = selectedTaskUuid.value
    if (
      !newConversationPipeline.value &&
      (!preserveSelection ||
        !conversations.value.some((item) => item.task_uuid === selectedTaskUuid.value))
    ) {
      const first = conversations.value[0]
      selectedTaskUuid.value = first?.task_uuid || ''
      selectedStepUuid.value = first?.latest.task_step_uuid || ''
      selectedProgressUuid.value = first?.latest.progress_uuid || ''
    }
    const selectionChanged = previousSelectedTaskUuid !== selectedTaskUuid.value
    if (!conversations.value.length) {
      selectedTaskUuid.value = ''
      clearSelectedTask()
    } else if (selectionChanged && selectedTaskUuid.value) {
      clearSelectedTask()
      restoreCachedTaskView(selectedTaskUuid.value)
    }

    const validTaskUuids = new Set(conversations.value.map((item) => item.task_uuid))
    await Promise.all([
      persistNotificationSnapshot(),
      persistViewState(),
      reconcileTaskViewCache(validTaskUuids),
    ])
    if (selectionChanged && selectedTaskUuid.value) void loadSelectedTask()
  } catch (error) {
    if (requestVersion !== notificationsLoadVersion) return
    if (hadCachedNotifications || conversations.value.length) {
      message.warning('已显示本地缓存，暂时无法获取最新会话')
    } else {
      notificationsError.value = error instanceof Error ? error.message : '任务会话加载失败'
      message.error(notificationsError.value)
    }
  } finally {
    if (requestVersion === notificationsLoadVersion) notificationsLoading.value = false
  }
}

async function loadSelectedTask(silent = false) {
  const taskUuid = selectedTaskUuid.value
  if (!taskUuid) return
  const requestVersion = ++taskLoadVersion
  clearRefreshTimer()
  if (!silent) {
    if (task.value?.uuid !== taskUuid) {
      const restoredFromCache = restoreCachedTaskView(taskUuid)
      if (restoredFromCache) void locateTarget('auto')
    }
    taskLoading.value = true
    taskError.value = ''
  }
  try {
    const [loadedTask, loadedProgress] = await Promise.all([
      apiClient.get<TaskWithDetails>(`/tasks/${taskUuid}`),
      apiClient.get<{ items: TaskProgress[] } | TaskProgress[]>(`/tasks/${taskUuid}/progress`),
    ])
    if (requestVersion !== taskLoadVersion || taskUuid !== selectedTaskUuid.value) return
    const progressItems = Array.isArray(loadedProgress)
      ? loadedProgress
      : loadedProgress.items || []
    const sortedProgress = [...progressItems].sort((a, b) => a.created_at - b.created_at)
    const nextSelectedStepUuid = resolveSelectedStepUuid(
      loadedTask,
      sortedProgress,
      selectedStepUuid.value,
    )
    taskViewCache.set(taskUuid, { task: loadedTask, progress: sortedProgress })
    task.value = loadedTask
    progress.value = sortedProgress
    selectedStepUuid.value = nextSelectedStepUuid
    taskError.value = ''
    void persistTaskView(taskUuid, loadedTask, sortedProgress)
    await locateTarget('auto')
  } catch (error) {
    if (requestVersion === taskLoadVersion && !silent) {
      if (task.value?.uuid === taskUuid) {
        taskError.value = ''
        message.warning('已显示本地缓存，暂时无法获取最新任务进度')
      } else {
        taskError.value = error instanceof Error ? error.message : '任务进度加载失败'
        message.error(taskError.value)
      }
    }
  } finally {
    if (requestVersion === taskLoadVersion) {
      taskLoading.value = false
      scheduleRefresh()
    }
  }
}

async function initialize() {
  notificationsLoading.value = true
  const restoredFromCache = await restorePersistentTaskConversationCache()
  if (selectedTaskUuid.value) void loadSelectedTask()
  await loadNotifications(restoredFromCache)
}

async function setTaskRead(taskUuid: string, isRead: boolean) {
  const conversation = conversations.value.find((item) => item.task_uuid === taskUuid)
  if (!conversation) return
  const previouslyUnread = conversation.items.filter((item) => !item.is_read)
  conversation.items.forEach((item) => {
    item.is_read = isRead
  })
  try {
    await apiClient.put(`/notifications/tasks/${taskUuid}/read`, { is_read: isRead })
  } catch {
    if (isRead) {
      await Promise.allSettled(
        previouslyUnread.map((item) => apiClient.put(`/notifications/${item.uuid}/read`, {})),
      )
    }
  } finally {
    await persistNotificationSnapshot()
  }
}

async function selectConversation(conversation: TaskConversation) {
  clearNewConversation()
  selectedTaskUuid.value = conversation.task_uuid
  selectedStepUuid.value = conversation.latest.task_step_uuid
  selectedProgressUuid.value = conversation.latest.progress_uuid || ''
  closeContextMenu()
  void persistViewState()
  await loadSelectedTask()
}

async function selectStep(uuid: string) {
  selectedStepUuid.value = uuid
  selectedProgressUuid.value = ''
  await locateTarget('smooth')
  const last = progress.value.filter((item) => item.task_step_uuid === uuid).at(-1)
  if (last) flashTarget(last.uuid)
}

async function locateTarget(behavior: 'auto' | 'smooth' = 'auto') {
  await nextTick()
  const requested = selectedProgressUuid.value
    ? progress.value.find((item) => item.uuid === selectedProgressUuid.value)
    : undefined
  const target =
    requested ||
    progress.value.filter((item) => item.task_step_uuid === selectedStepUuid.value).at(-1)
  if (!target) return
  const container = document.querySelector<HTMLElement>('.message-scroll')
  const targetElement = document.getElementById(`progress-${target.uuid}`)
  if (!container || !targetElement || !container.contains(targetElement)) return
  const containerRect = container.getBoundingClientRect()
  const targetRect = targetElement.getBoundingClientRect()
  const centeredOffset = Math.max((container.clientHeight - targetRect.height) / 2, 0)
  const top = container.scrollTop + targetRect.top - containerRect.top - centeredOffset
  container.scrollTo({ top: Math.max(top, 0), behavior })
}

function flashTarget(uuid: string) {
  highlightUuid.value = uuid
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    highlightUuid.value = ''
  }, 1200)
}

function stopSelectedConversation() {
  const sessionUuid = activeSelectedProgress.value?.session_uuid
  if (!sessionUuid || stoppingSessionUuid.value) {
    if (!sessionUuid) message.warning('未找到可停止的运行会话，请刷新后重试')
    return
  }
  if (isStopConfirmSuppressed()) {
    void stopConversation(sessionUuid)
    return
  }
  stopConfirmSessionUuid.value = sessionUuid
  stopConfirmOpen.value = true
}

function closeStopConfirm() {
  stopConfirmOpen.value = false
  stopConfirmSessionUuid.value = ''
}

function confirmStopConversation(suppressFutureConfirm: boolean) {
  const sessionUuid = stopConfirmSessionUuid.value
  if (suppressFutureConfirm) suppressStopConfirm()
  if (sessionUuid) void stopConversation(sessionUuid)
}

async function stopConversation(sessionUuid: string) {
  stoppingSessionUuid.value = sessionUuid
  try {
    await apiClient.post(`/tasks/sessions/${encodeURIComponent(sessionUuid)}/stop`, {})
    message.success('当前运行已停止')
    await Promise.all([loadNotifications(true), loadSelectedTask()])
  } catch (error) {
    message.error(error instanceof Error ? error.message : '停止运行失败')
  } finally {
    stoppingSessionUuid.value = ''
    closeStopConfirm()
  }
}

async function submitQuestion(submission: ChatComposerSubmission) {
  const text = submission.content.trim()
  const step = selectedStep.value
  if (!text || !step || !canAsk.value || submitting.value) return
  submitting.value = true
  try {
    await apiClient.post(`/tasks/${selectedTaskUuid.value}/steps/${step.uuid}/questions`, {
      question: text,
      display_question: submission.display_content?.trim() || text,
      request_id: crypto.randomUUID(),
      cli_type: submission.config.cli_type,
      model_name: submission.config.model_name,
    })
    question.value = ''
    composerRef.value?.resetAfterSubmit()
    selectedProgressUuid.value = ''
    message.success('消息已发送，继续选中 Agent 对话')
    await setTaskRead(selectedTaskUuid.value, true)
    await Promise.all([loadNotifications(true), loadSelectedTask()])
  } catch (error) {
    message.error(error instanceof Error ? error.message : '消息发送失败')
  } finally {
    submitting.value = false
  }
}

async function completeStep() {
  const step = currentStep.value
  if (!step || !canComplete.value || completing.value) return
  completing.value = true
  try {
    const autoStart = !['stopped', 'interrupted'].includes(
      latestSelectedAgentProgress.value?.status || '',
    )
    const result = await apiClient.post<CompleteStepResponse>(
      `/tasks/${selectedTaskUuid.value}/steps/${step.uuid}/complete`,
      { auto_start: autoStart },
    )
    const wasLastStep = sortedSteps.value.at(-1)?.uuid === step.uuid
    selectedProgressUuid.value = ''
    if (wasLastStep) {
      message.success('任务已完成')
    } else if (result.start_error) {
      message.warning(`已进入下一步，但自动启动失败：${result.start_error}`)
    } else {
      message.success(result.auto_started ? '已进入下一步并自动启动执行' : '已进入下一步')
    }
    await setTaskRead(selectedTaskUuid.value, true)
    await Promise.all([loadNotifications(true), loadSelectedTask()])
    const nextCurrent = task.value?.current_step_uuid
    if (nextCurrent && nextCurrent !== selectedStepUuid.value) {
      selectedStepUuid.value = nextCurrent
      await locateTarget()
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '进入下一步失败')
  } finally {
    completing.value = false
  }
}

async function copyResult(item: TaskProgress) {
  try {
    await copyText(isUserMessage(item) ? userMessageText(item) : resultText(item))
    message.success('已复制')
  } catch {
    message.warning('复制失败')
  }
}

function openContextMenu(event: MouseEvent, conversation: TaskConversation) {
  event.preventDefault()
  showContextMenu(conversation, event.clientX, event.clientY)
}

function showContextMenu(conversation: TaskConversation, x: number, y: number) {
  contextTaskUuid.value = conversation.task_uuid
  contextX.value = Math.min(x, window.innerWidth - 190)
  contextY.value = Math.min(y, window.innerHeight - 150)
}

function toggleConversationMenu(event: MouseEvent, conversation: TaskConversation) {
  if (contextTaskUuid.value === conversation.task_uuid) {
    closeContextMenu()
    return
  }
  const triggerRect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  showContextMenu(conversation, triggerRect.right - 174, triggerRect.bottom + 4)
}

function closeContextMenu() {
  contextTaskUuid.value = ''
}

function togglePipelineExpanded() {
  pipelineExpanded.value = !pipelineExpanded.value
  void persistViewState()
}

async function openConversationDetail() {
  const conversation = contextConversation.value
  if (!conversation) return
  if (selectedTaskUuid.value !== conversation.task_uuid) {
    await selectConversation(conversation)
  } else {
    closeContextMenu()
    if (task.value?.uuid !== conversation.task_uuid) await loadSelectedTask()
  }
  if (task.value?.uuid === conversation.task_uuid) detailModalOpen.value = true
}

async function toggleRead() {
  const conversation = contextConversation.value
  if (!conversation) return
  await setTaskRead(conversation.task_uuid, conversation.unread)
  closeContextMenu()
}

async function archiveConversation() {
  const conversation = contextConversation.value
  if (!conversation) return
  const taskUuid = conversation.task_uuid
  try {
    await apiClient.put(`/notifications/tasks/${taskUuid}/archive`, { is_archived: true })
    taskViewCache.delete(taskUuid)
    notifications.value = notifications.value.filter((item) => item.task_uuid !== taskUuid)
    temporaryNotifications.value = temporaryNotifications.value.filter(
      (item) => item.task_uuid !== taskUuid,
    )
    await Promise.all([removePersistentTaskView(taskUuid), persistNotificationSnapshot()])
    if (selectedTaskUuid.value === taskUuid) {
      const next = conversations.value[0]
      if (next) await selectConversation(next)
      else {
        selectedTaskUuid.value = ''
        clearSelectedTask()
        await persistViewState()
      }
    }
    message.success('任务会话已归档')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '归档失败')
  } finally {
    closeContextMenu()
  }
}

async function loadPipelines(force = false) {
  try {
    await pipelineStore.loadPipelines(force)
  } catch {
    // 请求错误由 Store 统一转换为菜单内的错误状态
  }
}

function handlePipelineMenuOpenChange(open: boolean) {
  pipelineMenuOpen.value = open
  if (open) void loadPipelines()
}

function clearNewConversation() {
  newConversationPipeline.value = undefined
  newConversationQuestion.value = ''
  newConversationWorkDir.value = ''
  newConversationSubmission.value = undefined
  newConversationComposerRef.value?.resetAfterSubmit()
  pipelineConfigModalOpen.value = false
}

function startNewConversation(pipeline: Pipeline) {
  closeContextMenu()
  pipelineMenuOpen.value = false
  selectedTaskUuid.value = ''
  clearSelectedTask()
  newConversationSubmission.value = undefined
  newConversationComposerRef.value?.resetAfterSubmit()
  newConversationPipeline.value = pipeline
  newConversationQuestion.value = ''
  newConversationWorkDir.value = ''
}

async function chooseNewConversationDirectory() {
  try {
    const selected = await selectDirectory(newConversationWorkDir.value)
    if (selected) newConversationWorkDir.value = selected
  } catch (error) {
    message.error(error instanceof Error ? error.message : '无法打开目录选择器')
  }
}

function createTaskTitle(text: string) {
  return text.replace(/\s+/g, ' ').trim().slice(0, 50)
}

function createStepConfigs(pipeline: Pipeline) {
  return (pipeline.steps || []).map((step) => ({
    step_uuid: step.uuid,
    source_step_id: step.source_step_id,
    cloud_step_id: step.cloud_step_id,
    cli_type: step.cli_type,
    model: step.model,
    model_name: step.model_name,
  }))
}

async function submitNewConversation(
  submission?: ChatComposerSubmission,
  pipelineOverride?: Pipeline,
) {
  if (submission) newConversationSubmission.value = submission
  const currentSubmission = submission || newConversationSubmission.value
  const promptText = currentSubmission?.content.trim() || ''
  const displayText = currentSubmission?.display_content?.trim() || promptText
  const pipeline = pipelineOverride || newConversationPipeline.value
  if (!displayText || !pipeline || submitting.value) return
  if (!newConversationWorkDir.value.trim()) {
    message.warning('请先选择工作目录')
    return
  }
  submitting.value = true
  try {
    const createdTask = await apiClient.post<CreatedTaskResponse>('/tasks', {
      title: createTaskTitle(displayText) || '图片任务',
      description: promptText,
      pipeline_uuid: pipeline.uuid,
      step_configs: createStepConfigs(pipeline),
      work_dir: newConversationWorkDir.value.trim(),
      work_dirs: [newConversationWorkDir.value.trim()],
      create_notification: true,
    })
    const notification =
      createdTask.notification ||
      ({
        task_uuid: createdTask.uuid,
        task_title: createdTask.title || createTaskTitle(displayText) || '图片任务',
        step_name: createdTask.steps?.[0]?.name || pipeline.steps?.[0]?.name || 'Agent 编排',
        session_uuid: '',
        status: 'created',
        summary: '任务已创建并开始执行',
        created_at: Date.now(),
      } satisfies CreateTaskNotification)
    const currentStepUuid = createdTask.current_step_uuid || createdTask.steps?.[0]?.uuid || ''
    temporaryNotifications.value = [
      {
        uuid: `created-${notification.task_uuid}-${notification.created_at}`,
        task_uuid: notification.task_uuid,
        task_title: notification.task_title,
        task_step_uuid: currentStepUuid,
        step_name: notification.step_name,
        status: notification.status,
        summary: notification.summary,
        is_read: true,
        created_at: notification.created_at,
      },
      ...temporaryNotifications.value.filter((item) => item.task_uuid !== notification.task_uuid),
    ]
    selectedTaskUuid.value = createdTask.uuid
    selectedStepUuid.value = currentStepUuid
    selectedProgressUuid.value = ''
    task.value = createdTask
    progress.value = []
    taskViewCache.set(createdTask.uuid, { task: createdTask, progress: [] })
    newConversationPipeline.value = undefined
    newConversationQuestion.value = ''
    newConversationWorkDir.value = ''
    newConversationSubmission.value = undefined
    newConversationComposerRef.value?.resetAfterSubmit()
    await Promise.all([
      persistNotificationSnapshot(),
      persistViewState(),
      persistTaskView(createdTask.uuid, createdTask, []),
    ])
    message.success(notification.summary)
    await loadSelectedTask()
  } catch (error) {
    if (error instanceof ApiError && error.code === 'pipeline_incomplete') {
      pipelineConfigModalOpen.value = true
      message.warning(error.message)
    } else {
      message.error(error instanceof Error ? error.message : '任务创建失败')
    }
  } finally {
    submitting.value = false
  }
}

function handleCreationPipelineSelected(pipeline: Pipeline) {
  newConversationPipeline.value = pipeline
  pipelineConfigModalOpen.value = false
  void submitNewConversation(undefined, pipeline)
}

function getPipelinePopupContainer(trigger: HTMLElement) {
  return trigger.parentElement || document.body
}

function showImagePreview(url: string) {
  previewImageUrl.value = url
  previewImageVisible.value = true
}

function syncSidebarMetrics() {
  const layoutWidth = taskLayoutRef.value?.getBoundingClientRect().width || MIN_SIDEBAR_WIDTH
  sidebarMaxWidth.value = Math.max(
    MIN_SIDEBAR_WIDTH,
    Math.min(MAX_SIDEBAR_WIDTH, Math.floor(layoutWidth)),
  )
  if (sidebarResized.value) {
    sidebarWidth.value = Math.min(sidebarMaxWidth.value, sidebarWidth.value)
    return
  }
  sidebarWidth.value = conversationSidebarRef.value?.getBoundingClientRect().width || 300
}

function restoreSidebarWidth() {
  syncSidebarMetrics()
  try {
    const cachedWidth = window.localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY)
    if (!cachedWidth) return
    const width = Number(cachedWidth)
    if (Number.isFinite(width)) setSidebarWidth(width)
  } catch {
    // 本地存储不可用时保留页面默认宽度
  }
}

function persistSidebarWidth() {
  try {
    window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(Math.round(sidebarWidth.value)))
  } catch {
    // 本地存储不可用不影响侧栏宽度调整
  }
}

function setSidebarWidth(width: number) {
  sidebarResized.value = true
  sidebarWidth.value = Math.min(sidebarMaxWidth.value, Math.max(MIN_SIDEBAR_WIDTH, width))
}

function startSidebarResize(event: PointerEvent) {
  if (event.button !== 0) return
  syncSidebarMetrics()
  sidebarResizeState = {
    pointerId: event.pointerId,
    pointerX: event.clientX,
    width: sidebarWidth.value,
  }
  sidebarDragging.value = true
  const target = event.currentTarget as HTMLElement
  target.setPointerCapture(event.pointerId)
  event.preventDefault()
}

function handleSidebarResize(event: PointerEvent) {
  if (!sidebarResizeState || sidebarResizeState.pointerId !== event.pointerId) return
  setSidebarWidth(sidebarResizeState.width + event.clientX - sidebarResizeState.pointerX)
  event.preventDefault()
}

function finishSidebarResize(event: PointerEvent) {
  if (!sidebarResizeState || sidebarResizeState.pointerId !== event.pointerId) return
  const target = event.currentTarget as HTMLElement
  sidebarResizeState = undefined
  sidebarDragging.value = false
  persistSidebarWidth()
  if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId)
}

function handleSidebarResizeKeydown(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  syncSidebarMetrics()
  if (event.key === 'Home') setSidebarWidth(MIN_SIDEBAR_WIDTH)
  else if (event.key === 'End') setSidebarWidth(sidebarMaxWidth.value)
  else {
    setSidebarWidth(
      sidebarWidth.value +
        (event.key === 'ArrowLeft' ? -SIDEBAR_KEYBOARD_STEP : SIDEBAR_KEYBOARD_STEP),
    )
  }
  persistSidebarWidth()
  event.preventDefault()
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  closeContextMenu()
  pipelineMenuOpen.value = false
}

useLocalWS('task.changed', (data: { task_uuid?: string }) => {
  void loadNotifications(true)
  if (data.task_uuid === selectedTaskUuid.value) void loadSelectedTask(true)
})

// CLI 活动推送（节流约 1s/条）：执行期间没有 task.changed 广播，需就地更新
// 运行中消息下方的实时活动小字（bash 等简述），否则推送模式下会一直停滞。
useLocalWS(
  'executor.activity',
  (data: {
    task_uuid?: string
    session_uuid?: string
    event_type?: string
    content?: string
    at?: number
  }) => {
    if (!data.session_uuid) return
    // 继续对话时同一 session 有两条 progress（用户提问 + AI 执行），
    // 只能更新 AI 执行项，用户气泡不渲染活动小字
    const target = progress.value.find(
      (item) => !isUserMessage(item) && item.session_uuid === data.session_uuid,
    )
    if (target) {
      target.latest_event_type = data.event_type
      target.latest_event_content = data.content
      target.latest_event_at = data.at
    }
  },
)

watch(wsConnected, (connected) => {
  if (connected) {
    // 重连成功后补偿拉取一次，随后继续依赖推送刷新
    if (selectedTaskUuid.value) void loadSelectedTask(true)
  } else {
    scheduleRefresh()
  }
})

watch(pipelines, (items) => {
  if (
    newConversationPipeline.value &&
    !items.some((item) => item.uuid === newConversationPipeline.value?.uuid)
  ) {
    newConversationPipeline.value = undefined
  }
})

onMounted(() => {
  document.addEventListener('click', closeContextMenu)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('resize', syncSidebarMetrics)
  restoreSidebarWidth()
  void initialize()
})

onBeforeUnmount(() => {
  taskLoadVersion += 1
  notificationsLoadVersion += 1
  clearRefreshTimer()
  if (highlightTimer) clearTimeout(highlightTimer)
  document.removeEventListener('click', closeContextMenu)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', syncSidebarMetrics)
})
</script>

<style scoped>
.task-page {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
}

.page-titlebar {
  display: flex;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}

.page-titlebar-copy {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.page-titlebar strong {
  color: #262626;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.page-titlebar p {
  overflow: hidden;
  margin: 0;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.overview-toggle-button {
  display: inline-flex;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 6px;
  background: transparent;
  cursor: pointer;
  transition: background-color 180ms ease;
}

.overview-toggle-button:hover {
  background: #e4e6eb;
}

.overview-toggle-button:active {
  background: #d9dce2;
}

.overview-toggle-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.overview-toggle-button img {
  display: block;
  width: 16px;
  height: 16px;
}

.task-overview {
  position: relative;
  flex: 0 0 auto;
  height: 140px;
  padding: 24px 24px 0 24px;
  overflow: hidden;
  box-sizing: border-box;
  border-bottom: 1px solid #d9d9d9;
  background: #fff;
}

.task-overview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 22px;
}

.task-overview-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.task-overview-title h2 {
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-status,
.task-priority {
  flex: 0 0 auto;
  border: 1px solid currentColor;
  border-radius: 6px;
  padding: 0px 6px;
  font-size: 12px;
  line-height: 16px;
}

.task-status {
  color: #d97706;
}

.task-status.status-done {
  color: #16a34a;
}

.task-status.status-blocked {
  color: #ef4444;
}

.task-priority {
  color: #ef4444;
}

.sidebar-add-button:focus-visible,
.conversation-select-button:focus-visible,
.conversation-menu-button:focus-visible,
.context-menu button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.task-step-region {
  overflow: hidden;
  height: auto;
  box-sizing: border-box;
}

.task-step-region :deep(.step-strip) {
  padding-bottom: 5px;
}

.step-region-empty {
  padding: 16px 0 20px;
  color: #8c8c8c;
  font-size: 14px;
}

.task-overview-skeleton {
  min-height: 60px;
}

.overview-skeleton-head {
  display: flex;
  min-height: 60px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 8px 24px 0;
}

.overview-skeleton-title,
.overview-skeleton-steps span,
.composer-loading-body {
  border-radius: 6px;
  background: #f0f1f3;
}

.overview-skeleton-title {
  width: min(360px, 42%);
  height: 28px;
}

.overview-skeleton-steps {
  display: flex;
  min-height: 72px;
  box-sizing: border-box;
  align-items: center;
  gap: 36px;
  padding: 0 24px 8px;
}

.overview-skeleton-steps span {
  width: 155px;
  height: 40px;
}

.task-layout {
  display: flex;
  min-height: 0;
  flex: 1;
}

.task-layout.is-resizing-sidebar {
  cursor: col-resize;
  user-select: none;
}

.conversation-sidebar {
  position: relative;
  display: flex;
  width: 300px;
  min-width: 140px;
  max-width: 500px;
  min-height: 0;
  flex: 0 0 300px;
  flex-direction: column;
  border-right: 1px solid #f0f0f0;
  background: #fff;
}

.sidebar-resize-handle {
  position: absolute;
  z-index: 1;
  top: 0;
  right: -6px;
  bottom: 0;
  width: 12px;
  cursor: col-resize;
  touch-action: none;
}

.sidebar-resize-handle::after {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 50%;
  width: 1px;
  background: transparent;
  content: '';
  transform: translateX(-50%);
}

.sidebar-resize-handle:hover::after,
.sidebar-resize-handle:focus-visible::after,
.is-resizing-sidebar .sidebar-resize-handle::after {
  width: 2px;
  background: #3157e2;
}

.sidebar-resize-handle:focus-visible {
  outline: none;
}

.sidebar-heading {
  position: relative;
  display: flex;
  min-height: 56px;
  flex: 0 0 56px;
  align-items: center;
  gap: 8px;
  padding: 16px 24px;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.sidebar-heading small {
  display: inline-flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  padding: 0;
  color: #8c8c8c;
  background: #edeff2;
  font-size: 12px;
  font-weight: 400;
  line-height: 14px;
}

.sidebar-add-button {
  appearance: none;
  position: absolute;
  top: 16px;
  right: 24px;
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 0;
  color: #2475fc;
  background: transparent;
  cursor: pointer;
}

.sidebar-add-button:hover,
.sidebar-add-button[aria-expanded='true'] {
  background: rgba(49, 87, 226, 0.08);
}

.pipeline-menu {
  width: 186px;
  overflow: hidden;
  border-radius: 6px;
  padding: 2px;
  background: #fff;
  box-shadow:
    0 8px 10px -5px rgba(0, 0, 0, 0.08),
    0 16px 24px 2px rgba(0, 0, 0, 0.04),
    0 6px 30px 5px rgba(0, 0, 0, 0.05);
}

.pipeline-menu-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px 4px;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
}

.pipeline-menu-list {
  max-height: 280px;
  overflow-y: auto;
  padding-bottom: 2px;
}

.pipeline-menu-item {
  display: flex;
  width: 100%;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  border: 0;
  border-radius: 6px;
  padding: 5px 16px;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
  text-align: left;
}

.pipeline-menu-item:hover,
.pipeline-menu-item:focus-visible {
  background: #f5f5f5;
}

.pipeline-menu-item:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}

.pipeline-menu-item img,
.pipeline-avatar-fallback {
  display: inline-flex;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  object-fit: cover;
}

.pipeline-avatar-fallback {
  color: #3157e2;
  background: #eef2ff;
  font-size: 10px;
  font-weight: 600;
}

.pipeline-menu-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pipeline-menu-state {
  display: flex;
  min-height: 72px;
  align-items: center;
  justify-content: center;
  padding: 12px;
  color: #8c8c8c;
  font-size: 12px;
  text-align: center;
}

.pipeline-menu-error {
  flex-direction: column;
  gap: 8px;
}

.pipeline-menu-error button,
.sidebar-error button,
.main-error button {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 0;
  border-radius: 6px;
  padding: 4px 8px;
  color: #3157e2;
  background: #eef2ff;
  cursor: pointer;
}

.conversation-list {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 4px;
  overflow-y: auto;
  padding: 8px 24px;
}

.conversation-item {
  position: relative;
  display: flex;
  width: 100%;
  min-height: 70px;
  align-items: center;
  gap: 10px;
  margin: 0;
  border: 0;
  border-radius: 6px;
  padding: 12px;
  color: inherit;
  background: #fff;
  text-align: left;
}

.conversation-item:hover {
  background: #f2f4f7;
}

.conversation-item.active {
  background: #e5efff;
}

.new-conversation-item {
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  cursor: default;
}

.conversation-select-button {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  border: 0;
  padding: 0;
  color: inherit;
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.conversation-menu-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 4px;
  color: #000;
  background: transparent;
  cursor: pointer;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity 180ms ease,
    background-color 180ms ease;
}

.conversation-item:hover .conversation-menu-button,
.conversation-item:focus-within .conversation-menu-button,
.conversation-menu-button[aria-expanded='true'] {
  opacity: 1;
  pointer-events: auto;
}

.conversation-menu-button:hover {
  background: #e4e6eb;
}

.conversation-menu-button:active {
  background: #d9dce2;
}

.conversation-menu-button img {
  display: block;
  width: 16px;
  height: 16px;
}

@media (prefers-reduced-motion: reduce) {
  .conversation-menu-button {
    transition: none;
  }
}

.conversation-title {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.conversation-title strong {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  color: #262626;
  font-size: 14px;
  font-weight: 500;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conversation-title i {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 50%;
  background: #fb363f;
}

.conversation-subtitle {
  width: 100%;
  overflow: hidden;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-loading,
.sidebar-error,
.sidebar-empty {
  display: flex;
  min-height: 220px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  color: #8c8c8c;
  font-size: 12px;
  text-align: center;
}

.sidebar-empty :deep(svg) {
  color: #cbd5e1;
  font-size: 24px;
}

.conversation-main {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  background: #fff;
}

.conversation-loading-shell {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

.message-scroll {
  position: relative;
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 16px 32px 24px;
}

.message-loading-skeleton {
  padding-top: 32px;
}

.message-loading-skeleton :deep(.ant-skeleton-paragraph) {
  margin: 0;
}

.message-loading-skeleton :deep(.ant-skeleton-paragraph > li) {
  height: 18px;
  margin-top: 22px;
}

.composer-loading-skeleton {
  min-height: 136px;
  flex: 0 0 136px;
  box-sizing: border-box;
  padding: 8px 22px 24px;
  background: #fff;
}

.composer-loading-body {
  height: 94px;
  border: 1px solid #e5e7eb;
  background: #f7f8fa;
}

.message-refresh-indicator {
  position: sticky;
  z-index: 2;
  top: 0;
  display: flex;
  height: 0;
  justify-content: center;
  pointer-events: none;
}

.main-refresh-indicator {
  position: absolute;
  z-index: 2;
  top: 16px;
  left: 50%;
  pointer-events: none;
  transform: translateX(-50%);
}

.message-refresh-indicator :deep(.ant-spin) {
  margin-top: 4px;
  border-radius: 999px;
  padding: 5px 9px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
}

.message-scroll :deep(.message-list-card) {
  overflow: visible;
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

.message-scroll :deep(.message-list) {
  padding: 0;
}

.main-state {
  position: relative;
  display: flex;
  min-height: 0;
  flex: 1;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  color: #8c8c8c;
  text-align: center;
}

.main-state > :deep(svg) {
  color: #cbd5e1;
  font-size: 34px;
}

.main-state h3 {
  margin: 4px 0 0;
  color: #595959;
  font-size: 14px;
}

.main-state p {
  margin: 0;
  font-size: 12px;
}

.new-conversation-state :deep(.pipeline-flow-icon) {
  width: 34px;
  height: 34px;
  color: #3157e2;
}

.context-menu {
  position: fixed;
  z-index: 1000;
  width: 174px;
  padding: 5px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.14);
}

.context-menu button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 8px 9px;
  border: 0;
  border-radius: 5px;
  color: #4b5563;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  text-align: left;
}

.context-menu button:hover {
  background: #f5f6f8;
}

.context-menu button > img {
  display: block;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
}

@media (max-width: 900px) {
  .conversation-sidebar {
    width: 240px;
    flex-basis: 240px;
  }

  .conversation-list,
  .sidebar-heading {
    padding-right: 12px;
    padding-left: 12px;
  }

  .message-scroll {
    padding-right: 20px;
    padding-left: 20px;
  }
}

@media (max-width: 700px) {
  .page-titlebar p {
    display: none;
  }

  .task-overview-head {
    align-items: flex-start;
    flex-direction: column;
    padding-bottom: 8px;
  }

  .task-overview-skeleton,
  .overview-skeleton-head {
    min-height: 88px;
  }

  .task-overview {
    min-height: 160px;
    max-height: 160px;
  }

  .task-overview-enter-from,
  .task-overview-leave-to {
    min-height: 0;
    max-height: 0;
  }

  .overview-skeleton-head {
    align-items: flex-start;
    justify-content: center;
    flex-direction: column;
    gap: 12px;
    padding-bottom: 8px;
  }

  .overview-skeleton-title {
    width: min(320px, 72%);
    height: 24px;
  }

  .task-overview-title h2 {
    font-size: 16px;
    line-height: 24px;
  }

  .task-step-region {
    padding-right: 16px;
    padding-left: 16px;
  }

  .composer-loading-skeleton {
    min-height: 176px;
    flex-basis: 176px;
    padding-right: 16px;
    padding-left: 16px;
  }

  .composer-loading-body {
    height: 134px;
  }

  .conversation-sidebar {
    width: 210px;
    flex-basis: 210px;
  }

  .conversation-item time {
    display: none;
  }

  .conversation-title,
  .conversation-subtitle {
    padding-right: 0;
  }
}
</style>
