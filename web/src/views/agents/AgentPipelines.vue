<template>
  <div class="agent-page">
    <header class="page-bar">
      <div class="page-identity">
        <!-- <h1>专家流水线</h1> -->
        <span
          class="agent-tabs"
          aria-label="Agent 类型"
        >
          <span
            class="agent-tab active"
            aria-current="page"
          >
            <ApartmentOutlined />流水线
          </span>
          <a-tooltip title="功能开发中，即将上线">
            <span class="agent-tab"><TeamOutlined />专家团</span>
          </a-tooltip>
        </span>
        <span class="dot" />
        <div class="page-description">管理流水线与 Agent 配置</div>
      </div>
      <!-- <a-button
        size="small"
        :loading="syncing"
        @click="syncCloudPipelines(true)"
      >
        <template #icon><CloudDownloadOutlined /></template>
        同步云端流水线
      </a-button> -->
    </header>

    <div class="agent-layout">
      <PipelineListPane
        :pipelines="pipelines"
        :selected-uuid="selectedUuid"
        @create="openPipelineModal()"
        @edit="openPipelineModal"
        @select="loadPipeline"
      />
      <PipelineStepsPane
        :pipeline="selectedPipeline"
        @create-agent="openAgentModal()"
        @copy-agent="copyModalOpen = true"
        @edit-agent="openAgentModal"
        @move-agent="moveStep"
        @remove-agent="removeStep"
      />
    </div>
    <div v-if="loading" class="loading-overlay">
      <a-spin />
    </div>

    <PipelineEditorModal
      v-model:open="pipelineModalOpen"
      :pipeline="editingPipeline"
      @created="handlePipelineCreated"
      @deleted="handlePipelineDeleted"
      @updated="handlePipelineUpdated"
    />
    <AgentEditorModal
      v-model:open="agentModalOpen"
      :pipeline-uuid="selectedUuid"
      :is-cloud-pipeline="isCloudPipeline"
      :step="editingStep"
      :current-avatar="editingStepAvatar"
      @saved="refreshSelectedPipeline"
    />
    <CopyAgentModal
      v-model:open="copyModalOpen"
      :target-pipeline-uuid="selectedUuid"
      :source-steps="allReusableSteps"
      @saved="refreshSelectedPipeline"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { message, Modal } from 'ant-design-vue'
