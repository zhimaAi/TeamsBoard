<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import apiClient from '@/api/client'

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
  steps: Array<{ step_key: string; name: string; sort_order: number; cli_type: string }>
}
interface APICollection {
  id: number
  name: string
  description: string
}
interface APIFolder {
  id: number
  collection_id: number
  name: string
  description: string
}
interface DBProfile {
  id: number
  name: string
  db_type: string
  database_name: string
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
const apiCollections = ref<APICollection[]>([])
const apiFolders = ref<APIFolder[]>([])
const selectedAPICollectionID = ref<number>()
const selectedAPIFolderID = ref<number>(0)
const foldersLoading = ref(false)
const dbProfiles = ref<DBProfile[]>([])
const selectedDBProfileIDs = ref<number[]>([])

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
  && normalizedWorkDirs.value.every(Boolean)
  && selectedAPICollectionID.value,
))
const dbProfileOptions = computed(() => dbProfiles.value.map(p => ({
  value: p.id,
  label: `${p.name}（${p.db_type} · ${p.database_name || '-'}）`,
})))

async function load() {
  loading.value = true
  errorText.value = ''
  selectedWorkItem.value = undefined
  try {
    const [workItemResult, agentResult, collectionResult, dbProfileResult] = await Promise.all([
      apiClient.get<{ items: WorkItem[] }>('/tasks/work-items'),
      apiClient.get<{ items: Agent[] }>('/tasks/agents'),
      apiClient.get<{ data: APICollection[] }>('/apis/collections'),
      apiClient.get<{ items: DBProfile[] }>('/config/database-profiles'),
    ])
    workItems.value = workItemResult.items || []
    agents.value = agentResult.items || []
    apiCollections.value = collectionResult.data || []
    dbProfiles.value = dbProfileResult.items || []
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '云端数据加载失败'
  } finally {
    loading.value = false
  }
}

async function chooseAPICollection(id: number) {
  selectedAPICollectionID.value = id
  selectedAPIFolderID.value = 0
  apiFolders.value = []
  foldersLoading.value = true
  try {
    const result = await apiClient.get<{ data: APIFolder[] }>('/apis/folders', { collection_id: id })
    apiFolders.value = result.data || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '接口文件夹加载失败')
  } finally {
    foldersLoading.value = false
  }
}

async function chooseAgent(id: number) {
  selectedAgentID.value = id
  agentSnapshot.value = undefined
  try {
    agentSnapshot.value = await apiClient.get<AgentSnapshot>(`/tasks/agents/${id}/snapshot`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'Agent 工作流加载失败')
  }
}

function addWorkDir() {
  workDirs.value.push('')
}

function removeWorkDir(index: number) {
  if (index === 0) return
  workDirs.value.splice(index, 1)
}

