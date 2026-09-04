<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { message } from 'ant-design-vue'
import {
  AppstoreOutlined,
  CalendarOutlined,
  CloseOutlined,
  DeploymentUnitOutlined,
  DownOutlined,
  EllipsisOutlined,
  FlagOutlined,
  FolderOpenOutlined,
  PlusOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import { useCreateTaskDefaults } from '@/composables/useCreateTaskDefaults'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import {
  formatPastedImageContent,
  useTaskImageAttachments,
} from '@/composables/useTaskImageAttachments'
import { usePipelineStore } from '@/stores/pipeline'
import type { Pipeline } from '@/types/pipeline'
import type { LocalProject } from '@/types/project'
import type { CloudWorkItem } from '@/types/workitem'
import { isPipelineCloud } from '@/utils/pipeline'

const props = defineProps<{
  open: boolean
  initialStatus?: string
  initialWorkItem?: CloudWorkItem
}>()
const emit = defineEmits<{ 'update:open': [value: boolean]; created: [uuid: string] }>()
const pipelineStore = usePipelineStore()
const { pipelines } = storeToRefs(pipelineStore)
const loading = ref(false)
const saving = ref(false)
const { getCreateTaskDefaults, saveCreateTaskDefaults } = useCreateTaskDefaults()
const {
  addImages,
  buildDraftAttachmentInputs,
  clear: clearImageAttachments,
  isSaving: isSavingPastedImages,
  removeAttachmentMarkers,
} = useTaskImageAttachments()
const descriptionRef = ref<HTMLTextAreaElement>()
const projects = ref<LocalProject[]>([])
const form = reactive({
  title: '',
  description: '',
  pipeline_uuid: '',
  project_uuid: '',
  child_project_uuids: [] as string[],
  work_dir: '',
  additional_dirs: [] as string[],
  priority: '',
  planned_start_date: '',
  planned_end_date: '',
})
const moreOpen = ref(false)
const moreChipRef = ref<HTMLElement | null>(null)
const moreMenuRef = ref<HTMLElement | null>(null)
const showDates = ref(false)
const showChildren = ref(false)
const showDirs = ref(false)

const selectedPipeline = computed(() =>
  pipelines.value.find((item) => item.uuid === form.pipeline_uuid),
)
const selectedProject = computed(() =>
  projects.value.find((item) => item.uuid === form.project_uuid),
)
const selectedChildren = computed(() =>
  projects.value.filter((item) => form.child_project_uuids.includes(item.uuid)),
)
const childProjectOptions = computed(() =>
  projects.value
    .filter((item) => item.uuid !== form.project_uuid)
    .map((item) => ({ label: item.name, value: item.uuid })),
)
const allWorkDirs = computed(() => {
  const dirs = [
    form.work_dir,
    ...selectedChildren.value.map((item) => item.local_dir),
    ...form.additional_dirs,
  ]
    .map((value) => value.trim())
    .filter(Boolean)
  return [...new Map(dirs.map((value) => [value.toLowerCase(), value])).values()]
})

function projectLabel(project?: LocalProject) {
  return project?.name || '所属项目'
}
function pipelineLabel(pipeline?: Pipeline) {
  return pipeline?.name || '流水线'
}
function reset() {
  clearImageAttachments()
  Object.assign(form, {
    title: props.initialWorkItem?.title || '',
    description: props.initialWorkItem?.description || '',
    pipeline_uuid: '',
    project_uuid: '',
    child_project_uuids: [],
    work_dir: '',
    additional_dirs: [],
    priority: '',
    planned_start_date: '',
    planned_end_date: '',
  })
  moreOpen.value = false
  showDates.value = false
  showChildren.value = false
  showDirs.value = false
}

async function handleDescriptionPaste(event: ClipboardEvent) {
  if (!event.clipboardData) return
  const files = [...event.clipboardData.items]
    .filter((item) => item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (!files.length) return
  if (isSavingPastedImages.value) {
    event.preventDefault()
    message.warning('上一批图片正在保存，请稍后再粘贴')
    return
  }
  const textarea = descriptionRef.value
  const value = form.description
  const start = textarea?.selectionStart ?? value.length
  const end = textarea?.selectionEnd ?? start
  const pastedText = event.clipboardData.getData('text/plain')
  const pastedHtml = event.clipboardData.getData('text/html')
  event.preventDefault()
  let insertedImageMarkers = false
  try {
    const attachments = addImages(files)
    insertPastedDescription(
      pastedText,
      pastedHtml,
      attachments.map((attachment) => attachment.marker),
      start,
      end,
    )
    insertedImageMarkers = true
    await nextTick()
    descriptionRef.value?.focus()
  } catch (error) {
    if (insertedImageMarkers) form.description = removeAttachmentMarkers(form.description)
    clearImageAttachments()
    message.error(error instanceof Error ? error.message : '图片粘贴失败')
  }
}

function insertPastedDescription(
  text: string,
  html: string,
  markers: string[],
  start: number,
  end: number,
) {
  const value = form.description
  const insertStart = Math.min(start, value.length)
  const insertEnd = Math.min(Math.max(end, insertStart), value.length)
  const inserted = formatPastedImageContent(
    text,
    html,
    markers,
    value.slice(0, insertStart),
    value.slice(insertEnd),
  )
  form.description = `${value.slice(0, insertStart)}${inserted}${value.slice(insertEnd)}`
  const caret = insertStart + inserted.length
  nextTick(() => {
    descriptionRef.value?.focus()
    descriptionRef.value?.setSelectionRange(caret, caret)
  })
}

async function load() {
  loading.value = true
  const [pipelineResult, projectResult] = await Promise.allSettled([
    pipelineStore.loadPipelines(true),
    apiClient.get<{ items: LocalProject[] }>('/projects'),
  ])
  projects.value = projectResult.status === 'fulfilled' ? projectResult.value.items || [] : []
  if (pipelineResult.status === 'rejected')
    message.warning('流水线加载失败，可不指定流水线创建任务')
  if (projectResult.status === 'rejected') message.warning('项目加载失败，仍可直接选择工作目录')
  applyCreateTaskDefaults()
  loading.value = false
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
  }
}

function chooseProject(uuid: string) {
  form.project_uuid = uuid
  const project = projects.value.find((item) => item.uuid === uuid)
  if (project) form.work_dir = project.local_dir
}

function choosePipeline(uuid: string) {
  form.pipeline_uuid = uuid
}

async function chooseMainDirectory() {
  try {
    const selected = await selectDirectory(form.work_dir)
    if (selected) {
      form.work_dir = selected
      form.project_uuid = ''
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '无法打开目录选择器')
  }
}

async function addDirectory() {
  if (isDesktopRuntime()) {
    try {
      const selected = await selectDirectory(form.work_dir)
      if (!selected) return
      if (allWorkDirs.value.some((dir) => dir.toLowerCase() === selected.toLowerCase())) {
        message.warning('该目录已在工作目录中')
        return
      }
      form.additional_dirs.push(selected)
    } catch (error) {
      message.error(error instanceof Error ? error.message : '无法打开目录选择器')
    }
  } else {
    form.additional_dirs.push('')
  }
}

async function create() {
  if (!form.title.trim()) return message.warning('请输入任务标题')
  if (!form.work_dir.trim()) return message.warning('请选择所属项目或主工作目录')
  if (isSavingPastedImages.value) return message.warning('图片正在保存，请稍后创建任务')
  if (form.additional_dirs.some((item) => !item.trim()))
    return message.warning('请填写或移除空的关联目录')
  saving.value = true
  try {
    const attachments = await buildDraftAttachmentInputs(form.description)
    const result = await apiClient.post<{ uuid: string }>('/tasks', {
      title: form.title.trim(),
      content: form.description,
      description: form.description,
      pipeline_uuid: form.pipeline_uuid || undefined,
      project_uuid: form.project_uuid || undefined,
      child_project_uuids: form.child_project_uuids,
      work_dir: form.work_dir,
      work_dirs: allWorkDirs.value,
      priority: form.priority || undefined,
      planned_start_date: form.planned_start_date || undefined,
      planned_end_date: form.planned_end_date || undefined,
      status: props.initialStatus || 'pending',
      work_item_type: props.initialWorkItem?.type,
      work_item_id: props.initialWorkItem?.id,
      attachments,
    })
    saveCreateTaskDefaults({
      project_uuid: form.project_uuid,
      work_dir: form.work_dir,
      pipeline_uuid: form.pipeline_uuid,
    })
    message.success('本地任务已创建')
    emit('created', result.uuid)
    emit('update:open', false)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '任务创建失败')
  } finally {
    saving.value = false
  }
}