import {
  ApartmentOutlined,
  TeamOutlined,
} from '@ant-design/icons-vue'
import { useRoute } from 'vue-router'
import apiClient, { ApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { usePipelineStore } from '@/stores/pipeline'
import type { Pipeline, PipelineStep } from '@/types/pipeline'
import AgentEditorModal from './components/AgentEditorModal.vue'
import CopyAgentModal from './components/CopyAgentModal.vue'
import PipelineEditorModal from './components/PipelineEditorModal.vue'
import PipelineListPane from './components/PipelineListPane.vue'
import PipelineStepsPane from './components/PipelineStepsPane.vue'
import {
  AGENT_AVATARS,
  type DeleteResponse,
  isPipelineCloud,
  resolveAgentAvatar,
  type ReorderResponse,
  type ReusablePipelineStep,
  sortPipelineSteps,
} from './components/agentPipeline'

const route = useRoute()
const authStore = useAuthStore()
const pipelineStore = usePipelineStore()
const { pipelines } = storeToRefs(pipelineStore)
const loading = ref(false)
const syncing = ref(false)
const selectedUuid = ref('')
const handledPipelineQuery = ref('')
const pipelineModalOpen = ref(false)
const agentModalOpen = ref(false)
const copyModalOpen = ref(false)
const editingPipeline = ref<Pipeline>()
const editingStep = ref<PipelineStep>()
const editingStepAvatar = ref<string>(AGENT_AVATARS[0])

const selectedPipeline = computed(() =>
  pipelines.value.find((item) => item.uuid === selectedUuid.value),
)
const isCloudPipeline = computed(() => isPipelineCloud(selectedPipeline.value))
const allReusableSteps = computed<ReusablePipelineStep[]>(() =>
  pipelines.value.flatMap((pipeline) =>
    (pipeline.steps || []).map((step) => ({ ...step, pipelineName: pipeline.name })),
  ),
)

function pipelineUuidFromQuery() {
  const value = route.query.pipeline
  return Array.isArray(value) ? value[0] || '' : value || ''
}

async function load(showError = true) {
  loading.value = true
  try {
    await pipelineStore.loadPipelines(true)
    const queryPipelineUuid = pipelineUuidFromQuery()
    if (
      queryPipelineUuid &&
      queryPipelineUuid !== handledPipelineQuery.value &&
      pipelines.value.some((item) => item.uuid === queryPipelineUuid)
    ) {
      selectedUuid.value = queryPipelineUuid
      handledPipelineQuery.value = queryPipelineUuid
    } else if (!pipelines.value.some((item) => item.uuid === selectedUuid.value)) {
      selectedUuid.value = pipelines.value[0]?.uuid || ''
    }
    if (selectedUuid.value) await loadPipeline(selectedUuid.value, false)
  } catch (error) {
    if (showError) message.error(error instanceof Error ? error.message : '流水线加载失败')
  } finally {
    loading.value = false
  }
}

async function loadPipeline(uuid: string, showError = true) {
  selectedUuid.value = uuid
  try {
    const detail = await apiClient.get<Pipeline>(`/pipelines/${uuid}`)
    pipelineStore.upsertPipeline(detail)
  } catch (error) {
    if (showError) message.error(error instanceof Error ? error.message : '流水线详情加载失败')
  }
}

async function selectPipelineFromQuery() {
  const uuid = pipelineUuidFromQuery()
  if (!uuid) {
    handledPipelineQuery.value = ''
    return
  }
  if (
    uuid === handledPipelineQuery.value ||
    !pipelines.value.some((item) => item.uuid === uuid)
  )
    return
  handledPipelineQuery.value = uuid
  if (uuid === selectedUuid.value) return
  await loadPipeline(uuid)
}

async function syncCloudPipelines(manual = false) {
  if (syncing.value) return
  syncing.value = true
  try {
    const result = await apiClient.post<{ synced?: number }>('/pipelines/sync-cloud', {})
    if (manual)
      message.success(
        `云端流水线同步完成${result.synced === undefined ? '' : `，更新 ${result.synced} 条`}`,
      )
    await load(false)
  } catch (error) {
    if (manual || !(error instanceof ApiError && error.status === 401))
      message.warning(error instanceof Error ? error.message : '云端流水线同步失败')
  } finally {
    syncing.value = false
  }
}

function openPipelineModal(pipeline?: Pipeline) {
  if (pipeline && isPipelineCloud(pipeline)) return
  editingPipeline.value = pipeline
  pipelineModalOpen.value = true
}

function openAgentModal(step?: PipelineStep) {
  editingStep.value = step
  if (step) {
    // 与步骤卡片展示一致：按排序后的位置取模轮换，避免编辑弹窗默认头像与卡片显示不一致
    const steps = sortPipelineSteps(selectedPipeline.value?.steps || [])
    const index = steps.findIndex((item) => item.uuid === step.uuid)
    editingStepAvatar.value = resolveAgentAvatar(step, index)
  } else {
    editingStepAvatar.value = AGENT_AVATARS[0]
  }
  agentModalOpen.value = true
}

async function handlePipelineCreated(pipeline: Pipeline) {
  selectedUuid.value = pipeline.uuid
  await load()
}

async function handlePipelineUpdated() {
  await load()
}

async function handlePipelineDeleted() {
  await load()
}

async function refreshSelectedPipeline() {
  if (selectedUuid.value) await loadPipeline(selectedUuid.value)
}

async function moveStep(step: PipelineStep, offset: number) {
  const pipeline = selectedPipeline.value
  if (!pipeline || isPipelineCloud(pipeline)) return
  const steps = [...(pipeline.steps || [])].sort((a, b) => a.sort_order - b.sort_order)
  const index = steps.findIndex((item) => item.uuid === step.uuid)
  const target = index + offset
  if (target < 0 || target >= steps.length) return

  const [moved] = steps.splice(index, 1)
  steps.splice(target, 0, moved)
  try {
    await apiClient.put<ReorderResponse>(`/pipelines/${selectedUuid.value}/steps/reorder`, {
      ids: steps.map((item) => item.uuid),
    })
    await refreshSelectedPipeline()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '排序失败')
  }
}

