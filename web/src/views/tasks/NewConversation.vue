<template>
  <div class="new-conversation-page">
    <div class="new-conversation-center">
      <div class="title-area">
        <h1 class="main-title">{{ t('workflows.task.newConversation.title') || '开始一个新任务' }}</h1>
        <p class="sub-title">
          {{ t('workflows.task.newConversation.subtitle') || '描述你的目标，选择工作目录和执行方式。' }}
        </p>
      </div>

      <div class="composer-stack">
        <OnboardingGuide
          v-if="guideVisible"
          :phase="onboardingPhase"
          :has-prompt="hasPrompt"
          @skip="dismissOnboarding"
          @start="startOnboarding"
          @fill-example="fillOnboardingExample"
        />

        <div
          class="task-composer-card"
          :class="{ 'is-onboarding-input': onboardingTarget === 'input' }"
          @paste="handlePaste"
          @dragover.prevent
          @drop.prevent="handleDrop"
        >
        <!-- 图片附件预览 -->
        <div
          v-if="attachments.length"
          class="attachments-preview-row"
        >
          <div
            v-for="attachment in attachments"
            :key="attachment.id"
            class="attachment-thumb"
          >
            <img
              :src="attachmentPreviews[attachment.id]?.url || ''"
              :alt="attachment.name"
              class="thumb-img"
              @click="previewImage(attachmentPreviews[attachment.id]?.url || '')"
            />
            <button
              type="button"
              class="remove-att-btn"
              :aria-label="t('workflows.task.progress.removeAttachment', { name: attachment.name })"
              @click.stop="removeAttachment(attachment.id)"
            >
              <CloseOutlined />
            </button>
          </div>
        </div>

        <!-- 文本输入区 -->
        <textarea
          ref="textareaRef"
          v-model="question"
          class="composer-textarea"
          :placeholder="t('workflows.task.newConversation.placeholder') || '描述你希望完成的任务⋯'"
          rows="4"
          @keydown="handleKeydown"
        />

        <!-- 隐藏的图片上传 input -->
        <input
          ref="fileInputRef"
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif,.png,.jpg,.jpeg,.webp,.gif"
          multiple
          class="hidden-file-input"
          @change="handleFileInputChange"
        />

        <!-- 底部工具栏 -->
        <div class="composer-toolbar">
          <div class="toolbar-left">
            <!-- 添加附件按钮 -->
            <button
              type="button"
              class="action-btn icon-only"
              :title="t('workflows.task.attachments.uploadImage') || '上传图片'"
              @click="triggerFileInput"
            >
              <PlusOutlined />
            </button>

            <!-- 选择执行方式：与看板、任务详情共用指定执行方式弹窗 -->
            <button
              type="button"
              class="action-btn"
              :class="{
                'is-active': !!executionMode,
                'is-onboarding-target': onboardingTarget === 'execution',
                'is-onboarding-pulse': onboardingTarget === 'execution',
              }"
              @click="executionPickerOpen = true"
            >
              <BranchesOutlined />
              <span class="btn-text">
                {{ executionMode?.displayName || t('workflows.task.newConversation.chooseExecutionMode') || '选择执行方式' }}
              </span>
            </button>

            <!-- 选择工作目录 -->
            <WorkDirectoryPicker
              v-model="workDirectory"
              @select="onWorkDirSelect"
            >
              <template #default="{ selectedDir }">
                <button
                  type="button"
                  class="action-btn"
                  :class="{
                    'is-active': !!selectedDir,
                    'is-onboarding-target': onboardingTarget === 'directory',
                  }"
                >
                  <FolderOutlined />
                  <span class="btn-text">
                    {{ displayWorkDir || t('workflows.task.newConversation.chooseDirectory') || '选择工作目录' }}
                  </span>
                </button>
              </template>
            </WorkDirectoryPicker>
          </div>

          <div class="toolbar-right">
            <!-- 发送按钮 -->
            <button
              type="button"
              class="send-btn"
              :class="{ 'is-onboarding-pulse': onboardingTarget === 'send' }"
              :disabled="!canSubmit"
              :title="t('workflows.task.newConversation.send') || '发送'"
              @click="handleSubmit"
            >
              <ArrowUpOutlined v-if="!submitting" />
              <LoadingOutlined v-else />
            </button>
          </div>
        </div>
        </div>
      </div>
    </div>

    <AssignExecutionModeModal
      v-model:open="executionPickerOpen"
      select-only
      intent="create"
      :initial-mode="executionMode?.mode || ''"
      :preferred-pipeline-uuid="executionMode?.pipelineUuid || ''"
      :preferred-expert-group-uuid="executionMode?.expertGroupUuid || ''"
      :initial-cli-type="executionMode?.mode === 'cli' ? executionMode.executionTool || '' : ''"
      :initial-model-name="executionMode?.mode === 'cli' ? executionMode.modelName || '' : ''"
      :initial-vibe-tool="executionMode?.mode === 'vibe_coding' ? executionMode.executionTool || '' : ''"
      @selected="onExecutionModeSelected"
    />

    <!-- 图片大图预览 -->
    <a-modal
      v-model:open="previewVisible"
      :footer="null"
      centered
      width="auto"
    >
      <img
        :src="previewImageUrl"
        alt="Preview"
        class="modal-preview-img"
      />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  PlusOutlined,
  BranchesOutlined,
  FolderOutlined,
  ArrowUpOutlined,
  CloseOutlined,
  LoadingOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import WorkDirectoryPicker from '@/components/WorkDirectoryPicker.vue'
