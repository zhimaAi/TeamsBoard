<template>
  <a-modal
    :open="open"
    width="448px"
    :title="t(replaceExisting ? 'workflows.task.assign.switchTitle' : 'workflows.task.assign.executionTitle')"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !canConfirm }"
    :cancel-button-props="{ disabled: saving }"
    :ok-text="t(replaceExisting ? 'workflows.task.assign.confirmSwitch' : 'workflows.task.assign.confirmAssign')"
    :cancel-text="t('workflows.task.assign.cancel')"
    :keyboard="!saving"
    :mask-closable="false"
    wrap-class-name="assign-execution-modal"
    @ok="confirm"
    @cancel="close"
  >
    <!-- 需求设计图为纯文本说明，不使用提示条容器 -->
    <I18nT
      tag="p"
      class="assign-description"
      :keypath="
        replaceExisting
          ? 'workflows.task.assign.executionDescriptionSwitch'
          : mode === 'board'
          ? 'workflows.task.assign.executionDescriptionBoard'
          : 'workflows.task.assign.executionDescriptionDetail'
      "
    >
      <template #title>
        <strong>{{ taskTitle }}</strong>
      </template>
    </I18nT>
    <p v-if="replaceExisting" class="assign-switch-warning">
      <ExclamationCircleFilled aria-hidden="true" />
      <span>{{ t('workflows.task.assign.switchWarning') }}</span>
    </p>

    <fieldset class="assign-mode-fieldset">
      <legend class="visually-hidden">{{ t('workflows.task.assign.executionMode') }}</legend>
      <section class="assign-section">
        <p class="assign-section__label">
          <DeploymentUnitOutlined
            class="assign-section__icon"
            aria-hidden="true"
          />
          {{ t('workflows.task.assign.pipelineField') }}
        </p>
        <a-spin
          class="pipeline-spin"
          :spinning="pipelinesLoading"
        >
        <div
          class="assign-list pipeline-list"
        >
          <div
            v-for="pipeline in pipelines"
            :key="pipeline.uuid"
            class="assign-card"
            :class="{
              'assign-card--selected': selectedPipelineUuid === pipeline.uuid,
              'assign-card--disabled': !isPipelineAvailable(pipeline),
            }"
          >
            <label class="assign-card__choice">
              <input
                class="assign-card__radio"
                type="radio"
                :name="executionModeRadioName"
                :value="`pipeline:${pipeline.uuid}`"
                :checked="selectedPipelineUuid === pipeline.uuid"
                :disabled="!isPipelineAvailable(pipeline)"
                @change="choosePipeline(pipeline.uuid)"
              />
              <span class="assign-card__avatar">
                <img
                  v-if="pipeline.avatar"
                  :src="pipeline.avatar"
                  alt=""
                />
                <span v-else>{{ pipeline.name.slice(0, 1) }}</span>
              </span>
              <span
                class="assign-card__name"
                :title="pipeline.name"
              >{{ pipeline.name }}</span>
              <CheckCircleFilled
                v-if="selectedPipelineUuid === pipeline.uuid"
                class="assign-card__check"
                aria-hidden="true"
              />
            </label>
            <button
              v-if="!isPipelineConfigured(pipeline)"
              type="button"
              class="assign-card__config assign-card__config--warning"
              @click="goToPipelineConfig"
            >
              <ExclamationCircleFilled
                class="assign-card__warning"
                aria-hidden="true"
              />
              {{ t('workflows.task.assign.configure') }}
            </button>
          </div>
        </div>
        <div
          v-if="!pipelinesLoading && !configuredPipelines.length"
          class="field-help"
        >
          <span>{{ t('workflows.task.assign.noConfiguredPipeline') }}</span>
          <button
            type="button"
            class="assign-card__config"
            @click="goToPipelineConfig"
          >
            {{ t('workflows.task.assign.configure') }}
          </button>
        </div>
        </a-spin>
      </section>

      <section class="assign-section">
        <p class="assign-section__label">
          <TeamOutlined
            class="assign-section__icon"
            aria-hidden="true"
          />
          {{ t('agents.expertTeam') }}
        </p>
        <a-spin :spinning="expertGroupsLoading">
          <div class="assign-list">
            <label
              v-for="group in expertGroups"
              :key="group.uuid"
              class="assign-card"
              :class="{
                'assign-card--selected': selectedMode === 'expert_group' && selectedTarget === group.uuid,
                'assign-card--disabled': expertModeLocked || !group.ready,
              }"
            >
              <input
                class="assign-card__radio"
                type="radio"
                :name="executionModeRadioName"
                :checked="selectedMode === 'expert_group' && selectedTarget === group.uuid"
                :disabled="expertModeLocked || !group.ready"
                @change="chooseExpertGroup(group.uuid)"
              />
              <span class="assign-card__avatar">
                <img
                  v-if="group.avatar"
                  :src="group.avatar"
                  alt=""
                />
                <span v-else>{{ group.name.slice(0, 1) }}</span>
              </span>
              <span class="assign-card__name">{{ group.name }}</span>
              <span
                v-if="!group.ready"
                class="assign-card__subtitle"
              >{{ t('expertGroups.notReady') }}</span>
              <CheckCircleFilled
                v-if="selectedMode === 'expert_group' && selectedTarget === group.uuid"
                class="assign-card__check"
                aria-hidden="true"
              />
            </label>
          </div>
          <div
            v-if="!expertGroupsLoading && !expertGroups.some((group) => group.ready)"
            class="field-help"
          >
            <span>{{ t('expertGroups.notReady') }}</span>
            <button
              type="button"
              class="assign-card__config"
              @click="goToExpertGroupConfig"
            >
              {{ t('expertGroups.listTitle') }}
            </button>
          </div>
        </a-spin>
      </section>

      <section class="assign-section">
        <p class="assign-section__label">
          <img
            class="assign-section__icon assign-section__icon--logo"
            :src="cliExecutionLogo"
            alt=""
          />
          {{ t('workflows.task.common.directExecution') }}
        </p>
        <div class="assign-list">
          <!-- CLI 调用：选中后 CLI 与模型下拉内嵌在卡片内，与需求设计图一致 -->
          <div
            class="assign-card assign-card--stacked"
            :class="{
              'assign-card--selected': selectedMode === 'cli',
              'assign-card--disabled': cliModeLocked,
            }"
          >
            <label class="assign-card__choice">
              <input
                class="assign-card__radio"
                type="radio"
                :name="executionModeRadioName"
                value="cli"
                :checked="selectedMode === 'cli'"
                :disabled="cliModeLocked"
                @change="chooseCLI"
              />
              <span class="assign-card__avatar assign-card__avatar--soft">
                <img
                  class="assign-card__logo assign-card__logo--cli"
                  :src="cliExecutionLogo"
                  alt=""
                />
              </span>
              <span class="assign-card__name">{{ t('workflows.task.assign.cliMode') }}</span>
              <CheckCircleFilled
                v-if="selectedMode === 'cli'"
                class="assign-card__check"
                aria-hidden="true"
              />
            </label>
            <div
              v-if="selectedMode === 'cli'"
              class="assign-runtime"
              @click.stop
            >
              <a-select
                class="assign-runtime__select"
                :value="selectedTarget || undefined"
                :placeholder="t('workflows.task.assign.chooseCli')"
                :options="cliSelectOptions"
                :loading="cliLoading"
                :disabled="saving"
                @change="chooseCLIType"
              />
              <a-select
                class="assign-runtime__select"
                :value="selectedModelName || undefined"
                :placeholder="
                  selectedTarget
                    ? t('workflows.task.assign.chooseModel')
                    : t('workflows.task.assign.chooseCliFirst')
                "
                :options="modelSelectOptions"
                :loading="modelLoading"
                :disabled="saving || !selectedTarget"
                @change="chooseCLIModel"
              />
            </div>
          </div>

          <!-- Vibe Coding：外部编码工具选择内嵌在卡片内，与需求设计图一致 -->
          <div
            class="assign-card assign-card--stacked"
            :class="{
              'assign-card--selected': selectedMode === 'vibe_coding',
              'assign-card--disabled': directModeLocked,
            }"
          >
            <label class="assign-card__choice">
              <input
                class="assign-card__radio"
                type="radio"
                :name="executionModeRadioName"
                value="vibe_coding"
                :checked="selectedMode === 'vibe_coding'"
                :disabled="directModeLocked"
                @change="chooseVibeCoding"
              />
              <span class="assign-card__avatar assign-card__avatar--soft">
                <img
                  class="assign-card__logo assign-card__logo--vibe"
                  :src="vibeCodingLogo"
                  alt=""
                />
              </span>
              <span class="assign-card__name">{{ t('workflows.task.assign.vibeCodingMode') }}</span>
              <span class="assign-card__subtitle assign-card__subtitle--end">
                {{ t('workflows.task.assign.vibeCodingCardHint') }}
              </span>
              <CheckCircleFilled
                v-if="selectedMode === 'vibe_coding'"
                class="assign-card__check"
                aria-hidden="true"
              />
            </label>
            <a-tooltip
              :title="
                codexCapability.available
                  ? ''
                  : codexCapability.message || t('workflows.task.codex.capabilityUnavailable')
              "
            >
              <button
                type="button"
                class="assign-tool"
                :class="{ 'assign-tool--selected': isCodexSelected }"
                :disabled="codexLocked"
                :aria-pressed="isCodexSelected"
                @click.stop="chooseCodex"
              >
                <img
                  class="assign-tool__logo"
                  :src="codexLogo"
                  alt=""
                />
                <span>{{ t('workflows.task.assign.codex') }}</span>
              </button>
            </a-tooltip>
          </div>
        </div>
        <p
          v-if="selectedMode === 'cli' && !cliLoading && !availableCliOptions.length"
          class="field-help"
        >
          <span>{{ t('workflows.task.assign.noCli') }}</span>
        </p>
      </section>
    </fieldset>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { message } from 'ant-design-vue'
