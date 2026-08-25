<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import apiClient from '@/api/client'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import type { CloudWorkItem } from '@/types/workitem'

interface RemoteStep {
  id: number | string
  uuid?: string
  name: string
  sort_order: number
}
interface Agent {
  id: number | string
  name: string
  color: string
  cli_type: string
  workflow_version: number
  description?: string
  steps?: RemoteStep[]
}
interface StepRuntimeConfig { cloud_step_id: number | string; cli_type: string; model: string }
const props = defineProps<{
  open: boolean
  initialStatus?: string
  initialWorkItem?: CloudWorkItem
}>()
const maskEl = ref<HTMLElement | null>(null)

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void
  (event: 'created', uuid: string): void
}>()

/** Upper limit on the total number of working directories (primary + child) */
const MAX_WORK_DIRS = 8

const STEP_TITLES = ['选择需求/缺陷', '选择流水线', '配置并导入']
const assignmentMode = computed(() => Boolean(props.initialWorkItem))
const minimumStep = computed(() => assignmentMode.value ? 1 : 0)
const modalTitle = computed(() => assignmentMode.value ? '导入任务' : '导入云端任务')

const currentStep = ref(0)
const loading = ref(false)
const creating = ref(false)
const errorText = ref('')

const workItems = ref<CloudWorkItem[]>([])
const agents = ref<Agent[]>([])

const keyword = ref('')
const typeFilter = ref<'all' | 'requirement' | 'defect'>('all')
const selectedWorkItem = ref<CloudWorkItem>()
const selectedAgentID = ref<number | string>()
const stepConfigs = ref<StepRuntimeConfig[]>([])
const mainWorkDir = ref('')
const subWorkDirs = ref<string[]>([])

const typeTabs = [
  { value: 'all', label: '全部' },
  { value: 'requirement', label: '需求' },
  { value: 'defect', label: '缺陷' },
] as const

const filteredItems = computed(() => workItems.value.filter((item) => {
  const matchesType = typeFilter.value === 'all' || item.type === typeFilter.value
  const text = keyword.value.trim().toLowerCase()
  const matchesKeyword = !text
    || item.title.toLowerCase().includes(text)
    || String(item.id).includes(text)
  return matchesType && matchesKeyword
}))

const workDirs = computed(() => [mainWorkDir.value, ...subWorkDirs.value].map(dir => dir.trim()))
const validWorkDirs = computed(() => workDirs.value.filter(Boolean))
const canAddSubWorkDir = computed(() => workDirs.value.length < MAX_WORK_DIRS)

const selectedAgent = computed(() => agents.value.find(item => String(item.id) === String(selectedAgentID.value)))

const summaryText = computed(() => {
  if (currentStep.value === 0) {
    return selectedWorkItem.value ? `已选择：${selectedWorkItem.value.title}` : `共 ${filteredItems.value.length} 项`
  }
  if (currentStep.value === 1) {
    return selectedAgent.value ? `已选择：${selectedAgent.value.name}` : `共 ${agents.value.length} 条流水线`
  }
  return `工作目录 ${validWorkDirs.value.length} 个`
})

const nextDisabled = computed(() => {
  if (currentStep.value === 0) return !selectedWorkItem.value
  if (currentStep.value === 1) return !selectedAgentID.value
  return stepConfigs.value.some(item => !item.cli_type.trim() || !item.model.trim())
})

function typeLabel(type: CloudWorkItem['type']) {
  if (type === 'defect') return '缺陷'
  if (type === 'task') return '任务'
  return '需求'
}

function workspaceLabel(item: CloudWorkItem) {
  return item.workspace_name || (item.workspace_id ? `工作区 #${item.workspace_id}` : '-')
}

function isSelected(item: CloudWorkItem) {
  return selectedWorkItem.value?.id === item.id && selectedWorkItem.value?.type === item.type
}

function toggleWorkItem(item: CloudWorkItem) {
  selectedWorkItem.value = isSelected(item) ? undefined : item
}

function close() {
  emit('update:open', false)
}

