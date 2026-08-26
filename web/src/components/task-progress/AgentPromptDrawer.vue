<template>
  <a-drawer
    :open="open"
    width="min(922px, 100vw)"
    :closable="false"
    :mask-closable="false"
    :body-style="{ padding: 0, overflow: 'hidden' }"
    root-class-name="agent-prompt-drawer"
    @close="requestClose"
  >
    <div class="prompt-drawer-shell">
      <header class="prompt-drawer-header">
        <div class="step-heading">
          <span class="step-avatar">
            <img
              v-if="displayStep?.avatar"
              :src="displayStep.avatar"
              :alt="`${displayStep.name}头像`"
            />
            <span v-else>{{ initials(displayStep?.name) }}</span>
          </span>
          <h2>{{ displayStep?.name || '当前步骤' }}</h2>
        </div>
        <button
          type="button"
          class="close-button"
          aria-label="关闭 Agent 提示词"
          @click="requestClose"
        >
          <CloseOutlined />
        </button>
      </header>

      <div class="prompt-toolbar">
        <label for="agent-prompt-editor">
          <img
            :src="agentIcon"
            alt=""
            aria-hidden="true"
          />
          提示词（System Prompt）
        </label>
        <a-button
          type="primary"
          :loading="saving"
          :disabled="loading || !loadedStep || !draftPrompt.trim() || !dirty"
          @click="savePrompt"
        >
          保存
        </a-button>
      </div>

      <div class="prompt-content">
        <div
          v-if="loading"
          class="prompt-state"
        >
          <a-spin />
        </div>
        <div
          v-else-if="loadError"
          class="prompt-state prompt-error"
        >
          <a-alert
            type="error"
            show-icon
            :message="loadError"
          />
          <a-button @click="loadPrompt">重试</a-button>
        </div>
        <a-textarea
          v-else
          id="agent-prompt-editor"
          v-model:value="draftPrompt"
          class="prompt-editor"
          :disabled="saving || !loadedStep"
          aria-label="当前步骤 Agent 提示词"
        />
      </div>
    </div>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { CloseOutlined } from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import agentIcon from '@/assets/icons/task-composer-agent.svg'
import type { PipelineStep } from '@/types/pipeline'
import type { TaskWithDetails } from '@/types/task-detail'
import { initials } from './utils'

interface PromptUpdateResponse {
  prompt_updated: boolean
  session_uuid?: string
  session_error?: string
  execution_deferred?: boolean
}

const props = defineProps<{
  open: boolean
  taskUuid: string
  currentStep?: PipelineStep
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  saved: []
}>()

const loadedStep = ref<PipelineStep>()
const draftPrompt = ref('')
const savedPrompt = ref('')
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
let loadVersion = 0

const displayStep = computed(() => loadedStep.value || props.currentStep)
const dirty = computed(() => draftPrompt.value !== savedPrompt.value)

watch(
  () => [props.open, props.taskUuid, props.currentStep?.uuid] as const,
  ([open]) => {
    if (open) void loadPrompt()
    else resetState()
  },
)

async function loadPrompt() {
  if (!props.taskUuid) {
    loadError.value = '缺少任务信息，无法加载 Agent 提示词'
    return
  }
  const requestVersion = ++loadVersion
  loading.value = true
  loadError.value = ''
  try {
    const task = await apiClient.get<TaskWithDetails>(
      `/tasks/${encodeURIComponent(props.taskUuid)}`,
    )
    if (requestVersion !== loadVersion) return
    const currentStepUuid = task.current_step_uuid || props.currentStep?.uuid || ''
    const step = task.steps?.find((item) => item.uuid === currentStepUuid)
    if (!step) throw new Error('当前任务没有可编辑的执行步骤')
    loadedStep.value = step
    draftPrompt.value = step.prompt_snapshot || ''
    savedPrompt.value = draftPrompt.value
  } catch (error) {
    if (requestVersion !== loadVersion) return
    loadedStep.value = undefined
    loadError.value = error instanceof Error ? error.message : 'Agent 提示词加载失败'
  } finally {
    if (requestVersion === loadVersion) loading.value = false
  }
}

async function savePrompt() {
  const step = loadedStep.value
  const prompt = draftPrompt.value.trim()
  if (!step || !props.taskUuid || saving.value || !dirty.value) return
  if (!prompt) {
    message.error('提示词不能为空')
    return
  }
  saving.value = true
  try {
    const result = await apiClient.put<PromptUpdateResponse>(
      `/tasks/${encodeURIComponent(props.taskUuid)}/steps/${encodeURIComponent(step.uuid)}/prompt`,
      { prompt_snapshot: draftPrompt.value },
    )
    savedPrompt.value = draftPrompt.value
    loadedStep.value = { ...step, prompt_snapshot: draftPrompt.value }
    if (result.session_error) {
      message.warning(`提示词已保存，但重新执行失败：${result.session_error}`)
    } else if (result.execution_deferred) {
      message.success('提示词已保存，将在流程推进到该步骤时生效')
    } else {
      message.success('提示词已保存，当前步骤将重新执行')
    }
    emit('saved')
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'Agent 提示词保存失败')
  } finally {
    saving.value = false
  }
}

function requestClose() {
  if (!dirty.value || saving.value) {
    if (!saving.value) emit('update:open', false)
    return
  }
  Modal.confirm({
    title: '放弃未保存的提示词修改？',
    content: '关闭后，本次尚未保存的提示词内容将丢失。',
    okText: '放弃修改',
    cancelText: '继续编辑',
    onOk: () => emit('update:open', false),
  })
}

function resetState() {
  loadVersion += 1
  loadedStep.value = undefined
  draftPrompt.value = ''
  savedPrompt.value = ''
  loading.value = false
  saving.value = false
  loadError.value = ''
}
</script>

<style scoped>
.prompt-drawer-shell {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  background: #fff;
}
.prompt-drawer-header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  padding: 26px 32px 18px;
}
.step-heading {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}
.step-avatar {
  display: inline-flex;
  overflow: hidden;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #3157e2;
  font-size: 16px;
  font-weight: 600;
}
.step-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.step-heading h2 {
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.close-button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-size: 20px;
}
.close-button:hover,
.close-button:focus-visible {
  background: #f3f4f6;
}
.close-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}
.prompt-toolbar {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 32px 8px;
}
.prompt-toolbar label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}
.prompt-toolbar label img {
  width: 16px;
  height: 16px;
}
.prompt-content {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
  padding: 0 32px 32px;
}
.prompt-editor {
  height: 100%;
  min-height: 320px;
  resize: none;
  border-radius: 18px;
  padding: 16px;
  color: #262626;
  font-family: "PingFang SC", sans-serif;
  font-size: 14px;
  line-height: 26px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}
.prompt-state {
  display: flex;
  width: 100%;
  min-height: 320px;
  align-items: center;
  justify-content: center;
  border: 1px solid #d9d9d9;
  border-radius: 18px;
}
.prompt-error {
  flex-direction: column;
  gap: 16px;
  padding: 24px;
}
:global(.agent-prompt-drawer .ant-drawer-content) {
  overflow: hidden;
  border-radius: 22px 0 0 22px;
}
@media (max-width: 640px) {
  .prompt-drawer-header {
    padding: 20px 16px 16px;
  }
  .step-heading h2 {
    font-size: 20px;
    line-height: 28px;
  }
  .prompt-toolbar {
    padding: 0 16px 8px;
  }
  .prompt-content {
    padding: 0 16px 16px;
  }
}
</style>
