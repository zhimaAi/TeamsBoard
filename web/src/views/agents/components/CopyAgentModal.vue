<template>
  <a-modal
    :open="open"
    title="选择 Agent"
    width="720px"
    :confirm-loading="saving"
    wrap-class-name="copy-agent-modal"
    @update:open="emit('update:open', $event)"
    @ok="copy"
  >
    <div class="copy-grid-wrapper">
      <div class="copy-grid">
        <button
          v-for="(step, index) in sourceSteps"
          :key="`${step.pipelineName}-${step.uuid}`"
          type="button"
          :class="{ active: selectedStepUuid === step.uuid }"
          :aria-pressed="selectedStepUuid === step.uuid"
          @click="selectedStepUuid = step.uuid"
        >
          <span class="agent-card__header">
            <img
              :src="resolveAgentAvatar(step, index)"
              alt=""
            />
            <span class="agent-card__identity">
              <strong>{{ step.name }}</strong>
              <span class="agent-card__pipeline-name">{{ step.pipelineName }}</span>
            </span>
          </span>
          <span class="agent-card__prompt">
            {{ step.prompt || step.prompt_snapshot || '该 Agent 暂未配置提示词。' }}
          </span>
          <span class="agent-card__tags">
            <span>
              <CodeOutlined />
              {{ step.cli_type || '未配置 CLI' }}
            </span>
            <span>
              <DeploymentUnitOutlined />
              {{ stepModel(step) || '未配置模型' }}
            </span>
          </span>
          <span
            v-if="selectedStepUuid === step.uuid"
            class="agent-card__selected"
            aria-hidden="true"
          >
            <CheckOutlined />
          </span>
        </button>
      </div>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { CheckOutlined, CodeOutlined, DeploymentUnitOutlined } from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import type { PipelineStep } from '@/types/pipeline'
import {
  resolveAgentAvatar,
  type ReusablePipelineStep,
  type StepInput,
  stepModel,
} from './agentPipeline'

const props = defineProps<{
  open: boolean
  targetPipelineUuid: string
  sourceSteps: ReusablePipelineStep[]
}>()

const emit = defineEmits<{
  saved: []
  'update:open': [open: boolean]
}>()

const saving = ref(false)
const selectedStepUuid = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) selectedStepUuid.value = ''
  },
)

async function copy() {
  const source = props.sourceSteps.find((step) => step.uuid === selectedStepUuid.value)
  if (!source || !props.targetPipelineUuid) return message.warning('请选择一个 Agent')

  saving.value = true
  try {
    const payload: StepInput = {
      name: source.name,
      description: source.description || '',
      avatar: source.avatar || '',
      prompt: source.prompt || source.prompt_snapshot || '',
      cli_type: source.cli_type || '',
      model_name: stepModel(source),
    }
    await apiClient.post<PipelineStep>(`/pipelines/${props.targetPipelineUuid}/steps`, payload)
    emit('update:open', false)
    emit('saved')
    message.success('已复制 Agent 配置，副本与原 Agent 互不关联')
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'Agent 复制失败')
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.copy-grid-wrapper {
  padding: 24px 0;
}

.copy-grid {
  display: grid;
  max-height: 440px;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px 14px;
  padding: 1px;
  overflow-y: auto;
}

.copy-grid button {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 172px;
  flex-direction: column;
  align-items: stretch;
  padding: 18px;
  border: 1px solid #e5e7eb;
  border-radius: 14px;
  color: #344054;
  background: #fff;
  cursor: pointer;
  text-align: left;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease;
}

.copy-grid button:hover {
  border-color: #91caff;
  background: #fafcff;
}

.copy-grid button.active {
  border-color: #1677ff;
  background: #f0f7ff;
}

.copy-grid button:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: 2px;
}

.copy-grid img {
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  border-radius: 50%;
  object-fit: cover;
}

.agent-card__header {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
  padding-right: 24px;
}

.agent-card__identity {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.agent-card__identity strong,
.agent-card__pipeline-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-card__identity strong {
  color: #1d2939;
  font-size: 17px;
  font-weight: 500;
  line-height: 24px;
}

.agent-card__identity small {
  color: #98a2b3;
  font-size: 13px;
  line-height: 20px;
}

.agent-card__prompt {
  display: -webkit-box;
  min-height: 44px;
  margin: 10px 0 12px;
  overflow: hidden;
  color: #667085;
  font-size: 14px;
  line-height: 22px;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.agent-card__tags {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: auto;
  flex-wrap: wrap;
}

.agent-card__tags > span {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  overflow: hidden;
  border-radius: 999px;
  color: #667085;
  background: #f5f5f5;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-card__tags :deep(.anticon) {
  flex: 0 0 auto;
  color: #98a2b3;
  font-size: 14px;
}

.agent-card__selected {
  position: absolute;
  top: 10px;
  right: 10px;
  display: flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #1677ff;
  font-size: 14px;
}

@media (max-width: 640px) {
  .copy-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .copy-grid button {
    min-height: 164px;
  }
}
</style>