function removeStep(step: PipelineStep) {
  if (isCloudPipeline.value) return
  Modal.confirm({
    title: `从流水线移除“${step.name}”？`,
    okType: 'danger',
    onOk: async () => {
      try {
        await apiClient.delete<DeleteResponse>(`/pipelines/${selectedUuid.value}/steps/${step.uuid}`)
        await refreshSelectedPipeline()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '移除 Agent 失败')
        throw error
      }
    },
  })
}

onMounted(async () => {
  await load()
  await syncCloudPipelines(false)
})

// 登出后立即从列表移除团队同步的流水线；重新登录后重新同步恢复
watch(
  () => authStore.cloudLoggedIn,
  (loggedIn) => {
    if (loggedIn) {
      void syncCloudPipelines(false)
      return
    }
    if (!pipelines.value.some((item) => item.uuid === selectedUuid.value)) {
      selectedUuid.value = pipelines.value[0]?.uuid || ''
      if (selectedUuid.value) void loadPipeline(selectedUuid.value, false)
    }
  },
)

watch(
  () => route.query.pipeline,
  () => void selectPipelineFromQuery(),
)
</script>

<style scoped>
.agent-page {
  display: flex;
  height: 100vh;
  flex-direction: column;
  overflow: hidden;
  position: relative;
  background: #f5f6f8;
  color: #111827;
}

.page-bar {
  display: flex;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}

.page-identity {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
}

.page-icon {
  display: flex;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: #fff;
  background: #1677ff;
}

.page-identity > h1 {
  margin: 0;
  color: #111827;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.agent-tabs {
  display: flex;
  height: 38px;
  align-items: center;
  padding: 3px;
  border-radius: 20px;
  background: #f1f2f4;
}

.agent-tab {
  display: flex;
  height: 32px;
  align-items: center;
  gap: 7px;
  padding: 0 16px;
  border-radius: 17px;
  color: #667085;
  font-size: 14px;
  font-weight: 400;
  line-height: 32px;
  white-space: nowrap;
  cursor: pointer;
}

.agent-tab :deep(svg) {
  font-size: 14px;
}

.agent-tab.active {
  color: #111827;
  background: #fff;
  box-shadow: 0 2px 7px rgba(15, 23, 42, 0.12);
  font-weight: 600;
}

.page-identity .dot {
  width: 4px;
  height: 4px;
  margin-left: 2px;
  border-radius: 50%;
  background: #d5dae1;
}

.page-description {
  color: #98a2b3;
  font-size: 12px;
  font-weight: 400;
}

.page-bar > :deep(.ant-btn) {
  height: 28px;
  padding-inline: 14px;
  border-radius: 14px;
  box-shadow: none;
}

.agent-layout {
  display: flex;
  min-height: 0;
  flex: 1;
  box-sizing: border-box;
  align-items: stretch;
  gap: 16px;
  overflow: hidden;
  padding: 24px;
  background: #fff;
}

.loading-overlay {
  position: absolute;
  z-index: 1;
  inset: 76px 0 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(245, 246, 248, 0.68);
}

@media (max-width: 1439px) {
  .agent-layout {
    padding: 16px;
  }
}

@media (max-width: 900px) {
  .agent-layout {
    align-items: stretch;
    overflow-y: auto;
    flex-direction: column;
  }

  .page-description,
  .page-identity .dot {
    display: none;
  }
}

@media (max-width: 640px) {
  .page-bar {
    padding: 0 16px;
  }

  .agent-layout {
    gap: 12px;
  }
}
</style>
