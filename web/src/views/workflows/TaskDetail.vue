<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import apiClient from '@/api/client'
import MarkdownPreview from '@/components/MarkdownPreview.vue'
import TaskExecutionConsole from '@/components/TaskExecutionConsole.vue'
import CliSelectModal from '@/components/CliSelectModal.vue'
import { useDocumentTitle } from '@/composables/useDocumentTitle'

interface TaskDocument {
  uuid: string
  client_document_id: string
  name: string
  content: string
  updated_at: number
}

interface Step {
  uuid: string
  step_key: string
  name: string
  sort_order: number
  cli_type: string
  status: string
  execution_status: string
  prompt_snapshot: string
  documents: TaskDocument[]
}
interface Task {
  uuid: string
  title: string
  status: string
  execution_status: string
  agent_id: string
  cli_type: string
  work_item_type: string
  work_item_id: string
  work_dir: string
  work_dirs: string[]
  api_collection_id: number
  api_collection_name: string
  api_folder_id: number
  api_folder_name: string
  created_at: number
  updated_at: number
  steps: Step[]
}
const route = useRoute()
const router = useRouter()
const props = withDefaults(defineProps<{
  taskUuid?: string
  embedded?: boolean
}>(), {
  embedded: false,
})
const emit = defineEmits<{
  close: []
}>()
const resolvedTaskUuid = computed(() => {
  if (props.taskUuid) return props.taskUuid
  const routeUuid = route.params.taskUuid
  return Array.isArray(routeUuid) ? routeUuid[0] || '' : routeUuid || ''
})
const loading = ref(false)
const task = ref<Task>()
const taskTitle = computed(() => task.value?.title)

useDocumentTitle(taskTitle)

const runningStep = ref<string>()
const deleting = ref(false)
const consoleOpen = ref(false)
const consoleStepKey = ref<string>()
const consoleSessionUuid = ref<string>()
const selectedStepKey = ref('')
const selectedContentKey = ref('prompt')
const cliModalOpen = ref(false)
const pendingRunStep = ref<Step>()
const statusOptions = [
  { label: '活跃中', value: 'active' },
  { label: '待开始', value: 'pending' },
  { label: '开发中', value: 'developing' },
  { label: '开发完成', value: 'developed' },
]

const completedCount = computed(() => task.value?.steps.filter(
  (step) => step.execution_status === 'success' || step.status === 'completed',
).length || 0)
const selectedStep = computed(() => task.value?.steps.find(
  (step) => step.step_key === selectedStepKey.value,
))
const selectedDocument = computed(() => selectedStep.value?.documents?.find(
  (document) => `document:${document.uuid || document.client_document_id || document.name}` === selectedContentKey.value,
))

async function load() {
  const taskUuid = resolvedTaskUuid.value
  if (!taskUuid) return
  loading.value = true
  try {
    const loadedTask = await apiClient.get<Task>(`/tasks/${taskUuid}`)
    if (taskUuid !== resolvedTaskUuid.value) return
    task.value = loadedTask
    if (!loadedTask.steps.some(step => step.step_key === selectedStepKey.value)) {
      selectedStepKey.value = loadedTask.steps[0]?.step_key || ''
      selectedContentKey.value = 'prompt'
    }
  } catch (error) {
    if (taskUuid !== resolvedTaskUuid.value) return
    message.error(error instanceof Error ? error.message : '任务详情加载失败')
  } finally {
    if (taskUuid === resolvedTaskUuid.value) loading.value = false
  }
}

async function updateStatus(status: string) {
  if (!task.value) return
  const taskUuid = resolvedTaskUuid.value
  try {
    await apiClient.put(`/tasks/${taskUuid}/status`, { status })
    task.value.status = status
    message.success('任务状态已更新')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '状态更新失败')
  }
}