import { Translation as I18nT } from 'vue-i18n'
import {
  CheckCircleFilled,
  DeploymentUnitOutlined,
  ExclamationCircleFilled,
  TeamOutlined,
} from '@ant-design/icons-vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'
import cliExecutionLogo from '@/assets/icons/task-composer-cli.svg'
import codexLogo from '@/assets/icons/codex-logo.svg'
import vibeCodingLogo from '@/assets/icons/vibe-coding-logo.svg'
import {
  loadCodexCapability,
  openTaskInCodex,
  type CodexCapability,
} from '@/composables/useTaskCodex'
import { useCliModelOptions } from '@/composables/useCliModelOptions'
import { useCliKickoffNavigation } from '@/composables/useCliKickoff'
import { useAppI18n } from '@/i18n'
import { usePipelineStore } from '@/stores/pipeline'
import { useExpertGroupStore } from '@/stores/expert-group'
import type { ExpertGroup, Pipeline, TaskExecutionMode } from '@/types/pipeline'

const props = withDefaults(
  defineProps<{
    open: boolean
    taskUuid: string
    taskTitle?: string
    mode?: 'detail' | 'board'
    initialMode?: Extract<TaskExecutionMode, 'pipeline' | 'expert_group' | 'vibe_coding' | 'cli'> | ''
    preferredPipelineUuid?: string
    preferredExpertGroupUuid?: string
    initialCliType?: string
    initialModelName?: string
    replaceExisting?: boolean
  }>(),
  {
    taskTitle: '',
    mode: 'detail',
    initialMode: '',
    preferredPipelineUuid: '',
    preferredExpertGroupUuid: '',
    initialCliType: '',
    initialModelName: '',
    replaceExisting: false,
  },
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  assigned: []
}>()

