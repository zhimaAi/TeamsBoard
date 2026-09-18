<template>
  <div class="chat-composer">
	<div v-if="expertMentionOpen" class="expert-mention-menu" role="listbox" :aria-label="t('agents.expertTeam')">
	  <button v-for="(member, index) in filteredExpertMembers" :key="member.uuid" type="button" role="option" :class="{ active: index === expertMentionActiveIndex }" :aria-selected="index === expertMentionActiveIndex" @mouseenter="expertMentionActiveIndex = index" @click="selectExpertMember(member)">
		<img :src="member.avatar || agentIcon" alt="" /><span><strong>{{ member.name }}</strong><small>{{ member.member_role === 'leader' ? t('expertGroups.leader') : t('expertGroups.members', { count: 1 }) }}</small></span>
	  </button>
	</div>
    <DocumentMentionMenu
      ref="documentMentionMenuRef"
      :open="documentMentionOpen"
      :query="documentMentionQuery"
      :items="documentMentionMatches"
      :status="documentMentionStatus"
      :error="documentMentionError"
      @close="dismissDocumentMention"
      @select="insertDocumentReference"
      @retry="reloadDocumentMention"
    />
    <div
      class="composer-shell"
      :class="{ 'is-disabled': !canAsk && !running }"
    >
      <!--
        附件行只服务附件：# 引用在输入框里有自己的内联胶囊，不再重复出现在这里。
        粘贴和「附件」按钮走的都是同一份附件列表，落点一致。
      -->
      <div
        v-if="attachmentItems.length"
        class="attachment-chips"
        :aria-label="t('workflows.task.progress.attachedFiles')"
      >
        <button
          v-for="attachment in attachmentItems"
          :key="attachment.id"
          type="button"
          class="attachment-chip"
          :title="t('workflows.task.progress.removeAttachment', { name: attachment.name })"
          @click="removeAttachment(attachment.id)"
        >
          <span class="attachment-chip-icon-slot">
            <PictureOutlined
              v-if="attachment.isImage"
              class="attachment-chip-icon"
            />
            <PaperClipOutlined
              v-else
              class="attachment-chip-icon"
            />
            <CloseOutlined class="attachment-chip-close" aria-hidden="true" />
          </span>
          <span class="attachment-chip-name">{{ attachment.name }}</span>
        </button>
      </div>
      <div
        class="composer-input"
        :class="{ 'is-disabled': !canAsk, 'is-composing': isComposing }"
      >
        <div
          ref="inputLayerRef"
          class="composer-input-layer"
          aria-hidden="true"
        >
          <span
            v-for="(token, index) in mentionTokens"
            :key="index"
            :class="{ 'mention-token': token.reference }"
          >
            <template v-if="token.reference">
              <svg
                class="mention-token-icon"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linejoin="round"
                stroke-linecap="round"
                aria-hidden="true"
              >
                <path d="M9.4 1.9H4.5a1.1 1.1 0 0 0-1.1 1.1v10a1.1 1.1 0 0 0 1.1 1.1h7a1.1 1.1 0 0 0 1.1-1.1V5z" />
                <path d="M9.4 1.9V5h3.2" />
              </svg>
              <span class="mention-token-syntax">{{ token.syntaxPrefix }}</span>
              <span class="mention-token-label">{{ token.label }}</span>
              <span class="mention-token-syntax">{{ token.syntaxSuffix }}</span>
            </template>
            <template v-else>{{ token.text }}</template>
          </span>
        </div>
        <textarea
          ref="textareaRef"
          :value="modelValue"
          rows="3"
          :disabled="!canAsk"
          :placeholder="effectivePlaceholder"
          :aria-label="t('workflows.task.progress.messageAria')"
          @input="handleInput"
          @click="handleComposerClick"
          @paste="handlePaste"
          @keydown="handleComposerKeydown"
          @scroll="syncInputLayer"
          @compositionstart="isComposing = true"
          @compositionend="handleCompositionEnd"
        />
      </div>
      <div class="composer-toolbar">
        <div class="composer-tools">
          <input
            ref="fileInputRef"
            class="attachment-file-input"
            type="file"
            multiple
            tabindex="-1"
            aria-hidden="true"
            @change="handleFileSelection"
          />
          <button
            type="button"
            class="tool-button"
            :disabled="fullyDisabled || !taskUuid || !canAsk || isSavingPastedImages"
            :title="t('workflows.task.progress.uploadFile')"
            :aria-label="t('workflows.task.progress.uploadFile')"
            @click="fileInputRef?.click()"
          >
            <PaperClipOutlined class="tool-button-icon" />
            <span>{{ t('workflows.task.progress.attachment') }}</span>
          </button>
          <button
            v-if="showWorkDirectory"
            type="button"
            class="tool-button work-directory-button"
            :class="{ selected: workDirectory }"
            :title="workDirectory || t('workflows.task.progress.chooseWorkDirectory')"
            :aria-label="t('workflows.task.progress.chooseWorkDirectory')"
            :disabled="fullyDisabled"
            @click="emit('select-work-directory')"
          >
            <FolderOpenOutlined class="tool-button-icon" />
            <span>{{ workDirectory || t('workflows.task.progress.chooseWorkDirectory') }}</span>
          </button>
          <GitBranchPicker
            v-if="taskUuid"
            :task-uuid="taskUuid"
            :disabled="running || submitting || preparingSubmission"
            @busy="branchChanging = $event"
          />
          <CliRuntimePicker
            v-if="runtimeEnabled && !fullyDisabled"
            v-model:open="cliPickerOpen"
            :cli-options="cliOptions"
            :cli-loading="cliLoading"
            :selected-cli-type="selectedCliType"
            :selected-model-name="selectedModelName"
            :model-options="availableModels"
            :model-loading="modelLoading"
            :model-counts="modelCounts"
            @select-cli="handleCliChange"
            @select-model="handleModelChange"
          >
            <button
              type="button"
              class="tool-button cli-picker-trigger"
              :title="runtimeTriggerLabel"
              aria-haspopup="dialog"
              :aria-expanded="cliPickerOpen"
              :aria-label="t('workflows.task.progress.chooseRuntime')"
            >
              <img
                class="tool-button-icon"
                :src="cliIcon"
                alt=""
                aria-hidden="true"
              />
              <span>{{ runtimeTriggerLabel }}</span>
            </button>
          </CliRuntimePicker>
          <span
            v-if="runtimeEnabled && !fullyDisabled"
            class="tool-divider"
            aria-hidden="true"
          />
          <button
            v-if="!hideAgentPrompt"
            type="button"
            class="tool-button"
            :disabled="fullyDisabled || !taskUuid"
            @click="agentPromptOpen = true"
          >
            <img
              class="tool-button-icon"
              :src="agentIcon"
              alt=""
            />{{ t('workflows.task.progress.agentPrompt') }}
          </button>
          <button
            type="button"
            class="tool-button"
			:disabled="fullyDisabled || !taskUuid || !documentsStep"
            @click="taskDocumentsOpen = true"
          >
            <img
              class="tool-button-icon"
              :src="documentIcon"
              alt=""
            />{{ t('workflows.task.progress.documents') }}
          </button>
        </div>
        <div class="composer-actions">
          <button
            v-if="running"
            type="button"
            class="send-button stop-button"
            :disabled="stopping"
            :title="t('workflows.task.progress.stopTask')"
            :aria-label="t('workflows.task.progress.stopTask')"
            @click="emit('stop')"
          >
            <LoadingOutlined
              v-if="stopping"
              spin
            />
            <span
              v-else
              class="stop-square"
              aria-hidden="true"
            />
          </button>
          <button
            v-else
            type="button"
            class="send-button submit-button"
            :disabled="!canSubmit"
            :title="resolvedSubmitButtonLabel"
            @click="handleSubmit"
          >
            <LoadingOutlined
              v-if="submitting || preparingSubmission"
              spin
            />
            <span
              v-else
              class="send-button-text"
            >
              {{ resolvedSubmitButtonLabel }}
            </span>
          </button>
        </div>
      </div>
    </div>
    <AgentPromptDrawer
      v-model:open="agentPromptOpen"
      :task-uuid="taskUuid"
      :current-step="currentStep"
      :steps="steps"
      :executing-step-uuid="executingStepUuid"
      @saved="emit('prompt-saved')"
    />
    <TaskDocumentsDrawer
      v-model:open="taskDocumentsOpen"
      :task-uuid="taskUuid"
	  :current-step="documentsStep"
      @reference="handleDrawerReference"
    />
    <p v-if="disabledReason" class="composer-hint">{{ disabledReason }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import {
  CloseOutlined,
  FolderOpenOutlined,
  LoadingOutlined,
  PaperClipOutlined,
  PictureOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import agentIcon from '@/assets/icons/task-composer-agent.svg'
import cliIcon from '@/assets/icons/task-composer-cli.svg'
import documentIcon from '@/assets/icons/task-composer-document.svg'
import { useChatRuntimeDefaults } from '@/composables/useChatRuntimeDefaults'
import { useCliModelOptions } from '@/composables/useCliModelOptions'
import { useTaskImageAttachments } from '@/composables/useTaskImageAttachments'
import { normalizeClipboardText } from '@/utils/clipboard'
import type { ChatComposerSubmission } from '@/types/task-attachments'
import type { PipelineStep } from '@/types/pipeline'
import AgentPromptDrawer from './AgentPromptDrawer.vue'
import CliRuntimePicker from './CliRuntimePicker.vue'
import DocumentMentionMenu from './DocumentMentionMenu.vue'
import TaskDocumentsDrawer from './TaskDocumentsDrawer.vue'
import GitBranchPicker from './GitBranchPicker.vue'
import {
  collectMarkerRanges,
  matchDocumentMentionFiles,
  resolveDocumentMentionContext,
  splitDocumentMentionTokens,
} from './documentMention'
import type { DocumentMentionContext, DocumentMentionRange } from './documentMention'
import type { MentionableTaskFile, TaskFileNode } from '@/types/task-files'
import { useTaskFilesIndex } from '@/composables/useTaskFilesIndex'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = withDefaults(
  defineProps<{
    modelValue: string
    canAsk: boolean
    submitting?: boolean
    running?: boolean
    stopping?: boolean
    placeholder: string
    contextText: string
    taskUuid: string
    currentStep?: PipelineStep
	documentStep?: PipelineStep
    steps?: PipelineStep[]
    executingStepUuid?: string
    cliType?: string
    modelName?: string
    startMode?: boolean
    showWorkDirectory?: boolean
    workDirectory?: string
    fullyDisabled?: boolean
    disabledReason?: string
	expertMembers?: Array<{ uuid: string; name: string; avatar?: string; member_role?: 'leader' | 'member' | '' }>
    /** 需求 2204：看板-任务详情的 CLI 任务输入框不展示 Agent 提示词入口。 */
    hideAgentPrompt?: boolean
    submitButtonLabel?: string
  }>(),
  {
    submitting: false,
    running: false,
    stopping: false,
    cliType: '',
    modelName: '',
    startMode: false,
    showWorkDirectory: false,
    workDirectory: '',
    fullyDisabled: false,
    disabledReason: '',
    steps: () => [],
    executingStepUuid: '',
	expertMembers: () => [],
    hideAgentPrompt: false,
    submitButtonLabel: '',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  submit: [submission: ChatComposerSubmission]
  stop: []
  'prompt-saved': []
  'select-work-directory': []
}>()

const agentPromptOpen = ref(false)
const taskDocumentsOpen = ref(false)
const textareaRef = ref<HTMLTextAreaElement>()
const inputLayerRef = ref<HTMLElement>()
const isComposing = ref(false)
const fileInputRef = ref<HTMLInputElement>()
const documentMentionMenuRef = ref<InstanceType<typeof DocumentMentionMenu>>()
const documentMentionContext = ref<DocumentMentionContext | null>(null)
const documentMentionDismissed = ref<DocumentMentionContext | null>(null)
const {
  entry: documentMentionIndex,
  bind: bindDocumentMentionIndex,
  load: loadDocumentMentionIndex,
} = useTaskFilesIndex()
const expertMentionOpen = ref(false)
const expertMentionStart = ref(0)
const expertMentionEnd = ref(0)
const expertMentionQuery = ref('')
const expertMentionActiveIndex = ref(0)
const selectedExpertMember = ref<{ uuid: string; name: string; avatar?: string; member_role?: 'leader' | 'member' | '' }>()
const filteredExpertMembers = computed(() => props.expertMembers.filter((member) => member.name.toLowerCase().includes(expertMentionQuery.value.toLowerCase())))
const selectedCliType = ref('')
const selectedModelName = ref('')
const runtimeTouched = ref(false)
const cliPickerOpen = ref(false)
const preparingSubmission = ref(false)
const branchChanging = ref(false)
/*
 * 已插入过的引用登记，只追加、不剪枝：标记从正文里消失（删除、清空重来）时
 * 登记先留着，撤销把标记原样放回来就能立刻恢复成胶囊、提交时也还能换成路径。
 * 不在正文里的登记本身无害 —— 渲染区间按正文匹配，序列化也是按标记替换。
 */
const documentReferences = ref<DocumentReference[]>([])
const {
  items: attachmentItems,
  remove: removeAttachment,
  clear: clearImageAttachments,
  isSaving: isSavingPastedImages,
  addFiles,
  addImages,
  uploadAttachmentsToTask,
} = useTaskImageAttachments()
const {
  cliLoading,
  cliOptions,
  loadCliOptions,
  loadModelOptions,
  modelCounts,
  modelLoading,
  modelOptions,
  prefetchModelCounts,
  resetModelOptions,
} = useCliModelOptions()
const { getChatRuntimeSelection, saveChatRuntimeSelection } = useChatRuntimeDefaults()
const runtimeEnabled = computed(() => Boolean(props.taskUuid && props.currentStep))
const documentsStep = computed(() => props.documentStep || props.currentStep)
const resolvedSubmitButtonLabel = computed(
  () =>
    props.submitButtonLabel ||
    (props.startMode && !props.modelValue.trim()
      ? t('workflows.task.progress.begin')
      : t('workflows.task.progress.send')),
)
const runtimeIdentity = computed(() => `${props.taskUuid}:${props.currentStep?.uuid || ''}`)
const availableModels = computed(() => {
  const result = [...modelOptions.value]
  if (selectedModelName.value && !result.includes(selectedModelName.value)) {
    result.unshift(selectedModelName.value)
  }
  return result
})
/** 附件只在上方附件行里，正文可能为空，所以「只有附件」也算可发送 */
const hasAttachments = computed(() => attachmentItems.value.length > 0)
const canSubmit = computed(
  () =>
    props.canAsk &&
    !props.submitting &&
    !branchChanging.value &&
    !preparingSubmission.value &&
    !isSavingPastedImages.value &&
    (Boolean(props.modelValue.trim()) || hasAttachments.value || props.startMode) &&
    (!runtimeEnabled.value || Boolean(selectedCliType.value && selectedModelName.value)),
)
const selectedCliLabel = computed(
  () =>
    cliOptions.value.find((item) => item.type === selectedCliType.value)?.name ||
    selectedCliType.value,
)
const runtimeTriggerLabel = computed(() => {
  if (selectedCliType.value && selectedModelName.value) {
    return `${selectedCliLabel.value} / ${selectedModelName.value}`
  }
  if (selectedCliType.value) return selectedCliLabel.value
  return t('workflows.task.progress.chooseCli')
})
/**
 * # 引用只需要任务上下文（文件列表接口就是 /tasks/:uuid/files）。
 * 不能复用 runtimeEnabled：它依赖 currentStep，而 CLI 直接执行与专家团这两种模式
 * 传进来的是 documentStep 而非 currentStep，会让引用菜单在这类任务里永远打不开。
 */
const documentMentionAvailable = computed(() => Boolean(props.taskUuid))
const documentMentionQuery = computed(() => documentMentionContext.value?.query ?? '')
/**
 * 在原有 placeholder 末尾补一句 # 引用提示，仅在有任务上下文时生效。
 * 不改各父组件的 i18n 词条——新会话等无 taskUuid 的场景不会出现 # 菜单，
 * 也不该看到这条提示。
 */
const effectivePlaceholder = computed(() => {
  if (!documentMentionAvailable.value) return props.placeholder
  const hint = t('workflows.task.progress.hashMentionHint')
  return props.placeholder ? `${props.placeholder}  ${hint}` : hint
})
const documentMentionStatus = computed(() => documentMentionIndex.value.status)
const documentMentionError = computed(() => documentMentionIndex.value.error)
const documentMentionMatches = computed(() =>
  documentMentionContext.value
    ? matchDocumentMentionFiles(
        documentMentionIndex.value.files,
        documentMentionContext.value.query,
      )
    : [],
)
/**
 * 输入框渲染层的分段：已登记的引用标记渲染成标签块，其余按原文本显示。
 * 末尾补一个换行，避免文本以换行结尾时渲染层比 textarea 少一行而错位。
 */
const mentionTokens = computed(() => [
  ...splitDocumentMentionTokens(
    props.modelValue,
    documentReferences.value.map((reference) => ({
      marker: reference.marker,
      name: reference.name,
    })),
  ),
  { text: '\n', reference: false },
])

/**
 * 已登记引用标记在正文里的区间。内联胶囊在视觉上是一个整体，
 * 删除、光标移动都按这个区间整块处理，而不是让用户逐字符退格。
 */
const documentReferenceRanges = computed(() =>
  collectMarkerRanges(
    props.modelValue,
    documentReferences.value.map((reference) => reference.marker),
  ),
)

/**
 * 命中才弹出：候选为空直接隐藏，不再出现「空列表 + 无匹配提示」的中间态。
 * 加载中同样不弹出，等索引就绪后由这个 computed 自动补上，避免先弹后收的闪烁。
 */
const documentMentionOpen = computed(() => {
  if (!documentMentionContext.value || !documentMentionAvailable.value) return false
  const status = documentMentionIndex.value.status
  if (status === 'error') return true
  if (status !== 'ready') return false
  return documentMentionMatches.value.length > 0
})
let runtimeRequestId = 0
let documentReferenceSequence = 0
const COMPOSER_MIN_HEIGHT = 66
const COMPOSER_MAX_HEIGHT = 212

function handleInput(event: Event) {
  const textarea = event.target as HTMLTextAreaElement
	if (selectedExpertMember.value && !textarea.value.includes(`@${selectedExpertMember.value.name}`)) {
	  selectedExpertMember.value = undefined
	}
  emit('update:modelValue', textarea.value)
	updateExpertMention(textarea)
  syncDocumentMention(textarea)
  resizeTextarea()
}

function updateExpertMention(textarea: HTMLTextAreaElement) {
  if (!props.expertMembers.length) { expertMentionOpen.value = false; return }
	if (selectedExpertMember.value && textarea.value.includes(`@${selectedExpertMember.value.name}`)) {
	  expertMentionOpen.value = false
	  return
	}
  const caret = textarea.selectionStart ?? textarea.value.length
  const before = textarea.value.slice(0, caret)
  const match = before.match(/(?:^|\s)@([^\s@]*)$/)
  if (!match) { expertMentionOpen.value = false; return }
  expertMentionStart.value = caret - match[1].length - 1
  expertMentionEnd.value = caret
  expertMentionQuery.value = match[1]
	 expertMentionActiveIndex.value = 0
  expertMentionOpen.value = true
}

function selectExpertMember(member: { uuid: string; name: string; avatar?: string; member_role?: 'leader' | 'member' | '' }) {
  const value = textareaRef.value?.value ?? props.modelValue
  const marker = `@${member.name} `
  const nextValue = value.slice(0, expertMentionStart.value) + marker + value.slice(expertMentionEnd.value)
  selectedExpertMember.value = member
  expertMentionOpen.value = false
  emit('update:modelValue', nextValue)
  nextTick(() => {
    const caret = expertMentionStart.value + marker.length
    textareaRef.value?.focus()
    textareaRef.value?.setSelectionRange(caret, caret)
  })
}

interface DocumentReference {
  id: string
  marker: string
  name: string
  absolutePath: string
}

function resizeTextarea() {
  const textarea = textareaRef.value
  if (!textarea) return

  textarea.style.height = `${COMPOSER_MIN_HEIGHT}px`
  const height = Math.min(Math.max(textarea.scrollHeight, COMPOSER_MIN_HEIGHT), COMPOSER_MAX_HEIGHT)
  textarea.style.height = `${height}px`
  textarea.style.overflowY = textarea.scrollHeight > COMPOSER_MAX_HEIGHT ? 'auto' : 'hidden'
  syncInputLayer()
}

/** 渲染层与 textarea 是两层独立元素，滚动位置只能靠 JS 手动对齐 */
function syncInputLayer() {
  const textarea = textareaRef.value
  const layer = inputLayerRef.value
  if (!textarea || !layer) return
  layer.scrollTop = textarea.scrollTop
  layer.scrollLeft = textarea.scrollLeft
}

function handleCompositionEnd() {
  isComposing.value = false
  // 组合输入结束后文本与行数都可能变化，下一帧再对齐一次
  nextTick(syncInputLayer)
}

/**
 * 内联引用胶囊的「整体」交互。
 *
 * 视觉上它是一颗胶囊，所以操作也按一整块处理：
 * - 点进胶囊内部 → 整颗标记被选中，用户直接看到它是一块
 * - 光标紧贴胶囊右侧按退格 / 紧贴左侧按删除键 → 整块删掉
 * - 光标落在胶囊内部、或选区与胶囊相交 → 先把范围扩到整颗胶囊再删
 * - 左右方向键整块跨过胶囊，不会停在中间
 *
 * 判定区间复用 documentReferenceRanges，和渲染层是同一份数据，
 * 不会出现「看着是胶囊、删掉的却是别的字符」。
 */
function markerRangeAt(caret: number): DocumentMentionRange | undefined {
  return documentReferenceRanges.value.find((range) => range.start < caret && caret < range.end)
}

function resolveMarkerDeletion(
  selectionStart: number,
  selectionEnd: number,
  key: 'Backspace' | 'Delete',
): DocumentMentionRange | null {
  const ranges = documentReferenceRanges.value
  if (!ranges.length) return null

  if (selectionStart !== selectionEnd) {
    // 只有真正压到胶囊才算命中；紧挨着不算，避免误删旁边的普通文字
    const touched = ranges.filter(
      (range) => range.start < selectionEnd && range.end > selectionStart,
    )
    if (!touched.length) return null
    return {
      start: Math.min(selectionStart, touched[0].start),
      end: Math.max(selectionEnd, touched[touched.length - 1].end),
    }
  }

  const inside = markerRangeAt(selectionStart)
  if (inside) return { ...inside }
  const adjacent = ranges.find((range) =>
    key === 'Backspace' ? range.end === selectionStart : range.start === selectionStart,
  )
  return adjacent ? { ...adjacent } : null
}

function handleMarkerDeletion(event: KeyboardEvent): boolean {
  if (event.key !== 'Backspace' && event.key !== 'Delete') return false
  const textarea = textareaRef.value
  if (!textarea) return false
  const target = resolveMarkerDeletion(
    textarea.selectionStart ?? 0,
    textarea.selectionEnd ?? 0,
    event.key,
  )
  if (!target) return false

  event.preventDefault()
  textarea.setSelectionRange(target.start, target.end)
  /*
   * 走原生编辑命令、而不是自己拼新值再 emit：只有原生编辑才会进撤销栈，
   * 否则删掉胶囊之后 Cmd+Z 撤不回来（普通输入的撤销是好的，别把它弄丢）。
   * 命令触发的原生 input 事件会走 handleInput，正文与引用登记随之更新。
   */
  if (document.execCommand('delete')) {
    nextTick(syncInputLayer)
    return true
  }

  // execCommand 不可用时退回手工替换：功能正确，但这一步不会留下撤销记录
  const nextValue = textarea.value.slice(0, target.start) + textarea.value.slice(target.end)
  emit('update:modelValue', nextValue)
  nextTick(() => {
    const updated = textareaRef.value
    if (!updated) return
    updated.focus()
    updated.setSelectionRange(target.start, target.start)
    resizeTextarea()
  })
  return true
}

/** 左右方向键整块跨过胶囊；带修饰键时交回浏览器（选区扩展等） */
function handleMarkerCaretMove(event: KeyboardEvent): boolean {
  if (event.shiftKey || event.altKey || event.metaKey || event.ctrlKey) return false
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return false
  const textarea = textareaRef.value
  if (!textarea) return false
  const caret = textarea.selectionStart ?? 0
  if (caret !== (textarea.selectionEnd ?? caret)) return false
  const range =
    event.key === 'ArrowLeft'
      ? documentReferenceRanges.value.find((item) => item.end === caret)
      : documentReferenceRanges.value.find((item) => item.start === caret)
  if (!range) return false
  event.preventDefault()
  const next = event.key === 'ArrowLeft' ? range.start : range.end
  textarea.setSelectionRange(next, next)
  return true
}

/** 点进胶囊内部时整块选中，让「它是一个整体」这件事直接被看到 */
function selectMarkerAtCaret() {
  const textarea = textareaRef.value
  if (!textarea) return
  const caret = textarea.selectionStart ?? 0
  if (caret !== (textarea.selectionEnd ?? caret)) return
  const range = markerRangeAt(caret)
  if (!range) return
  textarea.setSelectionRange(range.start, range.end)
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.isComposing) return
	if (expertMentionOpen.value) {
	  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
		event.preventDefault()
		const count = filteredExpertMembers.value.length
		if (count) expertMentionActiveIndex.value = (expertMentionActiveIndex.value + (event.key === 'ArrowDown' ? 1 : -1) + count) % count
		return
	  }
	  if (event.key === 'Escape') {
		event.preventDefault()
		expertMentionOpen.value = false
		return
	  }
	  const activeMember = filteredExpertMembers.value[expertMentionActiveIndex.value]
	  if (event.key === 'Enter' && activeMember) {
		event.preventDefault()
		selectExpertMember(activeMember)
		return
	  }
	}
  if (documentMentionMenuRef.value?.handleKeydown(event)) return
  if (handleMarkerCaretMove(event)) return
  if (handleMarkerDeletion(event)) return
  if (event.key !== 'Enter') return
  const plainEnter = !event.shiftKey && !event.altKey && !event.metaKey && !event.ctrlKey
  if (!plainEnter && !event.ctrlKey) return
  event.preventDefault()
  handleSubmit()
}

function syncDocumentMention(textarea: HTMLTextAreaElement) {
  if (!documentMentionAvailable.value) {
    documentMentionContext.value = null
    return
  }
  const context = resolveDocumentMentionContext(
    textarea.value,
    textarea.selectionStart ?? textarea.value.length,
  )
  if (!context) {
    documentMentionContext.value = null
    return
  }
  const dismissed = documentMentionDismissed.value
  // Esc 收起后只有查询词再次变化才重新弹出，避免刚关掉又被同一次输入顶开
  if (dismissed && dismissed.start === context.start && dismissed.query === context.query) {
    documentMentionContext.value = null
    return
  }
  documentMentionContext.value = context
}

function refreshDocumentMention(event: MouseEvent) {
  syncDocumentMention(event.target as HTMLTextAreaElement)
  // 点击输入区时按 TTL 静默校准列表，任务产出新文件后不必重开会话
  void loadDocumentMentionIndex()
}

function handleComposerClick(event: MouseEvent) {
  refreshDocumentMention(event)
  // 先算引用上下文再整块选中：点进胶囊内部时菜单本来也不该弹（查询词以 [ 开头）
  selectMarkerAtCaret()
}

/** 用户按 Esc 主动收起：记住当前上下文，改词之前不再自动弹出 */
function dismissDocumentMention() {
  const context = documentMentionContext.value
  documentMentionDismissed.value = context ? { ...context } : null
  documentMentionContext.value = null
}

/** 选中引用、提交消息或切换会话时的程序性关闭，不抑制后续弹出 */
function closeDocumentMention() {
  documentMentionDismissed.value = null
  documentMentionContext.value = null
}

function reloadDocumentMention() {
  void loadDocumentMentionIndex(true)
}

/** 在指定位置插入 #[名称] 引用标记 */
function insertDocumentMarker(file: MentionableTaskFile, context: DocumentMentionContext) {
  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const start = Math.min(context.start, value.length)
  const end = Math.min(Math.max(context.end, start), value.length)
  const before = value.slice(0, start)
  const after = value.slice(end)
  const leadingSpace = before && !/\s$/.test(before) ? ' ' : ''
  const trailingSpace = after && !/^\s/.test(after) ? ' ' : ''
  const name = file.name || basenameFromPath(file.absolute_path)
  const duplicateCount = documentReferences.value.filter(
    (reference) => reference.name === name,
  ).length
  const marker = duplicateCount ? `#[${name} · ${++documentReferenceSequence}]` : `#[${name}]`
  const inserted = `${leadingSpace}${marker}${trailingSpace}`
  const caret = start + inserted.length
  // 先登记引用再落笔：登记表只追加，插入过程中的 input 事件不会把它冲掉
  documentReferences.value.push({
    id: crypto.randomUUID(),
    marker,
    name,
    absolutePath: file.absolute_path,
  })
  closeDocumentMention()

  /*
   * 优先走原生插入命令。直接改 value（emit 新值让 Vue 回写）会清空原生撤销栈，
   * 用户刚插进来的引用就没法 Cmd+Z 撤掉，连插入之前的输入历史也会一起丢掉。
   * 命令触发的原生 input 事件会走 handleInput，正文与渲染层随之更新。
   */
  let applied = false
  if (textarea) {
    textarea.focus()
    textarea.setSelectionRange(start, end)
    applied = document.execCommand('insertText', false, inserted)
  }
  if (!applied) {
    // execCommand 不可用时退回手工替换：功能正确，但这一步不会留下撤销记录
    const nextValue = `${before}${inserted}${after}`
    emit('update:modelValue', nextValue)
  }
  nextTick(() => {
    textareaRef.value?.focus()
    textareaRef.value?.setSelectionRange(caret, caret)
    resizeTextarea()
  })
}

function insertDocumentReference(file: MentionableTaskFile) {
  const context = documentMentionContext.value
  if (!context) return
  insertDocumentMarker(file, context)
}

/** 产出文档抽屉里点「引用到输入框」：不依赖 # 触发上下文，直接在光标处插入标记 */
function handleDrawerReference(file: TaskFileNode) {
  if (!file.absolute_path) return
  const textarea = textareaRef.value
  const value = props.modelValue
  const caret = textarea
    ? Math.min(textarea.selectionStart ?? value.length, value.length)
    : value.length
  insertDocumentMarker(
    { ...file, absolute_path: file.absolute_path },
    { start: caret, end: caret, query: '' },
  )
  taskDocumentsOpen.value = false
}

function basenameFromPath(path: string) {
  const normalized = path.replace(/\\/g, '/').replace(/\/+$/, '')
  return normalized.split('/').filter(Boolean).at(-1) || path
}

async function handlePaste(event: ClipboardEvent) {
  if (!event.clipboardData) return
  const files = [...event.clipboardData.items]
    .filter((item) => item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (!files.length) return
  if (isSavingPastedImages.value) {
    event.preventDefault()
    message.warning(t('workflows.task.create.savingImages'))
    return
  }

  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const start = textarea?.selectionStart ?? value.length
  const end = textarea?.selectionEnd ?? start
  const pastedText = normalizeClipboardText(event.clipboardData.getData('text/plain'))
  event.preventDefault()
  // 图片一律进上方附件行，正文里只保留剪贴板里的文本，
  // 避免把 ![](attachments/...) 这类语法直接摊在输入框里
  if (pastedText) insertPlainText(pastedText, start, end)
  await attachFiles(files, addImages, t('workflows.task.create.pasteFailed'))
}

async function handleFileSelection(event: Event) {
  const input = event.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  if (!files.length) return
  if (isSavingPastedImages.value) {
    message.warning(t('workflows.task.progress.savingAttachments'))
    return
  }
  await attachFiles(files, addFiles, t('workflows.task.progress.attachmentUploadFailed'))
}

type AttachmentAdder = (files: File[]) => Array<{ marker: string }>

/**
 * 粘贴和「附件」按钮共用的落地流程：加入附件列表 → 上传 → 留在附件行。
 * 正文里不写任何占位标记，markdown 引用等上传完成后再在提交时拼到消息末尾。
 */
async function attachFiles(files: File[], add: AttachmentAdder, failureText: string) {
  let added: Array<{ marker: string }> = []
  try {
    added = add(files)
    await uploadAttachmentsToTask(props.taskUuid)
    await nextTick()
    textareaRef.value?.focus()
    resizeTextarea()
  } catch (error) {
    // 撤掉这一批，避免附件行里留下没上传成功的条目
    added.forEach((attachment) => removeAttachment(attachment.marker))
    message.error(error instanceof Error ? error.message : failureText)
  }
}

/** 按光标位置插入剪贴板文本；附件不进正文，所以这里只处理文本 */
function insertPlainText(text: string, start: number, end: number) {
  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const insertStart = Math.min(start, value.length)
  const insertEnd = Math.min(Math.max(end, insertStart), value.length)
  const nextValue = `${value.slice(0, insertStart)}${text}${value.slice(insertEnd)}`
  const caret = insertStart + text.length
  emit('update:modelValue', nextValue)
  nextTick(() => {
    const updatedTextarea = textareaRef.value
    if (!updatedTextarea) return
    updatedTextarea.focus()
    updatedTextarea.setSelectionRange(caret, caret)
    syncDocumentMention(updatedTextarea)
    resizeTextarea()
  })
}

/**
 * 附件在正文里没有位置，提交时统一拼到消息末尾。
 * 只取已落地的 markdown：还没上传完的条目拿不到引用，此时 canSubmit 也会挡住提交。
 */
function attachmentMarkdownText() {
  return attachmentItems.value
    .map((attachment) => attachment.markdown)
    .filter((markdown): markdown is string => Boolean(markdown))
    .join('\n')
}

function serializeDocumentReferences(value: string) {
  let serialized = value
  for (const reference of documentReferences.value) {
    serialized = serialized.split(reference.marker).join(reference.absolutePath)
  }
  return serialized
}

async function handleSubmit() {
  if (!canSubmit.value) return
  closeDocumentMention()
	expertMentionOpen.value = false
  preparingSubmission.value = true
  try {
    const rawContent =
      props.modelValue.trim() ||
      (props.startMode ? t('workflows.task.progress.startProcessing') : '')
    // 附件在正文里没有位置，统一追加到消息末尾；只有附件、没有文字时附件就是全部内容
    const displayContent = [rawContent, attachmentMarkdownText()].filter(Boolean).join('\n')
    const promptContent = serializeDocumentReferences(displayContent)
    emit('submit', {
      content: promptContent.trim(),
      // 聊天记录展示原始输入（保留 #[名称] 标记）：绝对路径只有 CLI 需要，
      // 不应该跟着落进会话记录里
      display_content: displayContent.trim(),
      config: {
        cli_type: selectedCliType.value,
        model_name: selectedModelName.value,
      },
	  member_uuid: selectedExpertMember.value?.uuid,
    })
	selectedExpertMember.value = undefined
    await nextTick()
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.progress.prepareMessageFailed'),
    )
  } finally {
    preparingSubmission.value = false
  }
}

function persistRuntimeSelection() {
  const stepUuid = props.currentStep?.uuid || ''
  if (!props.taskUuid || !stepUuid || !selectedCliType.value) return
  saveChatRuntimeSelection(props.taskUuid, stepUuid, {
    cli_type: selectedCliType.value,
    model_name: selectedModelName.value,
  })
}

function cliStillAvailable(cliType: string) {
  if (!cliType) return false
  const match = cliOptions.value.find((item) => item.type === cliType)
  return Boolean(match?.installed)
}

async function initialiseRuntimeConfig() {
  const requestId = ++runtimeRequestId
  runtimeTouched.value = false
  const cached = getChatRuntimeSelection(props.taskUuid, props.currentStep?.uuid || '')
  selectedCliType.value = cached?.cli_type || props.cliType || props.currentStep?.cli_type || ''
  selectedModelName.value =
    cached?.model_name ||
    props.modelName ||
    props.currentStep?.model_name ||
    props.currentStep?.model ||
    ''
  if (!runtimeEnabled.value) {
    resetModelOptions()
    return
  }

  try {
    if (!cliOptions.value.length) await loadCliOptions()
    if (requestId !== runtimeRequestId) return
    if (cached?.cli_type && !cliStillAvailable(cached.cli_type)) {
      selectedCliType.value = props.cliType || props.currentStep?.cli_type || ''
      selectedModelName.value =
        props.modelName || props.currentStep?.model_name || props.currentStep?.model || ''
    }
    void prefetchModelCounts(cliOptions.value.map((item) => item.type))
    if (!selectedCliType.value) {
      resetModelOptions()
      return
    }
    const models = await loadModelOptions(selectedCliType.value)
    if (requestId !== runtimeRequestId) return
    if (selectedModelName.value && models.length && !models.includes(selectedModelName.value)) {
      selectedModelName.value = models[0] || ''
    } else if (!selectedModelName.value) {
      selectedModelName.value = models[0] || ''
    }
  } catch (error) {
    if (requestId === runtimeRequestId) {
      message.warning(
        error instanceof Error ? error.message : t('workflows.task.progress.runtimeLoadFailed'),
      )
    }
  }
}

async function handleCliChange(cliType: string) {
  if (cliType === selectedCliType.value) return
  runtimeRequestId++
  runtimeTouched.value = true
  selectedCliType.value = cliType
  selectedModelName.value = ''
  persistRuntimeSelection()
  try {
    const models = await loadModelOptions(cliType)
    if (selectedCliType.value === cliType && !selectedModelName.value) {
      selectedModelName.value = models[0] || ''
      persistRuntimeSelection()
    }
  } catch (error) {
    if (selectedCliType.value === cliType) {
      message.warning(
        error instanceof Error ? error.message : t('components.cliSelect.modelsLoadFailed'),
      )
    }
  }
}

function handleModelChange(model: string) {
  runtimeTouched.value = true
  selectedModelName.value = model
  persistRuntimeSelection()
  cliPickerOpen.value = false
}

onMounted(resizeTextarea)

watch(
  runtimeIdentity,
  () => {
    closeDocumentMention()
    cliPickerOpen.value = false
    documentReferences.value = []
    clearImageAttachments()
    bindDocumentMentionIndex(props.taskUuid)
    // 挂载即预取产出文件列表，用户真正输入 # 时过滤已退化为内存计算
    void loadDocumentMentionIndex()
    void initialiseRuntimeConfig()
  },
  { immediate: true },
)

watch(cliPickerOpen, (open) => {
  if (!open || !cliOptions.value.length) return
  void prefetchModelCounts(cliOptions.value.map((item) => item.type))
})

watch(
  () => [props.cliType, props.modelName] as const,
  ([cliType, modelName], [previousCLIType, previousModelName]) => {
    if (runtimeTouched.value || (cliType === previousCLIType && modelName === previousModelName))
      return
    void initialiseRuntimeConfig()
  },
)

watch(
  () => props.modelValue,
  () => nextTick(resizeTextarea),
)

function resetAfterSubmit() {
  closeDocumentMention()
  documentReferences.value = []
  documentReferenceSequence = 0
  clearImageAttachments()
}

defineExpose({ resetAfterSubmit })
</script>

<style scoped>
.expert-mention-menu {
  position: absolute;
  z-index: 12;
  bottom: calc(100% + 8px);
  left: 24px;
  width: 280px;
  max-height: 240px;
  overflow-y: auto;
  padding: 6px;
  border: 1px solid #d9d9d9;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
}

.expert-mention-menu button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.expert-mention-menu button:hover,
.expert-mention-menu button:focus-visible,
.expert-mention-menu button.active { background: #f5f6f8; }
.expert-mention-menu img { width: 28px; height: 28px; border-radius: 50%; object-fit: cover; }
.expert-mention-menu span { display: flex; min-width: 0; flex-direction: column; }
.expert-mention-menu small { color: #8c8c8c; }
.chat-composer {
  position: relative;
  flex: 0 0 auto;
  background: #fff;
  padding: 8px 24px 24px;
}
.composer-shell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow: hidden;
  border: 2px solid #e5e7eb;
  border-radius: 12px;
  padding: 10px 12px;
  background: #fff;
  box-shadow: 0 4px 8px rgba(0, 0, 0, 0.08);
  transition:
    border-color 0.15s,
    box-shadow 0.15s;
}
.composer-shell:focus-within {
  border-color: #3157e2;
  box-shadow: 0 0 0 3px rgba(49, 87, 226, 0.08);
}
.composer-shell.is-disabled,
.composer-shell.is-disabled:focus-within {
  border-color: #e5e7eb;
  background: #f7f8fa;
  box-shadow: none;
}
.composer-input {
  position: relative;
}
/* 渲染层与 textarea 必须共用同一套排版，任何一项不一致都会让标签块与光标错位 */
.composer-input-layer,
.composer-shell textarea {
  margin: 0;
  border: 0;
  padding: 0;
  font-family: inherit;
  font-size: 14px;
  line-height: 22px;
  letter-spacing: normal;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  word-break: normal;
  tab-size: 4;
}
.composer-input-layer {
  position: absolute;
  inset: 0;
  overflow: hidden;
  color: #262626;
  background: #fff;
  pointer-events: none;
  user-select: none;
}
/*
 * # 引用标签块。视觉对齐 WorkBuddy 的文件引用胶囊（100px 圆角、浅灰底、深色字、前置文件图标）。
 *
 * 两条不能碰的约束——渲染层与 textarea 必须逐字同宽，否则光标会整体偏移：
 * 1. 字号/字重/字距必须与 textarea 完全一致，所以这里不覆盖 font-size、font-weight；
 *    需要更实的字重用 -webkit-text-stroke 描边实现，它不参与排版宽度计算。
 * 2. 只用纵向 padding 撑高度。行内元素的纵向 padding 不参与行盒高度计算，
 *    横向内边距则由不着色的 #[ 与 ] 两个语法字符天然提供。
 */
.mention-token {
  position: relative;
  padding: 3px 0;
  border-radius: 999px;
  color: rgba(0, 0, 0, 0.88);
  background: #f2f2f2;
  -webkit-text-stroke: 0.3px currentColor;
}
/* 语法字符只占位、不着色：撑出标签块左右内边距，同时保持逐字对齐 */
.mention-token-syntax {
  color: transparent;
  -webkit-text-stroke: 0;
}
.mention-token-label {
  white-space: pre;
}
.mention-token-icon {
  position: absolute;
  top: 50%;
  left: 2.5px;
  width: 9px;
  height: 9px;
  transform: translateY(-50%);
  color: rgba(0, 0, 0, 0.5);
  pointer-events: none;
}
/*
 * 不可编辑时不做富文本渲染，直接显示原生文本。
 * 中文组合输入期间同理：渲染层读的是 modelValue，而组合期间的 input 事件被
 * handleInput 按 isComposing 跳过，渲染层拿不到新值，所以必须让位给原生 textarea。
 *
 * 但 textarea 平时是 color: transparent 的（文字交给渲染层画），
 * 隐藏渲染层的同时必须把文字颜色还回来，否则预编辑的拼音、以及依赖
 * 预编辑文字位置的候选框会一起看不见。
 */
.composer-input.is-disabled .composer-input-layer,
.composer-input.is-composing .composer-input-layer {
  display: none;
}
.composer-input.is-composing textarea {
  color: #262626;
  -webkit-text-fill-color: #262626;
}
.composer-shell textarea {
  position: relative;
  z-index: 1;
  display: block;
  width: 100%;
  height: 66px;
  min-height: 66px;
  max-height: 212px;
  overflow-y: hidden;
  resize: none;
  outline: 0;
  /* 文字交给渲染层显示，这里只保留光标和选区 */
  color: transparent;
  -webkit-text-fill-color: transparent;
  caret-color: #262626;
  background: transparent;
  scrollbar-width: none;
}
.composer-shell textarea::-webkit-scrollbar {
  width: 0;
  height: 0;
}
.composer-shell textarea::selection {
  background: rgba(49, 87, 226, 0.18);
}
.composer-shell textarea::placeholder {
  color: #8c8c8c;
  -webkit-text-fill-color: #8c8c8c;
}
.composer-shell textarea:disabled {
  color: #9ca3af;
  background: transparent;
  cursor: not-allowed;
  opacity: 1;
  -webkit-text-fill-color: #9ca3af;
}
.composer-shell textarea:disabled::placeholder {
  color: #9ca3af;
  opacity: 1;
}
/*
 * 附件行：只放附件，# 引用在输入框里已有内联胶囊，不在这里重复。
 * 规格对齐 WorkBuddy 的文件标签——100px 圆角胶囊、23px 高、13px/500 深色字、
 * 浅灰底，悬停底色加深，同时图标原地换成关闭图标，不用额外占位一个删除按钮。
 */
.attachment-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}
.attachment-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 220px;
  height: 23px;
  padding: 0 8px;
  border: 0;
  border-radius: 100px;
  color: rgba(0, 0, 0, 0.9);
  background: #f2f2f2;
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
  font-weight: 500;
  transition: background-color 140ms ease;
}
.attachment-chip:hover {
  background: #e6e6e6;
}
.attachment-chip:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 1px;
}
.attachment-chip-icon-slot {
  position: relative;
  display: inline-flex;
  width: 14px;
  height: 14px;
  flex: 0 0 14px;
  align-items: center;
  justify-content: center;
}
.attachment-chip-icon {
  font-size: 12px;
}
.attachment-chip-close {
  position: absolute;
  inset: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: rgba(0, 0, 0, 0.55);
  font-size: 10px;
  visibility: hidden;
}
.attachment-chip:hover .attachment-chip-icon {
  visibility: hidden;
}
.attachment-chip:hover .attachment-chip-close {
  visibility: visible;
}
.attachment-chip-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.composer-toolbar {
  display: flex;
  min-height: 24px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.composer-tools {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.attachment-file-input {
  display: none;
}
.tool-divider {
  width: 1px;
  height: 12px;
  flex: 0 0 1px;
  background: #d9d9d9;
}
.cli-picker-trigger {
  max-width: 220px;
}
.cli-picker-trigger span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.composer-actions {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}
.tool-button {
  display: inline-flex;
  height: 24px;
  align-items: center;
  gap: 4px;
  border: 0;
  border-radius: 6px;
  padding: 1px 6px;
  color: #595959;
  background: #fff;
  cursor: pointer;
  font-family: inherit;
  font-size: 14px;
  line-height: 22px;
  letter-spacing: -0.1504px;
  transition: background-color 0.15s;
}
.tool-button:hover {
  background: #f2f4f7;
}
.tool-button:disabled {
  color: #bfbfbf;
  background: #fff;
  cursor: not-allowed;
}
.tool-button-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
}
.work-directory-button {
  max-width: 240px;
}
.work-directory-button span:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.work-directory-button.selected {
  color: #3157e2;
  background: #eef2ff;
}
.send-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 4px;
  color: #fff;
  background: #262626;
  cursor: pointer;
}
.send-button:not(.stop-button) {
  width: auto;
  min-width: 24px;
  white-space: nowrap;
}
.send-button:disabled {
  color: #aeb4be;
  background: #eceef2;
  cursor: not-allowed;
}
.stop-button:hover:not(:disabled) {
  background: #1f1f1f;
}
.stop-button:disabled {
  color: #fff;
  background: #595959;
}
.stop-square {
  width: 9px;
  height: 9px;
  border-radius: 1px;
  background: currentColor;
}
.submit-button {
  width: auto;
  min-width: 32px;
  padding: 2px 4px;
  border-radius: 6px;
  background: #262626;
  transition: background-color 180ms ease;
}
.submit-button:hover:not(:disabled) {
  background: #434343;
}
.submit-button:active:not(:disabled) {
  background: #141414;
}
.send-button:focus-visible,
.tool-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}
.send-button-text {
  font-size: 12px;
  line-height: 20px;
  white-space: nowrap;
}
.composer-hint {
  margin-top: 6px;
  font-size: 11px;
  color: #9ca3af;
  line-height: 1.4;
}
@media (max-width: 720px) {
  .chat-composer {
    padding: 8px 16px 16px;
  }
  .composer-toolbar {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }
  .composer-tools {
    width: 100%;
    flex-wrap: wrap;
  }
  .composer-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
@media (prefers-reduced-motion: reduce) {
  .composer-shell,
  .submit-button,
  .tool-button {
    transition: none;
  }
}
</style>
