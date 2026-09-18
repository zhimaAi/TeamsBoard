<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

interface WorkItem {
  type: 'requirement' | 'defect'
  id: number
  workspace_id: number
  title: string
}
interface Agent {
  id: number
  name: string
  color: string
  cli_type: string
  workflow_version: number
}
interface AgentSnapshot extends Agent {
  steps: Array<{ step_key: string; name: string; sort_order: number; cli_type: string; prompt?: string }>
}
const router = useRouter()
const loading = ref(false)
const creating = ref(false)
const errorText = ref('')
const workItems = ref<WorkItem[]>([])
const agents = ref<Agent[]>([])
const selectedWorkItem = ref<WorkItem>()
const selectedAgentID = ref<number>()
const agentSnapshot = ref<AgentSnapshot>()
const workDirs = ref<string[]>([''])
const keyword = ref('')
const typeFilter = ref<string>('all')

const filteredItems = computed(() => workItems.value.filter((item) => {
  const matchesType = typeFilter.value === 'all' || item.type === typeFilter.value
  const matchesKeyword = !keyword.value || item.title.toLowerCase().includes(keyword.value.toLowerCase())
    || String(item.id).includes(keyword.value)
  return matchesType && matchesKeyword
}))
const workflowSteps = computed(() => agentSnapshot.value?.steps.map((step) => ({
  title: step.name,
  description: `${step.cli_type || agentSnapshot.value?.cli_type || 'CLI'} · ${step.step_key}`,
})) || [])
const normalizedWorkDirs = computed(() => workDirs.value.map(workDir => workDir.trim()))
const isTaskReady = computed(() => Boolean(
  selectedWorkItem.value
  && selectedAgentID.value
  && normalizedWorkDirs.value.length > 0
  && normalizedWorkDirs.value.every(Boolean),
))

async function load() {
  loading.value = true
  errorText.value = ''
  selectedWorkItem.value = undefined
  try {
    const [workItemResult, agentResult] = await Promise.all([
      apiClient.get<{ items: WorkItem[] }>('/tasks/work-items'),
      apiClient.get<{ items: Agent[] }>('/tasks/agents'),
    ])
    workItems.value = workItemResult.items || []
    agents.value = agentResult.items || []
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : t('workflows.task.import.cloudLoadFailed')
  } finally {
    loading.value = false
  }
}