function resetState() {
  currentStep.value = minimumStep.value
  keyword.value = ''
  typeFilter.value = 'all'
  selectedWorkItem.value = props.initialWorkItem
  selectedAgentID.value = undefined
  stepConfigs.value = []
  mainWorkDir.value = ''
  subWorkDirs.value = []
  errorText.value = ''
}

function reasonText(reason: unknown) {
  return reason instanceof Error ? reason.message : String(reason)
}

// Remember the last selected Agent and pre-select it automatically next time it opens.
const LAST_AGENT_KEY = 'goteams.createTask.lastAgentID'

async function selectAgent(id: number | string) {
  selectedAgentID.value = id
  try {
    localStorage.setItem(LAST_AGENT_KEY, String(id))
  } catch {
    /* Ignore persistence failures in scenarios such as privacy mode */
  }
  try {
    const detail = await apiClient.get<Agent>(`/team/pipelines/${encodeURIComponent(String(id))}/snapshot`)
    const index = agents.value.findIndex(item => String(item.id) === String(id))
    if (index >= 0) agents.value[index] = detail
    stepConfigs.value = (detail.steps || []).sort((a, b) => a.sort_order - b.sort_order).map(step => ({
      cloud_step_id: step.id ?? step.uuid ?? '',
      cli_type: '',
      model: '',
    }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : '流水线编排加载失败')
  }
}

async function load() {
  loading.value = true
  errorText.value = ''
  try {
    // The two data sources are independent: a failure in one must not affect the other.
    const workItemRequest = props.initialWorkItem
      ? Promise.resolve({ items: [props.initialWorkItem] })
      : apiClient.get<{ items: CloudWorkItem[] }>('/team/my-work')
    const [workItemResult, agentResult] = await Promise.allSettled([
      workItemRequest,
      apiClient.get<{ items: Agent[] }>('/team/pipelines'),
    ])

    workItems.value = workItemResult.status === 'fulfilled' ? workItemResult.value.items || [] : []
    agents.value = agentResult.status === 'fulfilled' ? agentResult.value.items || [] : []

    // After the Agent list loads, auto-preselect the last selected Agent if it is still in the list.
    const savedAgentID = localStorage.getItem(LAST_AGENT_KEY) || ''
    if (savedAgentID && agents.value.some((a) => String(a.id) === savedAgentID)) {
      await selectAgent(savedAgentID)
    }

    // Only error on data sources that actually failed, to avoid "one fails, whole screen blank".
    const failures: string[] = []
    if (workItemResult.status === 'rejected') failures.push(`需求/缺陷（${reasonText(workItemResult.reason)}）`)
    if (agentResult.status === 'rejected') failures.push(`云端流水线（${reasonText(agentResult.reason)}）`)
    errorText.value = failures.length ? `以下数据加载失败：${failures.join('、')}` : ''
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '数据加载失败'
  } finally {
    loading.value = false
  }
}

async function addSubWorkDir() {
  if (!canAddSubWorkDir.value) {
    message.warning(`最多支持 ${MAX_WORK_DIRS} 个工作目录`)
    return
  }

  // The desktop flow selects a real folder first, so cancelling the native dialog
  // does not leave an empty child-directory row behind. Browser mode keeps manual
  // input as a compatibility fallback because it has no access to absolute paths.
  if (isDesktopRuntime()) {
    const selected = await pickWorkDir(mainWorkDir.value || subWorkDirs.value.at(-1))
    if (!selected) return

    const duplicate = validWorkDirs.value.some(dir => dir.toLowerCase() === selected.trim().toLowerCase())
    if (duplicate) {
      message.warning('工作目录不能重复')
      return
    }
    subWorkDirs.value.push(selected)
    return
  }

  subWorkDirs.value.push('')
}

function removeSubWorkDir(index: number) {
  subWorkDirs.value.splice(index, 1)
}

async function pickWorkDir(defaultPath?: string) {
  try {
    return await selectDirectory(defaultPath)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '无法打开文件夹选择器')
    return undefined
  }
}

async function chooseMainWorkDir() {
  const selected = await pickWorkDir(mainWorkDir.value)
  if (selected) mainWorkDir.value = selected
}

