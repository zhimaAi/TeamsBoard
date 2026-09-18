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
        :title="t('workflows.task.detail.back')"
        :aria-label="t('workflows.task.detail.back')"
        @click="returnToBoard"
      >
        <LeftOutlined />
      </button>
      <div class="detail-title">
        <h1 class="detail-title-text">{{ t('workflows.task.detail.title') }}</h1>
      </div>
      <div
        v-if="task"
        class="header-controls"
      >
        <a-select
          v-model:value="selectedTaskStatus"
          style="width: 120px"
          class="status-select"
          :disabled="changingStatus"
          :options="taskStatusOptions"
          :aria-label="t('workflows.task.detail.taskStatus')"
          @change="changeStatus"
        />
        <div
          class="work-dir"
          :title="task.work_dir || t('workflows.task.common.unsetDirectory')"
        >
          <FolderOpenOutlined /><span>{{
            task.work_dir || t('workflows.task.common.unsetDirectory')
          }}</span>
        </div>
        <button type="button" class="detail-button terminal-button" :disabled="openingTerminal || !(task.work_dir || task.task_dir)" :aria-label="t('workflows.task.progress.openTerminal')" :title="t('workflows.task.progress.openTerminal')" @click="openTaskTerminal">
          <CodeOutlined />
        </button>
        <a-tooltip v-if="showCodexActions">
          <template v-if="!codexCapability.available" #title>
            {{ codexCapability.message || t('workflows.task.codex.capabilityUnavailable') }}
          </template>
          <span
            class="codex-tooltip-trigger"
            :tabindex="codexCapability.available ? undefined : 0"
            :aria-label="
              codexCapability.available
                ? undefined
                : codexCapability.message || t('workflows.task.codex.capabilityUnavailable')
            "
          >
            <a-dropdown
              :disabled="codexBusy || !codexCapability.available"
              trigger="click"
            >
              <button
                type="button"
                class="codex-button"
                :disabled="codexBusy || !codexCapability.available"
                :aria-label="t('workflows.task.detail.codexActions')"
                aria-haspopup="menu"
              >
                <img
                  class="codex-button-logo"
                  :src="codexLogo"
                  alt=""
                  aria-hidden="true"
                />
                <span>{{ t('workflows.task.assign.codex') }}</span>
              </button>
              <template #overlay>
                <a-menu>
                  <a-menu-item key="open" @click="handleOpenCodex">
                    <DesktopOutlined /> {{ t('workflows.task.detail.openCodex') }}
                  </a-menu-item>
                  <a-menu-item key="copy" @click="handleCopyCodexPrompt">
                    <CopyOutlined /> {{ t('workflows.task.detail.copyCodexPrompt') }}
                  </a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
          </span>
        </a-tooltip>
        <button
          type="button"
          class="detail-button"
          :title="t('workflows.task.detail.viewDetails')"
          :aria-label="t('workflows.task.detail.viewDetails')"
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
          v-if="taskStatus === 'pending' && !isVibeCoding && !isCLI"
          type="button"
          class="start-button"
          :disabled="starting"
          @click="startTask"
        >
          {{ starting ? t('workflows.task.detail.starting') : t('workflows.task.detail.start') }}
        </button>
        <button
          type="button"
          class="delete-button"
          :disabled="deleting"
          :title="t('workflows.task.detail.delete')"
          :aria-label="t('workflows.task.detail.delete')"
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
      <div v-if="isVibeCoding" class="direct-execution">
        <img
          class="direct-execution-logo"
          :src="vibeCodingLogo"
          alt=""
          aria-hidden="true"
        />
        <span class="direct-execution-label">{{ t('workflows.task.common.directExecution') }}</span>
        <strong>{{ vibeCodingToolName }}</strong>
        <div class="direct-execution-actions">
          <button
            type="button"
            class="execution-action"
            @click="openExecutionModeModal(true)"
          >
            <img class="execution-switch-icon" :src="switchExecutionModeIcon" alt="" aria-hidden="true" />
            {{ t('workflows.task.detail.switchExecutionMode') }}
          </button>
        </div>
      </div>
      <div
        v-else-if="isCLI"
        class="direct-execution"
      >
        <img
          class="direct-execution-logo"
          :src="cliExecutionLogo"
          alt=""
          aria-hidden="true"
        />
        <span class="direct-execution-label">{{ t('workflows.task.common.directExecution') }}</span>
        <strong :title="cliRuntimeLabel">{{ cliRuntimeLabel }}</strong>
        <!-- CLI 直接执行不需要「执行流水线」入口：切到流水线走「切换执行方式」，
             那里的模式选择本来就包含流水线并带流水线列表 -->
        <div class="direct-execution-actions">
          <button
            type="button"
            class="execution-action"
            @click="openExecutionModeModal(true)"
          >
            <img class="execution-switch-icon" :src="switchExecutionModeIcon" alt="" aria-hidden="true" />
            {{ t('workflows.task.detail.switchExecutionMode') }}
          </button>
        </div>
      </div>
	  <div v-else-if="isExpertGroup" class="expert-summary">
		<img v-if="task.expert_group_avatar_snapshot" :src="task.expert_group_avatar_snapshot" alt="" />
		<span><small>{{ t('agents.expertTeam') }}</small><strong>{{ task.expert_group_name_snapshot }}</strong></span>
		<div class="expert-members"><span v-for="member in sortedSteps" :key="member.uuid" :title="member.name"><img v-if="member.avatar" :src="member.avatar" alt="" /><em v-else>{{ member.name.slice(0, 1) }}</em><b v-if="member.member_role === 'leader'">{{ t('expertGroups.leader') }}</b></span></div>
        <div class="direct-execution-actions">
          <button
            type="button"
            class="execution-action"
            @click="openExecutionModeModal(true)"
          >
            <img class="execution-switch-icon" :src="switchExecutionModeIcon" alt="" aria-hidden="true" />
            {{ t('workflows.task.detail.switchExecutionMode') }}
          </button>
        </div>
	  </div>
      <template v-else>
      <div class="pipeline-card-head">
        <PipelineFlowIcon class="pipeline-card-icon" />
        <span class="pipeline-card-label">{{ t('workflows.task.common.pipeline') }}</span>
        <span
          class="pipeline-card-sep"
          aria-hidden="true"
        ></span>
        <strong class="pipeline-card-name">{{
          task.pipeline_name_snapshot || t('workflows.task.common.unassignedPipeline')
        }}</strong>
        <button type="button" class="pipeline-toggle" :aria-expanded="pipelineExpanded" :title="pipelineExpanded ? t('workflows.task.detail.collapse') : t('workflows.task.detail.expand')" @click="pipelineExpanded = !pipelineExpanded">
          <DownOutlined :class="{ rotated: !pipelineExpanded }" />
        </button>
        <span
          v-if="sortedSteps.length"
          class="pipeline-card-progress"
          >{{
            t('workflows.task.common.stepCount', {
              current: currentStepIndex + 1,
              total: sortedSteps.length,
            })
          }}</span
        >
        <div v-if="task.execution_mode === 'pipeline'" class="direct-execution-actions">
          <button
            type="button"
            class="execution-action"
            @click="openExecutionModeModal(true)"
          >
            <img class="execution-switch-icon" :src="switchExecutionModeIcon" alt="" aria-hidden="true" />
            {{ t('workflows.task.detail.switchExecutionMode') }}
          </button>
        </div>
      </div>
      <AgentStepStrip
        v-if="hasPipelineSnapshot"
        v-show="pipelineExpanded"
        :steps="sortedSteps"
        :current-step-index="currentStepIndex"
        :current-step-uuid="effectiveCurrentStepUuid"
        :selected-step-uuid="selectedStepUuid"
        @select="selectStep"
      />
      <UnassignedPipelineGuide
        v-else
        v-show="pipelineExpanded"
        @assign="openExecutionModeModal(false)"
      />
      </template>
    </section>

    <div
      v-if="task"
      class="detail-scroll"
    >
      <div class="detail-content">
		<template v-if="hasPipelineSnapshot || isVibeCoding || isExpertGroup || isCLI">
          <NextStepButton
			v-if="!isVibeCoding && !isExpertGroup && !isCLI && showNextStepCard"
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
              {{
                descriptionExpanded
                  ? t('workflows.task.detail.collapse')
                  : t('workflows.task.detail.expand')
              }}
            </button>
          </div>
		  <p v-if="!isVibeCoding && !isExpertGroup && !isCLI" class="conversation-scope-note">
            <img
              class="conversation-scope-icon"
              :src="conversationFilterIcon"
              alt=""
              aria-hidden="true"
            />
            <span>{{
              t('workflows.task.detail.filteredActivity', {
                step: selectedStep?.name || t('workflows.task.common.currentStep'),
              })
            }}</span>
          </p>
          <!-- 需求设计图：CLI 直接执行没有多 Agent，动态区标题固定为「直接执行 · 单线程对话」 -->
          <button
            v-else-if="isCLI"
            type="button"
            class="conversation-scope-note conversation-scope-note--toggle"
            :aria-expanded="cliActivityExpanded"
            @click="cliActivityExpanded = !cliActivityExpanded"
          >
            <DownOutlined :class="{ rotated: !cliActivityExpanded }" />
            <span>{{ t('workflows.task.detail.directExecutionActivity') }}</span>
          </button>
          <p
            v-else
            class="conversation-scope-note conversation-scope-note--placeholder"
            aria-hidden="true"
          ></p>
          <StepMessageList
            ref="messageListRef"
            v-show="!isCLI || cliActivityExpanded"
            :items="displayedProgress"
			:steps="isVibeCoding || isCLI ? [] : sortedSteps"
            :highlight-uuid="highlightUuid"
            :loading="loading"
			:selected-step-name="isVibeCoding ? vibeCodingToolName : isCLI ? cliRuntimeLabel : isExpertGroup ? task.expert_group_name_snapshot || t('agents.expertTeam') : selectedStep?.name || ''"
			:fallback-actor-name="isVibeCoding ? vibeCodingToolName : isCLI ? task.execution_tool || t('workflows.task.assign.cliMode') : ''"
			:fallback-actor-logo="isVibeCoding ? vibeCodingActorLogo : isCLI ? cliExecutionLogo : ''"
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
          <p>{{ t('workflows.task.common.unassignedPipeline') }}</p>
          <span>{{ t('workflows.task.detail.unassignedDescription') }}</span>
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
      :description="t('workflows.task.detail.notFound')"
    />

    <VibeCodingConversationNotice
      v-if="task && isVibeCoding"
      :tool-name="vibeCodingToolName"
      :show-open-button="task.execution_tool === 'codex'"
      :opening="codexBusy"
      @open="handleOpenCodex"
    />

    <ChatComposer
      ref="composerRef"
      v-else-if="task && hasPipelineSnapshot"
      v-model="question"
      :can-ask="canAsk"
      :submitting="submitting"
      :running="Boolean(activeSelectedProgress)"
      :stopping="stoppingSessionUuid === activeSelectedProgress?.session_uuid"
      :cli-type="composerCliType"
      :model-name="composerModelName"
      :start-mode="selectedStepAwaitingStart"
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
	<ChatComposer
	  ref="expertComposerRef"
	  v-else-if="task && isExpertGroup"
	  v-model="question"
	  :can-ask="!activeExpertProgress"
	  :submitting="submitting"
	  :running="Boolean(activeExpertProgress)"
	  :stopping="stoppingSessionUuid === activeExpertProgress?.session_uuid"
	  :placeholder="activeExpertProgress ? t('workflows.task.feedback.agentRunning') : t('workflows.task.feedback.mentionPlaceholder')"
	  :context-text="t('agents.expertTeam')"
	  :task-uuid="resolvedTaskUuid"
	  :expert-members="sortedSteps"
	  :document-step="currentStep"
	  @submit="submitExpertMessage"
	  @stop="stopExpertConversation"
	/>
	<ChatComposer
	  v-else-if="task && isCLI"
	  ref="composerRef"
	  v-model="question"
	  :can-ask="cliCanAsk"
	  :submitting="submitting"
	  :running="Boolean(activeCLIProgress)"
	  :stopping="stoppingSessionUuid === activeCLIProgress?.session_uuid"
	  :placeholder="activeCLIProgress ? t('workflows.task.feedback.agentRunning') : t('workflows.task.detail.cliMessagePlaceholder')"
	  :context-text="cliRuntimeLabel"
	  :task-uuid="resolvedTaskUuid"
	  :document-step="currentStep"
	  :hide-agent-prompt="true"
	  @submit="submitCLIQuestion"
	  @stop="stopCLIConversation"
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

    <AssignExecutionModeModal
      v-model:open="assignModalOpen"
      :task-uuid="resolvedTaskUuid"
      :task-title="task?.title || ''"
	  :initial-mode="task?.execution_mode === 'pipeline' ? 'pipeline' : task?.execution_mode === 'expert_group' ? 'expert_group' : task?.execution_mode === 'cli' ? 'cli' : task?.execution_mode === 'vibe_coding' ? 'vibe_coding' : ''"
	  :preferred-expert-group-uuid="task?.selected_expert_group_uuid || ''"
      :preferred-pipeline-uuid="task?.selected_pipeline_uuid || ''"
      :initial-cli-type="task?.execution_mode === 'cli' ? task?.execution_tool || '' : ''"
	  :initial-model-name="task?.execution_mode === 'cli' ? task?.execution_model || '' : ''"
      :replace-existing="assignReplaceExisting"
      mode="detail"
      @assigned="handleAssigned"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  CopyOutlined,
  DesktopOutlined,
  DownOutlined,
  FolderOpenOutlined,
  LeftOutlined,
  UpOutlined,
  CodeOutlined,
} from '@ant-design/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import apiClient from '@/api/client'
import codexLogo from '@/assets/icons/codex-logo.svg'
import vibeCodingLogo from '@/assets/icons/vibe-coding-logo.svg'
import cliExecutionLogo from '@/assets/icons/task-composer-cli.svg'
import detailInfoIcon from '@/assets/icons/task-detail-view.svg'
import deleteTaskIcon from '@/assets/icons/task-detail-delete.svg'
import switchExecutionModeIcon from '@/assets/icons/task-switch-execution-mode.svg'
import conversationFilterIcon from '@/assets/icons/task-conversation-filter.svg'
import MarkdownIt from 'markdown-it'
import AssignExecutionModeModal from '@/components/AssignExecutionModeModal.vue'
import MarkdownPreview from '@/components/MarkdownPreview.vue'
import AgentStepStrip from '@/components/task-progress/AgentStepStrip.vue'
import ChatComposer from '@/components/task-progress/ChatComposer.vue'
import NextStepButton from '@/components/task-progress/NextStepButton.vue'
import PipelineFlowIcon from '@/components/task-progress/PipelineFlowIcon.vue'
import StepMessageList from '@/components/task-progress/StepMessageList.vue'
import StopExecutionConfirmModal from '@/components/task-progress/StopExecutionConfirmModal.vue'
import UnassignedPipelineGuide from '@/components/task-progress/UnassignedPipelineGuide.vue'
import VibeCodingConversationNotice from '@/components/task-progress/VibeCodingConversationNotice.vue'
import { isUserMessage, resultText, userMessageText } from '@/components/task-progress/utils'
import { copyText } from '@/utils/clipboard'
import { isStopConfirmSuppressed, suppressStopConfirm } from '@/utils/stopConfirm'
import { useDocumentTitle } from '@/composables/useDocumentTitle'
import { openTerminal } from '@/composables/useDesktop'
import { useLocalWS, useLocalWSStatus } from '@/composables/useLocalWebSocket'
import {
  assignTaskToCodex,
  copyTaskCodexPrompt,
  loadCodexCapability,
  openTaskInCodex,
  type CodexCapability,
} from '@/composables/useTaskCodex'
import type { TaskProgress } from '@/types/pipeline'
import type { ChatComposerSubmission } from '@/types/task-attachments'
import type { TaskWithDetails } from '@/types/task-detail'
import TaskDetailInfoModal from '@/views/workflows/components/TaskDetailInfoModal.vue'
import TaskImagePreviewModal from '@/views/workflows/components/TaskImagePreviewModal.vue'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

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
const codexBusy = ref(false)
const codexCapability = ref<CodexCapability>({ available: false })
const changingStatus = ref(false)
const selectedTaskStatus = ref('pending')
const detailModalOpen = ref(false)
const assignModalOpen = ref(false)
const assignReplaceExisting = ref(false)
const previewImageUrl = ref('')
const previewImageVisible = ref(false)
const descriptionExpanded = ref(false)
const pipelineExpanded = ref(true)
// 需求设计图：CLI 任务的动态区标题带折叠箭头。
const cliActivityExpanded = ref(true)
const descriptionNeedsExpand = ref(false)
const openingTerminal = ref(false)
const requirementContentRef = ref<HTMLElement>()
const taskTitle = computed(() => task.value?.title)
const taskStatus = computed(() => statusValue(task.value?.status))
const taskStatusOptions = computed(() => [
  { value: 'pending', label: t('workflows.task.status.pending') },
  { value: 'in_progress', label: t('workflows.task.status.inProgress') },
  { value: 'blocked', label: t('workflows.task.status.blocked') },
  { value: 'done', label: t('workflows.task.status.done') },
])
const hasPipelineSnapshot = computed(() =>
	Boolean(task.value?.execution_mode === 'pipeline' && (task.value?.pipeline_snapshot_uuid || task.value?.steps?.length)),
)
async function openTaskTerminal() {
  const workDir = task.value?.work_dir || task.value?.task_dir || ''
  if (!workDir || openingTerminal.value) return
  openingTerminal.value = true
  try { await openTerminal(workDir) } catch (error) { message.error(error instanceof Error ? error.message : t('workflows.task.progress.openTerminalFailed')) } finally { openingTerminal.value = false }
}
const isVibeCoding = computed(
  () => task.value?.execution_mode === 'vibe_coding',
)
const isCodexVibeCoding = computed(
  () => isVibeCoding.value && task.value?.execution_tool === 'codex',
)
const vibeCodingToolName = computed(() =>
  task.value?.execution_tool === 'codex'
    ? t('workflows.task.assign.codex')
    : task.value?.execution_tool || t('workflows.task.create.vibeCodingMode'),
)
const vibeCodingActorLogo = computed(() =>
  task.value?.execution_tool === 'codex' ? codexLogo : vibeCodingLogo,
)
const isExpertGroup = computed(() => task.value?.execution_mode === 'expert_group')
// CLI 直接执行：任务只有一条隐式步骤，CLI 与模型来自用户指派时的选择。
const isCLI = computed(() => task.value?.execution_mode === 'cli')
const cliRuntimeLabel = computed(() => {
  if (!isCLI.value) return ''
  const cli = task.value?.execution_tool || ''
  const model = task.value?.execution_model || sortedSteps.value[0]?.model_name || ''
  return model ? `${cli} · ${model}` : cli
})
const showCodexActions = computed(
  () => !task.value?.execution_mode || isCodexVibeCoding.value,
)
useDocumentTitle(taskTitle)