// 点击按钮/菜单以外的区域时收起更多菜单
function onOutsideClick(event: MouseEvent) {
  const target = event.target as Node
  if (moreChipRef.value?.contains(target) || moreMenuRef.value?.contains(target)) return
  moreOpen.value = false
}

watch(moreOpen, (open) => {
  if (open) document.addEventListener('click', onOutsideClick)
  else document.removeEventListener('click', onOutsideClick)
})
onUnmounted(() => document.removeEventListener('click', onOutsideClick))

watch(pipelines, (items) => {
  if (!items.some((item) => item.uuid === form.pipeline_uuid)) {
    form.pipeline_uuid = ''
  }
})

watch(
  () => props.open,
  (open) => {
    if (open) {
      reset()
      void load()
    }
  },
)
</script>

<template>
  <a-modal
    :open="open"
    width="879px"
    :footer="null"
    :closable="false"
    :mask-closable="false"
    centered
    wrap-class-name="linear-task-modal"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <header class="task-modal-header">
        <strong>新建任务</strong
        ><button
          type="button"
          aria-label="关闭新建任务弹窗"
          @click="emit('update:open', false)"
        >
          <CloseOutlined />
        </button>
      </header>
      <section class="task-primary">
        <a-input
          v-model:value="form.title"
          :maxlength="50"
          :bordered="false"
          placeholder="自定义名称"
          class="title-input"
        />
        <div class="description-box">
          <a-textarea
            ref="descriptionRef"
            v-model:value="form.description"
            :auto-size="false"
            :bordered="false"
            placeholder="添加描述…"
            class="description-input"
            @paste="handleDescriptionPaste"
          />
        </div>
      </section>
      <div
        class="chip-row"
        :class="{ 'has-expanded': showDates || showChildren || showDirs }"
      >
        <a-dropdown trigger="click">
          <button
            type="button"
            class="chip"
            :class="{ selected: selectedProject }"
          >
            <AppstoreOutlined class="chip-icon" />
            <span class="chip-label">{{ projectLabel(selectedProject) }}</span>
            <DownOutlined />
          </button>
          <template #overlay>
            <a-menu>
              <a-menu-item
                key="none"
                @click="form.project_uuid = ''"
                >不选择项目</a-menu-item
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
          title="选择任务主工作目录（必选）"
          @click="chooseMainDirectory"
        >
          <FolderOpenOutlined class="chip-icon" />
          <span class="chip-label">{{ form.work_dir || '选择工作目录' }}</span>
          <b>*</b>
        </button>
        <a-dropdown trigger="click">
          <button
            type="button"
            class="chip"
            :class="{ selected: form.priority }"
          >
            <FlagOutlined class="chip-icon" />
            <span
              v-if="form.priority"
              class="priority-value"
              :class="{
                high: form.priority === '高',
                medium: form.priority === '中',
                low: form.priority === '低',
              }"
              >{{ form.priority }}</span
            >
            <span
              v-else
              class="chip-label"
              >选择优先级</span
            >
            <DownOutlined />
          </button>
          <template #overlay>
            <a-menu>
              <a-menu-item
                key=""
                @click="form.priority = ''"
                >不选择</a-menu-item
              >
              <a-menu-item
                key="high"
                @click="form.priority = '高'"
                >🔴 高</a-menu-item
              >
              <a-menu-item
                key="medium"
                @click="form.priority = '中'"
                >🟡 中</a-menu-item
              >
              <a-menu-item
                key="low"
                @click="form.priority = '低'"
                >🔵 低</a-menu-item
              >
            </a-menu>
          </template>
        </a-dropdown>
        <a-dropdown trigger="click">
          <button
            type="button"
            class="chip pipeline-chip"
            :class="{ selected: selectedPipeline }"
          >
            <DeploymentUnitOutlined class="chip-icon" />
            <img
              v-if="selectedPipeline?.avatar"
              class="chip-avatar"
              :src="selectedPipeline.avatar"
              alt=""
            />
            <span class="chip-label">{{ pipelineLabel(selectedPipeline) }}</span>
            <DownOutlined />
          </button>
          <template #overlay>
            <a-menu class="pipeline-menu">
              <a-menu-item
                key="none"
                @click="choosePipeline('')"
                >暂不指定流水线</a-menu-item
              >
              <a-menu-divider />
              <a-menu-item
                v-for="pipeline in pipelines"
                :key="pipeline.uuid"
                @click="choosePipeline(pipeline.uuid)"
              >
                <span class="menu-primary">{{ pipeline.name }}</span>
                <small
                  >{{ isPipelineCloud(pipeline) ? '团队同步' : '本地流水线' }} ·
                  {{ pipeline.steps?.length || 0 }} 步</small
                >
              </a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
        <button
          ref="moreChipRef"
          type="button"
          class="more-chip"
          aria-label="显示更多任务信息"
          @click="moreOpen = !moreOpen"
        >
          <EllipsisOutlined />
        </button>
        <div
          v-if="moreOpen"
          ref="moreMenuRef"
          class="more-menu"
        >
          <button
            type="button"
            @click="showDates = !showDates"
          >
            <CalendarOutlined />预期开始/结束日期</button
          ><button
            type="button"
            @click="showChildren = !showChildren"
          >
            <AppstoreOutlined />关联项目</button
          ><button
            type="button"
            @click="showDirs = !showDirs"
          >
            <FolderOpenOutlined />关联目录
          </button>
        </div>
      </div>

      <section class="expanded-fields">
        <div
          v-if="showDates"
          class="expanded-row"
        >
          <span>预期开始</span
          ><a-date-picker
            v-model:value="form.planned_start_date"
            value-format="YYYY-MM-DD"
            placeholder="请选择日期"
            size="small"
          /><button
            type="button"
            class="expanded-close"
            aria-label="隐藏预期开始日期"
            @click="showDates = false"
          >
            <CloseOutlined />
          </button>
        </div>
        <div
          v-if="showDates"
          class="expanded-row"
        >
          <span>预期结束</span
          ><a-date-picker
            v-model:value="form.planned_end_date"
            value-format="YYYY-MM-DD"
            placeholder="请选择日期"
            size="small"
          /><button
            type="button"
            class="expanded-close"
            aria-label="隐藏预期结束日期"
            @click="showDates = false"
          >
            <CloseOutlined />
          </button>
        </div>
        <div
          v-if="showChildren"
          class="expanded-row children-row"
        >
          <span>关联项目</span
          ><a-select
            v-model:value="form.child_project_uuids"
            mode="multiple"
            :max-tag-count="1"
            :options="childProjectOptions"
            placeholder="请选择"
            size="small"
          /><button
            type="button"
            class="expanded-close"
            aria-label="隐藏关联项目"
            @click="showChildren = false"
          >
            <CloseOutlined />
          </button>
        </div>
        <div
          v-if="showDirs"
          class="expanded-row dirs-row"
          :class="{ 'has-directories': form.additional_dirs.length > 0 }"
        >
          <span>关联目录</span>
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
                <span :title="dir || '未填写'">{{ dir || '未填写' }}</span>
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
              ><PlusOutlined />添加关联目录</a-button
            >
          </div>
          <button
            type="button"
            class="expanded-close"
            aria-label="隐藏关联目录"
            @click="showDirs = false"
          >
            <CloseOutlined />
          </button>
        </div>
      </section>

      <footer class="task-modal-footer">
        <span v-if="selectedPipeline"
          >已预选“{{ selectedPipeline.name }}”，任务启动时配置 CLI/模型并生成执行副本</span
        ><span v-else>流水线可稍后在任务开始前指定</span>
        <div>
          <a-button @click="emit('update:open', false)">取消</a-button
          ><a-button
            type="primary"
            :loading="saving"
            @click="create"
            >创建任务</a-button
          >
        </div>
      </footer>
    </a-spin>
  </a-modal>
