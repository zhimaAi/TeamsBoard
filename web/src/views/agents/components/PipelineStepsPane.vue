<template>
  <section class="step-pane">
    <header class="pane-header">
      <div class="header-copy">
        <span class="header-eyebrow">
          <img
            :src="agentOrchestrationIcon"
            alt=""
            aria-hidden="true"
          />
          专家流水线
        </span>
        <h3>{{ pipeline?.name || '请选择流水线' }}</h3>
      </div>
      <div
        v-if="pipeline"
        class="header-actions"
      >
        <a-button
          v-if="sortedSteps.length"
          size="small"
          :class="{ 'batch-cancel-button': batchMode }"
          :disabled="batchSaving"
          @click="toggleBatchMode"
        >
          <template
            v-if="batchMode"
            #icon
          >
            <img
              class="batch-cancel-icon"
              :src="batchCancelIcon"
              alt=""
              aria-hidden="true"
            />
          </template>
          {{ batchMode ? '取消批量配置' : '批量配置' }}
        </a-button>
        <a-dropdown
          v-if="!isCloudPipeline && !batchMode"
          :open="addMenuOpen"
          :trigger="['click']"
          @update:open="addMenuOpen = $event"
        >
          <a-button
            type="primary"
            size="small"
            class="add-button"
            aria-haspopup="menu"
            :aria-expanded="addMenuOpen"
          >
            <template #icon><PlusOutlined /></template>
            添加
          </a-button>
          <template #overlay>
            <a-menu @click="handleAddMenuClick">
              <a-menu-item key="create">
                <img
                  class="add-menu-icon add-menu-icon--new"
                  :src="pipelineAddAgentIcon"
                  alt=""
                  aria-hidden="true"
                />
                新建 Agent
              </a-menu-item>
              <a-menu-item key="copy">
                <img
                  class="add-menu-icon"
                  :src="pipelineSelectAgentIcon"
                  alt=""
                  aria-hidden="true"
                />
                从已添加 Agent 选择
              </a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
      </div>
    </header>
    <div
      class="step-list"
      :class="{ 'step-list--batch': batchMode }"
    >
      <template
        v-for="(step, index) in sortedSteps"
        :key="step.uuid"
      >
        <PipelineStepCard
          :step="step"
          :index="index"
          :total="sortedSteps.length"
          :is-cloud-pipeline="isCloudPipeline"
          :batch-mode="batchMode"
          :selected="selectedStepUuids.includes(step.uuid)"
          @edit="emit('edit-agent', step)"
          @move="emit('move-agent', step, $event)"
          @remove="emit('remove-agent', step)"
          @toggle="toggleStepSelection(step.uuid)"
        />
      </template>
      <a-empty
        v-if="pipeline && !sortedSteps.length"
        description="当前流水线暂无 Agent，点击上方“添加”开始编排"
      />
      <a-empty
        v-else-if="!pipeline"
        description="请选择左侧流水线"
      />
    </div>
    <div
      v-if="pipeline && batchMode"
      class="batch-bar"
      role="toolbar"
      aria-label="批量配置 CLI"
    >
      <span class="batch-count">
        <img
          class="batch-count-icon"
          :src="batchSelectedCountIcon"
          alt=""
          aria-hidden="true"
        />
        已选 {{ selectedStepUuids.length }} 个
      </span>
      <a-select
        v-model:value="batchCliType"
        class="batch-select"
        :loading="cliLoading"
        :disabled="batchSaving"
        placeholder="请选择 CLI"
        @change="handleCliChange(String($event))"
      >
        <a-select-option
          v-for="cli in cliOptions"
          :key="cli.type"
          :value="cli.type"
          :disabled="!cli.installed"
        >
          {{ cli.name }}
        </a-select-option>
      </a-select>
      <a-select
        v-model:value="batchModelName"
        class="batch-select"
        :loading="modelLoading"
        :disabled="!batchCliType || batchSaving"
        show-search
        :placeholder="batchCliType ? '请选择模型' : '请先选择 CLI'"
      >
        <a-select-option
          v-for="model in modelOptions"
          :key="model"
          :value="model"
        >
          {{ model }}
        </a-select-option>
      </a-select>
      <a-button
        type="primary"
        size="small"
        class="batch-apply"
        :loading="batchSaving"
        :disabled="!canApplyBatch"
        @click="applyBatchConfiguration"
      >
        应用
      </a-button>
      <a-button
        size="small"
        class="batch-clear"
        :disabled="batchSaving"
        @click="clearBatchSelection"
      >
        取消
      </a-button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onDeactivated, ref, watch } from 'vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import apiClient from '@/api/client'