async function chooseSubWorkDir(index: number) {
  const selected = await pickWorkDir(subWorkDirs.value[index])
  if (!selected) return

  const duplicate = validWorkDirs.value.some((dir, dirIndex) => (
    dirIndex !== index + 1 && dir.toLowerCase() === selected.trim().toLowerCase()
  ))
  if (duplicate) {
    message.warning('工作目录不能重复')
    return
  }
  subWorkDirs.value[index] = selected
}

function goPrev() {
  if (currentStep.value > minimumStep.value) currentStep.value -= 1
}

function goNext() {
  if (currentStep.value === 0) {
    if (!selectedWorkItem.value) {
      message.warning('请选择一个需求或缺陷')
      return
    }
    currentStep.value = 1
    return
  }
  if (currentStep.value === 1) {
    if (!selectedAgentID.value) {
      message.warning('请选择云端流水线')
      return
    }
    currentStep.value = 2
    return
  }
  createTask()
}

async function createTask() {
  if (!selectedWorkItem.value || !selectedAgentID.value) {
    message.warning('请先完成前面的步骤')
    return
  }
  if (!mainWorkDir.value.trim()) {
    message.warning('请填写工作目录地址')
    return
  }
  if (!stepConfigs.value.length || stepConfigs.value.some(item => !item.cli_type.trim() || !item.model.trim())) {
    message.warning('请为每一个 Agent 编排配置 CLI 和模型')
    return
  }
  if (subWorkDirs.value.some(dir => !dir.trim())) {
    message.warning('子工作目录不能为空，请填写或移除')
    return
  }
  const unique = new Set(validWorkDirs.value.map(dir => dir.toLowerCase()))
  if (unique.size !== validWorkDirs.value.length) {
    message.warning('工作目录不能重复')
    return
  }
  creating.value = true
  try {
    const result = await apiClient.post<{ uuid: string }>('/team/imports', {
      work_item_type: selectedWorkItem.value.type,
      work_item_id: selectedWorkItem.value.id,
      cloud_pipeline_id: selectedAgentID.value,
      step_configs: stepConfigs.value,
      work_dir: validWorkDirs.value[0],
      work_dirs: validWorkDirs.value,
      status: props.initialStatus || '',
    })
    message.success('任务已导入到本地看板')
    emit('created', result.uuid)
    close()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '任务创建失败')
  } finally {
    creating.value = false
  }
}

watch(() => props.open, (open) => {
  if (open) {
    resetState()
    load()
  }
})
</script>

