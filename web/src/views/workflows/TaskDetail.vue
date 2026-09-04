<template>
  <div
    class="task-detail-page"
    :class="{ embedded }"
  >
    <header class="detail-header">
      <button
        v-if="!embedded"
        type="button"
        class="back-button"
        title="返回"
        aria-label="返回"
        @click="returnToBoard"
      >
        <LeftOutlined />
      </button>
      <div class="detail-title">
        <h1 class="detail-title-text">看板详情</h1>
      </div>
      <div
        v-if="task"
        class="header-controls"
      >
        <a-select
          v-model:value="selectedTaskStatus"
          class="status-select"
          :disabled="changingStatus"
          :options="taskStatusOptions"
          aria-label="任务状态"
          @change="changeStatus"
        />
        <div
          class="work-dir"
          :title="task.work_dir || '未设置工作目录'"
        >
          <FolderOpenOutlined /><span>{{ task.work_dir || '未设置工作目录' }}</span>
        </div>
        <button
          type="button"
          class="detail-button"
          title="查看任务详情"
          aria-label="查看任务详情"
          @click="detailModalOpen = true"
        >
          <img
            class="detail-info-icon"
            :src="detailInfoIcon"
            alt=""
            aria-hidden="true"
          />
        </button>
        <button
          v-if="taskStatus === 'pending'"
          type="button"
          class="start-button"
          :disabled="starting"
          @click="startTask"
        >
          {{ starting ? '启动中…' : '启动任务' }}
        </button>
        <button
          type="button"
          class="delete-button"
          :disabled="deleting"
          title="删除任务"
          aria-label="删除任务"
          @click="deleteTask"
        >
          <img
            :src="deleteTaskIcon"
            alt=""
            aria-hidden="true"
          />
        </button>
      </div>
    </header>

    <section
      v-if="task"
      class="pipeline-card"
    >
      <div class="pipeline-card-head">
        <PipelineFlowIcon class="pipeline-card-icon" />
        <span class="pipeline-card-label">执行流水线</span>
        <span
          class="pipeline-card-sep"
          aria-hidden="true"
        ></span>
        <strong class="pipeline-card-name">{{
          task.pipeline_name_snapshot || '未指派流水线'
        }}</strong>
        <span
          v-if="sortedSteps.length"
          class="pipeline-card-progress"
          >第 {{ currentStepIndex + 1 }}/{{ sortedSteps.length }} 步</span
        >
      </div>
      <AgentStepStrip
        v-if="hasPipelineSnapshot"
        :steps="sortedSteps"
        :current-step-index="currentStepIndex"
        :current-step-uuid="effectiveCurrentStepUuid"
        :selected-step-uuid="selectedStepUuid"
        @select="selectStep"
      />
      <UnassignedPipelineGuide
        v-else
        @assign="assignModalOpen = true"
      />
    </section>

    <div
      v-if="task"
      class="detail-scroll"
    >
      <div class="detail-content">
        <template v-if="hasPipelineSnapshot">
          <NextStepButton
            v-if="showNextStepCard"
            :next-step-name="nextStepName"
            :is-last-step="isLastStep"
            :disabled="!canComplete"
            :completing="completing"
            @confirm="completeStep"
          />
          <h2 class="task-title">{{ task.title }}</h2>
          <div
            v-if="task.description"
            class="task-desc"
            :class="{ expanded: descriptionExpanded }"
          >
            <div
              ref="requirementContentRef"
              class="task-desc-content"
              @click="openImagePreview"
            >
              <div
                v-if="descriptionUsesHtml"
                v-html="renderedDescription"
              />
              <MarkdownPreview
                v-else
                :content="task.description"
                :task-uuid="resolvedTaskUuid"
                @attachments-loaded="measureDescriptionOverflow"
              />
            </div>
            <button
              v-if="descriptionNeedsExpand"
              type="button"
              class="desc-toggle"
              @click="descriptionExpanded = !descriptionExpanded"
            >
              <DownOutlined v-if="!descriptionExpanded" />
              <UpOutlined v-else />
              {{ descriptionExpanded ? '收起' : '展开更多' }}
            </button>
          </div>
          <p class="conversation-scope-note">
            <img
              class="conversation-scope-icon"
              :src="conversationFilterIcon"
              alt=""
              aria-hidden="true"
            />
            <span
              >仅展示「{{
                selectedStep?.name || '当前步骤'
              }}」的动态与你的留言，点击执行流水线可切换</span
            >
          </p>
          <StepMessageList
            ref="messageListRef"
            :items="selectedProgress"
            :steps="sortedSteps"
            :highlight-uuid="highlightUuid"
            :loading="loading"
            :selected-step-name="selectedStep?.name || ''"
            :task-uuid="resolvedTaskUuid"
            @copy="copyResult"
          />
        </template>
        <div
          v-else
          class="pipeline-empty"
        >
          <div class="pipeline-empty-icon">
            <img
              src="@/assets/icons/task-unassigned-pipeline.png"
              alt=""
              aria-hidden="true"
            />
          </div>
          <p>未指派流水线</p>
          <span>请先分配流水线，动态记录将自动展示</span>
        </div>
      </div>
    </div>
    <div
      v-else-if="loading"
      class="detail-loading"
    >
      <a-spin />
    </div>
    <a-empty
      v-else
      class="detail-empty"
      description="任务不存在"
    />

    <ChatComposer
      ref="composerRef"
      v-if="task && hasPipelineSnapshot"
      v-model="question"
      :can-ask="canAsk"
      :submitting="submitting"
      :running="Boolean(activeSelectedProgress)"
      :stopping="stoppingSessionUuid === activeSelectedProgress?.session_uuid"
      :cli-type="composerCliType"
      :model-name="composerModelName"
      :placeholder="composerPlaceholder"
      :context-text="composerContextText"
      :task-uuid="resolvedTaskUuid"
      :current-step="selectedStep"
      :steps="sortedSteps"
      :executing-step-uuid="effectiveCurrentStepUuid"
      @submit="submitQuestion"
      @stop="stopSelectedConversation"
      @prompt-saved="load"
    />

    <StopExecutionConfirmModal
      :open="stopConfirmOpen"
      :loading="Boolean(stoppingSessionUuid)"
      @close="closeStopConfirm"
      @confirm="confirmStopConversation"
    />

    <TaskDetailInfoModal
      v-model:open="detailModalOpen"
      :task="task"
      :status="taskStatus"
      :rendered-description="renderedDescription"
      @preview-image="showImagePreview"
      @saved="handleTaskSaved"
    />

    <TaskImagePreviewModal
      v-model:open="previewImageVisible"
      :image-url="previewImageUrl"
    />

    <AssignPipelineModal
      v-model:open="assignModalOpen"
      :task-uuid="resolvedTaskUuid"
      :task-title="task?.title || ''"
      :preferred-pipeline-uuid="task?.selected_pipeline_uuid || ''"
      mode="detail"
      @assigned="handleAssigned"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { DownOutlined, FolderOpenOutlined, LeftOutlined, UpOutlined } from '@ant-design/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import apiClient from '@/api/client'
