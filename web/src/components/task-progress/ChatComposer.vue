<template>
  <div class="chat-composer">
    <DocumentMentionMenu
      ref="documentMentionMenuRef"
      :open="documentMentionOpen"
      :task-uuid="taskUuid"
      :query="documentMentionQuery"
      @close="closeDocumentMention"
      @select="insertDocumentReference"
    />
    <div
      class="composer-shell"
      :class="{ 'is-disabled': !canAsk && !running }"
    >
      <div
        v-if="documentReferences.length"
        class="reference-chips"
        aria-label="已引用文档"
      >
        <button
          v-for="reference in documentReferences"
          :key="reference.id"
          type="button"
          class="reference-chip"
          :title="`移除引用：${reference.name}`"
          @click="removeDocumentReference(reference.id)"
        >
          <FolderOpenOutlined class="reference-chip-icon" />
          <span class="reference-chip-name">{{ reference.name }}</span>
          <CloseOutlined
            class="reference-chip-close"
            aria-hidden="true"
          />
        </button>
      </div>
      <textarea
        ref="textareaRef"
        :value="modelValue"
        rows="3"
        :disabled="!canAsk"
        :placeholder="placeholder"
        @input="handleInput"
        @click="refreshDocumentMention"
        @paste="handlePaste"
        @keydown="handleComposerKeydown"
      />
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
            :disabled="!taskUuid || !canAsk || isSavingPastedImages"
            title="上传文件（不支持目录）"
            aria-label="上传文件"
            @click="fileInputRef?.click()"
          >
            <PaperClipOutlined class="tool-button-icon" />
            <span>附件</span>
          </button>
          <button
            v-if="showWorkDirectory"
            type="button"
            class="tool-button work-directory-button"
            :class="{ selected: workDirectory }"
            :title="workDirectory || '选择工作目录'"
            aria-label="选择工作目录"
            @click="emit('select-work-directory')"
          >
            <FolderOpenOutlined class="tool-button-icon" />
            <span>{{ workDirectory || '选择工作目录' }}</span>
          </button>
          <CliRuntimePicker
            v-if="runtimeEnabled"
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
              aria-label="选择 CLI 和模型"
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
            v-if="runtimeEnabled"
            class="tool-divider"
            aria-hidden="true"
          />
          <button
            type="button"
            class="tool-button"
            :disabled="!taskUuid"
            @click="agentPromptOpen = true"
          >
            <img
              class="tool-button-icon"
              :src="agentIcon"
              alt=""
            />Agent提示词
          </button>
          <button
            type="button"
            class="tool-button"
            :disabled="!taskUuid || !currentStep"
            @click="taskDocumentsOpen = true"
          >
            <img
              class="tool-button-icon"
              :src="documentIcon"
              alt=""
            />产出文档
          </button>
        </div>
        <div class="composer-actions">
          <button
            v-if="running"
            type="button"
            class="send-button stop-button"
            :disabled="stopping"
            title="停止任务"
            aria-label="停止任务"
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
            class="send-button"
            :disabled="!canSubmit"
            title="发送消息"
            @click="handleSubmit"
          >
            <LoadingOutlined
              v-if="submitting || preparingSubmission"
              spin
            />
            <img
              v-else
              class="send-button-icon"
              :src="sendIcon"
              alt=""
            />
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
      :current-step="currentStep"
    />
    <p class="composer-hint"></p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import {
  CloseOutlined,
  FolderOpenOutlined,
  LoadingOutlined,
  PaperClipOutlined,
} from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import agentIcon from '@/assets/icons/task-composer-agent.svg'
import cliIcon from '@/assets/icons/task-composer-cli.svg'
import documentIcon from '@/assets/icons/task-composer-document.svg'
import sendIcon from '@/assets/icons/task-composer-send.svg'
import { useChatRuntimeDefaults } from '@/composables/useChatRuntimeDefaults'
import { useCliModelOptions } from '@/composables/useCliModelOptions'
import {
  formatPastedImageContent,
  useTaskImageAttachments,
} from '@/composables/useTaskImageAttachments'
import type { ChatComposerSubmission } from '@/types/task-attachments'
import type { PipelineStep } from '@/types/pipeline'
import AgentPromptDrawer from './AgentPromptDrawer.vue'
import CliRuntimePicker from './CliRuntimePicker.vue'
import DocumentMentionMenu from './DocumentMentionMenu.vue'
import TaskDocumentsDrawer from './TaskDocumentsDrawer.vue'

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
    steps?: PipelineStep[]
    executingStepUuid?: string
    cliType?: string
    modelName?: string
    showWorkDirectory?: boolean
    workDirectory?: string
  }>(),
  {
    submitting: false,
    running: false,
    stopping: false,
    cliType: '',
    modelName: '',
    showWorkDirectory: false,
    workDirectory: '',
    steps: () => [],
    executingStepUuid: '',
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
const fileInputRef = ref<HTMLInputElement>()
const documentMentionMenuRef = ref<InstanceType<typeof DocumentMentionMenu>>()
const documentMentionOpen = ref(false)
const documentMentionQuery = ref('')
const documentMentionStart = ref(0)
const documentMentionEnd = ref(0)
const selectedCliType = ref('')
const selectedModelName = ref('')
const runtimeTouched = ref(false)
const cliPickerOpen = ref(false)
const preparingSubmission = ref(false)
const documentReferences = ref<DocumentReference[]>([])
const {
  clear: clearImageAttachments,
  isSaving: isSavingPastedImages,
  addFiles,
  addImages,
  removeAttachmentMarkers,
  resolveAttachmentMarkdown,
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
const runtimeIdentity = computed(() => `${props.taskUuid}:${props.currentStep?.uuid || ''}`)
const availableModels = computed(() => {
  const result = [...modelOptions.value]
  if (selectedModelName.value && !result.includes(selectedModelName.value)) {
    result.unshift(selectedModelName.value)
  }
  return result
})
const canSubmit = computed(
  () =>
    props.canAsk &&
    !props.submitting &&
    !preparingSubmission.value &&
    !isSavingPastedImages.value &&
    Boolean(props.modelValue.trim()) &&
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
  return '请选择 CLI'
})
let runtimeRequestId = 0
let documentReferenceSequence = 0
const COMPOSER_MIN_HEIGHT = 66
const COMPOSER_MAX_HEIGHT = 212

function handleInput(event: Event) {
  const textarea = event.target as HTMLTextAreaElement
  emit('update:modelValue', textarea.value)
  syncDocumentReferences(textarea.value)
  updateDocumentMention(textarea)
  resizeTextarea()
}

interface DocumentReference {
  id: string
  marker: string
  name: string
  absolutePath: string
}

function syncDocumentReferences(value: string) {
  documentReferences.value = documentReferences.value.filter((reference) =>
    value.includes(reference.marker),
  )
}

function resizeTextarea() {
  const textarea = textareaRef.value
  if (!textarea) return

  textarea.style.height = `${COMPOSER_MIN_HEIGHT}px`
  const height = Math.min(Math.max(textarea.scrollHeight, COMPOSER_MIN_HEIGHT), COMPOSER_MAX_HEIGHT)
  textarea.style.height = `${height}px`
  textarea.style.overflowY = textarea.scrollHeight > COMPOSER_MAX_HEIGHT ? 'auto' : 'hidden'
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (documentMentionMenuRef.value?.handleKeydown(event)) return
  if (event.key !== 'Enter') return
  const plainEnter = !event.shiftKey && !event.altKey && !event.metaKey && !event.ctrlKey
  if (!plainEnter && !event.ctrlKey) return
  event.preventDefault()
  handleSubmit()
}

function updateDocumentMention(textarea: HTMLTextAreaElement) {
  if (!runtimeEnabled.value) {
    closeDocumentMention()
    return
  }
  const caret = textarea.selectionStart ?? textarea.value.length
  const beforeCaret = textarea.value.slice(0, caret)
  const lineStart = beforeCaret.lastIndexOf('\n') + 1
  const currentLine = beforeCaret.slice(lineStart)
  const hashOffset = currentLine.lastIndexOf('#')
  if (hashOffset < 0) {
    closeDocumentMention()
    return
  }
  const query = currentLine.slice(hashOffset + 1)
  if (query.length > 120 || (query.startsWith('[') && query.includes(']'))) {
    closeDocumentMention()
    return
  }
  documentMentionStart.value = lineStart + hashOffset
  documentMentionEnd.value = caret
  documentMentionQuery.value = query
  documentMentionOpen.value = true
}

function refreshDocumentMention(event: MouseEvent) {
  updateDocumentMention(event.target as HTMLTextAreaElement)
}

function closeDocumentMention() {
  documentMentionOpen.value = false
  documentMentionQuery.value = ''
}

function insertDocumentReference(file: { name: string; absolute_path: string }) {
  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const start = Math.min(documentMentionStart.value, value.length)
  const end = Math.min(Math.max(documentMentionEnd.value, start), value.length)
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
  const nextValue = `${before}${inserted}${after}`
  const caret = before.length + inserted.length
  emit('update:modelValue', nextValue)
  documentReferences.value.push({
    id: crypto.randomUUID(),
    marker,
    name,
    absolutePath: file.absolute_path,
  })
  closeDocumentMention()
  nextTick(() => {
    textareaRef.value?.focus()
    textareaRef.value?.setSelectionRange(caret, caret)
    resizeTextarea()
  })
}

function removeDocumentReference(id: string) {
  const reference = documentReferences.value.find((item) => item.id === id)
  if (!reference) return
  const value = props.modelValue
  const nextValue = value.split(reference.marker).join('')
  documentReferences.value = documentReferences.value.filter((item) => item.id !== id)
  if (nextValue !== value) emit('update:modelValue', nextValue)
  nextTick(() => {
    textareaRef.value?.focus()
    resizeTextarea()
  })
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
    message.warning('上一批图片正在保存，请稍后再粘贴')
    return
  }

  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const start = textarea?.selectionStart ?? value.length
  const end = textarea?.selectionEnd ?? start
  const pastedText = event.clipboardData.getData('text/plain')
  const pastedHtml = event.clipboardData.getData('text/html')
  event.preventDefault()
  let insertedImageMarkers = false
  try {
    const attachments = addImages(files)
    insertPastedContent(
      pastedText,
      pastedHtml,
      attachments.map((attachment) => attachment.marker),
      start,
      end,
    )
    insertedImageMarkers = true
    await uploadAttachmentsToTask(props.taskUuid)
    const resolved = resolveAttachmentMarkdown(
      textareaRef.value?.value ?? props.modelValue,
      textareaRef.value?.selectionStart,
    )
    emit('update:modelValue', resolved.content)
    clearImageAttachments()
    await nextTick()
    textareaRef.value?.focus()
    textareaRef.value?.setSelectionRange(resolved.caret, resolved.caret)
    resizeTextarea()
  } catch (error) {
    if (insertedImageMarkers) {
      const value = textareaRef.value?.value ?? props.modelValue
      emit('update:modelValue', removeAttachmentMarkers(value))
    }
    clearImageAttachments()
    message.error(error instanceof Error ? error.message : '图片粘贴失败')
  }
}