async function chooseAgent(id: number) {
  selectedAgentID.value = id
  agentSnapshot.value = undefined
  try {
    agentSnapshot.value = await apiClient.get<AgentSnapshot>(`/tasks/agents/${id}/snapshot`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.import.agentLoadFailed'))
  }
}

function addWorkDir() {
  workDirs.value.push('')
}

function removeWorkDir(index: number) {
  if (index === 0) return
  workDirs.value.splice(index, 1)
}

async function chooseWorkDir(index: number) {
  const selected = await selectDirectory(workDirs.value[index])
  if (selected) workDirs.value[index] = selected
}

async function createTask() {
  if (!selectedWorkItem.value) {
    message.warning(t('workflows.task.import.chooseWorkItem'))
    return
  }
  if (!selectedAgentID.value) {
    message.warning(t('workflows.task.import.chooseAgent'))
    return
  }
  if (normalizedWorkDirs.value.length === 0 || normalizedWorkDirs.value.some(workDir => !workDir)) {
    message.warning(t('workflows.task.import.workDirectoriesRequired'))
    return
  }
  const uniqueWorkDirs = new Set(normalizedWorkDirs.value.map(workDir => workDir.toLowerCase()))
  if (uniqueWorkDirs.size !== normalizedWorkDirs.value.length) {
    message.warning(t('workflows.task.import.duplicateDirectories'))
    return
  }
  creating.value = true
  try {
    const result = await apiClient.post<{ uuid: string }>('/tasks', {
      work_item_type: selectedWorkItem.value.type,
      work_item_id: selectedWorkItem.value.id,
      agent_id: selectedAgentID.value,
      work_dir: normalizedWorkDirs.value[0],
      work_dirs: normalizedWorkDirs.value,
    })
    message.success(t('workflows.task.import.created'))
    router.push(`/workflows/task/${result.uuid}`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.taskCreateFailed'))
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="import-page">
    <a-page-header :title="t('workflows.task.import.title')" :sub-title="t('workflows.task.import.subtitle')" @back="router.push('/workflows')" />

    <a-alert
      v-if="errorText"
      type="error"
      show-icon
      :message="t('workflows.task.import.alert')"
      :description="errorText"
      style="margin-bottom: 16px"
    >
      <template #action><a-button danger @click="load">{{ t('common.actions.retry') }}</a-button></template>
    </a-alert>

    <a-spin :spinning="loading">
      <div class="import-grid">
        <a-card :title="t('workflows.task.import.chooseItemStep')">
          <template #extra><span class="muted">{{ t('workflows.task.import.itemCount', { count: filteredItems.length }) }}</span></template>
          <a-space style="margin-bottom: 14px">
            <a-input-search v-model:value="keyword" :placeholder="t('workflows.task.import.search')" allow-clear />
            <a-radio-group v-model:value="typeFilter" button-style="solid">
              <a-radio-button value="all">{{ t('workflows.task.import.all') }}</a-radio-button>
              <a-radio-button value="requirement">{{ t('workflows.task.import.requirement') }}</a-radio-button>
              <a-radio-button value="defect">{{ t('workflows.task.import.defect') }}</a-radio-button>
            </a-radio-group>
          </a-space>
          <a-empty
            v-if="!loading && filteredItems.length === 0"
            :description="t('workflows.task.import.emptyItems')"
          />
          <div v-else class="select-list">
            <button
              v-for="item in filteredItems"
              :key="`${item.type}-${item.id}`"
              type="button"
              class="select-item"
              :class="{ selected: selectedWorkItem?.id === item.id && selectedWorkItem?.type === item.type }"
              @click="selectedWorkItem = item"
            >
              <a-tag :color="item.type === 'defect' ? 'red' : 'blue'">
                {{ item.type === 'defect' ? t('workflows.task.import.defect') : t('workflows.task.import.requirement') }}
              </a-tag>
              <span class="item-title">{{ item.title }}</span>
              <span class="muted">#{{ item.id }}</span>
            </button>
          </div>
        </a-card>

        <a-card :title="t('workflows.task.import.chooseAgentStep')">
          <a-empty v-if="!loading && agents.length === 0" :description="t('workflows.task.import.emptyAgents')" />
          <div v-else class="agent-list">
            <button
              v-for="agent in agents"
              :key="agent.id"
              type="button"
              class="agent-item"
              :class="{ selected: selectedAgentID === agent.id }"
              @click="chooseAgent(agent.id)"
            >
              <span class="agent-color" :style="{ background: agent.color || '#3157e2' }" />
              <span>
                <strong>{{ agent.name }}</strong>
                <small>{{ agent.cli_type || t('workflows.task.import.defaultCli') }} · {{ t('workflows.task.import.workflowVersion', { version: agent.workflow_version || 1 }) }}</small>
              </span>
            </button>
          </div>

          <div v-if="agentSnapshot" class="workflow-preview">
            <h4>{{ t('workflows.task.import.workflowSteps') }}</h4>
            <a-steps
              direction="vertical"
              size="small"
              :current="-1"
              :items="workflowSteps"
            />
          </div>
        </a-card>
      </div>

      <a-card :title="t('workflows.task.import.directoriesStep')" class="work-dir-card">
        <template #extra>
          <a-button type="link" @click="addWorkDir">{{ t('workflows.task.import.addDirectory') }}</a-button>
        </template>
        <div class="work-dir-list">
          <div v-for="(_, index) in workDirs" :key="index" class="work-dir-row">
            <a-form-item
              :label="index === 0 ? t('workflows.task.import.mainDirectory') : t('workflows.task.import.relatedDirectory', { index })"
              required
              class="work-dir-form-item"
            >
              <div class="work-dir-picker">
                <a-input
                  v-model:value="workDirs[index]"
                  :placeholder="index === 0
                    ? t('workflows.task.import.mainDirectoryPlaceholder')
                    : t('workflows.task.import.relatedDirectoryPlaceholder')"
                  @press-enter="createTask"
                />
                <a-button v-if="isDesktopRuntime()" @click="chooseWorkDir(index)">{{ t('workflows.task.import.browse') }}</a-button>
              </div>
            </a-form-item>
            <a-button v-if="index > 0" danger class="work-dir-remove" @click="removeWorkDir(index)">
              {{ t('workflows.task.import.remove') }}
            </a-button>
          </div>
        </div>
        <span class="muted">
          {{ t('workflows.task.import.directoryHint') }}
        </span>
      </a-card>

    </a-spin>

    <div class="action-bar">
      <span v-if="isTaskReady">
        {{ t('workflows.task.import.ready', { title: selectedWorkItem?.title || '' }) }}
      </span>
      <span v-else class="muted">{{ t('workflows.task.import.notReady') }}</span>
      <a-space>
        <a-button @click="router.push('/workflows')">{{ t('common.actions.cancel') }}</a-button>
        <a-button type="primary" :loading="creating" @click="createTask">{{ t('workflows.task.import.create') }}</a-button>
      </a-space>
    </div>
  </div>
</template>

<style scoped>
.import-page {
  max-width: 1180px;
  margin: 0 auto;
  padding-bottom: 76px;
}
.import-grid {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(320px, 2fr);
  gap: 16px;
}
.work-dir-card {
  margin-top: 16px;
}
.work-dir-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.work-dir-row {
  display: flex;
  align-items: flex-end;
  gap: 10px;
}
.work-dir-picker {
  display: flex;
  width: 100%;
  gap: 8px;
}
.work-dir-form-item {
  flex: 1;
  margin-bottom: 8px;
}
.work-dir-remove {
  margin-bottom: 8px;
}
.select-list,
.agent-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 460px;
  overflow: auto;
}
.select-item,
.agent-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 12px;
  color: inherit;
  text-align: left;
  border: 1px solid #e8e8e8;
  border-radius: 7px;
  background: #fff;
  cursor: pointer;
}
.select-item:hover,
.agent-item:hover,
.select-item.selected,
.agent-item.selected {
  border-color: #3157e2;
  background: #f3f6ff;
}
.item-title {
  flex: 1;
}
.muted {
  color: #8c8c8c;
}
.agent-color {
  width: 12px;
  height: 36px;
  border-radius: 6px;
}
.agent-item span:nth-child(2) {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.agent-item small {
  color: #8c8c8c;
}
.workflow-preview {
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}
.action-bar {
  position: fixed;
  right: 24px;
  bottom: 0;
  left: 244px;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 24px;
  border-top: 1px solid #e8e8e8;
  background: #fff;
  box-shadow: 0 -4px 16px rgb(0 0 0 / 5%);
}
</style>