const { t } = useAppI18n()
const { openCliConversation } = useCliKickoffNavigation()
const router = useRouter()
const pipelineStore = usePipelineStore()
const { pipelines, loading: pipelinesLoading } = storeToRefs(pipelineStore)
const expertGroupStore = useExpertGroupStore()
const { items: expertGroups, loading: expertGroupsLoading } = storeToRefs(expertGroupStore)
const saving = ref(false)
const codexCapability = ref<CodexCapability>({ available: false })
const radioGroupId = useId()
const executionModeRadioName = `assign-execution-mode-${radioGroupId}`
const selectedMode = ref<Extract<TaskExecutionMode, 'pipeline' | 'expert_group' | 'vibe_coding' | 'cli'> | undefined>()
const selectedTarget = ref<string>()
// CLI 直接执行没有 Agent 编排，模型选择与 CLI 一起构成完整的执行目标。
const selectedModelName = ref('')

interface ExecutionModeAssignmentResult {
  switched?: boolean
  start_status?: 'started' | 'failed'
  start_error?: string
}

const {
  cliLoading,
  cliOptions,
  loadCliOptions,
  loadModelOptions,
  modelLoading,
  modelOptions,
} = useCliModelOptions()

const availableCliOptions = computed(() => cliOptions.value.filter((cli) => cli.installed))
const cliSelectOptions = computed(() => {
  const items = cliOptions.value.map((cli) => ({
    value: cli.type,
    label: cli.name,
    disabled: !cli.installed,
  }))
  // 已指派任务的 CLI 可能来自其他设备（本机未安装），保留可回显。
  const current = props.initialCliType
  if (current && !items.some((item) => item.value === current)) {
    items.unshift({ value: current, label: current, disabled: false })
  }
  return items
})
const modelSelectOptions = computed(() =>
  modelOptions.value.map((model) => ({ value: model, label: model })),
)