async function handleFileSelection(event: Event) {
  const input = event.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  if (!files.length) return
  if (isSavingPastedImages.value) {
    message.warning('上一批附件正在保存，请稍后再选择')
    return
  }

  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const start = textarea?.selectionStart ?? value.length
  const end = textarea?.selectionEnd ?? start
  let insertedAttachmentMarkers = false
  try {
    const attachments = addFiles(files)
    insertAttachmentMarkers(
      attachments.map((attachment) => attachment.marker),
      start,
      end,
    )
    insertedAttachmentMarkers = true
    await uploadAttachmentsToTask(props.taskUuid)
    const resolved = resolveAttachmentMarkdown(
      textareaRef.value?.value ?? props.modelValue,
      textareaRef.value?.selectionStart,
    )
    emit('update:modelValue', resolved.content)
    clearImageAttachments()
    await nextTick()
    textareaRef.value?.focus()
    textareaRef.value?.setSelectionRange(resolved.caret, resolved.caret)
    resizeTextarea()
  } catch (error) {
    if (insertedAttachmentMarkers) {
      emit('update:modelValue', removeAttachmentMarkers(textareaRef.value?.value ?? props.modelValue))
    }
    clearImageAttachments()
    message.error(error instanceof Error ? error.message : '附件上传失败')
  }
}