function openExecutionModeModal(replaceExisting: boolean) {
  assignReplaceExisting.value = replaceExisting
  assignModalOpen.value = true
}

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
const expertComposerRef = ref<InstanceType<typeof ChatComposer>>()
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
const displayedProgress = computed(() => isVibeCoding.value || isExpertGroup.value || isCLI.value ? progress.value : selectedProgress.value)
const activeExpertProgress = computed(() => [...progress.value].reverse().find((item) => !isUserMessage(item) && ['created', 'running'].includes(item.status)))
// CLI 直接执行与专家团一致：动态不按步骤过滤，活动会话取最近一条未结束的记录。
const activeCLIProgress = computed(() =>
  [...progress.value].reverse().find((item) => !isUserMessage(item) && ['created', 'running'].includes(item.status)),
)
const cliCanAsk = computed(() => !activeCLIProgress.value)
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
const selectedStepAwaitingStart = computed(
  () =>
    selectedIsCurrent.value &&
    !currentStepLocked.value &&
    selectedStep.value?.status === 'active' &&
    selectedProgress.value.length === 0,
)
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
  if (!selectedStepReached.value) return t('workflows.task.detail.stepUnavailable')
  if (hasRunningSelectedConversation.value) return t('workflows.task.detail.stepRunning')
  return t('workflows.task.detail.messagePlaceholder')
})
const composerContextText = computed(() =>
  t('workflows.task.detail.conversationContext', {
    step: selectedStep.value?.name || t('workflows.task.common.currentStep'),
    index: Math.max(selectedStepIndex.value + 1, 1),
  }),
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

async function ensureCodexAssigned() {
  if (!task.value || task.value.execution_mode) return
  await assignTaskToCodex(resolvedTaskUuid.value, false)
  task.value.execution_mode = 'vibe_coding'
  task.value.execution_tool = 'codex'
  emit('changed')
}

async function handleOpenCodex() {
  if (!task.value || codexBusy.value) return
  codexBusy.value = true
  try {
    await ensureCodexAssigned()
    const openResult = await openTaskInCodex(resolvedTaskUuid.value)
    if (openResult.opened) {
      message.success(t('workflows.task.detail.codexOpened'))
    } else if (openResult.copied) {
      message.warning(t('workflows.task.detail.codexCopiedFallback'))
    } else {
      message.warning(t('workflows.task.detail.codexUnavailableAfterAssign'))
    }
    await load(true)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.detail.codexOpenFailed'))
  } finally {
    codexBusy.value = false
  }
}

async function handleCopyCodexPrompt() {
  if (!task.value || codexBusy.value) return
  codexBusy.value = true
  try {
    await ensureCodexAssigned()
    await copyTaskCodexPrompt(resolvedTaskUuid.value)
    message.success(t('workflows.task.detail.codexPromptCopied'))
    await load(true)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.detail.codexCopyFailed'))
  } finally {
    codexBusy.value = false
  }
}

async function changeStatus(next: string) {
  if (!task.value || changingStatus.value) return
  const previous = statusValue(task.value.status)
  if (next === previous) return
  if (
    next === 'in_progress' &&
    (!task.value.execution_mode ||
      (task.value.execution_mode === 'pipeline' && !hasPipelineSnapshot.value))
  ) {
    selectedTaskStatus.value = previous
    openExecutionModeModal(false)
    return
  }
  // CLI 直接执行任务缺隐式步骤时同样回分配弹窗补全（正常不会发生，兜底防御）。
  if (next === 'in_progress' && task.value.execution_mode === 'cli' && !sortedSteps.value.length) {
    selectedTaskStatus.value = previous
    openExecutionModeModal(false)
    return
  }
  // 启动依赖完整的步骤配置；缺项时回到分配弹窗补全，避免提交不可执行快照。
  if (
    next === 'in_progress' &&
    task.value.execution_mode === 'pipeline' &&
    hasPipelineSnapshot.value &&
    task.value.steps?.some((s) => !s.prompt_snapshot || !s.cli_type || !s.model_name)
  ) {
    selectedTaskStatus.value = previous
    openExecutionModeModal(false)
    return
  }
  changingStatus.value = true
  try {
    await apiClient.put(`/tasks/${resolvedTaskUuid.value}/status`, { status: next })
    task.value.status = next
    if (next === 'in_progress' && previous === 'pending') {
      message.success(t('workflows.board.started'))
      void load()
    }
    emit('changed')
  } catch (error) {
    selectedTaskStatus.value = previous
    message.error(error instanceof Error ? error.message : t('workflows.board.statusUpdateFailed'))
  } finally {
    changingStatus.value = false
  }
}

async function startTask() {
  if (!task.value || starting.value) return
  if (!hasPipelineSnapshot.value && !isExpertGroup.value && !isCLI.value) {
    openExecutionModeModal(false)
    return
  }
  starting.value = true
  try {
    await apiClient.post(`/tasks/${resolvedTaskUuid.value}/start`, {})
    task.value.status = 'in_progress'
    message.success(t('workflows.task.detail.started'))
    void load()
    emit('changed')
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.detail.startFailed'))
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
    title: t('workflows.task.detail.deleteConfirm'),
    content: t('workflows.task.detail.deleteWarning'),
    okText: t('common.actions.delete'),
    okType: 'danger',
    cancelText: t('common.actions.cancel'),
    onOk: async () => {
      deleting.value = true
      try {
        await apiClient.delete(`/tasks/${resolvedTaskUuid.value}`)
        message.success(t('workflows.task.detail.deleted'))
        emit('changed')
        returnToBoard()
      } catch (error) {
        message.error(
          error instanceof Error ? error.message : t('workflows.task.detail.deleteFailed'),
        )
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
    const [loadedTask, loadedProgress, loadedCodexCapability] = await Promise.all([
      apiClient.get<TaskWithDetails>(`/tasks/${resolvedTaskUuid.value}`),
      apiClient.get<{ items: TaskProgress[] } | TaskProgress[]>(
        `/tasks/${resolvedTaskUuid.value}/progress`,
      ),
      loadCodexCapability().catch(() => ({
        available: false,
        message: t('workflows.task.codex.capabilityUnavailable'),
      })),
    ])
    task.value = loadedTask
    codexCapability.value = loadedCodexCapability
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
    if (!silent)
      message.error(
        error instanceof Error ? error.message : t('workflows.task.feedback.progressLoadFailed'),
      )
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
    if (!sessionUuid) message.warning(t('workflows.task.feedback.sessionNotFound'))
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
    message.success(t('workflows.task.feedback.stopped'))
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.stopFailed'))
  } finally {
    stoppingSessionUuid.value = ''
    closeStopConfirm()
  }
}

async function submitQuestion(submission: ChatComposerSubmission) {
  const text = submission.content.trim()
  const step = selectedStep.value
  if (!text || !step || !canAsk.value) return
  const startingStep = selectedStepAwaitingStart.value
  submitting.value = true
  try {
    const path = startingStep
      ? `/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/runs`
      : `/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/questions`
    await apiClient.post(path, {
      question: text,
      display_question: submission.display_content?.trim() || text,
      request_id: crypto.randomUUID(),
      cli_type: submission.config.cli_type,
      model_name: submission.config.model_name,
    })
    question.value = ''
    composerRef.value?.resetAfterSubmit()
    message.success(
      startingStep
        ? t('workflows.task.feedback.agentStarted')
        : t('workflows.task.feedback.messageSent'),
    )
    await load()
    await locateTarget()
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.messageFailed'),
    )
  } finally {
    submitting.value = false
  }
}

async function submitExpertMessage(submission: ChatComposerSubmission) {
  const text = submission.content.trim()
  if (!text || submitting.value || activeExpertProgress.value) return
  submitting.value = true
  try {
    await apiClient.post(`/tasks/${resolvedTaskUuid.value}/expert-messages`, {
      content: text,
      display_content: submission.display_content?.trim() || text,
      member_uuid: submission.member_uuid,
      request_id: crypto.randomUUID(),
    })
    question.value = ''
    expertComposerRef.value?.resetAfterSubmit()
    message.success(t('workflows.task.feedback.messageSent'))
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.messageFailed'))
  } finally {
    submitting.value = false
  }
}

function stopExpertConversation() {
  const sessionUUID = activeExpertProgress.value?.session_uuid
  if (sessionUUID) stopSelectedConversationFor(sessionUUID)
}

// CLI 直接执行：首条消息触发 initial_run，后续消息走 questions 续聊。
async function submitCLIQuestion(submission: ChatComposerSubmission) {
  const text = submission.content.trim()
  if (!text || submitting.value || activeCLIProgress.value) return
  const step = sortedSteps.value[0]
  if (!step) return
  const hasConversation = progress.value.some((item) => !isUserMessage(item))
  submitting.value = true
  try {
    const path = hasConversation
      ? `/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/questions`
      : `/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/runs`
    await apiClient.post(path, {
      question: text,
      display_question: submission.display_content?.trim() || text,
      request_id: crypto.randomUUID(),
    })
    question.value = ''
    composerRef.value?.resetAfterSubmit()
    message.success(t('workflows.task.feedback.messageSent'))
    await load()
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.messageFailed'),
    )
  } finally {
    submitting.value = false
  }
}

function stopCLIConversation() {
  const sessionUUID = activeCLIProgress.value?.session_uuid
  if (sessionUUID) stopSelectedConversationFor(sessionUUID)
}

function stopSelectedConversationFor(sessionUUID: string) {
  if (isStopConfirmSuppressed()) {
    void stopConversation(sessionUUID)
    return
  }
  stopConfirmSessionUuid.value = sessionUUID
  stopConfirmOpen.value = true
}

async function completeStep() {
  const step = currentStep.value
  if (!step || !canComplete.value) return
  completing.value = true
  try {
    await apiClient.post(`/tasks/${resolvedTaskUuid.value}/steps/${step.uuid}/complete`, {
      auto_start: false,
    })
    const wasLastStep = sortedSteps.value.at(-1)?.uuid === step.uuid
    if (wasLastStep) {
      message.success(t('workflows.task.feedback.taskCompleted'))
    } else {
      message.success(t('workflows.task.feedback.nextStepManual'))
    }
    await load()
    const nextCurrent = task.value?.current_step_uuid
    if (nextCurrent && nextCurrent !== selectedStepUuid.value) {
      selectedStepUuid.value = nextCurrent
      await locateTarget()
    }
    if (wasLastStep) emit('changed')
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.nextStepFailed'),
    )
  } finally {
    completing.value = false
  }
}

