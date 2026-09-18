<template>
  <a-modal
    :open="open"
    class="copy-agent-modal"
    :title="t('agents.chooseAgent')"
    width="730px"
    centered
    @update:open="emit('update:open', $event)"
  >

	<a-spin :spinning="loading">
	<div class="agent-list">
      <div class="agent-list__scroll">
        <button
          v-for="(step, index) in sourceSteps"
          :key="`${step.pipelineName}-${step.uuid}`"
          type="button"
          class="agent-list-item"
          :class="{ active: selectedStepUuid === step.uuid }"
          :aria-pressed="selectedStepUuid === step.uuid"
          @click="selectedStepUuid = step.uuid"
        >
          <img
            class="agent-list-item__avatar"
            :src="resolveAgentAvatar(step, index)"
            alt=""
          />
          <span class="agent-list-item__content">
            <strong>{{ step.name }}</strong>
            <span class="agent-list-item__description">
              {{ step.description || step.prompt || step.prompt_snapshot || t('agents.agentNoDescription') }}
            </span>
            <span class="agent-list-item__tags">
              <span>
                <CodeOutlined />
                {{ step.cli_type || t('agents.cliNotConfigured') }}
              </span>
              <span>
                <DeploymentUnitOutlined />
                {{ stepModel(step) || t('agents.modelNotConfigured') }}
              </span>
            </span>
          </span>
          <span
            v-if="selectedStepUuid === step.uuid"
            class="agent-list-item__selected"
            aria-hidden="true"
          >
            <CheckOutlined />
          </span>
        </button>

        <a-empty
          v-if="sourceSteps.length === 0"
          :description="t('agents.noAgentsToChoose')"
        />
      </div>
    </div>
	</a-spin>

    <template #footer>
      <div class="copy-agent-modal__footer-actions">
        <a-button @click="emit('update:open', false)">{{ t('agents.cancel') }}</a-button>
        <a-button
          type="primary"
          :loading="saving"
          @click="copy"
        >
          {{ t('agents.confirm') }}
        </a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { CheckOutlined, CodeOutlined, DeploymentUnitOutlined } from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import type { ExpertMember, PipelineStep } from '@/types/pipeline'
import {
  resolveAgentAvatar,
  type ReusablePipelineStep,
  type StepInput,
  stepModel,
} from './agentPipeline'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = defineProps<{
  open: boolean
	targetPipelineUuid?: string
	targetExpertGroupUuid?: string
	expertRole?: 'leader' | 'member'
  sourceSteps: ReusablePipelineStep[]
	loading?: boolean
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
	if (!source || (!props.targetPipelineUuid && !props.targetExpertGroupUuid)) {
	  return message.warning(t('agents.chooseOneAgent'))
	}

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
	if (props.targetExpertGroupUuid) {
	  await apiClient.post<ExpertMember>(
		`/expert-groups/${encodeURIComponent(props.targetExpertGroupUuid)}/members`,
		{ ...payload, member_role: props.expertRole || 'member' },
	  )
	} else {
	  await apiClient.post<PipelineStep>(
		`/pipelines/${encodeURIComponent(props.targetPipelineUuid || '')}/steps`,
		payload,
	  )
	}
    emit('update:open', false)
    emit('saved')
    message.success(t('agents.agentCopied'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('agents.agentCopyFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.agent-list {
  overflow: hidden;
  border: 1px solid #d9d9d9;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 2px 24px rgba(0, 0, 0, 0.08);
}

.agent-list__scroll {
  max-height: 627px;
  overflow-y: auto;
  scrollbar-color: #d8dde5 transparent;
  scrollbar-width: thin;
}

.agent-list-item {
  position: relative;
  display: flex;
  width: 100%;
  min-height: 125px;
  align-items: center;
  gap: 16px;
  padding: 24px 56px 24px 24px;
  border: 0;
  border-bottom: 1px solid #f0f0f0;
  color: #262626;
  background: #fff;
  cursor: pointer;
  text-align: left;
  transition: background-color 0.2s ease;
}

.agent-list-item:last-of-type {
  border-bottom: 0;
}

.agent-list-item:hover,
.agent-list-item.active {
  background: #f7f9ff;
}

.agent-list-item:focus-visible {
  z-index: 1;
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}

.agent-list-item__avatar {
  display: block;
  width: 48px;
  height: 48px;
  flex: 0 0 auto;
  border-radius: 50%;
  object-fit: cover;
}

.agent-list-item__content {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.agent-list-item__content strong {
  overflow: hidden;
  color: #1d1d1f;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-list-item__description {
  margin-top: 2px;
  overflow: hidden;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 26px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-list-item__tags {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 5px;
  overflow: hidden;
}

.agent-list-item__tags > span {
  display: inline-flex;
  max-width: 50%;
  height: 26px;
  align-items: center;
  gap: 5px;
  padding: 2px 10px;
  overflow: hidden;
  border-radius: 13px;
  color: #595959;
  background: #f5f5f5;
  font-size: 12px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-list-item__tags :deep(.anticon) {
  flex: 0 0 auto;
  color: #8c8c8c;
  font-size: 14px;
}

.agent-list-item__selected {
  position: absolute;
  top: 50%;
  right: 24px;
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #3157e2;
  font-size: 14px;
  transform: translateY(-50%);
}

.copy-agent-modal__footer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

:deep(.copy-agent-modal) {
  max-width: calc(100vw - 32px);
  padding-bottom: 0;
}

:deep(.copy-agent-modal .ant-modal-content) {
  overflow: hidden;
  padding: 0;
  border-radius: 24px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
}

:deep(.copy-agent-modal .ant-modal-header) {
  margin-bottom: 0;
  padding: 24px;
  border-radius: 24px 24px 0 0;
  background: #fff;
}

:deep(.copy-agent-modal .ant-modal-title) {
  color: #262626;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
}

:deep(.copy-agent-modal .ant-modal-body) {
  max-height: calc(100vh - 144px);
  overflow-y: auto;
  padding: 16px 48px 8px;
}

:deep(.copy-agent-modal .ant-modal-footer) {
  margin-top: 0;
  padding: 16px 48px;
  border-top: 0;
}

:deep(.copy-agent-modal .ant-modal-footer .ant-btn) {
  min-width: 65px;
  height: 32px;
  padding: 5px 16px;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

@media (max-width: 760px) {
  :deep(.copy-agent-modal .ant-modal-header) {
    padding: 20px 24px;
  }

  :deep(.copy-agent-modal .ant-modal-title) {
    font-size: 20px;
    line-height: 28px;
  }

  :deep(.copy-agent-modal .ant-modal-body) {
    padding: 16px 24px 24px;
  }

  :deep(.copy-agent-modal .ant-modal-footer) {
    padding: 16px 24px;
  }

  .agent-list-item {
    padding-right: 48px;
  }

  .agent-list-item__selected {
    right: 16px;
  }
}
</style>