async function deleteTask() {
  const taskUuid = resolvedTaskUuid.value
  Modal.confirm({
    title: '确认删除任务',
    content: '删除后该任务及其步骤、会话等数据将无法恢复，确定要删除吗？',
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    onOk: async () => {
      deleting.value = true
      try {
        await apiClient.delete(`/tasks/${taskUuid}`)
        message.success('任务已删除')
        if (props.embedded) {
          emit('close')
        } else {
          router.push('/workflows')
        }
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
      } finally {
        deleting.value = false
      }
    },
  })
}

async function runStep(step: Step) {
  if (!task.value?.work_dir && !task.value?.work_dirs?.length) {
    message.error('该任务没有工作目录，请重新创建任务')
    return
  }
  pendingRunStep.value = step
  cliModalOpen.value = true
}

async function onCliConfirm(payload: { cli_type: string; model: string }) {
  const step = pendingRunStep.value
  if (!step) return
  pendingRunStep.value = undefined
  runningStep.value = step.step_key
  const taskUuid = resolvedTaskUuid.value
  try {
    const result = await apiClient.post<{ session_uuid: string }>(
      `/tasks/${taskUuid}/steps/${encodeURIComponent(step.step_key)}/runs`,
      {
        request_id: crypto.randomUUID(),
        cli_type: payload.cli_type,
        model: payload.model,
      },
    )
    message.success(`步骤已启动，会话 ${result.session_uuid.slice(0, 8)}`)
    consoleStepKey.value = step.step_key
    consoleSessionUuid.value = result.session_uuid
    consoleOpen.value = true
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '步骤启动失败')
  } finally {
    runningStep.value = undefined
  }
}

function openConsole(stepKey?: string) {
  consoleStepKey.value = stepKey || task.value?.steps[0]?.step_key
  consoleSessionUuid.value = undefined
  consoleOpen.value = true
}

function selectStep(step: Step) {
  selectedStepKey.value = step.step_key
  selectedContentKey.value = 'prompt'
}

function documentContentKey(document: TaskDocument) {
  return `document:${document.uuid || document.client_document_id || document.name}`
}

function statusColor(status: string) {
  return { success: 'green', running: 'processing', failed: 'red', stopped: 'orange' }[status] || 'default'
}

function returnToBoard() {
  if (props.embedded) {
    emit('close')
  } else {
    router.push('/workflows')
  }
}

watch(resolvedTaskUuid, (taskUuid) => {
  task.value = undefined
  selectedStepKey.value = ''
  selectedContentKey.value = 'prompt'
  consoleOpen.value = false
  cliModalOpen.value = false
  if (taskUuid) void load()
}, { immediate: true })
</script>