<template>
  <Teleport to="body">
    <Transition name="ct-fade">
      <div v-if="props.open" ref="maskEl" class="ct-mask" @click.self="close">
        <div class="ct-modal" role="dialog" aria-modal="true" :aria-label="modalTitle">
          <!-- Dialog header -->
          <header class="ct-header">
            <span class="ct-header-title">{{ modalTitle }}</span>
            <button type="button" class="ct-close" aria-label="关闭" @click="close">
              <svg viewBox="0 0 12 12" width="12" height="12" fill="none" aria-hidden="true">
                <path d="M1 1l10 10M11 1L1 11" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" />
              </svg>
            </button>
          </header>

          <!-- Dialog body -->
          <div class="ct-body">
            <!-- Step bar -->
            <nav class="ct-steps" :aria-label="`${modalTitle}步骤`">
              <template v-for="(title, index) in STEP_TITLES" :key="title">
                <span v-if="index > 0" class="ct-step-line" />
                <div
                  class="ct-step"
                  :class="{
                    done: index < currentStep,
                    active: index === currentStep,
                  }"
                >
                  <span class="ct-step-index">
                    <svg v-if="index < currentStep" viewBox="0 0 12 10" width="12" height="10" fill="none" aria-hidden="true">
                      <path d="M1 5l3.5 3.5L11 1.5" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                    <template v-else>{{ index + 1 }}</template>
                  </span>
                  <span class="ct-step-title">{{ title }}</span>
                </div>
              </template>
            </nav>

            <div v-if="errorText" class="ct-error">
              {{ errorText }}
              <button type="button" class="ct-retry" @click="load">重试</button>
            </div>

            <!-- Step 1: select requirement / defect -->
            <section v-show="currentStep === 0" class="ct-step-panel">
              <div class="ct-filter-row">
                <div class="ct-segmented" role="tablist">
                  <button
                    v-for="tab in typeTabs"
                    :key="tab.value"
                    type="button"
                    role="tab"
                    :aria-selected="typeFilter === tab.value"
                    class="ct-segmented-item"
                    :class="{ active: typeFilter === tab.value }"
                    @click="typeFilter = tab.value"
                  >
                    {{ tab.label }}
                  </button>
                </div>
                <div class="ct-search">
                  <input v-model="keyword" type="search" placeholder="请输入标题搜索" aria-label="搜索工作项标题">
                  <svg viewBox="0 0 16 16" width="16" height="16" fill="none" aria-hidden="true">
                    <circle cx="7" cy="7" r="5" stroke="currentColor" stroke-width="1.3" />
                    <path d="M11 11l3.2 3.2" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" />
                  </svg>
                </div>
              </div>

              <div class="ct-table" role="listbox" aria-label="需求或缺陷列表">
                <p v-if="loading" class="ct-placeholder">加载中…</p>
                <p v-else-if="filteredItems.length === 0" class="ct-placeholder">暂无可导入的需求或缺陷</p>
                <div
                  v-for="item in filteredItems"
                  v-else
                  :key="`${item.type}-${item.id}`"
                  class="ct-row"
                  :class="{ selected: isSelected(item) }"
                  role="option"
                  :aria-selected="isSelected(item)"
                  tabindex="0"
                  @click="toggleWorkItem(item)"
                  @keydown.enter.prevent="toggleWorkItem(item)"
                  @keydown.space.prevent="toggleWorkItem(item)"
                >
                  <div class="ct-cell-main">
                    <span class="ct-checkbox" :class="{ checked: isSelected(item) }" aria-hidden="true">
                      <svg viewBox="0 0 10 8" width="10" height="8" fill="none">
                        <path d="M1 4l2.6 2.6L9 1.2" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
                      </svg>
                    </span>
                    <span class="ct-tag" :class="`ct-tag-${item.type}`">{{ typeLabel(item.type) }}</span>
                    <span class="ct-row-title" :title="item.title">{{ item.title }}</span>
                  </div>
                  <div class="ct-cell-side">{{ workspaceLabel(item) }}</div>
                </div>
              </div>
            </section>

            <!-- Step 2: select cloud pipeline -->
            <section v-show="currentStep === 1" class="ct-step-panel">
              <p v-if="loading" class="ct-placeholder">加载中…</p>
              <p v-else-if="agents.length === 0" class="ct-placeholder">暂无可用流水线，请先在云端配置</p>
              <div v-else class="ct-agent-grid">
                <button
                  v-for="(agent, index) in agents"
                  :key="agent.id"
                  type="button"
                  class="ct-agent-card"
                  :class="{ selected: String(selectedAgentID) === String(agent.id) }"
                  :aria-pressed="String(selectedAgentID) === String(agent.id)"
                  @click="selectAgent(agent.id)"
                >
                  <span class="ct-agent-icon">
                    <svg v-if="index % 3 === 0" viewBox="0 0 26 26" width="26" height="26" fill="none" aria-hidden="true">
                      <rect x="3" y="5" width="20" height="13" rx="2" stroke="currentColor" stroke-width="1.95" />
                      <path d="M9 22h8" stroke="currentColor" stroke-width="1.95" stroke-linecap="round" />
                    </svg>
                    <svg v-else-if="index % 3 === 1" viewBox="0 0 26 26" width="26" height="26" fill="none" aria-hidden="true">
                      <ellipse cx="13" cy="6.5" rx="7" ry="3" stroke="currentColor" stroke-width="1.95" />
                      <path d="M6 6.5v13c0 1.66 3.13 3 7 3s7-1.34 7-3v-13" stroke="currentColor" stroke-width="1.95" stroke-linecap="round" />
                      <path d="M6 13c0 1.66 3.13 3 7 3s7-1.34 7-3" stroke="currentColor" stroke-width="1.95" stroke-linecap="round" />
                    </svg>
                    <svg v-else viewBox="0 0 26 26" width="26" height="26" fill="none" aria-hidden="true">
                      <circle cx="13" cy="13" r="9.5" stroke="currentColor" stroke-width="1.95" />
                      <circle cx="13" cy="13" r="5.5" stroke="currentColor" stroke-width="1.95" />
                      <circle cx="13" cy="13" r="2" stroke="currentColor" stroke-width="1.95" />
                    </svg>
                  </span>
                  <span class="ct-agent-name">{{ agent.name }}</span>
                  <span class="ct-agent-desc">
                    {{ agent.description || `${agent.steps?.length || 0} 个 Agent 编排步骤` }}
                  </span>
                </button>
              </div>
            </section>

            <!-- Step 3: set workspace -->
            <section v-show="currentStep === 2" class="ct-step-panel ct-step-form">
              <div class="ct-field">
                <label class="ct-label required">Agent 编排执行配置</label>
                <p v-if="!selectedAgent?.steps?.length" class="ct-placeholder">流水线没有可执行的 Agent 编排</p>
                <div v-for="(step, index) in selectedAgent?.steps || []" :key="String(step.id || step.uuid)" class="ct-runtime-step">
                  <div class="ct-runtime-title"><span>{{ index + 1 }}</span><strong>{{ step.name }}</strong></div>
                  <div class="ct-runtime-fields">
                    <input v-model="stepConfigs[index].cli_type" type="text" placeholder="CLI 类型，如 codex" aria-label="CLI 类型">
                    <input v-model="stepConfigs[index].model" type="text" placeholder="模型，如 gpt-5" aria-label="模型类型">
                  </div>
                </div>
                <p class="ct-hint">每一个 Agent 编排都必须指定本机 CLI 和模型，导入后会保存为本次任务的永久快照。</p>
              </div>
              <div class="ct-field">
                <label class="ct-label required" for="ct-main-workdir">工作目录地址</label>
                <button
                  v-if="isDesktopRuntime()"
                  id="ct-main-workdir"
                  type="button"
                  class="ct-directory-button"
                  :title="mainWorkDir || '选择主工作目录'"
                  @click="chooseMainWorkDir"
                >
                  <svg class="ct-input-icon" viewBox="0 0 20 20" width="20" height="20" fill="none" aria-hidden="true">
                    <path d="M2.5 5.5A1.5 1.5 0 014 4h3.6l1.4 2h7A1.5 1.5 0 0117.5 7.5v7A1.5 1.5 0 0116 16H4a1.5 1.5 0 01-1.5-1.5v-9z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
                  </svg>
                  <span class="ct-directory-path" :class="{ placeholder: !mainWorkDir }">
                    {{ mainWorkDir || '点击选择主工作目录' }}
                  </span>
                  <span class="ct-directory-action">{{ mainWorkDir ? '重新选择' : '选择文件夹' }}</span>
                  <span class="ct-input-counter">{{ workDirs.length }}/{{ MAX_WORK_DIRS }}</span>
                </button>
                <div v-else class="ct-input">
                  <svg class="ct-input-icon" viewBox="0 0 20 20" width="20" height="20" fill="none" aria-hidden="true">
                    <path d="M2.5 5.5A1.5 1.5 0 014 4h3.6l1.4 2h7A1.5 1.5 0 0117.5 7.5v7A1.5 1.5 0 0116 16H4a1.5 1.5 0 01-1.5-1.5v-9z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
                  </svg>
                  <input
                    id="ct-main-workdir"
                    v-model="mainWorkDir"
                    type="text"
                    placeholder="如 /home/projects/my-app 或 C:\Projects\my-app"
                  >
                  <span class="ct-input-counter">{{ workDirs.length }}/{{ MAX_WORK_DIRS }}</span>
                </div>
                <p class="ct-hint">
                  CLI 在主工作目录启动；可添加子工作目录，Agent 可通过绝对路径访问它们。
                  <button type="button" class="ct-link" :disabled="!canAddSubWorkDir" @click="addSubWorkDir">
                    + {{ isDesktopRuntime() ? '选择子工作目录' : '添加子工作目录' }}
                  </button>
                </p>
              </div>

              <div v-for="(_, index) in subWorkDirs" :key="`sub-${index}`" class="ct-field">
                <label class="ct-label" :for="`ct-sub-workdir-${index}`">子工作目录 {{ index + 1 }}</label>
                <div v-if="isDesktopRuntime()" class="ct-directory-row">
                  <button
                    :id="`ct-sub-workdir-${index}`"
                    type="button"
                    class="ct-directory-button"
                    :title="subWorkDirs[index]"
                    @click="chooseSubWorkDir(index)"
                  >
                    <svg class="ct-input-icon" viewBox="0 0 20 20" width="20" height="20" fill="none" aria-hidden="true">
                      <path d="M2.5 5.5A1.5 1.5 0 014 4h3.6l1.4 2h7A1.5 1.5 0 0117.5 7.5v7A1.5 1.5 0 0116 16H4a1.5 1.5 0 01-1.5-1.5v-9z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
                    </svg>
                    <span class="ct-directory-path">{{ subWorkDirs[index] }}</span>
                    <span class="ct-directory-action">重新选择</span>
                  </button>
                  <button type="button" class="ct-input-remove ct-directory-remove" aria-label="移除该子工作目录" @click="removeSubWorkDir(index)">
                    移除
                  </button>
                </div>
                <div v-else class="ct-input">
                  <svg class="ct-input-icon" viewBox="0 0 20 20" width="20" height="20" fill="none" aria-hidden="true">
                    <path d="M2.5 5.5A1.5 1.5 0 014 4h3.6l1.4 2h7A1.5 1.5 0 0117.5 7.5v7A1.5 1.5 0 0116 16H4a1.5 1.5 0 01-1.5-1.5v-9z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
                  </svg>
                  <input
                    :id="`ct-sub-workdir-${index}`"
                    v-model="subWorkDirs[index]"
                    type="text"
                    placeholder="请输入需要关联处理的其他目录"
                  >
                  <button type="button" class="ct-input-remove" aria-label="移除该子工作目录" @click="removeSubWorkDir(index)">
                    移除
                  </button>
                </div>
              </div>

            </section>
          </div>

          <!-- Dialog footer -->
          <footer class="ct-footer">
            <span class="ct-summary">{{ summaryText }}</span>
            <div class="ct-actions">
              <button type="button" class="ct-btn" @click="close">取 消</button>
              <button v-if="currentStep > minimumStep" type="button" class="ct-btn" @click="goPrev">上一步</button>
              <button
                type="button"
                class="ct-btn primary"
                :disabled="nextDisabled || creating"
                @click="goNext"
              >
                {{ currentStep === 2 ? (creating ? '创建中…' : '创建任务') : '下一步' }}
              </button>
            </div>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ct-mask {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 189px 16px 24px;
  overflow: auto;
  background: rgb(0 0 0 / 45%);
}
.ct-modal {
  display: flex;
  flex-direction: column;
  width: 746px;
  max-width: 100%;
  overflow: hidden;
  font-family: 'PingFang SC', 'Microsoft YaHei', sans-serif;
  background: #fff;
  border-radius: 16px;
  box-shadow: 0 12px 48px rgb(0 0 0 / 18%);
}
.ct-runtime-step {
  display: grid;
  grid-template-columns: minmax(150px, 1fr) 2fr;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
  padding: 10px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fafafa;
}
.ct-runtime-title { display: flex; min-width: 0; align-items: center; gap: 8px; }
.ct-runtime-title > span { display: inline-flex; width: 24px; height: 24px; flex: 0 0 24px; align-items: center; justify-content: center; border-radius: 50%; color: #fff; background: #3157e2; font-size: 12px; }
.ct-runtime-title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ct-runtime-fields { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.ct-runtime-fields input { min-width: 0; height: 34px; padding: 0 10px; border: 1px solid #d9d9d9; border-radius: 6px; outline: none; }
.ct-runtime-fields input:focus { border-color: #3157e2; box-shadow: 0 0 0 2px rgba(49,87,226,.1); }

/* Header */
.ct-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 56px;
  padding: 16px 24px;
}
.ct-header-title {
  font-size: 16px;
  font-weight: 600;
  color: #262626;
}
.ct-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  color: rgb(0 0 0 / 45%);
  background: none;
  border: 0;
  cursor: pointer;
}
.ct-close:hover {
  color: #262626;
}

/* Body */
.ct-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 414px;
  max-height: calc(100vh - 300px);
}

/* Step bar */
.ct-steps {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  gap: 16px;
  height: 80px;
  padding: 24px 28px;
}
.ct-step {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ct-step-index {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  font-size: 14px;
  font-weight: 600;
  color: #9aa8bc;
  background: #f0f3f8;
  border: 1px solid transparent;
  border-radius: 16px;
}
.ct-step-title {
  font-size: 16px;
  font-weight: 600;
  color: #9aa8bc;
  white-space: nowrap;
}
.ct-step.active .ct-step-index {
  color: #fff;
  background: #3157e2;
}
.ct-step.done .ct-step-index {
  color: #3157e2;
  background: #fff;
  border-color: #3157e2;
}
.ct-step.active .ct-step-title,
.ct-step.done .ct-step-title {
  color: #262626;
}
.ct-step-line {
  flex: none;
  width: 50px;
  height: 2px;
  background: #dce4ef;
}

/* Error hint */
.ct-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 0 24px 12px;
  padding: 8px 12px;
  font-size: 13px;
  color: #cf1322;
  background: #fff1f0;
  border: 1px solid #ffccc7;
  border-radius: 6px;
}
.ct-retry {
  color: #cf1322;
  background: none;
  border: 0;
  cursor: pointer;
  text-decoration: underline;
}

/* Step panel */
.ct-step-panel {
  flex: 1;
  min-height: 0;
  padding: 0 24px 16px;
  overflow-y: auto;
}
.ct-placeholder {
  margin: 48px 0;
  font-size: 14px;
  color: #8c8c8c;
  text-align: center;
}

/* Step 1: filter area */
.ct-filter-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  margin-bottom: 17px;
}
.ct-segmented {
  display: flex;
  gap: 2px;
  padding: 2px;
  background: #edeff2;
  border-radius: 6px;
}
.ct-segmented-item {
  min-width: 60px;
  height: 28px;
  padding: 3px 16px;
  font-size: 14px;
  color: #262626;
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
}
.ct-segmented-item.active {
  color: #2475fc;
  background: #fff;
}
.ct-search {
  position: relative;
  width: 240px;
  height: 32px;
}
.ct-search input {
  width: 100%;
  height: 100%;
  padding: 0 32px 0 12px;
  font-size: 14px;
  color: #262626;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  outline: none;
}
.ct-search input::placeholder {
  color: rgb(0 0 0 / 25%);
}
.ct-search input:focus {
  border-color: #3157e2;
}
.ct-search svg {
  position: absolute;
  top: 8px;
  right: 10px;
  color: rgb(0 0 0 / 65%);
  pointer-events: none;
}

