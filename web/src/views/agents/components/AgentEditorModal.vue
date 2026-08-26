<template>
  <a-modal
    :open="open"
    :title="step ? '编辑 Agent' : '新建 Agent'"
    width="680px"
    :confirm-loading="saving"
    @update:open="emit('update:open', $event)"
    @ok="save"
  >
    <div
      v-if="cloudLocked"
      class="cloud-lock"
    >
      <CloudDownloadOutlined />该 Agent 来自团队流水线同步，名称、头像与提示词由云端管理，仅可配置 CLI 与模型
    </div>
    <a-form layout="vertical">
      <a-form-item
        label="Agent 名称（最多20个字）"
        required
      >
        <a-input
          v-model:value="form.name"
          :disabled="cloudLocked"
          :maxlength="20"
          show-count
          placeholder="请输入 Agent 名称"
        />
      </a-form-item>
      <a-form-item label="头像">
        <div
          class="avatar-options"
          :class="{ locked: cloudLocked }"
        >
          <button
            v-for="avatar in AGENT_AVATARS"
            :key="avatar"
            type="button"
            :disabled="cloudLocked"
            :class="{ active: form.avatar === avatar }"
            @click="form.avatar = avatar"
          >
            <img
              :src="avatar"
              alt=""
            />
          </button>
        </div>
      </a-form-item>
      <a-form-item label="指令（提示词）">
        <a-textarea
          v-model:value="form.prompt"
          :disabled="cloudLocked"
          :rows="5"
          placeholder="请设置 Agent 的提示词，定义其行为和能力"
        />
      </a-form-item>
      <a-form-item
        label="编程工具 CLI（必填，自动检测本机运行状态）"
        required
      >
        <div class="cli-row">
          <a-select
            v-model:value="form.cli_type"
            :loading="cliLoading"
            placeholder="请选择 CLI"
            @change="loadModels(String($event))"
          >
            <a-select-option
              v-for="cli in cliOptions"
              :key="cli.type"
              :value="cli.type"
              :disabled="!cli.installed"
            >
              {{ cli.name }}（{{ cli.installed ? '已检测到运行' : '未检测到运行' }}）
            </a-select-option>
          </a-select>
          <a-button
            :loading="cliLoading"
            @click="detectCli()"
          >
            <ReloadOutlined />重新检测
          </a-button>
        </div>
        <p
          class="cli-hint"
          :class="form.cli_type ? 'ok' : ''"
        >
          {{
            form.cli_type
              ? `${cliOptions.find((item) => item.type === form.cli_type)?.name || form.cli_type}（本机已检测到运行）`
              : '请选择本机已检测到运行的 CLI'
          }}
        </p>
      </a-form-item>
      <a-form-item
        label="模型（必填，随 CLI 提供方联动）"
        required
      >
        <a-select
          v-model:value="form.model"
          :loading="modelLoading"
          :disabled="!form.cli_type"
          show-search
        >
          <a-select-option
            v-for="model in modelOptions"
            :key="model"
            :value="model"
          >
            {{ model }}
          </a-select-option>
        </a-select>
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { CloudDownloadOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import apiClient from '@/api/client'
import type { PipelineStep } from '@/types/pipeline'
import {
  AGENT_AVATARS,
  type CloudStepExecutionInput,
  type DiscoveredCLI,
  type StepInput,
  stepModel,
} from './agentPipeline'

const props = defineProps<{
  open: boolean
  pipelineUuid: string
  isCloudPipeline: boolean
  step?: PipelineStep
  currentAvatar?: string
}>()

const emit = defineEmits<{
  saved: []
  'update:open': [open: boolean]
}>()

const saving = ref(false)
const cliLoading = ref(false)
const modelLoading = ref(false)
const cliOptions = ref<DiscoveredCLI[]>([])
const modelOptions = ref<string[]>([])
const form = reactive({
  name: '',
  description: '',
  avatar: AGENT_AVATARS[0],
  prompt: '',
  cli_type: '',
  model: '',
})
let modelRequestId = 0

const cloudLocked = computed(() => props.isCloudPipeline && Boolean(props.step))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    void initialiseForm()
  },
)