<template>
  <div class="task-detail-page" :class="{ 'task-detail-page--embedded': embedded }">
    <a-spin :spinning="loading">
      <template v-if="task">
        <div class="task-workflow-shell">
          <header class="task-workflow-header">
            <div class="task-workflow-header__title-row">
              <h1 class="task-workflow-header__title" :title="task.title">{{ task.title }}</h1>
              <div class="task-workflow-header__actions">
                <a-button class="task-workflow-home-btn" title="返回任务清单" @click="returnToBoard">
                  ←
                </a-button>
                <a-button :loading="loading" @click="load">刷新</a-button>
                <a-button class="workflow-primary-button" @click="openConsole()">打开执行窗口</a-button>
                <a-select class="status-select" :value="task.status" @change="updateStatus">
                  <a-select-option v-for="option in statusOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </a-select-option>
                </a-select>
                <a-button danger :loading="deleting" @click="deleteTask">删除</a-button>
              </div>
            </div>

            <div class="task-workflow-header__meta">
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">任务来源</span>
                <span class="task-workflow-header__field-value">
                  {{ task.work_item_type === 'defect' ? '缺陷' : '需求' }} #{{ task.work_item_id }}
                </span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">任务状态</span>
                <span class="task-workflow-header__field-value">
                  {{ statusOptions.find(item => item.value === task?.status)?.label || task.status }}
                </span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">执行状态</span>
                <span class="task-workflow-header__field-value">{{ task.execution_status || '-' }}</span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">Agent</span>
                <span class="task-workflow-header__field-value">#{{ task.agent_id }}</span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">执行 CLI</span>
                <span class="task-workflow-header__field-value">{{ task.cli_type || task.steps[0]?.cli_type || '-' }}</span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">步骤进度</span>
                <span class="task-workflow-header__field-value">{{ completedCount }} / {{ task.steps.length }}</span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">接口集合</span>
                <span class="task-workflow-header__field-value">
                  {{ task.api_collection_name || '-' }}<template v-if="task.api_collection_id"> #{{ task.api_collection_id }}</template>
                </span>
              </div>
              <div class="task-workflow-header__field">
                <span class="task-workflow-header__field-label">接口文件夹</span>
                <span class="task-workflow-header__field-value">
                  {{ task.api_folder_name || '-' }}<template v-if="task.api_folder_id"> #{{ task.api_folder_id }}</template>
                </span>
              </div>
              <div class="task-workflow-header__field task-workflow-header__field--directory">
                <span class="task-workflow-header__field-label">工作目录</span>
                <span
                  class="task-workflow-header__directory-list"
                  :title="(task.work_dirs?.length ? task.work_dirs : [task.work_dir]).filter(Boolean).join('\n')"
                >
                  <span
                    v-for="(workDir, index) in (task.work_dirs?.length ? task.work_dirs : [task.work_dir]).filter(Boolean)"
                    :key="workDir"
                    class="task-workflow-header__directory"
                  >
                    {{ index === 0 ? '主目录' : `关联目录 ${index}` }}：{{ workDir }}
                  </span>
                  <span v-if="!task.work_dir && !task.work_dirs?.length">未配置</span>
                </span>
              </div>
            </div>
          </header>

          <div v-if="task.steps.length > 0" class="task-workflow-nodes" aria-label="工作流步骤">
            <button
              v-for="step in task.steps"
              :key="step.uuid"
              type="button"
              class="task-workflow-node"
              :class="{
                'task-workflow-node--active': selectedStepKey === step.step_key,
                'task-workflow-node--success': step.execution_status === 'success' || step.status === 'completed',
                'task-workflow-node--running': step.execution_status === 'running',
                'task-workflow-node--failed': step.execution_status === 'failed',
              }"
              @click="selectStep(step)"
            >
              <div class="task-workflow-node__row">
                <span class="task-workflow-node__badge">{{ step.sort_order }}</span>
                <span class="task-workflow-node__label" :title="step.name">{{ step.name }}</span>
                <span
                  class="task-workflow-node__status"
                  :class="{
                    'task-workflow-node__status--success': step.execution_status === 'success' || step.status === 'completed',
                    'task-workflow-node__status--running': step.execution_status === 'running',
                    'task-workflow-node__status--failed': step.execution_status === 'failed',
                  }"
                >
                  {{ step.execution_status === 'success' || step.status === 'completed' ? '✓' : '●' }}
                </span>
              </div>
            </button>
          </div>

          <section class="task-workflow-content">
            <a-empty v-if="!selectedStep" description="该 Agent 没有配置工作流步骤" />
            <div v-else class="step-tab-layout">
              <aside class="step-tab-sidebar">
                <button
                  type="button"
                  class="step-tab-button step-tab-button--prompt"
                  :class="{ 'step-tab-button--active': selectedContentKey === 'prompt' }"
                  @click="selectedContentKey = 'prompt'"
                >
                  提示词
                </button>
                <button
                  v-for="document in selectedStep.documents || []"
                  :key="documentContentKey(document)"
                  type="button"
                  class="step-tab-button"
                  :class="{ 'step-tab-button--active': selectedContentKey === documentContentKey(document) }"
                  :title="document.name"
                  @click="selectedContentKey = documentContentKey(document)"
                >
                  {{ document.name }}
                </button>

                <div class="step-tab-actions">
                  <div class="step-tab-actions__title">步骤操作</div>
                  <div class="step-tab-actions__status">
                    <span>当前状态</span>
                    <a-tag :color="statusColor(selectedStep.execution_status)">
                      {{ selectedStep.execution_status }}
                    </a-tag>
                  </div>
                  <a-button block @click="openConsole(selectedStep.step_key)">查看对话</a-button>
                  <a-button
                    block
                    type="primary"
                    :loading="runningStep === selectedStep.step_key"
                    :disabled="!task.work_dir || (runningStep !== undefined && runningStep !== selectedStep.step_key)"
                    @click="runStep(selectedStep)"
                  >
                    {{ selectedStep.execution_status === 'success' ? '重新执行' : '执行步骤' }}
                  </a-button>
                  <div v-if="!task.work_dir" class="step-tab-actions__warning">
                    未配置工作目录，无法执行
                  </div>
                </div>
              </aside>

              <div class="step-tab-content">
                <div class="step-content-toolbar">
                  <div>
                    <strong>{{ selectedContentKey === 'prompt' ? selectedStep.name : selectedDocument?.name }}</strong>
                    <span>{{ selectedContentKey === 'prompt' ? 'Prompt' : 'Markdown 文档' }}</span>
                  </div>
                  <a-tag>{{ selectedStep.cli_type || '默认 CLI' }}</a-tag>
                </div>
                <div class="step-markdown-view">
                  <MarkdownPreview
                    v-if="selectedContentKey === 'prompt'"
                    :content="selectedStep.prompt_snapshot"
                    empty-text="该步骤没有 Prompt"
                  />
                  <MarkdownPreview
                    v-else
                    :content="selectedDocument?.content || ''"
                    empty-text="该文档暂无内容"
                  />
                </div>
              </div>
            </div>
          </section>
        </div>
      </template>
      <div v-else-if="!loading" class="task-result">
        <a-result status="404" title="任务不存在">
          <template #extra><a-button type="primary" @click="returnToBoard">返回任务看板</a-button></template>
        </a-result>
      </div>
    </a-spin>

    <TaskExecutionConsole
      v-if="task"
      v-model:open="consoleOpen"
      :task-uuid="resolvedTaskUuid"
      :steps="task.steps"
      :focus-step-key="consoleStepKey"
      :focus-session-uuid="consoleSessionUuid"
      @refresh="load"
    />

    <CliSelectModal
      v-model:open="cliModalOpen"
      @confirm="onCliConfirm"
    />
  </div>
