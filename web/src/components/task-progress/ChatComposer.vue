<template>
  <div class="chat-composer">
    <div
      class="composer-shell"
      :class="{ 'is-disabled': !canAsk }"
    >
      <textarea
        ref="textareaRef"
        :value="modelValue"
        rows="3"
        :disabled="!canAsk"
        :placeholder="placeholder"
        @input="handleInput"
        @keydown.enter.exact.prevent="handleSubmit"
        @keydown.ctrl.enter.prevent="handleSubmit"
      />
      <div class="composer-toolbar">
        <div class="composer-tools">
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
          <button
            type="button"
            class="tool-button"
            :disabled="!taskUuid || !currentStep"
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
            type="button"
            class="send-button"
            :disabled="!canAsk || !modelValue.trim()"
            title="发送消息"
            @click="handleSubmit"
          >
            <LoadingOutlined
              v-if="submitting"
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
import { nextTick, onMounted, ref, watch } from 'vue'
import { FolderOpenOutlined, LoadingOutlined } from '@ant-design/icons-vue'
import agentIcon from '@/assets/icons/task-composer-agent.svg'
import documentIcon from '@/assets/icons/task-composer-document.svg'
import sendIcon from '@/assets/icons/task-composer-send.svg'
import type { PipelineStep } from '@/types/pipeline'
import AgentPromptDrawer from './AgentPromptDrawer.vue'
import TaskDocumentsDrawer from './TaskDocumentsDrawer.vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    canAsk: boolean
    submitting?: boolean
    placeholder: string
    contextText: string
    taskUuid: string
    currentStep?: PipelineStep
    showWorkDirectory?: boolean
    workDirectory?: string
  }>(),
  { submitting: false, showWorkDirectory: false, workDirectory: '' },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  submit: []
  'prompt-saved': []
  'select-work-directory': []
}>()

const agentPromptOpen = ref(false)
const taskDocumentsOpen = ref(false)
const textareaRef = ref<HTMLTextAreaElement>()
const COMPOSER_MIN_HEIGHT = 66
const COMPOSER_MAX_HEIGHT = 212

function handleInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
  resizeTextarea()
}

function resizeTextarea() {
  const textarea = textareaRef.value
  if (!textarea) return

  textarea.style.height = `${COMPOSER_MIN_HEIGHT}px`
  const height = Math.min(Math.max(textarea.scrollHeight, COMPOSER_MIN_HEIGHT), COMPOSER_MAX_HEIGHT)
  textarea.style.height = `${height}px`
  textarea.style.overflowY = textarea.scrollHeight > COMPOSER_MAX_HEIGHT ? 'auto' : 'hidden'
}

function handleSubmit() {
  if (!props.canAsk || props.submitting || !props.modelValue.trim()) return
  emit('submit')
}

onMounted(resizeTextarea)

watch(() => props.modelValue, () => {
  nextTick(resizeTextarea)
})

</script>

<style scoped>
.chat-composer {
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
  font-family: "PingFang SC", sans-serif;
  font-size: 14px;
  line-height: 22px;
}
.composer-shell textarea::placeholder {
  color: rgba(0, 0, 0, 0.25);
}
.composer-shell.is-disabled textarea {
  color: #9ca3af;
  background: #f7f8fa;
  cursor: not-allowed;
  opacity: 1;
  -webkit-text-fill-color: #9ca3af;
}
.composer-shell.is-disabled textarea::placeholder {
  color: #9ca3af;
  opacity: 1;
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
  font-family: Inter, "PingFang SC", sans-serif;
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