async function initialiseForm() {
  modelRequestId += 1
  modelOptions.value = []
  form.name = props.step?.name || ''
  form.description = props.step?.description || ''
  form.avatar = props.step?.avatar || props.currentAvatar || AGENT_AVATARS[0]
  form.prompt = props.step?.prompt || props.step?.prompt_snapshot || ''
  form.cli_type = props.step?.cli_type || ''
  form.model = props.step ? stepModel(props.step) : ''
  await detectCli(false)
  await loadModels(form.cli_type, form.model)
}

async function detectCli(showSuccess = true) {
  cliLoading.value = true
  try {
    const result = await apiClient.get<{ items: DiscoveredCLI[] }>('/tasks/cli-discovery')
    // 已安装的 CLI 排在前面，未安装的置后且不可选择
    cliOptions.value = [...(result.items || [])].sort((a, b) => Number(b.installed) - Number(a.installed))
    if (showSuccess) message.success('CLI 检测完成')
  } catch (error) {
    if (showSuccess) message.error(error instanceof Error ? error.message : 'CLI 检测失败')
  } finally {
    cliLoading.value = false
  }
  // 模型列表与 CLI 状态联动刷新，避免"重新检测"后仍展示旧模型
  if (form.cli_type) {
    await loadModels(form.cli_type, form.model)
  }
}

async function loadModels(cliType: string, keepModel = '') {
  const requestId = ++modelRequestId
  modelOptions.value = []
  form.model = keepModel
  if (!cliType) return

  modelLoading.value = true
  try {
    const result = await apiClient.get<{ models: string[] }>('/tasks/cli-models', {
      cli_type: cliType,
    })
    if (requestId !== modelRequestId || cliType !== form.cli_type) return
    modelOptions.value = result.models || []
    if (!form.model) form.model = modelOptions.value[0] || ''
  } catch {
    if (requestId !== modelRequestId || cliType !== form.cli_type) return
    modelOptions.value = cliOptions.value.find((item) => item.type === cliType)?.models || []
  } finally {
    if (requestId === modelRequestId) modelLoading.value = false
  }
}

async function save() {
  if (!props.pipelineUuid) return
  if (!form.name.trim()) return message.warning('请输入 Agent 名称')
  if (form.name.trim().length > 20) return message.warning('Agent 名称最多20个字')
  if (!form.cli_type || !form.model.trim())
    return message.warning('请选择本机可用 CLI 和模型')

  saving.value = true
  try {
    const payload: StepInput = {
      name: form.name.trim(),
      description: form.description.trim(),
      avatar: form.avatar,
      prompt: form.prompt.trim(),
      cli_type: form.cli_type,
      model_name: form.model.trim(),
    }
    if (cloudLocked.value && props.step) {
      const executionPayload: CloudStepExecutionInput = {
        cli_type: payload.cli_type,
        model_name: payload.model_name,
      }
      await apiClient.put<PipelineStep>(
        `/pipelines/${props.pipelineUuid}/steps/${props.step.uuid}`,
        executionPayload,
      )
    } else if (props.step) {
      await apiClient.put<PipelineStep>(
        `/pipelines/${props.pipelineUuid}/steps/${props.step.uuid}`,
        payload,
      )
    } else {
      await apiClient.post<PipelineStep>(`/pipelines/${props.pipelineUuid}/steps`, payload)
    }
    emit('update:open', false)
    emit('saved')
    message.success(props.step ? 'Agent 已更新' : 'Agent 已添加')
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'Agent 保存失败')
  } finally {
    saving.value = false
  }
}

</script>

<style scoped>
.cloud-lock {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 18px;
  padding: 9px 12px;
  border-radius: 6px;
  color: #b45309;
  background: #fff7e6;
  font-size: 11px;
}

.avatar-options {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.avatar-options button {
  width: 48px;
  height: 48px;
  padding: 2px;
  border: 2px solid transparent;
  border-radius: 50%;
  background: #fff;
  cursor: pointer;
}

.avatar-options button.active {
  border-color: #1677ff;
}

.avatar-options button:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: 2px;
}

.avatar-options img {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  object-fit: cover;
}

.avatar-options.locked {
  opacity: 0.45;
}

.cli-row {
  display: flex;
  gap: 8px;
}

.cli-row :deep(.ant-select) {
  flex: 1;
}

.cli-hint {
  margin: 5px 0 0;
  color: #b45309;
  font-size: 11px;
}

.cli-hint.ok {
  color: #15803d;
}
</style>