import { useCliModelOptions } from '@/composables/useCliModelOptions'
import type { Pipeline, PipelineStep } from '@/types/pipeline'
import agentOrchestrationIcon from '@/assets/icons/agent-orchestration.svg'
import batchCancelIcon from '@/assets/icons/batch-cancel.svg'
import batchSelectedCountIcon from '@/assets/icons/batch-selected-count.svg'
import pipelineAddAgentIcon from '@/assets/icons/pipeline-add-agent.svg'
import pipelineSelectAgentIcon from '@/assets/icons/pipeline-select-agent.svg'
import { isPipelineCloud as isCloudPipelineSource, sortPipelineSteps } from './agentPipeline'
import PipelineStepCard from './PipelineStepCard.vue'

const props = defineProps<{
  pipeline?: Pipeline
}>()

const emit = defineEmits<{
  'create-agent': []
  'copy-agent': []
  'edit-agent': [step: PipelineStep]
  'move-agent': [step: PipelineStep, offset: number]
  'remove-agent': [step: PipelineStep]
  updated: [pipeline: Pipeline]
}>()

const isCloudPipeline = computed(() => isCloudPipelineSource(props.pipeline))
const sortedSteps = computed(() => sortPipelineSteps(props.pipeline?.steps || []))
const addMenuOpen = ref(false)
const batchMode = ref(false)
const batchSaving = ref(false)
const selectedStepUuids = ref<string[]>([])
const batchCliType = ref('')
const batchModelName = ref('')
const {
  cliLoading,
  cliOptions,
  loadCliOptions,
  loadModelOptions,
  modelLoading,
  modelOptions,
  resetModelOptions,
} = useCliModelOptions()
const canApplyBatch = computed(
  () =>
    selectedStepUuids.value.length > 0 &&
    Boolean(batchCliType.value) &&
    Boolean(batchModelName.value) &&
    !batchSaving.value,
)

watch(
  () => props.pipeline?.uuid,
  () => resetBatchState(),
)
watch(
  () => sortedSteps.value.map((step) => step.uuid).join(','),
  () => {
    const available = new Set(sortedSteps.value.map((step) => step.uuid))
    selectedStepUuids.value = selectedStepUuids.value.filter((stepUuid) => available.has(stepUuid))
  },
)

function handleAddMenuClick({ key }: { key: string | number }) {
  addMenuOpen.value = false
  if (key === 'create') {
    emit('create-agent')
    return
  }
  if (key === 'copy') emit('copy-agent')
}

async function toggleBatchMode() {
  if (batchMode.value) {
    resetBatchState()
    return
  }
  addMenuOpen.value = false
  batchMode.value = true
  try {
    await loadCliOptions()
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'CLI 列表加载失败')
  }
}

function resetBatchState() {
  batchMode.value = false
  batchSaving.value = false
  selectedStepUuids.value = []
  batchCliType.value = ''
  batchModelName.value = ''
  resetModelOptions()
}

function clearBatchSelection() {
  if (batchSaving.value) return
  selectedStepUuids.value = []
  batchCliType.value = ''
  batchModelName.value = ''
  resetModelOptions()
}