</template>

<style scoped>
.task-modal-header {
  display: flex;
  height: 56px;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
}

.task-modal-header strong {
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.task-modal-header button,
.expanded-close {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: #8c8c8c;
  background: transparent;
  cursor: pointer;
}

.task-modal-header button:hover,
.expanded-close:hover {
  color: #4b5563;
  background: #f5f6f8;
}

.task-primary {
  padding: 16px 24px 0;
}

/* antd borderless 的 border:none 选择器特异性更高，下边框需 !important 才能稳定生效（含聚焦态） */
.title-input {
  height: 51px;
  border: 0;
  border-bottom: 1px solid #f0f0f0 !important;
  border-radius: 0;
  color: #262626;
  font-size: 22px;
  font-weight: 500;
  line-height: 34px;
}

.description-box {
  display: flex;
  height: 293px;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
}

.description-input {
  height: 293px;
  min-height: 0;
  flex: 1;
  border: 0;
  border-radius: 0;
  color: #262626;
  font-size: 14px;
  line-height: 28px;
  overflow-y: auto;
  resize: none;
}

.task-primary :deep(.ant-input),
.task-primary :deep(textarea) {
  padding-right: 0 !important;
  padding-left: 0 !important;
}

.task-primary :deep(.description-input) {
  padding: 8px 4px !important;
}

.description-input::-webkit-scrollbar {
  width: 11px;
}

.description-input::-webkit-scrollbar-track {
  background: transparent;
}

.description-input::-webkit-scrollbar-thumb {
  border: 3px solid transparent;
  border-radius: 16px;
  background: #a1a7b2;
  background-clip: content-box;
}

.chip-row {
  position: relative;
  display: flex;
  min-height: 52px;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px 24px 12px;
}

.chip-row.has-expanded {
  min-height: 56px;
  padding-bottom: 16px;
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

/* chip 前置图标颜色固定，不随选中态文字变色 */
.chip-icon {
  flex: 0 0 auto;
  color: #bfbfbf;
  font-size: 16px;
}

.chip :deep(.anticon-down) {
  display: none;
}

.chip.selected {
  border-color: #d9d9d9;
  color: #262626;
  background: #fff;
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

.priority-value {
  display: inline-flex;
  height: 22px;
  align-items: center;
  padding: 0 9px;
  border: 1px solid currentColor;
  border-radius: 12px;
  font-size: 12px;
  line-height: 20px;
}

.priority-value.high {
  color: #fb363f;
}

.priority-value.medium {
  color: #d97706;
}

.priority-value.low {
  color: #3157e2;
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
  border-radius: 50%;
  color: #262626;
}

.more-menu {
  position: absolute;
  z-index: 30;
  top: 48px;
  right: 24px;
  width: 200px;
  padding: 5px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.14);
}

.more-menu button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 0;
  border-radius: 6px;
  color: #4b5563;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  text-align: left;
}

.more-menu button:hover {
  background: #f8fafc;
}

.expanded-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  padding: 0 24px;
}

