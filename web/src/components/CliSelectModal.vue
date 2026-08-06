<script setup lang="ts">
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import apiClient from '@/api/client'

interface DiscoveredCLI {
  type: string
  name: string
  installed: boolean
  exec_path: string
  version: string
  models: string[]
}

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  confirm: [payload: { cli_type: string; model: string }]
}>()

const loading = ref(false)
const cliList = ref<DiscoveredCLI[]>([])
const selectedType = ref('')
const selectedModel = ref('')
const useCustomModel = ref(false)
const customModel = ref('')

const selectedCli = () => cliList.value.find(item => item.type === selectedType.value)

// ---- Local persistence of custom models ----
const CUSTOM_MODELS_KEY = 'cli_custom_models'

function loadCustomModels(cliType: string): string[] {
  try {
    const raw = localStorage.getItem(CUSTOM_MODELS_KEY)
    if (!raw) return []
    const all = JSON.parse(raw) as Record<string, string[]>
    return all[cliType] || []
  } catch {
    return []
  }
}

function saveCustomModel(cliType: string, model: string) {
  if (!model) return
  try {
    const raw = localStorage.getItem(CUSTOM_MODELS_KEY)
    const all: Record<string, string[]> = raw ? JSON.parse(raw) : {}
    const list = all[cliType] || []
    if (!list.includes(model)) {
      list.unshift(model)
      // Keep at most 20 entries
      all[cliType] = list.slice(0, 20)
      localStorage.setItem(CUSTOM_MODELS_KEY, JSON.stringify(all))
    }
  } catch { /* ignore */ }
}

/** Merge probed models + locally recorded custom models */
function mergedModels(cli: DiscoveredCLI): { name: string; custom: boolean }[] {
  const customs = loadCustomModels(cli.type)
  const result: { name: string; custom: boolean }[] = cli.models.map(m => ({ name: m, custom: false }))
  for (const m of customs) {
    if (!cli.models.includes(m)) {
      result.push({ name: m, custom: true })
    }
  }
  return result
}

watch(() => props.open, async (visible) => {
  if (visible) {
    selectedType.value = ''
    selectedModel.value = ''
    useCustomModel.value = false
    customModel.value = ''
    await loadCLIs()
  }
})

async function loadCLIs() {
  loading.value = true
  try {
    const result = await apiClient.get<{ items: DiscoveredCLI[] }>('/tasks/cli-discovery')
    cliList.value = result.items || []
    // Auto-select the first installed CLI
    const firstInstalled = cliList.value.find(item => item.installed)
    if (firstInstalled) {
      selectedType.value = firstInstalled.type
      const models = mergedModels(firstInstalled)
      if (models.length > 0) {
        selectedModel.value = models[0].name
        useCustomModel.value = false
      } else {
        useCustomModel.value = true
      }
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'CLI 探测失败')
  } finally {
    loading.value = false
  }
}

function selectCli(type: string) {
  const cli = cliList.value.find(item => item.type === type)
  if (!cli || !cli.installed) return
  selectedType.value = type
  const models = mergedModels(cli)
  selectedModel.value = models.length > 0 ? models[0].name : ''
  useCustomModel.value = models.length === 0
  customModel.value = ''
}

function handleConfirm() {
  if (!selectedType.value) {
    message.warning('请选择一个 CLI 工具')
    return
  }
  const cli = selectedCli()
  if (!cli) return

  const model = useCustomModel.value ? customModel.value.trim() : selectedModel.value
  // Persist manually entered models so they can be picked directly from the list next time
  if (useCustomModel.value && model) {
    saveCustomModel(selectedType.value, model)
  }
  emit('confirm', { cli_type: selectedType.value, model })
  emit('update:open', false)
}

function handleCancel() {
  emit('update:open', false)
}
</script>

