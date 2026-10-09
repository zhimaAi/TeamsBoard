<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="configure-root"
      :class="{ 'is-fullscreen': fullscreen }"
      role="dialog"
      aria-modal="true"
      @keydown.esc="emit('update:open', false)"
    >
      <div
        class="configure-mask"
        @click="emit('update:open', false)"
      />
      <section class="configure-panel">
        <header class="configure-header">
          <strong>{{ t('teamwork.configure.title') }}</strong>
          <div class="configure-header-actions">
            <button
              type="button"
              :aria-label="
                fullscreen
                  ? t('workflows.task.common.exitFullscreen')
                  : t('workflows.task.common.fullscreen')
              "
              @click="fullscreen = !fullscreen"
            >
              <FullscreenExitOutlined v-if="fullscreen" /><FullscreenOutlined v-else />
            </button>
            <button
              type="button"
              :aria-label="t('workflows.task.create.close')"
              @click="emit('update:open', false)"
            >
              <CloseOutlined />
            </button>
          </div>
        </header>

        <a-spin :spinning="loading">
          <div class="configure-body">
            <p class="configure-hint">
              <InfoCircleFilled
                class="hint-icon"
                aria-hidden="true"
              />
              <span>{{ t('teamwork.configure.hint', { name: creatorName }) }}</span>
            </p>

            <!-- 需求标题与描述本期不支持修改，只作为只读信息展示。 -->
            <h2 class="work-item-title">{{ item?.title }}</h2>
            <div class="work-item-description scrollbar--subtle">
              <RichTextContent
                :content="item?.description || ''"
                :empty-text="t('teamwork.workItem.empty')"
              />
            </div>

            <div
              ref="chipRowRef"
              class="chip-row"
            >
              <a-dropdown
                trigger="click"
                placement="bottomLeft"
                overlay-class-name="chip-select-menu"
                @open-change="handleChipMenuOpenChange"
              >
                <button
                  type="button"
                  class="chip"
                  :class="{ selected: selectedProject }"
                >
                  <AppstoreOutlined class="chip-icon" />
                  <span class="chip-label">{{
                    selectedProject?.name || t('teamwork.configure.project')
                  }}</span>
                  <DownOutlined />
                </button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item
                      key="none"
                      @click="form.project_uuid = ''"
                      >{{ t('workflows.task.create.noProject') }}</a-menu-item
                    >
                    <a-menu-divider />
                    <a-menu-item
                      v-for="project in projects"
                      :key="project.uuid"
                      @click="chooseProject(project.uuid)"
                    >
                      <span class="menu-primary">{{ project.name }}</span>
                      <small>{{ project.local_dir }}</small>
                    </a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>

              <button
                type="button"
                class="chip"
                :class="{ selected: form.work_dir }"
                @click="chooseWorkDir"
              >
                <FolderOpenOutlined class="chip-icon" />
                <span class="chip-label">{{
                  form.work_dir || t('teamwork.configure.workDir')
                }}</span>
                <b>*</b>
              </button>

              <a-dropdown
                trigger="click"
                placement="bottomLeft"
                overlay-class-name="chip-select-menu chip-select-menu--rows"
                @open-change="handleChipMenuOpenChange"
              >
                <button
                  type="button"
                  class="chip"
                  :class="{ selected: form.priority }"
                >
                  <FlagOutlined class="chip-icon" />
                  <span class="chip-label">{{
                    priorityLabel(form.priority) || t('teamwork.configure.priority')
                  }}</span>
                  <DownOutlined />
                </button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item
                      key=""
                      @click="form.priority = ''"
                      >{{ t('workflows.task.create.none') }}</a-menu-item
                    >
                    <a-menu-item
                      key="high"
                      @click="form.priority = '高'"
                      >🔴 {{ t('workflows.task.priority.high') }}</a-menu-item
                    >
                    <a-menu-item
                      key="medium"
                      @click="form.priority = '中'"
                      >🟡 {{ t('workflows.task.priority.medium') }}</a-menu-item
                    >
                    <a-menu-item
                      key="low"
                      @click="form.priority = '低'"
                      >🔵 {{ t('workflows.task.priority.low') }}</a-menu-item
                    >
                  </a-menu>
                </template>
              </a-dropdown>

              <button
                type="button"
                class="chip"
                :class="{ selected: form.execution_mode }"
                @click="executionPickerOpen = true"
              >
                <img
                  v-if="isCodexSelected"
                  class="chip-avatar"
                  :src="vibeToolLogo(form.execution_tool)"
                  alt=""
                />
                <img
                  v-else-if="selectedPipeline?.avatar"
                  class="chip-avatar"
                  :src="selectedPipeline.avatar"
                  alt=""
                />
                <img
                  v-else-if="selectedExpertGroup?.avatar"
                  class="chip-avatar"
                  :src="selectedExpertGroup.avatar"
                  alt=""
                />
                <DeploymentUnitOutlined
                  v-else
                  class="chip-icon"
                />
                <span class="chip-label">{{ executionChipLabel() }}</span>
              </button>

              <a-dropdown
                trigger="click"
                placement="bottomRight"
                overlay-class-name="chip-select-menu chip-select-menu--rows"
                @open-change="handleChipMenuOpenChange"
              >
                <button
                  type="button"
                  class="more-chip"
                  :aria-label="t('teamwork.configure.more')"
                >
                  <EllipsisOutlined />
                </button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item
                      key="projects"
                      @click="showChildren = !showChildren"
                      ><AppstoreOutlined />{{ t('teamwork.configure.moreProject') }}</a-menu-item
                    >
                    <a-menu-item
                      key="dirs"
                      @click="showDirs = !showDirs"
                      ><FolderOpenOutlined />{{ t('teamwork.configure.moreDir') }}</a-menu-item
                    >
                  </a-menu>
                </template>
              </a-dropdown>
            </div>

            <section
              v-if="showChildren || showDirs"
              class="expanded-fields"
            >
              <div
                v-if="showChildren"
                class="expanded-row"
              >
                <span>{{ t('teamwork.configure.moreProject') }}</span>
                <a-select
                  v-model:value="form.child_project_uuids"
                  mode="multiple"
                  :max-tag-count="1"
                  :options="childProjectOptions"
                  :placeholder="t('workflows.task.create.choose')"
                  size="small"
                />
              </div>
              <div
                v-if="showDirs"
                class="expanded-row"
              >
                <span>{{ t('teamwork.configure.moreDir') }}</span>
                <div class="directory-content">
                  <div
                    v-if="form.additional_dirs.length"
                    class="directory-list"
                  >
                    <div
                      v-for="(dir, index) in form.additional_dirs"
                      :key="`${dir}-${index}`"
                      class="directory-item"
                    >
                      <span :title="dir">{{ dir }}</span>
                      <button
                        type="button"
                        @click="form.additional_dirs.splice(index, 1)"
                      >
                        ×
                      </button>
                    </div>
                  </div>
                  <a-button
                    size="small"
                    class="directory-add"
                    @click="addDirectory"
                  >
                    <PlusOutlined />{{ t('workflows.task.create.addRelatedDirectory') }}
                  </a-button>
                </div>
              </div>
            </section>
          </div>
        </a-spin>

        <footer class="configure-footer">
          <a-button @click="emit('update:open', false)">{{
            t('teamwork.configure.cancel')
          }}</a-button>
          <a-button
            type="primary"
            :loading="saving"
            @click="confirm"
            >{{ t('teamwork.configure.confirm') }}</a-button
          >
        </footer>
      </section>
    </div>
    <AssignExecutionModeModal
      v-model:open="executionPickerOpen"
      select-only
      :task-title="item?.title || ''"
      :initial-mode="form.execution_mode"
      :preferred-pipeline-uuid="form.pipeline_uuid"
      :preferred-expert-group-uuid="form.expert_group_uuid"
      :initial-cli-type="form.execution_mode === 'cli' ? form.execution_tool : ''"
      :initial-model-name="form.execution_mode === 'cli' ? form.model_name : ''"
      :initial-vibe-tool="form.execution_mode === 'vibe_coding' ? form.execution_tool : ''"
      @selected="applyExecutionSelection"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { message } from 'ant-design-vue'