function insertAttachmentMarkers(markers: string[], start: number, end: number) {
  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const insertStart = Math.min(start, value.length)
  const insertEnd = Math.min(Math.max(end, insertStart), value.length)
  const before = value.slice(0, insertStart)
  const after = value.slice(insertEnd)
  const markerContent = markers.join('\n')
  const leadingBreak = before && !before.endsWith('\n') ? '\n' : ''
  const trailingBreak = after && !after.startsWith('\n') ? '\n' : ''
  const inserted = `${leadingBreak}${markerContent}${trailingBreak}`
  const nextValue = `${before}${inserted}${after}`
  const caret = insertStart + inserted.length
  emit('update:modelValue', nextValue)
  syncDocumentReferences(nextValue)
  nextTick(() => {
    textareaRef.value?.focus()
    textareaRef.value?.setSelectionRange(caret, caret)
    resizeTextarea()
  })
}

function insertPastedContent(
  text: string,
  html: string,
  markers: string[],
  start: number,
  end: number,
) {
  const textarea = textareaRef.value
  const value = textarea?.value ?? props.modelValue
  const insertStart = Math.min(start, value.length)
  const insertEnd = Math.min(Math.max(end, insertStart), value.length)
  const inserted = formatPastedImageContent(
    text,
    html,
    markers,
    value.slice(0, insertStart),
    value.slice(insertEnd),
  )
  const nextValue = `${value.slice(0, insertStart)}${inserted}${value.slice(insertEnd)}`
  const caret = insertStart + inserted.length
  emit('update:modelValue', nextValue)
  syncDocumentReferences(nextValue)
  nextTick(() => {
    const updatedTextarea = textareaRef.value
    if (!updatedTextarea) return
    updatedTextarea.focus()
    updatedTextarea.setSelectionRange(caret, caret)
    updateDocumentMention(updatedTextarea)
    resizeTextarea()
  })
}