const configuredPipelines = computed(() => pipelines.value.filter(isPipelineConfigured))
const selectedPipelineUuid = computed(() =>
  selectedMode.value === 'pipeline' ? selectedTarget.value || '' : '',
)
const isCodexSelected = computed(
  () => selectedMode.value === 'vibe_coding' && selectedTarget.value === 'codex',
)
// 看板仍会用本组件补全已预选但尚未冻结的流水线；该入口不是执行方式切换，
// 只能继续配置当前方式。详情页显式切换时 replaceExisting=true，才解锁其他方式。
const pipelineLocked = computed(
  () => !props.replaceExisting && Boolean(props.initialMode) && props.initialMode !== 'pipeline',
)
const directModeLocked = computed(
  () => !props.replaceExisting && Boolean(props.initialMode) && props.initialMode !== 'vibe_coding',
)
const cliModeLocked = computed(
  () => !props.replaceExisting && Boolean(props.initialMode) && props.initialMode !== 'cli',
)
const expertModeLocked = computed(
  () => !props.replaceExisting && Boolean(props.initialMode) && props.initialMode !== 'expert_group',
)
const codexLocked = computed(
  () => directModeLocked.value || !codexCapability.value.available,
)
const selectionChanged = computed(() => {
  if (!props.replaceExisting) return true
  if (selectedMode.value !== props.initialMode) return true
  if (selectedMode.value === 'pipeline') return selectedTarget.value !== props.preferredPipelineUuid
  if (selectedMode.value === 'expert_group') return selectedTarget.value !== props.preferredExpertGroupUuid
  if (selectedMode.value === 'cli') {
    return selectedTarget.value !== props.initialCliType || selectedModelName.value !== props.initialModelName
  }
  return false
})
const canConfirm = computed(() => {
  if (!selectionChanged.value) return false
  if (!selectedMode.value || !selectedTarget.value) return false
  if (selectedMode.value === 'cli') {
    return Boolean(selectedModelName.value)
  }
  if (selectedMode.value === 'vibe_coding') {
    return codexCapability.value.available && selectedTarget.value === 'codex'
  }
  if (selectedMode.value === 'expert_group') {
    return expertGroups.value.some((group: ExpertGroup) => group.ready && group.uuid === selectedTarget.value)
  }
  return configuredPipelines.value.some((pipeline) => pipeline.uuid === selectedTarget.value)
})