import AssignExecutionModeModal, {
  type ExecutionModeSelection as AssignedExecutionMode,
} from '@/components/AssignExecutionModeModal.vue'
import type { ExecutionModeSelection } from '@/components/ExecutionModePicker.vue'
import OnboardingGuide from '@/views/tasks/components/OnboardingGuide.vue'
import { resolveOnboardingTarget, useOnboardingGuide } from '@/composables/useOnboardingGuide'
import { useConversationStore, getLastDirSegment } from '@/stores/conversation'
import { useCreateTaskDefaults } from '@/composables/useCreateTaskDefaults'
import { useTaskImageAttachments } from '@/composables/useTaskImageAttachments'
import { openTaskInCodex } from '@/composables/useTaskCodex'
import { pendingCliKickoffUuid } from '@/composables/useCliKickoff'
import { usePipelineStore } from '@/stores/pipeline'
import { useExpertGroupStore } from '@/stores/expert-group'
import { useCliNames } from '@/composables/useCliNames'
import { vibeToolLabelKey } from '@/composables/useVibeCoding'
import { useAppI18n } from '@/i18n'

interface AttachmentPreview {
  file: File
  url: string
}

const LAST_EXEC_MODE_KEY = 'goteams.newConversation.lastExecutionMode'
const LAST_WORK_DIR_KEY = 'goteams.newConversation.lastWorkDir'

const route = useRoute()
const router = useRouter()
const { t } = useAppI18n()
const {
  status: onboardingStatus,
  phase: onboardingPhase,
  start: beginOnboardingSteps,
  reconcile: reconcileOnboarding,
  skip: dismissOnboarding,
  complete: completeOnboarding,
} = useOnboardingGuide()
const conversationStore = useConversationStore()
const pipelineStore = usePipelineStore()
const expertGroupStore = useExpertGroupStore()
const { cliDisplayName, ensureLoaded: ensureCliNamesLoaded } = useCliNames()
const executionPickerOpen = ref(false)
const { getCreateTaskDefaults, saveCreateTaskDefaults } = useCreateTaskDefaults()
const {
  items: attachments,
  addImages,
  buildDraftAttachmentInputs,
  clear: clearAttachments,
  remove: removeAttachmentItem,
} = useTaskImageAttachments()

const question = ref('')
const workDirectory = ref('')
const executionMode = ref<ExecutionModeSelection | null>(null)
const submitting = ref(false)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const attachmentPreviews = ref<Record<string, AttachmentPreview>>({})
const previewVisible = ref(false)
const previewImageUrl = ref('')

const displayWorkDir = computed(() => {
  if (!workDirectory.value) return ''
  const info = conversationStore.resolveWorkDirInfo(workDirectory.value)
  return info.projectName || getLastDirSegment(workDirectory.value) || workDirectory.value
})

const canSubmit = computed(() => {
  return (
    !submitting.value &&
    question.value.trim().length > 0 &&
    workDirectory.value.trim().length > 0 &&
    executionMode.value !== null
  )
})

const hasPrompt = computed(() => question.value.trim().length > 0)
const guideVisible = computed(() => onboardingStatus.value === 'active')
const onboardingTarget = computed(() =>
  resolveOnboardingTarget(onboardingStatus.value, onboardingPhase.value, hasPrompt.value),
)