function toggleStepSelection(stepUuid: string) {
  if (batchSaving.value) return
  selectedStepUuids.value = selectedStepUuids.value.includes(stepUuid)
    ? selectedStepUuids.value.filter((item) => item !== stepUuid)
    : [...selectedStepUuids.value, stepUuid]
}

async function handleCliChange(cliType: string) {
  batchModelName.value = ''
  try {
    await loadModelOptions(cliType)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '模型列表加载失败')
  }
}

async function applyBatchConfiguration() {
  if (!props.pipeline || !canApplyBatch.value) return
  batchSaving.value = true
  try {
    const updated = await apiClient.put<Pipeline>(
      `/pipelines/${encodeURIComponent(props.pipeline.uuid)}/steps/execution-config`,
      {
        step_uuids: selectedStepUuids.value,
        cli_type: batchCliType.value,
        model_name: batchModelName.value,
      },
    )
    emit('updated', updated)
    message.success(`已更新 ${selectedStepUuids.value.length} 个 Agent 的执行配置`)
    resetBatchState()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '批量配置失败')
  } finally {
    batchSaving.value = false
  }
}

onDeactivated(() => {
  addMenuOpen.value = false
})
</script>

<style scoped>
.step-pane {
  position: relative;
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
}

.pane-header {
  position: relative;
  z-index: 1;
  display: flex;
  min-height: 84px;
  flex: 0 0 84px;
  align-items: center;
  justify-content: space-between;
  padding: 16px 24px;
  background: #fff;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.header-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.header-eyebrow {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #86868b;
  font-size: 12px;
  line-height: 20px;
}

.header-eyebrow img {
  width: 14px;
  height: 14px;
  flex: 0 0 14px;
}

.pane-header h3 {
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pane-header :deep(.ant-btn) {
  height: 32px;
  padding-inline: 16px;
  border-radius: 6px;
  box-shadow: none;
  font-size: 14px;
}

.pane-header :deep(.ant-btn-primary) {
  border-color: #3157e2;
  background: #3157e2;
}

.pane-header :deep(.ant-btn-primary:hover) {
  border-color: #2475fc;
  background: #2475fc;
}

.pane-header :deep(.ant-btn-primary:active) {
  border-color: #3157e2;
  background: #3157e2;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pane-header :deep(.batch-cancel-button.ant-btn) {
  display: inline-flex;
  align-items: center;
  color: #3157e2;
  border-color: transparent;
  background: #e5efff;
}

.pane-header :deep(.batch-cancel-button.ant-btn:hover:not(:disabled)) {
  color: #2475fc;
  border-color: transparent;
  background: #dce8ff;
}

.batch-cancel-icon {
  width: 16px;
  height: 16px;
}

.batch-bar {
  position: absolute;
  z-index: 2;
  right: 24px;
  bottom: 24px;
  left: 24px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 12px;
  border: 1px solid #abcafc;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.18);
}

.batch-count {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  color: #595959;
  font-size: 14px;
  line-height: 22px;
  white-space: nowrap;
}

.batch-count-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
}

.batch-select {
  min-width: 0;
  flex: 1 1 180px;
}

.batch-apply,
.batch-clear {
  flex: 0 0 auto;
}

.batch-bar :deep(.batch-apply.ant-btn-primary:disabled) {
  color: #fff;
  border-color: #a7c8fe;
  background: #a7c8fe;
}

.step-list {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 0;
  scrollbar-color: #d8dde5 transparent;
  scrollbar-width: thin;
}

.step-list--batch {
  padding: 0 0 96px;
}

.add-menu-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
  vertical-align: -3px;
}

.add-menu-icon--new {
  box-sizing: border-box;
  padding: 2.35px;
}

@media (max-width: 900px) {
  .step-pane {
    height: auto;
    min-height: 420px;
  }

  .batch-bar {
    right: 16px;
    bottom: 16px;
    left: 16px;
    flex-wrap: wrap;
  }

  .batch-select {
    min-width: 160px;
  }
}
</style>