</template>

<style scoped>
.task-detail-page {
  min-height: calc(100vh - 56px);
  margin: -24px;
  padding: 16px;
  box-sizing: border-box;
  background: linear-gradient(180deg, #fdfdfb 0%, #f8faf5 100%);
}

.task-detail-page--embedded {
  width: 100%;
  height: 100%;
  min-height: 0;
  margin: 0;
  padding: 12px;
  overflow: auto;
}

.task-detail-page :deep(.ant-spin-nested-loading),
.task-detail-page :deep(.ant-spin-container) {
  min-height: calc(100vh - 88px);
}

.task-detail-page--embedded :deep(.ant-spin-nested-loading),
.task-detail-page--embedded :deep(.ant-spin-container) {
  height: 100%;
  min-height: 0;
}

.task-workflow-shell {
  display: flex;
  height: calc(100vh - 88px);
  min-height: 0;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.task-detail-page--embedded .task-workflow-shell {
  height: 100%;
  min-height: 620px;
}

.task-workflow-header {
  flex-shrink: 0;
  padding: 20px 24px;
  border: 1px solid #e8e8e0;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}

.task-workflow-header__title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
}

.task-workflow-header__title {
  flex: 1;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: #303133;
  font-size: 22px;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-workflow-header__actions {
  display: flex;
  flex-shrink: 0;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.task-workflow-header__actions :deep(.ant-btn) {
  height: 32px;
  border-color: #e0e0d8;
  border-radius: 7px;
  color: #606266;
  box-shadow: none;
}

.task-workflow-header__actions :deep(.ant-btn:hover) {
  border-color: #9fb39a;
  color: #3a7a3a;
}

.task-workflow-header__actions :deep(.ant-btn-dangerous) {
  color: #c25555;
}

.task-workflow-header__actions :deep(.ant-btn-dangerous:hover) {
  border-color: #d99b9b;
  color: #b33c3c;
}

.task-workflow-header__actions .workflow-primary-button {
  border-color: #cbd8c5;
  background: #f3f8ef;
  color: #3a7a3a;
}

.task-workflow-home-btn {
  width: 32px;
  padding: 0;
  border-radius: 50% !important;
  color: #909399 !important;
  font-size: 18px;
}

.status-select {
  width: 120px;
}

.status-select :deep(.ant-select-selector) {
  border-color: #e0e0d8 !important;
  border-radius: 7px !important;
  box-shadow: none !important;
}

.status-select:hover :deep(.ant-select-selector) {
  border-color: #9fb39a !important;
}

.task-workflow-header__meta {
  display: grid;
  grid-template-columns: repeat(6, minmax(100px, 1fr));
  gap: 10px 20px;
  margin-top: 10px;
  padding: 12px 16px;
  border: 1px solid #e8ecf1;
  border-radius: 8px;
  background: #f9fafb;
}

.task-workflow-header__field {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
}

.task-workflow-header__field--directory {
  grid-column: span 2;
}

.task-workflow-header__directory-list {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.task-workflow-header__directory {
  overflow: hidden;
  color: #303133;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-workflow-header__field-label,
.execution-setting__label {
  color: #909399;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.task-workflow-header__field-value,
.execution-setting__value {
  overflow: hidden;
  color: #303133;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-workflow-nodes {
  display: flex;
  flex-shrink: 0;
  gap: 10px;
  overflow-x: auto;
  padding: 1px 0 2px;
}

.task-workflow-node {
  display: flex;
  min-width: 120px;
  min-height: 46px;
  flex: 1 0 120px;
  align-items: center;
  justify-content: center;
  padding: 8px 10px;
  box-sizing: border-box;
  border: 1px solid #e8e8e0;
  border-radius: 8px;
  background: #fff;
  color: inherit;
  cursor: pointer;
  font: inherit;
  text-align: center;
  transition: border-color 0.2s, background 0.2s, box-shadow 0.2s, transform 0.2s;
}

.task-workflow-node:hover {
  border-color: #b7c9a8;
  transform: translateY(-1px);
}

.task-workflow-node--success {
  border-color: #b7c9a8;
  background: #f3f8ef;
}

.task-workflow-node--running {
  border-color: #ead39e;
  background: #fff9ec;
}

.task-workflow-node--failed {
  border-color: #ecc8c5;
  background: #fff5f4;
}

.task-workflow-node--active {
  border-color: #3a7a3a;
  background: #f3f8ef;
  box-shadow: 0 6px 18px rgba(58, 122, 58, 0.14);
}

.task-workflow-node__row {
  display: flex;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  overflow: hidden;
}

.task-workflow-node__badge {
  display: inline-flex;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #9fb39a;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
}

.task-workflow-node__label {
  overflow: hidden;
  color: #303133;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-workflow-node__status {
  flex-shrink: 0;
  color: #b5b8bd;
  font-size: 12px;
}

.task-workflow-node__status--success {
  color: #74b66f;
  font-size: 15px;
}

.task-workflow-node__status--running {
  color: #d5a442;
}

.task-workflow-node__status--failed {
  color: #c85b55;
}

.task-workflow-content {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  overflow: hidden;
  padding: 16px 20px 20px;
  border: 1px solid #e8e8e0;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}

.step-tab-layout {
  display: grid;
  grid-template-columns: 164px minmax(0, 1fr);
  min-height: 0;
  flex: 1;
  overflow: hidden;
  border: 1px solid #e5e9e1;
  border-radius: 9px;
  background: #fff;
}

.step-tab-sidebar {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 7px;
  overflow-y: auto;
  padding: 10px;
  border-right: 1px solid #e8ece5;
  background: #f9faf8;
}

.step-tab-button {
  width: 100%;
  overflow: hidden;
  padding: 9px 11px;
  border: 1px solid transparent;
  border-radius: 7px;
  background: transparent;
  color: #606266;
  cursor: pointer;
  font: inherit;
  font-size: 12px;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: border-color 0.2s, background 0.2s, color 0.2s;
}

.step-tab-button:hover {
  border-color: #d6e0d1;
  background: #f3f7f0;
  color: #3a7a3a;
}

.step-tab-button--active {
  border-color: #afc4a7;
  background: #eef5e9;
  color: #315f31;
  font-weight: 600;
}

.step-tab-button--prompt {
  margin-bottom: 2px;
}

.step-tab-actions {
  display: flex;
  position: sticky;
  z-index: 1;
  bottom: -10px;
  flex-direction: column;
  flex-shrink: 0;
  gap: 8px;
  margin-top: auto;
  padding: 12px 0 10px;
  border-top: 1px solid #e2e7df;
  background: #f9faf8;
}

.step-tab-actions__title {
  color: #909399;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.step-tab-actions__status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-width: 0;
  color: #777d78;
  font-size: 12px;
}

.step-tab-actions__status :deep(.ant-tag) {
  margin-inline-end: 0;
}

.step-tab-actions :deep(.ant-btn) {
  border-color: #dce3d8;
  border-radius: 6px;
  box-shadow: none;
  font-size: 12px;
}

.step-tab-actions :deep(.ant-btn-primary) {
  border-color: #6f9469;
  background: #6f9469;
}

.step-tab-actions :deep(.ant-btn-primary:hover) {
  border-color: #547b50;
  background: #547b50;
}

.step-tab-actions__warning {
  color: #b75a54;
  font-size: 11px;
  line-height: 1.5;
}

.step-tab-content {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.step-content-toolbar {
  display: flex;
  min-height: 54px;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 9px 16px;
  box-sizing: border-box;
  border-bottom: 1px solid #e8ece5;
  background: #fbfcfa;
}

.step-content-toolbar > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 1px;
}

.step-content-toolbar strong {
  overflow: hidden;
  color: #303133;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.step-content-toolbar span {
  color: #909399;
  font-size: 11px;
}

.step-content-toolbar :deep(.ant-tag) {
  margin-inline-end: 0;
  border-radius: 5px;
}

.step-markdown-view {
  min-height: 0;
  flex: 1;
  overflow: auto;
  background: #fff;
}

.task-result {
  min-height: calc(100vh - 88px);
  padding-top: 80px;
  border: 1px solid #e8e8e0;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}

@media (max-width: 1180px) {
  .task-workflow-header__meta {
    grid-template-columns: repeat(3, minmax(100px, 1fr));
  }
}

@media (max-width: 900px) {
  .task-detail-page {
    padding: 12px;
  }

  .task-workflow-header {
    padding: 16px;
  }

  .task-workflow-shell {
    height: auto;
    min-height: calc(100vh - 80px);
  }

  .task-detail-page--embedded .task-workflow-shell {
    height: auto;
    min-height: 100%;
  }

  .task-workflow-header__title-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .task-workflow-header__title {
    width: 100%;
    white-space: normal;
  }

  .task-workflow-header__actions {
    justify-content: flex-start;
  }

  .task-workflow-header__meta {
    grid-template-columns: 1fr;
  }

  .task-workflow-header__field--directory {
    grid-column: auto;
  }

  .step-tab-layout {
    grid-template-columns: 1fr;
    min-height: 520px;
  }

  .step-tab-sidebar {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
    border-right: 0;
    border-bottom: 1px solid #e8ece5;
  }

  .step-tab-actions {
    position: static;
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: repeat(2, minmax(120px, 1fr));
    align-items: center;
  }

  .step-tab-actions__title,
  .step-tab-actions__warning {
    grid-column: 1 / -1;
  }

  .step-markdown-view {
    height: 520px;
    flex: none;
  }
}

@media (max-width: 640px) {
  .task-detail-page {
    margin: -24px;
  }

  .task-detail-page--embedded {
    margin: 0;
    padding: 8px;
  }

  .task-workflow-header__actions {
    width: 100%;
  }

  .status-select {
    width: 100%;
  }
}
</style>