function onboardingFacts() {
  return {
    hasExecutionMode: executionMode.value !== null,
    hasWorkDirectory: workDirectory.value.trim().length > 0,
  }
}

function startOnboarding() {
  beginOnboardingSteps(onboardingFacts())
}

function fillOnboardingExample() {
  question.value = t('workflows.task.newConversation.onboarding.examplePrompt')
  nextTick(() => {
    const input = textareaRef.value
    if (!input) return
    input.focus()
    const end = input.value.length
    input.setSelectionRange(end, end)
  })
}

watch([executionMode, workDirectory], () => {
  reconcileOnboarding(onboardingFacts())
})

watch(onboardingTarget, (target) => {
  if (target !== 'input') return
  nextTick(() => textareaRef.value?.focus())
})

function restorePreferences() {
  const queryDir = (route.query.workDir as string | undefined)?.trim()
  if (queryDir) {
    workDirectory.value = queryDir
  } else {
    try {
      const savedDir = localStorage.getItem(LAST_WORK_DIR_KEY)
      if (savedDir) {
        workDirectory.value = savedDir
      } else {
        const defaults = getCreateTaskDefaults()
        if (defaults.work_dir) workDirectory.value = defaults.work_dir
      }
    } catch {
      // ignore
    }
  }

  try {
    const rawMode = localStorage.getItem(LAST_EXEC_MODE_KEY)
    if (rawMode) {
      executionMode.value = JSON.parse(rawMode) as ExecutionModeSelection
    }
  } catch {
    // ignore
  }
}

watch(
  () => route.query.workDir,
  (newDir) => {
    if (typeof newDir === 'string' && newDir.trim()) {
      workDirectory.value = newDir.trim()
    }
  },
)

function onWorkDirSelect(dir: string) {
  workDirectory.value = dir
  try {
    localStorage.setItem(LAST_WORK_DIR_KEY, dir)
  } catch {
    // ignore
  }
  const defaults = getCreateTaskDefaults()
  saveCreateTaskDefaults({ ...defaults, work_dir: dir })
}

function selectionDisplayName(selection: AssignedExecutionMode): string {
  if (selection.mode === 'pipeline') {
    const name = pipelineStore.pipelines.find((item) => item.uuid === selection.pipelineUuid)?.name
    return name
      ? `${t('workflows.task.common.pipeline')} · ${name}`
      : t('workflows.task.common.pipeline')
  }
  if (selection.mode === 'expert_group') {
    const name = expertGroupStore.items.find((item) => item.uuid === selection.expertGroupUuid)?.name
    return name ? `${t('agents.expertTeam')} · ${name}` : t('agents.expertTeam')
  }
  if (selection.mode === 'vibe_coding') {
    return `${t('workflows.task.create.vibeCodingMode')} · ${t(vibeToolLabelKey(selection.tool))}`
  }
  const cliName = cliDisplayName(selection.tool)
  return selection.modelName
    ? `${t('workflows.task.common.directExecution')} · ${cliName} (${selection.modelName})`
    : `${t('workflows.task.common.directExecution')} · ${cliName}`
}

async function onExecutionModeSelected(selection: AssignedExecutionMode) {
  if (selection.mode === 'cli') await ensureCliNamesLoaded()
  const mode: ExecutionModeSelection = {
    mode: selection.mode,
    pipelineUuid: selection.pipelineUuid || undefined,
    expertGroupUuid: selection.expertGroupUuid || undefined,
    executionTool: selection.tool || undefined,
    modelName: selection.modelName || undefined,
    displayName: selectionDisplayName(selection),
  }
  executionMode.value = mode
  try {
    localStorage.setItem(LAST_EXEC_MODE_KEY, JSON.stringify(mode))
  } catch {
    // ignore
  }
}

function triggerFileInput() {
  fileInputRef.value?.click()
}

function attachImages(files: File[]) {
  if (!files.length) return
  try {
    const added = addImages(files)
    const next = { ...attachmentPreviews.value }
    added.forEach((attachment, index) => {
      const file = files[index]
      next[attachment.marker] = { file, url: URL.createObjectURL(file) }
    })
    attachmentPreviews.value = next
  } catch (err) {
    message.error(err instanceof Error ? err.message : t('workflows.task.attachments.uploadFailed'))
  }
}