import {
  AppstoreOutlined,
  CloseOutlined,
  DeploymentUnitOutlined,
  DownOutlined,
  EllipsisOutlined,
  FlagOutlined,
  FolderOpenOutlined,
  FullscreenExitOutlined,
  FullscreenOutlined,
  InfoCircleFilled,
  PlusOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import AssignExecutionModeModal, {
  type ExecutionModeSelection,
} from '@/components/AssignExecutionModeModal.vue'
import RichTextContent from '@/components/RichTextContent.vue'
import { isVibeCodingTool, vibeToolLabelKey, vibeToolLogo } from '@/composables/useVibeCoding'
import { useCreateTaskDefaults } from '@/composables/useCreateTaskDefaults'
import { useCliKickoffNavigation } from '@/composables/useCliKickoff'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import { openTaskInCodex } from '@/composables/useTaskCodex'
import { usePipelineStore } from '@/stores/pipeline'
import { useExpertGroupStore } from '@/stores/expert-group'
import type { Pipeline, TaskExecutionMode } from '@/types/pipeline'
import type { LocalProject } from '@/types/project'
import type { MyWorkItem } from '@/types/workitem'
import { useAppI18n } from '@/i18n'
import { ipcErrorMessage } from '@/utils/ipcError'

const { t } = useAppI18n()
const { getCreateTaskDefaults, saveCreateTaskDefaults } = useCreateTaskDefaults()
const { openCliConversation } = useCliKickoffNavigation()

const props = defineProps<{
  open: boolean
  item?: MyWorkItem
}>()
const emit = defineEmits<{ 'update:open': [value: boolean]; created: [uuid: string] }>()

const pipelineStore = usePipelineStore()
const { pipelines } = storeToRefs(pipelineStore)
const expertGroupStore = useExpertGroupStore()
const { items: expertGroups } = storeToRefs(expertGroupStore)

const loading = ref(false)
const saving = ref(false)
const fullscreen = ref(false)
const executionPickerOpen = ref(false)
const projects = ref<LocalProject[]>([])
const showChildren = ref(false)
const showDirs = ref(false)
const chipRowRef = ref<HTMLElement | null>(null)

const form = reactive({
  project_uuid: '',
  child_project_uuids: [] as string[],
  work_dir: '',
  additional_dirs: [] as string[],
  priority: '',
  pipeline_uuid: '',
  execution_mode: '' as TaskExecutionMode,
  execution_tool: '',
  model_name: '',
  expert_group_uuid: '',
})

const selectedProject = computed(() =>
  projects.value.find((project) => project.uuid === form.project_uuid),
)
const detailCreatorName = ref('')
const creatorName = computed(
  () =>
    props.item?.creator_name?.trim() ||
    detailCreatorName.value ||
    t('teamwork.configure.unknownCreator'),
)
const selectedPipeline = computed(() =>
  pipelines.value.find((pipeline) => pipeline.uuid === form.pipeline_uuid),
)
const selectedExpertGroup = computed(() =>
  expertGroups.value.find((group) => group.uuid === form.expert_group_uuid),
)
const isCodexSelected = computed(
  () => form.execution_mode === 'vibe_coding' && isVibeCodingTool(form.execution_tool),
)
const isCLISelected = computed(() => form.execution_mode === 'cli')
const childProjectOptions = computed(() =>
  projects.value
    .filter((project) => project.uuid !== form.project_uuid)
    .map((project) => ({ label: project.name, value: project.uuid })),
)

const workDirs = computed(() => [
  ...new Set(
    [
      form.work_dir,
      ...(selectedProject.value ? [selectedProject.value.local_dir] : []),
      ...form.additional_dirs,
    ]
      .map((value) => value.trim())
      .filter(Boolean),
  ),
])

function pipelineLabel(pipeline?: Pipeline) {
  return pipeline?.name || t('workflows.task.create.pipeline')
}

function executionChipLabel() {
  if (isCodexSelected.value) return t(vibeToolLabelKey(form.execution_tool))
  if (isCLISelected.value) {
    return form.execution_tool
      ? `${t('workflows.task.assign.cliMode')} · ${form.execution_tool}`
      : t('workflows.task.assign.cliMode')
  }
  if (form.execution_mode === 'pipeline') return pipelineLabel(selectedPipeline.value)
  if (form.execution_mode === 'expert_group')
    return selectedExpertGroup.value?.name || t('agents.expertTeam')
  return t('workflows.task.create.executionMode')
}

function priorityLabel(priority: string) {
  if (priority === '高') return t('workflows.task.priority.high')
  if (priority === '中') return t('workflows.task.priority.medium')
  if (priority === '低') return t('workflows.task.priority.low')
  return ''
}

function reset() {
  Object.assign(form, {
    project_uuid: '',
    child_project_uuids: [],
    work_dir: '',
    additional_dirs: [],
    priority: '',
    pipeline_uuid: '',
    execution_mode: '',
    execution_tool: '',
    model_name: '',
    expert_group_uuid: '',
  })
  showChildren.value = false
  showDirs.value = false
  executionPickerOpen.value = false
  detailCreatorName.value = ''
  fullscreen.value = false
}

function applyCreateTaskDefaults() {
  const defaults = getCreateTaskDefaults()
  if (defaults.project_uuid && projects.value.some((item) => item.uuid === defaults.project_uuid)) {
    form.project_uuid = defaults.project_uuid
  }
  if (defaults.work_dir) {
    form.work_dir = defaults.work_dir
  } else if (form.project_uuid) {
    const project = projects.value.find((item) => item.uuid === form.project_uuid)
    if (project?.local_dir) form.work_dir = project.local_dir
  }
  if (
    defaults.pipeline_uuid &&
    pipelines.value.some((item) => item.uuid === defaults.pipeline_uuid)
  ) {
    form.pipeline_uuid = defaults.pipeline_uuid
    form.execution_mode = 'pipeline'
  }
}

function applyExecutionSelection(selection: ExecutionModeSelection) {
  form.execution_mode = selection.mode
  form.pipeline_uuid = selection.pipelineUuid
  form.expert_group_uuid = selection.expertGroupUuid
  form.execution_tool = selection.tool
  form.model_name = selection.modelName
}

async function load() {
  loading.value = true
  const item = props.item
  const workspaceId = Number(item?.workspace_id)
  const workItemId = Number(item?.id)
  const [pipelineResult, expertResult, projectResult, detailResult] = await Promise.allSettled([
    pipelineStore.loadPipelines(true),
    expertGroupStore.load(true),
    apiClient.get<{ items: LocalProject[] }>('/projects'),
    // 「我的工作」列表只有创建人 id。姓名在工作项详情里，提示条要用这个名字。
    workspaceId && workItemId
      ? apiClient.get<{ item?: { creator_name?: string } }>(
          `/team/work-items/${workspaceId}/${workItemId}`,
        )
      : Promise.resolve(null),
  ])
  projects.value = projectResult.status === 'fulfilled' ? projectResult.value.items || [] : []
  if (detailResult.status === 'fulfilled') {
    detailCreatorName.value = detailResult.value?.item?.creator_name?.trim() || ''
  }
  if (pipelineResult.status === 'rejected')
    message.warning(t('workflows.task.create.pipelineLoadFailed'))
  if (expertResult.status === 'rejected') message.warning(t('expertGroups.loadFailed'))
  if (projectResult.status === 'rejected')
    message.warning(t('workflows.task.create.projectLoadFailed'))
  applyCreateTaskDefaults()
  loading.value = false
}

function chooseProject(uuid: string) {
  form.project_uuid = uuid
  const project = projects.value.find((item) => item.uuid === uuid)
  if (project?.local_dir) form.work_dir = project.local_dir
}

async function chooseWorkDir() {
  try {
    const selected = await selectDirectory(form.work_dir)
    if (selected) {
      form.work_dir = selected
      form.project_uuid = ''
    }
  } catch (error) {
    message.error(ipcErrorMessage(error, t('workflows.task.create.directoryPickerFailed')))
  }
}

async function addDirectory() {
  if (isDesktopRuntime()) {
    try {
      const selected = await selectDirectory(form.work_dir)
      if (!selected) return
      if (workDirs.value.some((dir) => dir.toLowerCase() === selected.toLowerCase())) {
        message.warning(t('workflows.task.create.duplicateDirectory'))
        return
      }
      form.additional_dirs.push(selected)
    } catch (error) {
      message.error(ipcErrorMessage(error, t('workflows.task.create.directoryPickerFailed')))
    }
  } else {
    form.additional_dirs.push('')
  }
}

async function confirm() {
  const item = props.item
  if (!item) return
  if (!form.work_dir.trim()) return message.warning(t('teamwork.configure.workDirRequired'))
  if (form.additional_dirs.some((dir) => !dir.trim()))
    return message.warning(t('workflows.task.create.emptyRelatedDirectory'))
  if (form.execution_mode === 'pipeline' && !form.pipeline_uuid)
    return message.warning(t('workflows.task.create.executionTargetRequired'))
  // Vibe Coding 已支持多个工具（Codex CLI / Pi / Qoder / CodeBuddy / Claude），
  // 这里只校验是否为合法工具，不能限定 codex，否则选中其它工具会被误判为未选。
  if (form.execution_mode === 'vibe_coding' && !isVibeCodingTool(form.execution_tool))
    return message.warning(t('workflows.task.create.executionTargetRequired'))
  if (form.execution_mode === 'cli' && (!form.execution_tool || !form.model_name))
    return message.warning(t('workflows.task.create.executionTargetRequired'))
  if (form.execution_mode === 'expert_group' && !selectedExpertGroup.value?.ready)
    return message.warning(t('expertGroups.notReady'))
  saving.value = true
  const createdExecutionMode = form.execution_mode
  // 创建成功后弹窗可能被重置，先固定本次选定的 vibe 工具，供打开提示使用。
  const createdVibeTool = form.execution_tool
  let createdTaskUuid = ''
  try {
    const result = await apiClient.post<{ uuid: string }>('/tasks', {
      // 标题与描述沿用团队需求，本期不开放修改。
      title: item.title,
      content: item.description || '',
      description: item.description || '',
      project_uuid: form.project_uuid || undefined,
      child_project_uuids: form.child_project_uuids,
      work_dir: form.work_dir,
      work_dirs: workDirs.value,
      priority: form.priority || undefined,
      pipeline_uuid: form.pipeline_uuid || undefined,
      execution_mode: form.execution_mode || undefined,
      execution_tool: form.execution_tool || undefined,
      model_name: form.model_name || undefined,
      expert_group_uuid: form.expert_group_uuid || undefined,
      // CLI 创建后直接进入进行中并打开对话；其他方式保持待开始，和新建任务一致。
      status: form.execution_mode === 'cli' ? 'active' : 'pending',
      // 绑定云端工作项后，「我的工作」与团队看板都能识别这条本地任务。
      work_item_type: item.type,
      work_item_id: item.id,
      // 创建即写入通知：对话页列表按通知分组，配置完成后（待开始）就该能在对话页看到该任务。
      create_notification: true,
      // 云端返回的 workspace_id 是字符串（如 "25"），而本地创建任务接口该字段是 int64：
      // 不转换会因 JSON 绑定失败被 400「请求参数无效」拒绝整次创建。
      workspace_id: Number(item.workspace_id) || undefined,
    })
    saveCreateTaskDefaults({
      project_uuid: form.project_uuid,
      work_dir: form.work_dir,
      pipeline_uuid: form.pipeline_uuid,
    })
    message.success(t('teamwork.configure.created'))
    createdTaskUuid = result.uuid
    emit('created', result.uuid)
    emit('update:open', false)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('teamwork.configure.createFailed'))
  } finally {
    saving.value = false
  }
  if (!createdTaskUuid) return
  if (createdExecutionMode === 'cli') {
    await openCliConversation(createdTaskUuid)
    return
  }
  if (createdExecutionMode !== 'vibe_coding') return
  // 提示文案跟随实际选定的工具，不能固定写 Codex。
  const toolName = t(vibeToolLabelKey(createdVibeTool))
  const openResult = await openTaskInCodex(createdTaskUuid)
  if (openResult.opened) {
    message.success(t('workflows.task.detail.toolOpened', { tool: toolName }))
  } else if (openResult.copied) {
    message.warning(t('workflows.task.detail.toolCopiedFallback', { tool: toolName }))
  } else if (openResult.error) {
    message.error(openResult.error.message)
  } else {
    message.warning(t('workflows.task.detail.toolUnavailable', { tool: toolName }))
  }
}