function isPipelineConfigured(pipeline: Pipeline) {
  const steps = pipeline.steps || []
  return steps.length > 0 && steps.every((step) => Boolean(step.cli_type && (step.model_name || step.model)))
}

function isPipelineAvailable(pipeline: Pipeline) {
  return !pipelineLocked.value && isPipelineConfigured(pipeline)
}

function close() {
  if (!saving.value) emit('update:open', false)
}

function choosePipeline(uuid: string) {
  selectedMode.value = 'pipeline'
  selectedTarget.value = uuid
}

function chooseVibeCoding() {
  selectedMode.value = 'vibe_coding'
  selectedTarget.value = undefined
}

function chooseCLI() {
  selectedMode.value = 'cli'
  selectedTarget.value = props.initialMode === 'cli' ? props.initialCliType || undefined : undefined
  selectedModelName.value = props.initialMode === 'cli' ? props.initialModelName : ''
  if (selectedTarget.value) void loadModelOptions(selectedTarget.value)
}

function chooseCLIType(cliType: string) {
  selectedTarget.value = cliType
  selectedModelName.value = ''
  void loadModelOptions(cliType)
}

function chooseCLIModel(model: string) {
  selectedModelName.value = model
}

function chooseExpertGroup(uuid: string) {
  selectedMode.value = 'expert_group'
  selectedTarget.value = uuid
}

function chooseCodex() {
  selectedMode.value = 'vibe_coding'
  selectedTarget.value = 'codex'
}

function goToPipelineConfig() {
  close()
  void router.push({ name: 'agents' })
}

function goToExpertGroupConfig() {
  close()
  void router.push({ name: 'agents', query: { mode: 'expert_group' } })
}