function handleFileInputChange(e: Event) {
  const target = e.target as HTMLInputElement
  attachImages([...(target.files || [])])
  target.value = ''
}

function handlePaste(e: ClipboardEvent) {
  const files = [...(e.clipboardData?.items || [])]
    .filter((item) => item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (!files.length) return
  e.preventDefault()
  attachImages(files)
}

function handleDrop(e: DragEvent) {
  attachImages([...(e.dataTransfer?.files || [])])
}

function removeAttachment(id: string) {
  const preview = attachmentPreviews.value[id]
  if (preview) URL.revokeObjectURL(preview.url)
  const next = { ...attachmentPreviews.value }
  delete next[id]
  attachmentPreviews.value = next
  removeAttachmentItem(id)
}

function previewImage(url: string) {
  previewImageUrl.value = url
  previewVisible.value = true
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    if (canSubmit.value) {
      void handleSubmit()
    }
  }
}

async function handleSubmit() {
  if (!canSubmit.value) {
    if (!workDirectory.value.trim()) {
      message.warning(t('workflows.task.feedback.chooseDirectory') || '请先选择工作目录')
      return
    }
    if (!executionMode.value) {
      message.warning(t('workflows.task.newConversation.chooseExecutionMode') || '请选择执行方式')
      return
    }
    return
  }

  const promptText = question.value.trim()
  const workDir = workDirectory.value.trim()
  const mode = executionMode.value
  if (!mode) return

  submitting.value = true
  try {
    conversationStore.recordWorkingDir(workDir)
    try {
      localStorage.setItem(LAST_WORK_DIR_KEY, workDir)
      localStorage.setItem(LAST_EXEC_MODE_KEY, JSON.stringify(mode))
    } catch {
      // ignore
    }

    const attachmentInputs = await buildDraftAttachmentInputs()
    const taskDescription = [promptText, ...attachmentInputs.map((item) => item.marker)]
      .filter(Boolean)
      .join('\n\n')
    const payload: Record<string, unknown> = {
      title: promptText.slice(0, 50).replace(/[\r\n]+/g, ' '),
      description: taskDescription,
      work_dir: workDir,
      work_dirs: [workDir],
      status: 'active',
      create_notification: true,
      attachments: attachmentInputs,
    }

    if (mode.mode === 'pipeline') {
      await pipelineStore.loadPipelines()
      const pipeline = pipelineStore.pipelines.find((item) => item.uuid === mode.pipelineUuid)
      if (!pipeline) throw new Error(t('workflows.task.common.noPipeline'))
      payload.execution_mode = 'pipeline'
      payload.pipeline_uuid = pipeline.uuid
      payload.step_configs = (pipeline.steps || []).map((step) => ({
        step_uuid: step.uuid,
        source_step_id: step.source_step_id == null ? '' : String(step.source_step_id),
        cloud_step_id: step.cloud_step_id == null ? '' : String(step.cloud_step_id),
        cli_type: step.cli_type || '',
        model: step.model || '',
        model_name: step.model_name || '',
      }))
    } else if (mode.mode === 'expert_group') {
      payload.execution_mode = 'expert_group'
      payload.expert_group_uuid = mode.expertGroupUuid
    } else if (mode.mode === 'vibe_coding') {
      payload.execution_mode = 'vibe_coding'
      payload.execution_tool = mode.executionTool || 'codex'
    } else if (mode.mode === 'cli') {
      payload.execution_mode = 'cli'
      payload.execution_tool = mode.executionTool
      payload.model_name = mode.modelName
    }

    const created = await apiClient.post<{ uuid: string; title: string }>('/tasks', payload)
    completeOnboarding()
    message.success(t('workflows.task.feedback.taskCreated') || '任务创建成功')

    if (mode.mode === 'vibe_coding') {
      try {
        const openResult = await openTaskInCodex(created.uuid)
        if (openResult.opened) {
          message.success(t('workflows.task.detail.toolOpened', { tool: mode.displayName }))
        } else if (openResult.copied) {
          message.warning(t('workflows.task.detail.toolCopiedFallback', { tool: mode.displayName }))
        } else if (openResult.error) {
          message.error(openResult.error.message)
        } else {
          message.warning(t('workflows.task.detail.toolUnavailable', { tool: mode.displayName }))
        }
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('workflows.task.detail.toolUnavailable', { tool: mode.displayName }))
      }
    }

    if (mode.mode === 'cli') {
      pendingCliKickoffUuid.value = created.uuid
    }

    await conversationStore.loadConversations(true)
    void router.push({ name: 'tasks', query: { taskUuid: created.uuid } })
  } catch (err) {
    message.error(err instanceof Error ? err.message : t('workflows.task.feedback.taskCreateFailed') || '任务创建失败')
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  void ensureCliNamesLoaded()
  restorePreferences()
  nextTick(() => {
    const target = onboardingTarget.value
    if (target === 'execution' || target === 'directory') return
    textareaRef.value?.focus()
  })
})