/** chip 下拉统一向下展开，按 chip 行下沿到视口底部的距离限制菜单高度。 */
function syncChipMenuMaxHeight() {
  const row = chipRowRef.value
  if (!row) return
  const available = window.innerHeight - row.getBoundingClientRect().bottom - 24
  document.documentElement.style.setProperty(
    '--chip-menu-max-height',
    `${Math.max(140, Math.min(380, available))}px`,
  )
}

function handleChipMenuOpenChange(open: boolean) {
  if (open) syncChipMenuMaxHeight()
}

watch(
  () => props.open,
  (open) => {
    if (!open) {
      executionPickerOpen.value = false
      return
    }
    reset()
    void load()
    void nextTick(syncChipMenuMaxHeight)
  },
)
</script>

<style scoped>
.configure-root {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
}

.configure-mask {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
}

.configure-panel {
  position: relative;
  z-index: 1;
  display: flex;
  width: 879px;
  /* 设计稿 3115:16087 是固定尺寸弹窗 879x600（上 56 + 内容 492 + 下 52）；
     标题块省掉了设计稿里给「0 / 200」计数器预留的高度，所以这里直接钉住总高，
     多出的空间由可滚动的描述区吸收。窗口过矮时由 max-height 兜底。 */
  height: 600px;
  max-width: calc(100% - 32px);
  max-height: calc(100% - 32px);
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: #fff;
}