async function confirm() {
  if (!canConfirm.value || saving.value) return
  saving.value = true
  let shouldOpenCodex = false
  // 需求 2204（评论 5）：指派给 CLI 后跳到对话界面，由对话页自动向 CLI
  // 发送 task.md 引用与起始提示词，不再预填输入框。
  let shouldOpenCliConversation = false
  let result: ExecutionModeAssignmentResult | undefined
  let successKey: string
  try {
    if (selectedMode.value === 'pipeline') {
      result = await apiClient.put<ExecutionModeAssignmentResult>(`/tasks/${encodeURIComponent(props.taskUuid)}/execution-mode`, {
        mode: 'pipeline',
        pipeline_uuid: selectedTarget.value,
        start: true,
        replace_existing: props.replaceExisting,
      })
      successKey = props.replaceExisting ? 'workflows.task.assign.switchSuccess' : 'workflows.task.assign.pipelineAssigned'
    } else if (selectedMode.value === 'expert_group') {
      result = await apiClient.put<ExecutionModeAssignmentResult>(`/tasks/${encodeURIComponent(props.taskUuid)}/execution-mode`, {
        mode: 'expert_group',
        expert_group_uuid: selectedTarget.value,
        start: true,
        replace_existing: props.replaceExisting,
      })
      successKey = props.replaceExisting ? 'workflows.task.assign.switchSuccess' : 'expertGroups.ready'
    } else if (selectedMode.value === 'cli') {
      result = await apiClient.put<ExecutionModeAssignmentResult>(`/tasks/${encodeURIComponent(props.taskUuid)}/execution-mode`, {
        mode: 'cli',
        tool: selectedTarget.value,
        model_name: selectedModelName.value,
        start: true,
        replace_existing: props.replaceExisting,
      })
      successKey = props.replaceExisting ? 'workflows.task.assign.switchSuccess' : 'workflows.task.assign.cliAssigned'
      shouldOpenCliConversation = true
    } else {
      result = await apiClient.put<ExecutionModeAssignmentResult>(`/tasks/${encodeURIComponent(props.taskUuid)}/execution-mode`, {
        mode: 'vibe_coding',
        tool: 'codex',
        start: props.replaceExisting || props.mode === 'board',
        replace_existing: props.replaceExisting,
      })
      shouldOpenCodex = true
      successKey = props.replaceExisting ? 'workflows.task.assign.switchSuccess' : 'workflows.task.assign.codexAssigned'
    }
    if (result?.start_status === 'failed') {
      message.warning(result.start_error || t('workflows.task.assign.switchStartFailed'))
    } else {
      message.success(t(successKey))
    }
    emit('assigned')
    emit('update:open', false)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.assign.executionFailed'))
  } finally {
    saving.value = false
  }
  if (shouldOpenCliConversation) {
    await openCliConversation(props.taskUuid)
    return
  }
  if (!shouldOpenCodex) return

  const openResult = await openTaskInCodex(props.taskUuid)
  if (openResult.opened) {
    message.success(t('workflows.task.assign.codexOpened'))
  } else if (openResult.copied) {
    message.warning(t('workflows.task.assign.codexCopiedFallback'))
  } else {
    message.warning(t('workflows.task.assign.codexUnavailableAfterAssign'))
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    selectedMode.value = props.initialMode || undefined
    selectedTarget.value =
      props.initialMode === 'vibe_coding'
        ? 'codex'
        : props.initialMode === 'cli'
          ? props.initialCliType || undefined
          : undefined
    selectedModelName.value = props.initialMode === 'cli' ? props.initialModelName : ''
    const [pipelineResult, expertResult, codexResult, cliResult] = await Promise.allSettled([
      pipelineStore.loadPipelines(true),
      expertGroupStore.load(true),
      loadCodexCapability(),
      loadCliOptions(),
    ])
    if (cliResult.status === 'fulfilled' && selectedMode.value === 'cli' && selectedTarget.value) {
      void loadModelOptions(selectedTarget.value)
    }
    codexCapability.value = codexResult.status === 'fulfilled'
      ? codexResult.value
      : { available: false, message: t('workflows.task.codex.capabilityUnavailable') }
    if (!codexCapability.value.available && selectedMode.value === 'vibe_coding') {
      selectedTarget.value = undefined
    }
    if (pipelineResult.status === 'fulfilled') {
      if (selectedMode.value === 'pipeline') {
        selectedTarget.value = configuredPipelines.value.some(
          (pipeline) => pipeline.uuid === props.preferredPipelineUuid,
        ) ? props.preferredPipelineUuid : configuredPipelines.value[0]?.uuid
      }
    } else {
      const error = pipelineResult.reason
      message.warning(error instanceof Error ? error.message : t('workflows.task.assign.loadFailed'))
    }
    if (expertResult.status === 'fulfilled' && selectedMode.value === 'expert_group') {
      selectedTarget.value = expertGroups.value.some(
        (group) => group.ready && group.uuid === props.preferredExpertGroupUuid,
      )
        ? props.preferredExpertGroupUuid
        : expertGroups.value.find((group) => group.ready)?.uuid
    }
  },
)
</script>