function clearPastedImagesFromInput() {
  const nextValue = removeAttachmentMarkers(props.modelValue)
  clearImageAttachments()
  if (nextValue === props.modelValue) return
  emit('update:modelValue', nextValue)
  syncDocumentReferences(nextValue)
  nextTick(resizeTextarea)
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
  preparingSubmission.value = true
  try {
    const promptContent = serializeDocumentReferences(props.modelValue)
    emit('submit', {
      content: promptContent.trim(),
      display_content: promptContent.trim(),
      config: {
        cli_type: selectedCliType.value,
        model_name: selectedModelName.value,
      },
    })
    await nextTick()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '准备消息失败')
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
      message.warning(error instanceof Error ? error.message : '执行配置加载失败')
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
      message.warning(error instanceof Error ? error.message : '模型列表加载失败')
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
    clearPastedImagesFromInput()
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
  () => {
    syncDocumentReferences(props.modelValue)
    nextTick(resizeTextarea)
  },
)

function resetAfterSubmit() {
  closeDocumentMention()
  documentReferences.value = []
  documentReferenceSequence = 0
  clearPastedImagesFromInput()
}

defineExpose({ resetAfterSubmit })
</script>

<style scoped>
.chat-composer {
  position: relative;
  flex: 0 0 auto;
  background: #fff;
  padding: 8px 22px;
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
.composer-shell textarea {
  display: block;
  width: 100%;
  height: 66px;
  min-height: 66px;
  max-height: 212px;
  overflow-y: hidden;
  resize: none;
  border: 0;
  padding: 0;
  outline: 0;
  color: #262626;
  background: #fff;
  font-family: 'PingFang SC', sans-serif;
  font-size: 14px;
  line-height: 22px;
}
.composer-shell textarea::placeholder {
  color: rgba(0, 0, 0, 0.25);
}
.composer-shell textarea:disabled {
  color: #9ca3af;
  background: #f7f8fa;
  cursor: not-allowed;
  opacity: 1;
  -webkit-text-fill-color: #9ca3af;
}
.composer-shell textarea:disabled::placeholder {
  color: #9ca3af;
  opacity: 1;
}
.reference-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}
.reference-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 4px 10px;
  border: 1px solid #d8dde5;
  border-radius: 999px;
  color: #3157e2;
  background: #e5efff;
  cursor: pointer;
  font-size: 12px;
  line-height: 18px;
}
.reference-chip:hover {
  border-color: #b8c7f5;
  background: #dce8ff;
}
.reference-chip-icon {
  flex: 0 0 auto;
  font-size: 12px;
}
.reference-chip-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.reference-chip-close {
  flex: 0 0 auto;
  font-size: 11px;
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
  font-family: Inter, 'PingFang SC', sans-serif;
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
.send-button-icon {
  width: 16px;
  height: 16px;
}
.composer-hint {
  margin-top: 6px;
  font-size: 11px;
  color: #9ca3af;
  line-height: 1.4;
}
@media (max-width: 720px) {
  .chat-composer {
    padding: 8px 16px;
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
</style>