.configure-root.is-fullscreen .configure-panel {
  width: 100%;
  max-width: 100%;
  height: 100%;
  max-height: 100%;
  border-radius: 0;
}

/* a-spin 的包裹层是面板的直接 flex 子项，必须一起参与纵向伸缩，
   否则固定 600px 高的弹窗会把底部按钮挤出可视区。 */
.configure-panel :deep(.ant-spin-nested-loading),
.configure-panel :deep(.ant-spin-container) {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
}

.configure-header {
  display: flex;
  height: 56px;
  flex: 0 0 56px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
}

.configure-header strong {
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.configure-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.configure-header-actions button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: #595959;
  background: transparent;
  cursor: pointer;
}

.configure-header-actions button:hover,
.configure-header-actions button:focus-visible {
  outline: none;
  background: #f5f6f8;
}

/* 设计稿 body 为 492px：提示条 + 只读标题 + 只读描述 + chip 行都在 body 内，
   描述区按设计稿取 293px 并允许伸缩（窗口变高时变高，展开更多配置时让位）。 */
.configure-body {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 16px;
  padding: 20px 24px;
}

.configure-hint {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  margin: 0;
  padding: 12px 16px;
  border-radius: 12px;
  color: #3a4559;
  background: linear-gradient(90deg, #f0f2fd, #dee4ff);
  font-size: 14px;
  line-height: 22px;
}

.hint-icon {
  flex: 0 0 16px;
  color: #3157e2;
  font-size: 16px;
}

.work-item-title {
  flex: 0 0 auto;
  margin: 0;
  padding: 4px 0 10px;
  border-bottom: 1px solid #f0f0f0;
  color: #262626;
  font-size: 22px;
  font-weight: 500;
  line-height: 27px;
  word-break: break-word;
}

.work-item-description {
  height: 293px;
  min-height: 0;
  flex: 1 1 auto;
  overflow-y: auto;
}

.work-item-description :deep(.markdown-preview-shell) {
  padding: 0;
}

.work-item-description :deep(.markdown-preview) {
  color: #262626;
  font-size: 14px;
  line-height: 28px;
}

.chip-row {
  position: relative;
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.chip,
.more-chip {
  display: flex;
  min-width: 0;
  height: 32px;
  max-width: 180px;
  align-items: center;
  gap: 6px;
  padding: 0 12px;
  border: 1px solid #d9d9d9;
  border-radius: 16px;
  color: #bfbfbf;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}

.chip-icon {
  flex: 0 0 auto;
  color: #bfbfbf;
  font-size: 16px;
}

.chip :deep(.anticon-down) {
  display: none;
}

.chip.selected {
  color: #262626;
}

.chip b {
  flex: 0 0 auto;
  color: #e34d59;
  font-weight: 500;
}

.chip-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chip-avatar {
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  border-radius: 50%;
  object-fit: cover;
}

.more-chip {
  width: 32px;
  margin-left: auto;
  justify-content: center;
  padding: 0;
  border-radius: 100%;
  color: #262626;
}

.expanded-fields {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  gap: 8px;
}

.expanded-row {
  display: flex;
  min-width: 0;
  min-height: 48px;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-radius: 6px;
  background: #f2f4f7;
}

.expanded-row > span:first-child {
  flex: 0 0 104px;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

.expanded-row :deep(.ant-select) {
  min-width: 0;
  flex: 1;
}

.directory-content {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}

.directory-list {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: 6px;
}

.directory-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 6px;
  color: #4b5563;
  background: #fff;
  font-size: 11px;
}

.directory-item > span {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.directory-item > button {
  flex: 0 0 auto;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
}

.directory-add {
  height: 32px;
  border-color: #d9d9d9;
  border-radius: 6px;
  color: #4b5563;
  background: #fff;
}

.configure-footer {
  display: flex;
  height: 52px;
  flex: 0 0 52px;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 0 24px;
}

.menu-primary {
  display: block;
  color: #374151;
}

:global(.chip-select-menu .ant-dropdown-menu) {
  min-width: 240px;
  max-height: var(--chip-menu-max-height, 380px);
  overflow-y: auto;
  padding: 8px;
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
}

:global(.chip-select-menu .ant-dropdown-menu-item) {
  border-radius: 8px;
}

:global(.chip-select-menu .ant-dropdown-menu-item:hover) {
  background: #f5f6f8;
}

:global(.chip-select-menu--rows .ant-dropdown-menu-item) {
  height: 36px;
  min-height: 36px;
  padding: 0 12px;
  border-radius: 8px;
  color: #262626;
  font-size: 14px;
  line-height: 36px;
}

:global(.chip-select-menu--rows .ant-dropdown-menu-item .anticon) {
  margin-right: 8px;
  color: #8c8c8c;
}

:global(.ant-dropdown-menu-item small) {
  display: block;
  max-width: 270px;
  overflow: hidden;
  color: #9ca3af;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