/* Step 1: table */
.ct-table {
  border-top: 1px solid #f0f0f0;
}
.ct-row {
  display: flex;
  align-items: center;
  height: 40px;
  padding: 0 8px;
  border-bottom: 1px solid #f0f0f0;
  cursor: pointer;
}
.ct-row:hover {
  background: #f7f9fc;
}
.ct-row.selected {
  background: #f2f4f7;
}
.ct-row:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}
.ct-cell-main {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.ct-cell-side {
  flex: none;
  width: 177px;
  padding-left: 8px;
  font-size: 12px;
  color: #8c8c8c;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ct-checkbox {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  color: transparent;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 2px;
}
.ct-checkbox.checked {
  color: #fff;
  background: #2475fc;
  border-color: #2475fc;
}
.ct-tag {
  flex: none;
  padding: 0 3px;
  font-family: Inter, sans-serif;
  font-size: 10px;
  line-height: 14px;
  color: #fff;
  border-radius: 2px;
}
.ct-tag-requirement {
  background: #3582fb;
}
.ct-tag-defect {
  background: #f85e5e;
}
.ct-tag-task {
  background: #6524fc;
}
.ct-row-title {
  flex: 1;
  min-width: 0;
  font-family: Inter, 'PingFang SC', sans-serif;
  font-size: 14px;
  font-weight: 500;
  color: #182b50;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ct-cell-main .ct-tag + .ct-row-title {
  margin-left: -12px;
}

/* Step 2: Agent card */
.ct-agent-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  padding: 6px 0;
}
.ct-agent-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 221px;
  padding: 48px 20px;
  background: #fff;
  border: 1px solid #e4e6eb;
  border-radius: 14px;
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
}
.ct-agent-card:hover {
  border-color: #3157e2;
}
.ct-agent-card.selected {
  background: #f5f9ff;
  border-color: #3157e2;
}
.ct-agent-icon {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 50px;
  height: 50px;
  color: #3157e2;
  background: #edf4ff;
  border-radius: 15px;
}
.ct-agent-card.selected .ct-agent-icon {
  color: #fff;
  background: #3157e2;
}
.ct-agent-name {
  margin-top: 16px;
  font-size: 18px;
  font-weight: 600;
  color: #262626;
  text-align: center;
}
.ct-agent-desc {
  margin-top: 10px;
  font-size: 14px;
  color: #8c8c8c;
  text-align: center;
}