import detailInfoIcon from '@/assets/icons/task-detail-view.svg'
import deleteTaskIcon from '@/assets/icons/task-detail-delete.svg'
import conversationFilterIcon from '@/assets/icons/task-conversation-filter.svg'
import MarkdownIt from 'markdown-it'
import AssignPipelineModal from '@/components/AssignPipelineModal.vue'
import MarkdownPreview from '@/components/MarkdownPreview.vue'
import AgentStepStrip from '@/components/task-progress/AgentStepStrip.vue'
import ChatComposer from '@/components/task-progress/ChatComposer.vue'
import NextStepButton from '@/components/task-progress/NextStepButton.vue'
import PipelineFlowIcon from '@/components/task-progress/PipelineFlowIcon.vue'
import StepMessageList from '@/components/task-progress/StepMessageList.vue'
import StopExecutionConfirmModal from '@/components/task-progress/StopExecutionConfirmModal.vue'
import UnassignedPipelineGuide from '@/components/task-progress/UnassignedPipelineGuide.vue'
import { isUserMessage, resultText, userMessageText } from '@/components/task-progress/utils'
import { copyText } from '@/utils/clipboard'
import { isStopConfirmSuppressed, suppressStopConfirm } from '@/utils/stopConfirm'
import { useDocumentTitle } from '@/composables/useDocumentTitle'
import { useLocalWS, useLocalWSStatus } from '@/composables/useLocalWebSocket'
import type { CompleteStepResponse, TaskProgress } from '@/types/pipeline'
import type { ChatComposerSubmission } from '@/types/task-attachments'
import type { TaskWithDetails } from '@/types/task-detail'
import TaskDetailInfoModal from '@/views/workflows/components/TaskDetailInfoModal.vue'
import TaskImagePreviewModal from '@/views/workflows/components/TaskImagePreviewModal.vue'