<style scoped>
/* 需求设计图：说明文案为普通正文，不使用提示条容器 */
.assign-description {
  margin: 0 0 16px;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

.assign-description strong {
  font-weight: 600;
}

.assign-switch-warning {
  display: flex;
  margin: -4px 0 16px;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  color: #ad6800;
  background: #fffbe6;
  font-size: 13px;
  line-height: 20px;
}

.assign-switch-warning :deep(.anticon) {
  margin-top: 2px;
  flex: 0 0 auto;
}

.assign-section {
  margin-top: 16px;
}

.assign-section__label {
  display: flex;
  margin: 0 0 8px;
  align-items: center;
  gap: 4px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}

.assign-section__icon {
  flex: 0 0 auto;
  color: #8c8c8c;
  font-size: 12px;
}

.assign-section__icon--logo {
  width: 12px;
  height: 12px;
}

.assign-mode-fieldset {
  min-width: 0;
  margin: 0;
  border: 0;
  padding: 0;
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  padding: 0;
  border: 0;
  margin: -1px;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
}

.pipeline-spin {
  display: block;
  min-height: 56px;
}

/* 需求设计图：所有卡片单列全宽排列 */
.assign-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.pipeline-list {
  max-height: 220px;
  overflow-y: auto;
  padding: 2px;
  scrollbar-gutter: stable;
}

.assign-card {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 50px;
  align-items: center;
  gap: 8px;
  border: 1px solid #e5e5e5;
  padding: 0 12px;
  border-radius: 12px;
  background: #fff;
  box-sizing: border-box;
}

.assign-card--selected {
  border-color: #3157e2;
  background: #e5efff;
}

.assign-card--disabled {
  border-color: #efefef;
  background: #fff;
}

/* 内嵌下拉/工具的卡片：纵向堆叠，行高自适应 */
.assign-card--stacked {
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  padding: 8px 12px;
}

.assign-card--stacked .assign-card__choice {
  flex: 0 0 auto;
  width: 100%;
  min-height: 34px;
}

.assign-card__choice {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.assign-card--disabled .assign-card__choice {
  cursor: not-allowed;
}

/* 原生单选保留键盘可达，仅做视觉隐藏 */
.assign-card__radio {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
}

.assign-card:has(.assign-card__radio:focus-visible) {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

label.assign-card {
  cursor: pointer;
}

label.assign-card.assign-card--disabled {
  cursor: not-allowed;
}

.assign-card__avatar {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 8px;
  color: #3157e2;
  background: #e5efff;
  font-size: 14px;
}

.assign-card__avatar--soft {
  background: #f5f5f5;
}

.assign-card__avatar img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.assign-card__avatar .assign-card__logo {
  width: 18px;
  height: 18px;
  object-fit: contain;
}

.assign-card__avatar .assign-card__logo--vibe {
  width: 15px;
  height: 15px;
}

.assign-card__avatar .assign-card__logo--cli {
  width: 16px;
  height: 16px;
}

.assign-card__name {
  min-width: 0;
  flex: 0 1 auto;
  overflow: hidden;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assign-card--disabled .assign-card__name {
  color: #bfbfbf;
}

.assign-card__subtitle {
  min-width: 0;
  overflow: hidden;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assign-card__subtitle--end {
  flex: 1 1 auto;
  text-align: right;
}

.assign-card__check {
  flex: 0 0 auto;
  margin-left: auto;
  color: #3157e2;
  font-size: 18px;
}

/* 未配置的流水线：红色感叹号 + 去配置入口 */
.assign-card__config {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  border: 0;
  padding: 0;
  color: #3157e2;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}

.assign-card__config--warning {
  color: #ff7875;
}

.assign-card__warning {
  color: #ff7875;
  font-size: 14px;
}

/* CLI 调用卡片内的 CLI / 模型下拉，与需求设计图宽度比例一致 */
.assign-runtime {
  display: grid;
  grid-template-columns: minmax(0, 1.44fr) minmax(0, 1fr);
  gap: 7px;
  width: 100%;
}

.assign-runtime__select {
  width: 100%;
}

/* Vibe Coding 卡片内的外部编码工具 */
.assign-tool {
  display: inline-flex;
  height: 32px;
  align-self: flex-start;
  align-items: center;
  gap: 6px;
  border: 1px solid #e5e5e5;
  padding: 0 12px;
  border-radius: 8px;
  color: #262626;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
}

.assign-tool--selected {
  border-color: #3157e2;
  background: #e5efff;
}

.assign-tool:disabled {
  color: #bfbfbf;
  cursor: not-allowed;
}

.assign-tool__logo {
  width: 16px;
  height: 16px;
  object-fit: contain;
}

.field-help {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}

:global(.assign-execution-modal .ant-modal) {
  max-width: calc(100vw - 32px);
}

:global(.assign-execution-modal .ant-modal-content) {
  border-radius: 12px;
}

@media (max-width: 640px) {
  .pipeline-list {
    max-height: min(240px, 32vh);
  }
}
</style>