/* Step 3: form */
.ct-step-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 4px 95px 20px;
}
.ct-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ct-label {
  font-size: 14px;
  color: #262626;
}
.ct-label.required::before {
  margin-right: 2px;
  color: #fb363f;
  content: '*';
}
.ct-input {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 9px 12px;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
}
.ct-input:focus-within {
  border-color: #3157e2;
}
.ct-input-icon {
  flex: none;
  color: #8594aa;
}
.ct-input input {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  color: #262626;
  background: none;
  border: 0;
  outline: none;
}
.ct-input input::placeholder {
  color: rgb(0 0 0 / 25%);
}
.ct-input-counter {
  flex: none;
  font-size: 14px;
  color: #bfbfbf;
}
.ct-directory-button {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  height: 40px;
  padding: 9px 12px;
  color: #262626;
  text-align: left;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  cursor: pointer;
  font: inherit;
}
.ct-directory-button:hover,
.ct-directory-button:focus-visible {
  border-color: #3157e2;
  outline: none;
}
.ct-directory-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ct-directory-path.placeholder {
  color: rgb(0 0 0 / 45%);
}
.ct-directory-action {
  flex: none;
  color: #1677ff;
}
.ct-directory-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.ct-directory-remove {
  padding: 0 4px;
}
.ct-input-remove {
  flex: none;
  font-size: 13px;
  color: #f85e5e;
  background: none;
  border: 0;
  cursor: pointer;
}
.ct-hint {
  margin: 0;
  font-size: 13px;
  line-height: 20px;
  color: #8c8c8c;
}
.ct-hint--warning {
  color: #d46b08;
}
.ct-link {
  padding: 0;
  font-size: 13px;
  color: #3157e2;
  background: none;
  border: 0;
  cursor: pointer;
}
.ct-link:disabled {
  color: #bfbfbf;
  cursor: not-allowed;
}
.ct-select {
  width: 100%;
}
.ct-select :deep(.ant-select-selector) {
  min-height: 40px;
  padding: 4px 12px;
  border-color: #d9d9d9 !important;
  border-radius: 6px;
}
.ct-select :deep(.ant-select-selection-search-input),
.ct-select :deep(.ant-select-selection-item),
.ct-select :deep(.ant-select-selection-placeholder) {
  line-height: 30px;
}

/* Footer */
.ct-footer {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  height: 52px;
  padding: 10px 24px;
}
.ct-summary {
  font-size: 14px;
  color: #595959;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ct-actions {
  display: flex;
  flex: none;
  gap: 8px;
}
.ct-btn {
  height: 32px;
  padding: 5px 16px;
  font-size: 14px;
  color: #595959;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  cursor: pointer;
}
.ct-btn:hover {
  color: #3157e2;
  border-color: #3157e2;
}
.ct-btn.primary {
  color: #fff;
  background: #3157e2;
  border-color: #3157e2;
}
.ct-btn.primary:hover {
  background: #2647c4;
  border-color: #2647c4;
}
.ct-btn:disabled {
  color: rgb(0 0 0 / 25%);
  background: #f5f5f5;
  border-color: #d9d9d9;
  cursor: not-allowed;
}

.ct-fade-enter-active,
.ct-fade-leave-active {
  transition: opacity 0.2s ease;
}
.ct-fade-enter-from,
.ct-fade-leave-to {
  opacity: 0;
}
</style>