const route = useRoute()
const router = useRouter()
const props = withDefaults(
  defineProps<{ taskUuid?: string; embedded?: boolean; autoStart?: boolean }>(),
  { embedded: false, autoStart: false },
)
const emit = defineEmits<{ close: []; changed: [] }>()
const resolvedTaskUuid = computed(() => props.taskUuid || String(route.params.taskUuid || ''))
const task = ref<TaskWithDetails>()
const deleting = ref(false)
const starting = ref(false)
const changingStatus = ref(false)
const selectedTaskStatus = ref('pending')
const detailModalOpen = ref(false)
const assignModalOpen = ref(false)
const previewImageUrl = ref('')
const previewImageVisible = ref(false)
const descriptionExpanded = ref(false)
const descriptionNeedsExpand = ref(false)
const requirementContentRef = ref<HTMLElement>()
const taskTitle = computed(() => task.value?.title)
const taskStatus = computed(() => statusValue(task.value?.status))
const taskStatusOptions = [
  { value: 'pending', label: '待开始' },
  { value: 'in_progress', label: '进行中' },
  { value: 'blocked', label: '已阻塞' },
  { value: 'done', label: '已完成' },
]
const hasPipelineSnapshot = computed(() =>
  Boolean(task.value?.pipeline_snapshot_uuid || task.value?.steps?.length),
)
useDocumentTitle(taskTitle)

watch(
  taskStatus,
  (status) => {
    selectedTaskStatus.value = status
  },
  { immediate: true },
)

// 轮询、步骤选择和会话展示共享进度数据，由详情容器统一维护以避免重复请求。
const loading = ref(false)
const submitting = ref(false)
const completing = ref(false)
const stoppingSessionUuid = ref('')
const progress = ref<TaskProgress[]>([])
const selectedStepUuid = ref('')
const question = ref('')
const highlightUuid = ref('')
const messageListRef = ref<InstanceType<typeof StepMessageList>>()
const stopConfirmOpen = ref(false)
const stopConfirmSessionUuid = ref('')
const composerRef = ref<InstanceType<typeof ChatComposer>>()
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let highlightTimer: ReturnType<typeof setTimeout> | undefined
const wsConnected = useLocalWSStatus()

const sortedSteps = computed(() =>
  [...(task.value?.steps || [])].sort((a, b) => a.sort_order - b.sort_order),
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
const canAsk = computed(() => selectedStepReached.value && !hasRunningSelectedConversation.value)
const canComplete = computed(
  () =>
    selectedIsCurrent.value &&
    !currentStepLocked.value &&
    canAsk.value &&
    !submitting.value &&
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
  if (!selectedStepReached.value) return '执行到当前步骤后才可发起对话'
  if (hasRunningSelectedConversation.value) return '当前步骤正在执行，请等待本轮完成'
  return '输入留言，与该步骤沟通…'
})
const composerContextText = computed(
  () =>
    `正在与「${selectedStep.value?.name || '当前步骤'}」沟通 · 步骤 ${Math.max(selectedStepIndex.value + 1, 1)}`,
)

function handleTaskSaved() {
  emit('changed')
  void load()
}

function returnToBoard() {
  if (props.embedded) emit('close')
  else void router.push('/board')
}

function statusValue(status?: string) {
  if (['todo', 'pending'].includes(status || '')) return 'pending'
  if (['active', 'running', 'developing', 'developed', 'in_progress'].includes(status || ''))
    return 'in_progress'
  if (['done', 'completed'].includes(status || '')) return 'done'
  return status || 'pending'
}