<template>
  <a-modal
    :open="open"
    title="选择执行 CLI"
    :width="560"
    :mask-closable="false"
    @ok="handleConfirm"
    @cancel="handleCancel"
  >
    <template #footer>
      <a-button @click="handleCancel">取消</a-button>
      <a-button type="primary" :disabled="!selectedType" @click="handleConfirm">开始执行</a-button>
    </template>

    <a-spin :spinning="loading">
      <div class="cli-select-body">
        <div class="cli-card-list">
          <div
            v-for="cli in cliList"
            :key="cli.type"
            class="cli-card"
            :class="{
              'cli-card--selected': selectedType === cli.type,
              'cli-card--disabled': !cli.installed,
            }"
            @click="selectCli(cli.type)"
          >
            <div class="cli-card__header">
              <span class="cli-card__name">{{ cli.name }}</span>
              <a-tag v-if="!cli.installed" color="default">未安装</a-tag>
              <a-tag v-else-if="selectedType === cli.type" color="green">已选择</a-tag>
            </div>
            <div v-if="cli.installed" class="cli-card__meta">
              <span v-if="cli.version" class="cli-card__version">{{ cli.version }}</span>
              <span v-if="cli.models.length" class="cli-card__models">{{ cli.models.length }} 个模型</span>
            </div>
            <div v-else class="cli-card__meta">
              <span class="cli-card__hint">请先安装并加入 PATH</span>
            </div>
          </div>
        </div>

        <div v-if="selectedCli()?.installed" class="cli-model-section">
          <div class="cli-model-section__label">模型配置</div>
          <a-select
            v-if="!useCustomModel && mergedModels(selectedCli()!).length > 0"
            v-model:value="selectedModel"
            placeholder="选择模型"
            style="width: 100%"
            show-search
            :filter-option="(input: string, option: any) => (option?.value || '').toLowerCase().includes(input.toLowerCase())"
          >
            <a-select-option v-for="m in mergedModels(selectedCli()!)" :key="m.name" :value="m.name">
              {{ m.name }}
              <a-tag v-if="m.custom" color="blue" size="small" style="margin-left: 6px">自定义</a-tag>
            </a-select-option>
          </a-select>
          <a-input
            v-else
            v-model:value="customModel"
            placeholder="输入模型名称（可留空使用 CLI 默认模型）"
            allow-clear
          />
          <div class="cli-model-section__footer">
            <a
              v-if="mergedModels(selectedCli()!).length > 0"
              class="cli-model-section__toggle"
              @click="useCustomModel = !useCustomModel"
            >
              {{ useCustomModel ? '← 从列表选择' : '手动输入模型' }}
            </a>
            <span v-if="useCustomModel" class="cli-model-section__hint">留空则使用 CLI 默认模型，输入后会自动记录到列表</span>
          </div>
        </div>
      </div>
    </a-spin>
  </a-modal>
</template>

<style scoped>
.cli-select-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.cli-card-list {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.cli-card {
  padding: 12px 14px;
  border: 1.5px solid #e8e8e8;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  user-select: none;
}

.cli-card:hover:not(.cli-card--disabled) {
  border-color: #91caff;
  background: #f0f7ff;
}

.cli-card--selected {
  border-color: #1677ff;
  background: #e6f4ff;
}

.cli-card--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.cli-card__header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.cli-card__name {
  font-weight: 600;
  font-size: 14px;
}

.cli-card__meta {
  display: flex;
  gap: 10px;
  font-size: 12px;
  color: #8c8c8c;
}

.cli-card__hint {
  color: #bfbfbf;
}

.cli-model-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.cli-model-section__label {
  font-weight: 500;
  font-size: 13px;
  color: #595959;
}

.cli-model-section__footer {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cli-model-section__toggle {
  font-size: 12px;
  color: #1677ff;
  cursor: pointer;
}

.cli-model-section__toggle:hover {
  text-decoration: underline;
}

.cli-model-section__hint {
  font-size: 12px;
  color: #8c8c8c;
}
</style>