async function createTask() {
  if (!selectedWorkItem.value) {
    message.warning('请选择一个需求或缺陷')
    return
  }
  if (!selectedAgentID.value) {
    message.warning('请选择执行 Agent')
    return
  }
  if (normalizedWorkDirs.value.length === 0 || normalizedWorkDirs.value.some(workDir => !workDir)) {
    message.warning('请填写所有工作目录地址')
    return
  }
  const uniqueWorkDirs = new Set(normalizedWorkDirs.value.map(workDir => workDir.toLowerCase()))
  if (uniqueWorkDirs.size !== normalizedWorkDirs.value.length) {
    message.warning('工作目录不能重复')
    return
  }
  if (!selectedAPICollectionID.value) {
    message.warning('请选择接口集合')
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
      api_collection_id: selectedAPICollectionID.value,
      api_folder_id: selectedAPIFolderID.value || 0,
      database_profile_ids: selectedDBProfileIDs.value,
    })
    message.success('任务创建成功')
    router.push(`/workflows/task/${result.uuid}`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '任务创建失败')
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="import-page">
    <a-page-header title="导入需求 / 缺陷" sub-title="从云端工作项创建本地执行任务" @back="router.push('/workflows')" />

    <a-alert
      v-if="errorText"
      type="error"
      show-icon
      message="无法读取云端工作项或 Agent"
      :description="errorText"
      style="margin-bottom: 16px"
    >
      <template #action><a-button danger @click="load">重试</a-button></template>
    </a-alert>

    <a-spin :spinning="loading">
      <div class="import-grid">
        <a-card title="1. 选择需求或缺陷">
          <template #extra><span class="muted">共 {{ filteredItems.length }} 项</span></template>
          <a-space style="margin-bottom: 14px">
            <a-input-search v-model:value="keyword" placeholder="搜索标题或编号" allow-clear />
            <a-radio-group v-model:value="typeFilter" button-style="solid">
              <a-radio-button value="all">全部</a-radio-button>
              <a-radio-button value="requirement">需求</a-radio-button>
              <a-radio-button value="defect">缺陷</a-radio-button>
            </a-radio-group>
          </a-space>
          <a-empty
            v-if="!loading && filteredItems.length === 0"
            description="云端暂无可导入的需求或缺陷"
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
                {{ item.type === 'defect' ? '缺陷' : '需求' }}
              </a-tag>
              <span class="item-title">{{ item.title }}</span>
              <span class="muted">#{{ item.id }}</span>
            </button>
          </div>
        </a-card>

        <a-card title="2. 选择 Agent">
          <a-empty v-if="!loading && agents.length === 0" description="暂无可用 Agent，请先在云端配置 Agent" />
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
                <small>{{ agent.cli_type || '默认 CLI' }} · 工作流 v{{ agent.workflow_version || 1 }}</small>
              </span>
            </button>
          </div>

          <div v-if="agentSnapshot" class="workflow-preview">
            <h4>工作流步骤</h4>
            <a-steps
              direction="vertical"
              size="small"
              :current="-1"
              :items="workflowSteps"
            />
          </div>
        </a-card>
      </div>

      <a-card title="3. 设置工作目录" class="work-dir-card">
        <template #extra>
          <a-button type="link" @click="addWorkDir">+ 添加工作目录</a-button>
        </template>
        <div class="work-dir-list">
          <div v-for="(_, index) in workDirs" :key="index" class="work-dir-row">
            <a-form-item
              :label="index === 0 ? '主工作目录地址' : `关联工作目录 ${index}`"
              required
              class="work-dir-form-item"
            >
              <a-input
                v-model:value="workDirs[index]"
                :placeholder="index === 0
                  ? '请输入本机真实存在的目录，例如 D:\\work\\your-project'
                  : '请输入需要关联处理的其他目录'"
                @press-enter="createTask"
              />
            </a-form-item>
            <a-button v-if="index > 0" danger class="work-dir-remove" @click="removeWorkDir(index)">
              移除
            </a-button>
          </div>
        </div>
        <span class="muted">
          创建时会逐一检查目录是否存在且为文件夹。CLI 在主目录中启动，Agent 可通过绝对路径处理所有关联目录。
        </span>
      </a-card>

      <a-card title="4. 选择接口集合与文件夹" class="work-dir-card">
        <a-form layout="vertical">
          <a-form-item label="接口集合" required>
            <a-select
              :value="selectedAPICollectionID"
              placeholder="请选择“接口开发”中的集合"
              @change="chooseAPICollection"
            >
              <a-select-option v-for="collection in apiCollections" :key="collection.id" :value="collection.id">
                {{ collection.name }}
              </a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item label="接口文件夹" style="margin-bottom: 8px">
            <a-select
              v-model:value="selectedAPIFolderID"
              :disabled="!selectedAPICollectionID"
              :loading="foldersLoading"
              placeholder="不选择则按任务名自动创建"
            >
              <a-select-option :value="0">按任务名自动创建文件夹</a-select-option>
              <a-select-option v-for="folder in apiFolders" :key="folder.id" :value="folder.id">
                {{ folder.name }}
              </a-select-option>
            </a-select>
          </a-form-item>
        </a-form>
        <span class="muted">AI 通过 goteams-api 创建、查询、修改或删除的接口都会被限制在该任务文件夹内。</span>
      </a-card>

      <a-card title="5. 选择数据库（可选）" class="work-dir-card">
        <a-form layout="vertical">
          <a-form-item
            label="数据库连接"
            tooltip="选择后，若提示词包含 goteams-db 占位符（如 {Db操作}），将把对应 database_profile_id 注入任务，AI 即可通过 goteams-db 查询这些数据库。"
          >
            <a-select
              v-model:value="selectedDBProfileIDs"
              mode="multiple"
              placeholder="可选择多个数据库连接"
              :options="dbProfileOptions"
              :disabled="dbProfiles.length === 0"
            />
          </a-form-item>
        </a-form>
        <span class="muted">
          该配置为可选。选中的数据库会被限定为本次任务可访问的 database_profile_id 范围；
          若未配置数据库，则 AI 不会注入数据库范围约束。
        </span>
      </a-card>
    </a-spin>

    <div class="action-bar">
      <span v-if="isTaskReady">
        将使用所选 Agent 为“{{ selectedWorkItem?.title }}”创建任务
      </span>
      <span v-else class="muted">完成工作项、Agent、工作目录和接口集合配置后即可创建任务</span>
      <a-space>
        <a-button @click="router.push('/workflows')">取消</a-button>
        <a-button type="primary" :loading="creating" @click="createTask">创建任务</a-button>
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