async function changeStatus(next: string) {
  if (!task.value || changingStatus.value) return
  const previous = statusValue(task.value.status)
  if (next === previous) return
  if (next === 'in_progress' && !hasPipelineSnapshot.value) {
    selectedTaskStatus.value = previous
    assignModalOpen.value = true
    return
  }
  // 启动依赖完整的步骤配置；缺项时回到分配弹窗补全，避免提交不可执行快照。
  if (
    next === 'in_progress' &&
    hasPipelineSnapshot.value &&
    task.value.steps?.some((s) => !s.prompt_snapshot || !s.cli_type || !s.model_name)
  ) {
    selectedTaskStatus.value = previous
    assignModalOpen.value = true
    return
  }
  changingStatus.value = true
  try {
    await apiClient.put(`/tasks/${resolvedTaskUuid.value}/status`, { status: next })
    task.value.status = next
    if (next === 'in_progress' && previous === 'pending') {
      message.success('任务已切换为进行中并自动启动')
      void load()
    }
    emit('changed')
  } catch (error) {
    selectedTaskStatus.value = previous
    message.error(error instanceof Error ? error.message : '状态更新失败')
  } finally {
    changingStatus.value = false
  }
}

async function startTask() {
  if (!task.value || starting.value) return
  if (!hasPipelineSnapshot.value) {
    assignModalOpen.value = true
    return
  }
  starting.value = true
  try {
    await apiClient.post(`/tasks/${resolvedTaskUuid.value}/start`, {})
    task.value.status = 'in_progress'
    message.success('任务已启动')
    void load()
    emit('changed')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '任务启动失败')
  } finally {
    starting.value = false
  }
}

function handleAssigned() {
  void load()
  emit('changed')
}

function deleteTask() {
  Modal.confirm({
    title: '确认删除任务',
    content:
      '将同时删除该任务的临时流水线、Agent 编排快照、进度、通知和任务专属目录。此操作无法恢复。',
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    onOk: async () => {
      deleting.value = true
      try {
        await apiClient.delete(`/tasks/${resolvedTaskUuid.value}`)
        message.success('任务已删除')
        emit('changed')
        returnToBoard()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      } finally {
        deleting.value = false
      }
    },
  })
}

const md = new MarkdownIt({ breaks: true, linkify: true })
const descriptionUsesHtml = computed(() => /<[a-z][\s\S]*>/i.test(task.value?.description || ''))

const renderedDescription = computed(() => {
  const text = task.value?.description
  if (!text) return ''
  // 兼容接口可能返回的历史 HTML 描述；纯文本和 Markdown 仍统一渲染。
  if (descriptionUsesHtml.value) return text
  return md.render(text)
})

function measureDescriptionOverflow() {
  const content = requirementContentRef.value
  if (content) descriptionNeedsExpand.value = content.scrollHeight > content.clientHeight + 1
}

async function load(silent = false) {
  if (!resolvedTaskUuid.value) return
  if (!silent) loading.value = true
  if (refreshTimer) clearTimeout(refreshTimer)
  try {
    const [loadedTask, loadedProgress] = await Promise.all([
      apiClient.get<TaskWithDetails>(`/tasks/${resolvedTaskUuid.value}`),
      apiClient.get<{ items: TaskProgress[] } | TaskProgress[]>(
        `/tasks/${resolvedTaskUuid.value}/progress`,
      ),
    ])
    task.value = loadedTask
    const progressItems = Array.isArray(loadedProgress)
      ? loadedProgress
      : loadedProgress.items || []
    progress.value = [...progressItems].sort((a, b) => a.created_at - b.created_at)
    if (
      !selectedStepUuid.value ||
      !loadedTask.steps.some((step) => step.uuid === selectedStepUuid.value)
    ) {
      const latestProgressStep = progress.value.at(-1)?.task_step_uuid || ''
      const validLatestProgressStep =
        latestProgressStep && loadedTask.steps.some((step) => step.uuid === latestProgressStep)
          ? latestProgressStep
          : ''
      selectedStepUuid.value =
        validLatestProgressStep ||
        loadedTask.current_step_uuid ||
        loadedTask.steps.find((step) => step.status === 'active')?.uuid ||
        loadedTask.steps[0]?.uuid ||
        ''
    }
    descriptionExpanded.value = false
    await nextTick()
    measureDescriptionOverflow()
  } catch (error) {
    if (!silent) message.error(error instanceof Error ? error.message : '任务进度加载失败')
  } finally {
    if (!silent) loading.value = false
    scheduleRefresh()
  }
}

