<template>
  <a-modal
    :open="open"
    width="520px"
    :closable="false"
    :footer="null"
    :mask-closable="false"
    wrap-class-name="assign-pipeline-modal"
    @cancel="close"
  >
    <a-spin :spinning="pipelinesLoading">
      <div class="assign-modal">
        <button
          type="button"
          class="assign-modal__close"
          aria-label="关闭"
          @click="close"
        >
          <img
            :src="closeIcon"
            alt=""
          />
        </button>

        <h2>
          {{
            mode === 'create'
              ? '完善流水线配置'
              : mode === 'board'
                ? '指定流水线'
                : '分配流水线'
          }}
        </h2>
        <p class="assign-hint">
          <template v-if="mode === 'create'">
            当前流水线配置不完整，请选择一个已完成 CLI 和模型配置的流水线，或前往配置：
          </template>
          <template v-else>
            任务「{{ taskTitle }}」尚未指定流水线，{{
              mode === 'board' ? '设为进行中前请先选择流水线' : '分配后任务将自动变为进行中'
            }}：
          </template>
        </p>

        <div
          v-if="pipelines.length"
          class="pipeline-options"
          role="radiogroup"
          aria-label="可分配的流水线"
        >
          <div
            v-for="pipeline in pipelines"
            :key="pipeline.uuid"
            class="pipeline-option"
            :class="{
              'pipeline-option--disabled': !isPipelineConfigured(pipeline),
            }"
          >
            <label class="pipeline-option__choice">
              <input
                v-model="selectedPipelineUuid"
                type="radio"
                name="assign-pipeline"
                :value="pipeline.uuid"
                :disabled="!isPipelineConfigured(pipeline)"
              />
              <span class="pipeline-option__avatar">
                <img
                  v-if="pipeline.avatar"
                  :src="pipeline.avatar"
                  alt=""
                />
                <span v-else>{{ pipeline.name.slice(0, 1) }}</span>
              </span>
              <span
                class="pipeline-option__name"
                :title="pipeline.name"
              >
                {{ pipeline.name }}
              </span>
              <CheckCircleFilled
                v-if="selectedPipelineUuid === pipeline.uuid"
                class="pipeline-option__selected-icon"
                aria-hidden="true"
              />
            </label>

            <template v-if="!isPipelineConfigured(pipeline)">
              <a-tooltip>
                <template #title>
                  <b>{{ pipelineConfigWarningTitle(pipeline) }}</b><br />请先完成配置
                </template>
                <span
                  class="pipeline-option__warning"
                  aria-label="流水线尚未配置完整"
                  tabindex="0"
                >
                  <ExclamationCircleFilled aria-hidden="true" />
                </span>
              </a-tooltip>
              <a-button
                class="pipeline-option__config"
                @click="goToPipelineConfig(pipeline.uuid)"
              >
                去配置
                <RightOutlined />
              </a-button>
            </template>
          </div>
        </div>
        <div
          v-else-if="!pipelinesLoading"
          class="pipeline-empty"
        >
          暂无流水线，请先前往 Agent 流水线管理页创建并配置。
        </div>

        <footer class="assign-footer">
          <a-button @click="close">取消</a-button>
          <a-button
            type="primary"
            :disabled="!canConfirm"
            :loading="starting"
            @click="confirm"
          >
            {{ mode === 'create' ? '继续创建' : '确定' }}
          </a-button>
        </footer>
      </div>
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { message } from 'ant-design-vue'
import {
  CheckCircleFilled,
  ExclamationCircleFilled,
  RightOutlined,
} from '@ant-design/icons-vue'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'
import closeIcon from '@/assets/icons/task-detail-close.svg'
import { usePipelineStore } from '@/stores/pipeline'
import type { Pipeline } from '@/types/pipeline'

const props = withDefaults(
  defineProps<{
    open: boolean
    taskUuid?: string
    taskTitle?: string
    preferredPipelineUuid?: string
    mode?: 'detail' | 'board' | 'create'
  }>(),
  { taskTitle: '', preferredPipelineUuid: '', mode: 'detail' },
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  assigned: []
  selected: [pipeline: Pipeline]
}>()
const router = useRouter()
const pipelineStore = usePipelineStore()
const { pipelines, loading: pipelinesLoading } = storeToRefs(pipelineStore)

const starting = ref(false)
const selectedPipelineUuid = ref('')

const canConfirm = computed(() =>
  pipelines.value.some(
    (pipeline) =>
      pipeline.uuid === selectedPipelineUuid.value && isPipelineConfigured(pipeline),
  ),
)

function isPipelineConfigured(pipeline: Pipeline) {
  const steps = pipeline.steps || []
  return (
    steps.length > 0 &&
    steps.every((step) => Boolean(step.cli_type && (step.model_name || step.model)))
  )
}

function close() {
  emit('update:open', false)
}

async function load() {
  selectedPipelineUuid.value = ''
  try {
    await pipelineStore.loadPipelines(true)
    const configuredPipelines = pipelines.value.filter(isPipelineConfigured)
    const preferred = configuredPipelines.find(
      (pipeline) => pipeline.uuid === props.preferredPipelineUuid,
    )
    selectedPipelineUuid.value = preferred?.uuid || configuredPipelines[0]?.uuid || ''
  } catch (error) {
    message.error(error instanceof Error ? error.message : '流水线加载失败')
  }
}