onUnmounted(() => {
  Object.values(attachmentPreviews.value).forEach(({ url }) => URL.revokeObjectURL(url))
  clearAttachments()
})
</script>

<style scoped>
.new-conversation-page {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: safe center;
  min-height: 100%;
  height: 100%;
  padding: 40px 24px;
  background: #ffffff;
  overflow-y: auto;
}

.new-conversation-center {
  width: 100%;
  max-width: 800px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 32px;
}

.composer-stack {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.title-area {
  text-align: center;
}

.main-title {
  font-size: 32px;
  font-weight: 600;
  color: #17181a;
  line-height: 40px;
  letter-spacing: -1.12px;
  margin: 0;
}

.sub-title {
  font-size: 14px;
  color: #8c8c8c;
  line-height: 22px;
  margin: 8px 0 0;
}

.task-composer-card {
  width: 100%;
  background: #ffffff;
  border: 2px solid #e5e7eb;
  border-radius: 12px;
  box-shadow: 0 4px 8px rgba(0, 0, 0, 0.08);
  padding: 12px 14px 10px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: border-color 0.2s;
}

.task-composer-card:focus-within {
  border-color: #3157e2;
}

.composer-textarea {
  width: 100%;
  min-height: 100px;
  border: none;
  outline: none;
  resize: vertical;
  font-size: 14px;
  line-height: 22px;
  color: #262626;
  font-family: inherit;
  background: transparent;
}

.composer-textarea::placeholder {
  color: #8c8c8c;
}

.attachments-preview-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding-bottom: 6px;
}

.attachment-thumb {
  position: relative;
  width: 60px;
  height: 60px;
  border-radius: 6px;
  border: 1px solid #d9d9d9;
  overflow: hidden;
}

.thumb-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  cursor: pointer;
}

.remove-att-btn {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.6);
  color: #ffffff;
  border: none;
  font-size: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.hidden-file-input {
  display: none;
}

.composer-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 6px;
  border-top: 1px solid #f0f0f0;
}

.toolbar-left {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #f9f9f9;
  color: #595959;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s;
}

.action-btn:hover {
  background: #f0f0f0;
  border-color: #d1d5db;
  color: #262626;
}

.action-btn.is-active {
  background: #f0f7ff;
  border-color: #bfdbfe;
  color: #2563eb;
}

.action-btn.is-onboarding-target {
  background: #e5efff;
  border-color: #3157e2;
  color: #3157e2;
}

.task-composer-card.is-onboarding-input {
  border-color: #3157e2;
}

.task-composer-card.is-onboarding-input .composer-textarea {
  background: #f5f9ff;
  border-radius: 8px;
}

.action-btn.icon-only {
  width: 28px;
  padding: 0;
  justify-content: center;
}

.btn-text {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.toolbar-right {
  display: flex;
  align-items: center;
}

.send-btn {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: #262626;
  color: #ffffff;
  border: none;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  cursor: pointer;
  transition: background 0.2s, opacity 0.2s;
}

.send-btn:hover:not(:disabled) {
  background: #17181a;
}

.send-btn:disabled {
  background: #d9d9d9;
  color: #8c8c8c;
  cursor: not-allowed;
}

/* 设计稿要求当前目标以 1s 缓入缓出闪烁；减少动效时只保留静态焦点环。 */
@keyframes onboarding-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(49, 87, 226, 0.45);
  }
  50% {
    box-shadow: 0 0 0 5px rgba(49, 87, 226, 0);
  }
}

.is-onboarding-pulse {
  animation: onboarding-pulse 1s ease-in-out infinite;
}

@media (prefers-reduced-motion: reduce) {
  .is-onboarding-pulse {
    animation: none;
    box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.35);
  }
}

.modal-preview-img {
  max-width: 80vw;
  max-height: 80vh;
  object-fit: contain;
}
</style>