// WS 连接正常时进度刷新依赖 task.changed 推送，轮询仅在断线期间兜底，避免长任务持续请求 /progress。
function scheduleRefresh() {
  if (refreshTimer) clearTimeout(refreshTimer)
  if (
    wsConnected.value ||
    !progress.value.some(
      (item) => !isUserMessage(item) && (item.status === 'created' || item.status === 'running'),
    )
  ) {
    return
  }
  refreshTimer = setTimeout(() => {
    void load(true)
  }, 2000)
}

useLocalWS('task.changed', (data: { task_uuid?: string }) => {
  if (data.task_uuid === resolvedTaskUuid.value) {
    void load(true)
    emit('changed')
  }
})

// CLI 活动推送（节流约 1s/条）：就地更新正在执行消息下方的实时活动小字，避免重新拉 /progress。
useLocalWS(
  'executor.activity',
  (data: {
    task_uuid?: string
    session_uuid?: string
    event_type?: string
    content?: string
    at?: number
  }) => {
    if (!data.session_uuid || data.task_uuid !== resolvedTaskUuid.value) return
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
    void load(true)
  } else {
    scheduleRefresh()
  }
})

async function locateTarget() {
  await nextTick()
  const last = progress.value
    .filter((item) => item.task_step_uuid === selectedStepUuid.value)
    .at(-1)
  if (last) messageListRef.value?.scrollToItem(last.uuid)
}

function selectStep(uuid: string) {
  selectedStepUuid.value = uuid
  const last = progress.value.filter((item) => item.task_step_uuid === uuid).at(-1)
  if (last) flashTarget(last.uuid)
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
    await load()
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
  if (!text || !step || !canAsk.value) return
  submitting.value = true
  try {
    await apiClient.post(`/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/questions`, {
      question: text,
      display_question: submission.display_content?.trim() || text,
      request_id: crypto.randomUUID(),
      cli_type: submission.config.cli_type,
      model_name: submission.config.model_name,
    })
    question.value = ''
    composerRef.value?.resetAfterSubmit()
    message.success('消息已发送，继续选中 Agent 对话')
    await load()
    await locateTarget()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '消息发送失败')
  } finally {
    submitting.value = false
  }
}