.expanded-row {
  display: flex;
  min-width: 0;
  height: 48px;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-radius: 6px;
  background: #f2f4f7;
}

.expanded-row > span:first-child {
  flex: 0 0 104px;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

.expanded-row :deep(.ant-picker) {
  min-width: 0;
  height: 32px;
  flex: 1;
  border-color: #d9d9d9;
  border-radius: 6px;
  background: #fff;
}

.expanded-row :deep(.ant-picker-input > input) {
  color: #8c8c8c;
  font-size: 14px;
}

.expanded-close {
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  color: #8c8c8c;
}

.expanded-close :deep(.anticon) {
  font-size: 14px;
}

.dirs-row.has-directories {
  height: auto;
  min-height: 82px;
  align-items: flex-start;
  padding-top: 8px;
  padding-bottom: 8px;
}

.dirs-row.has-directories > span:first-child {
  margin-top: 5px;
}

.directory-content {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}

.directory-list {
  display: flex;
  width: 100%;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}

.children-row :deep(.ant-select) {
  min-width: 0;
  flex: 1;
}

.children-row,
.dirs-row {
  grid-column: 1 / -1;
}

.children-row :deep(.ant-select-selector) {
  min-height: 32px !important;
  border-color: #d9d9d9 !important;
  border-radius: 6px !important;
  background: #fff !important;
}

.children-row :deep(.ant-select-selection-placeholder) {
  color: #bfbfbf;
  font-size: 14px;
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

.task-modal-footer {
  display: flex;
  height: 64px;
  align-items: center;
  justify-content: flex-end;
  gap: 16px;
  padding: 0 24px;
}

.task-modal-footer > span {
  display: none;
}

.task-modal-footer > div {
  display: flex;
  gap: 8px;
}

:global(.linear-task-modal .ant-modal) {
  max-width: calc(100vw - 32px);
}

:global(.linear-task-modal .ant-modal-content) {
  overflow: visible;
  padding: 0;
  border-radius: 16px;
}

:global(.linear-task-modal .ant-modal-body) {
  padding: 0;
}

:global(.linear-task-modal .ant-dropdown-menu-item) {
  min-width: 220px;
}

.menu-primary {
  display: block;
  color: #374151;
}

.pipeline-menu small,
:global(.ant-dropdown-menu-item small) {
  display: block;
  max-width: 270px;
  overflow: hidden;
  color: #9ca3af;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 760px) {
  .description-box,
  .description-input {
    height: 180px;
  }

  .expanded-fields {
    grid-template-columns: 1fr;
  }

  .task-modal-footer > span {
    display: none;
  }
}
</style>