function goToPipelineConfig(pipelineUuid: string) {
  close()
  void router.push({ name: 'agents', query: { pipeline: pipelineUuid } })
}

function pipelineConfigWarningTitle(pipeline: Pipeline) {
  const steps = pipeline.steps || []
  return steps.length ? '有 Agent 未配置 CLI 或模型' : '流水线尚未添加 Agent'
}

async function confirm() {
  if (!canConfirm.value || starting.value) return
  const pipeline = pipelines.value.find((item) => item.uuid === selectedPipelineUuid.value)
  if (!pipeline) return
  if (props.mode === 'create') {
    emit('selected', pipeline)
    close()
    return
  }
  if (!props.taskUuid) return message.error('任务信息缺失，无法分配流水线')
  starting.value = true
  try {
    await apiClient.post('/tasks/' + props.taskUuid + '/assign-pipeline', {
      pipeline_uuid: selectedPipelineUuid.value,
    })
    message.success('流水线已分配，任务已设为进行中并启动')
    emit('assigned')
    close()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '流水线分配失败')
  } finally {
    starting.value = false
  }
}

watch(
  pipelines,
  (items) => {
    if (!items.some((item) => item.uuid === selectedPipelineUuid.value)) {
      selectedPipelineUuid.value =
        items.find(isPipelineConfigured)?.uuid || ''
    }
  },
)

watch(
  () => props.open,
  (open) => {
    if (open) void load()
  },
)
</script>

<style scoped>
.assign-modal {
  position: relative;
  overflow: hidden;
  border-radius: 24px;
  color: #262626;
  background: #fff;
}

.assign-modal__close {
  position: absolute;
  z-index: 1;
  top: 12px;
  right: 12px;
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
}

.assign-modal__close img {
  width: 12px;
  height: 12px;
}

.assign-modal h2 {
  margin: 0;
  padding: 24px 32px 0;
  color: #262626;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
}

.assign-hint {
  margin: 24px 32px 8px 30px;
  color: #595959;
  font-size: 14px;
  line-height: 22px;
}

.pipeline-options {
  max-height: 220px;
  margin: 0 32px 0 30px;
  overflow-y: auto;
  border: 1px solid #d9d9d9;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.pipeline-option {
  position: relative;
  display: flex;
  min-height: 72px;
  align-items: center;
  gap: 12px;
  margin: 0 24px;
}

.pipeline-option:not(:last-child) {
  border-bottom: 1px solid #f0f0f0;
}

.pipeline-option__choice {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 12px;
  cursor: pointer;
}

.pipeline-option__choice input {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  opacity: 0;
  pointer-events: none;
}

.pipeline-option__choice:focus-within {
  border-radius: 8px;
  outline: 2px solid rgba(49, 87, 226, 0.24);
  outline-offset: 3px;
}

.pipeline-option__avatar {
  display: flex;
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 8px;
  color: #3a4559;
  background: #e4e6eb;
  font-size: 14px;
  font-weight: 600;
}

.pipeline-option__avatar img {
  width: 16px;
  height: 16px;
  object-fit: contain;
}

.pipeline-option__name {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pipeline-option__selected-icon {
  flex: 0 0 auto;
  color: #262626;
  font-size: 16px;
}

.pipeline-option--disabled .pipeline-option__choice {
  cursor: not-allowed;
}

.pipeline-option--disabled .pipeline-option__name {
  color: #bfbfbf;
}

.pipeline-option__warning {
  display: flex;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  align-items: center;
  justify-content: center;
  padding: 0;
  color: #ff7a45;
  cursor: pointer;
  font-size: 16px;
}

.pipeline-option__config {
  display: inline-flex;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  padding: 4px 16px;
  border-radius: 6px;
  color: #595959;
  font-size: 14px;
  line-height: 22px;
}

.pipeline-option__config :deep(.anticon) {
  font-size: 12px;
}

.pipeline-empty {
  display: flex;
  min-height: 144px;
  align-items: center;
  justify-content: center;
  margin: 0 32px 0 30px;
  padding: 24px;
  border: 1px solid #d9d9d9;
  border-radius: 18px;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
  text-align: center;
}

.assign-footer {
  display: flex;
  height: 72px;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 0 32px;
}

.assign-footer :deep(.ant-btn) {
  height: 32px;
  padding: 4px 16px;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

.assign-footer :deep(.ant-btn-primary) {
  background: #3157e2;
}

:global(.assign-pipeline-modal .ant-modal) {
  max-width: calc(100vw - 32px);
}

:global(.assign-pipeline-modal .ant-modal-content) {
  overflow: hidden;
  padding: 0;
  border-radius: 24px;
}

:global(.assign-pipeline-modal .ant-modal-body) {
  padding: 0;
}

@media (max-width: 560px) {
  .assign-modal h2 {
    padding-right: 48px;
  }

  .pipeline-option {
    margin: 0 16px;
  }

  .pipeline-option__config {
    padding-inline: 10px;
  }
}
</style>