async function copyResult(item: TaskProgress) {
  try {
    await copyText(isUserMessage(item) ? userMessageText(item) : resultText(item))
    message.success(t('components.feedback.copied'))
  } catch {
    message.warning(t('components.feedback.copyFailed'))
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

.codex-button {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 6px;
  border: 0;
  border-radius: 6px;
  padding: 3px 8px;
  color: #8c8c8c;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}

.codex-button-logo {
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
}

.codex-tooltip-trigger {
  display: inline-flex;
}

.codex-button:hover:not(:disabled) {
  background: #f2f4f7;
}

.codex-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.codex-button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
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

.direct-execution {
  display: flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
}

.direct-execution-logo {
  width: 14px;
  height: 14px;
  flex: 0 0 auto;
}

/* 需求设计图用「直接执行：」的冒号分隔，不再使用竖线 */
.direct-execution-label::after {
  content: ':';
}

/* 右侧「切换执行方式」描边按钮 */
.direct-execution-actions {
  display: flex;
  margin-left: auto;
  align-items: center;
  gap: 8px;
}

.pipeline-card-head > .direct-execution-actions {
  margin-left: 8px;
}

.execution-action {
  display: inline-flex;
  height: 28px;
  align-items: center;
  gap: 6px;
  border: 1px solid #e5e5e5;
  padding: 0 12px;
  border-radius: 8px;
  color: #262626;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
  line-height: 20px;
}

.execution-action:hover {
  border-color: #3157e2;
  color: #3157e2;
}

.execution-switch-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
}

.direct-execution strong {
  color: #262626;
  font-weight: 500;
}

.expert-summary { display:flex; align-items:center; gap:12px; }
.expert-summary>img { width:36px; height:36px; border-radius:50%; object-fit:cover; }
.expert-summary>span { display:flex; flex-direction:column; }
.expert-summary small { color:#8c8c8c; }
.expert-summary>.expert-members { display:flex; margin-left:auto; }
.expert-summary>.expert-members>span { position:relative; margin-left:-5px; }
.expert-summary>.expert-members img,.expert-summary>.expert-members em { display:flex; width:30px; height:30px; align-items:center; justify-content:center; border:2px solid #fff; border-radius:50%; background:#e5efff; object-fit:cover; font-style:normal; }
.expert-summary>.expert-members b { position:absolute; right:-4px; bottom:-8px; padding:0 4px; border-radius:8px; color:#d97706; background:#fff5e5; font-size:9px; white-space:nowrap; }
.expert-summary>.direct-execution-actions { margin-left:16px; }

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
.pipeline-toggle { margin-left: auto; border: 0; padding: 4px; color: #8c8c8c; background: transparent; cursor: pointer; }
.pipeline-toggle .rotated { transform: rotate(-90deg); }

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

/* 需求设计图：CLI 任务的动态区标题可折叠 */
.conversation-scope-note--toggle {
  border: 0;
  padding: 0;
  background: transparent;
  cursor: pointer;
}

.conversation-scope-note--toggle:hover {
  color: #262626;
}

.conversation-scope-note--toggle .rotated {
  transform: rotate(-90deg);
}

.pipeline-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  padding: 80px 0;
  color: #8c8c8c;
}

.conversation-scope-note--placeholder {
  min-height: 0;
  margin: 24px 0 0;
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