async function completeStep() {
  const step = currentStep.value
  if (!step || !canComplete.value) return
  completing.value = true
  try {
    const autoStart = !['stopped', 'interrupted'].includes(
      latestSelectedAgentProgress.value?.status || '',
    )
    const result = await apiClient.post<CompleteStepResponse>(
      `/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/complete`,
      { auto_start: autoStart },
    )
    const wasLastStep = sortedSteps.value.at(-1)?.uuid === step.uuid
    if (wasLastStep) {
      message.success('任务已完成')
    } else if (result.start_error) {
      message.warning(`已进入下一步，但自动启动失败：${result.start_error}`)
    } else {
      message.success(result.auto_started ? '已进入下一步并自动启动执行' : '已进入下一步')
    }
    await load()
    const nextCurrent = task.value?.current_step_uuid
    if (nextCurrent && nextCurrent !== selectedStepUuid.value) {
      selectedStepUuid.value = nextCurrent
      await locateTarget()
    }
    if (wasLastStep) emit('changed')
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

function openImagePreview(e: MouseEvent) {
  const target = e.target
  if (target instanceof HTMLImageElement) showImagePreview(target.src)
}

function showImagePreview(url: string) {
  previewImageUrl.value = url
  previewImageVisible.value = true
}

watch(
  resolvedTaskUuid,
  () => {
    selectedStepUuid.value = ''
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (refreshTimer) clearTimeout(refreshTimer)
  if (highlightTimer) clearTimeout(highlightTimer)
})
</script>

<style scoped>
.task-detail-page {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  background: #fff;
}

.detail-header {
  display: flex;
  min-height: 44px;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  padding: 8px 24px;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}

.detail-title {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
}

.back-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 4px;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-size: 16px;
}

.back-button:hover {
  background: #f5f5f5;
}

.detail-button,
.delete-button {
  display: inline-flex;
  width: 28px;
  height: 28px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 6px;
  color: #8c8c8c;
  background: #fff;
  cursor: pointer;
}

.detail-button img,
.delete-button img {
  display: block;
  width: 16px;
  height: 16px;
}

.detail-button .detail-info-icon {
  width: 11.3px;
  height: 13.3px;
}

.detail-button:hover,
.delete-button:hover {
  color: #262626;
  background: #f5f5f5;
}

.detail-header h1 {
  min-width: 120px;
  flex: 1;
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-controls {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 4px;
}

.status-select {
  width: 92px;
}

.status-select :deep(.ant-select-selector) {
  height: 28px;
  border: 0;
  border-radius: 6px;
  padding: 0 20px 0 4px;
  box-shadow: none;
}

.status-select :deep(.ant-select-selection-item) {
  color: #8c8c8c;
  font-size: 14px;
  line-height: 28px;
}

.status-select :deep(.ant-select-arrow) {
  right: 4px;
  width: 16px;
  height: 16px;
  margin-top: -8px;
  align-items: center;
  justify-content: center;
}

.work-dir {
  display: flex;
  min-height: 28px;
  max-width: 320px;
  align-items: center;
  gap: 4px;
  padding: 3px 4px;
  border-radius: 6px;
  color: #8c8c8c;
  background: #f2f4f7;
  font-size: 14px;
  line-height: 22px;
}

.work-dir :deep(.anticon) {
  font-size: 16px;
}

.work-dir span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.start-button {
  display: inline-flex;
  height: 28px;
  align-items: center;
  border: 1px solid #3157e2;
  border-radius: 6px;
  padding: 2px 8px;
  border-color: #3157e2;
  color: #fff;
  background: #3157e2;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}

.start-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.delete-button {
  color: #595959;
}

.delete-button:hover {
  color: #ef4444;
  background: #fff1f2;
}

.detail-scroll {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow-y: auto;
}

.pipeline-card {
  flex: 0 0 auto;
  padding: 16px 24px 12px 24px;
  background: #fff;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.08);
  z-index: 1;
}

.pipeline-card-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 15px;
}

.pipeline-card-label {
  color: #8c8c8c;
  font-size: 14px;
}

.pipeline-card-sep {
  width: 1px;
  height: 12px;
  background: #f0f0f0;
}

.pipeline-card-name {
  color: #262626;
  font-size: 14px;
  font-weight: 400;
}

.pipeline-card-progress {
  color: #8c8c8c;
  font-size: 14px;
}

.detail-content {
  flex: 1 0 auto;
  padding: 0 24px 24px;
}

.task-title {
  margin: 28px 0 0;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.task-desc {
  margin-top: 12px;
}

.task-desc :deep(.task-desc-content) {
  display: -webkit-box;
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 14px;
  line-height: 28px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 5;
}

.task-desc.expanded :deep(.task-desc-content) {
  display: block;
  -webkit-line-clamp: unset;
}

.task-desc :deep(.task-desc-content p) {
  display: inline;
  margin: 0;
  color: inherit;
  font-size: inherit;
}

.task-desc :deep(.markdown-preview-shell) {
  min-height: 0;
  padding: 0;
  background: transparent;
}

.desc-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 8px;
  border: 0;
  padding: 0;
  color: #1d5ec9;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}

.conversation-scope-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 24px 0 12px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 19.2px;
}

.conversation-scope-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
}

.pipeline-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  padding: 80px 0;
  color: #8c8c8c;
}

.pipeline-empty-icon {
  width: 164px;
  height: 45px;
}

.pipeline-empty-icon img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.pipeline-empty p {
  margin: 16px 0 8px;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.pipeline-empty span {
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
}

.detail-loading,
.detail-empty {
  display: flex;
  min-height: 0;
  flex: 1;
  align-items: center;
  justify-content: center;
}

/* 限制详情图片尺寸，避免长图撑开页面；完整内容通过预览弹窗查看。 */
.task-desc :deep(.task-desc-content img) {
  max-width: 100px;
  max-height: 60px;
  object-fit: cover;
  cursor: pointer;
  border-radius: 3px;
  border: 1px solid #e5e7eb;
  margin: 0 2px;
  vertical-align: middle;
}

.task-desc :deep(.task-desc-content img):hover {
  border-color: #3157e2;
  box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.15);
}

@media (max-width: 760px) {
  .work-dir,
  .detail-button {
    display: none;
  }
}

@media (max-width: 720px) {
  .pipeline-card {
    padding: 12px 16px 8px;
  }

  .detail-content {
    padding: 0 16px 20px;
  }

  .task-title {
    margin-top: 20px;
    font-size: 24px;
    line-height: 32px;
  }
}
</style>
