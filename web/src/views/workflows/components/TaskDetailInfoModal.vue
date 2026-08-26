<template>
  <a-modal
    :open="open"
    :footer="null"
    :closable="false"
    :body-style="{ padding: 0 }"
    width="922px"
    centered
    wrap-class-name="task-detail-info-modal"
    @cancel="handleOpenChange(false)"
    @update:open="handleOpenChange"
  >
    <div class="task-detail-modal">
      <header class="modal-header">
        <h2>需求详情</h2>
        <button type="button" aria-label="关闭需求详情弹窗" @click="handleOpenChange(false)">
          <img :src="closeIcon" alt="" aria-hidden="true" />
        </button>
      </header>

      <div v-if="task" class="modal-body">
        <section class="requirement-pane">
          <div class="requirement-summary">
            <h3>{{ task.title }}</h3>
            <div class="requirement-meta">
              <span class="meta-tag status-tag" :class="statusClass">{{ statusLabel }}</span>
              <span v-if="task.priority" class="meta-tag priority-tag" :class="priorityClass">
                {{ priorityLabel }}
              </span>
              <span class="meta-divider" aria-hidden="true"></span>
              <span class="deadline">截止日期：{{ plannedEndDateLabel }}</span>
            </div>
          </div>
          <div
            class="detail-desc-markdown"
            v-html="descriptionContent"
            @click="handleDescriptionClick"
          />
        </section>

        <aside class="task-info-pane">
          <h3>任务信息</h3>
          <div class="task-info-list">
            <div class="task-info-item">
              <span class="info-icon">
                <img :src="workDirIcon" alt="" aria-hidden="true" />
              </span>
              <div class="info-content">
                <span class="info-label">工作目录</span>
                <span class="info-value" :title="primaryWorkDir">{{ primaryWorkDir }}</span>
              </div>
            </div>

            <div class="task-info-item">
              <span class="info-icon">
                <img :src="projectIcon" alt="" aria-hidden="true" />
              </span>
              <div class="info-content">
                <span class="info-label">所属项目</span>
                <span class="info-value info-value-with-avatar">
                  <span
                    class="project-symbol"
                    :style="projectIconStyle(task.project_icon)"
                  >
                    {{ projectIconSymbol(task.project_icon) }}
                  </span>
                  <span :title="task.project_name || '未设置'">
                    {{ task.project_name || '未设置' }}
                  </span>
                </span>
              </div>
            </div>

            <div class="task-info-item">
              <span class="info-icon">
                <img :src="pipelineIcon" alt="" aria-hidden="true" />
              </span>
              <div class="info-content">
                <span class="info-label">流水线</span>
                <span class="info-value info-value-with-avatar">
                  <img
                    v-if="task.pipeline_avatar_snapshot"
                    :src="task.pipeline_avatar_snapshot"
                    alt=""
                  />
                  <span :title="task.pipeline_name_snapshot || '未分配'">
                    {{ task.pipeline_name_snapshot || '未分配' }}
                  </span>
                </span>
              </div>
            </div>

            <div class="task-info-item">
              <span class="info-icon">
                <img :src="dateIcon" alt="" aria-hidden="true" />
              </span>
              <div class="info-content">
                <span class="info-label">预期开始 / 结束</span>
                <span class="info-value" :title="plannedDateRange">{{ plannedDateRange }}</span>
              </div>
            </div>

            <div v-if="relatedProjects.length" class="task-info-item">
              <span class="info-icon">
                <img :src="relatedProjectIcon" alt="" aria-hidden="true" />
              </span>
              <div class="info-content">
                <span class="info-label">关联项目</span>
                <span
                  v-for="project in relatedProjects"
                  :key="project.uuid"
                  class="info-value info-value-with-avatar"
                >
                  <span
                    class="project-symbol"
                    :style="projectIconStyle(project.icon_type || project.icon)"
                  >
                    {{ projectIconSymbol(project.icon_type || project.icon) }}
                  </span>
                  <span :title="project.name">{{ project.name }}</span>
                </span>
              </div>
            </div>

            <div v-if="relatedDirectories.length" class="task-info-item">
              <span class="info-icon">
                <img :src="workDirIcon" alt="" aria-hidden="true" />
              </span>
              <div class="info-content">
                <span class="info-label">关联目录</span>
                <span
                  v-for="dir in relatedDirectories"
                  :key="dir"
                  class="info-value"
                  :title="dir"
                >
                  {{ dir }}
                </span>
              </div>
            </div>
          </div>
        </aside>
      </div>

      <div v-else class="modal-empty">暂无任务信息</div>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import closeIcon from '@/assets/icons/task-detail-close.svg'
import dateIcon from '@/assets/icons/task-detail-date.svg'
import pipelineIcon from '@/assets/icons/task-detail-pipeline.svg'
import projectIcon from '@/assets/icons/task-detail-project.svg'
import relatedProjectIcon from '@/assets/icons/task-detail-related-project.svg'
import workDirIcon from '@/assets/icons/task-detail-work-dir.svg'
import { projectIconPreset } from '@/constants/project-icons'
import type { TaskWithDetails } from '@/types/task-detail'

const props = defineProps<{
  open: boolean
  task?: TaskWithDetails
  status: string
  renderedDescription: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'preview-image': [url: string]
}>()

const statusLabel = computed(() => {
  if (props.status === 'pending') return '待开始'
  if (props.status === 'in_progress') return '进行中'
  if (props.status === 'blocked') return '已阻塞'
  return '已完成'
})
const statusClass = computed(() => `status-${props.status || 'done'}`)
const priorityLabel = computed(() => {
  if (props.task?.priority === 'high' || props.task?.priority === '高') return '高'
  if (props.task?.priority === 'medium' || props.task?.priority === '中') return '中'
  return '低'
})
const priorityClass = computed(() => `priority-${priorityLabel.value}`)
const plannedStartDateLabel = computed(() => formatDate(props.task?.planned_start_date))
const plannedEndDateLabel = computed(() => formatDate(props.task?.planned_end_date))
const plannedDateRange = computed(
  () => `${plannedStartDateLabel.value} ～ ${plannedEndDateLabel.value}`,
)
const descriptionContent = computed(() => props.renderedDescription || '暂无描述')
const primaryWorkDir = computed(() => props.task?.work_dir || props.task?.task_dir || '未设置')
const relatedProjects = computed(() =>
  (props.task?.projects || []).filter((project) => project.uuid !== props.task?.project_uuid),
)
const relatedDirectories = computed(() => {
  const directories = [
    ...(props.task?.work_dirs || []),
    ...relatedProjects.value.map((project) => project.local_dir || project.main_dir || ''),
  ]
  return [...new Set(directories.filter((dir) => dir && dir !== primaryWorkDir.value))]
})

function projectIconStyle(type?: string) {
  const icon = projectIconPreset(type)
  return { color: icon.color, background: icon.background }
}

function projectIconSymbol(type?: string) {
  return projectIconPreset(type).symbol
}

function formatDate(value?: string | number) {
  if (!value) return '未设置'
  if (typeof value === 'string' && !/^\d+$/.test(value)) return value
  const timestamp = Number(value)
  const date = new Date(timestamp < 1e12 ? timestamp * 1000 : timestamp)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function handleOpenChange(open: boolean) {
  emit('update:open', open)
}

function handleDescriptionClick(event: MouseEvent) {
  const target = event.target
  if (target instanceof HTMLImageElement) emit('preview-image', target.src)
}
</script>

<style scoped>
.task-detail-modal {
  overflow: hidden;
  border-radius: 16px;
  color: #262626;
  background: #fff;
  font-family: "PingFang SC", "Microsoft YaHei", sans-serif;
}

.modal-header {
  display: flex;
  height: 56px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  padding: 16px 24px;
  border-bottom: 1px solid #f0f0f0;
}

.modal-header h2,
.task-info-pane h3 {
  margin: 0;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.modal-header button {
  display: flex;
  width: 16px;
  height: 16px;
  padding: 0;
  border: 0;
  align-items: center;
  justify-content: center;
  background: transparent;
  cursor: pointer;
}

.modal-header button img {
  width: 12px;
  height: 12px;
}

.modal-body {
  display: grid;
  height: min(644px, calc(100vh - 88px));
  min-height: 420px;
  grid-template-columns: minmax(0, 600px) minmax(0, 322px);
}

.requirement-pane {
  min-width: 0;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 24px;
  border-right: 1px solid #f0f0f0;
  scrollbar-color: #a1a7b2 transparent;
  scrollbar-width: thin;
}

.requirement-pane::-webkit-scrollbar,
.task-info-pane::-webkit-scrollbar {
  width: 4px;
}

.requirement-pane::-webkit-scrollbar-thumb,
.task-info-pane::-webkit-scrollbar-thumb {
  border-radius: 16px;
  background: #a1a7b2;
}

.requirement-summary {
  margin-bottom: 16px;
}

.requirement-summary h3 {
  margin: 0 0 12px;
  color: #262626;
  font-size: 22px;
  font-weight: 600;
  line-height: 32px;
  overflow-wrap: anywhere;
}

.requirement-meta {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.meta-tag {
  display: inline-flex;
  height: 22px;
  box-sizing: border-box;
  flex: 0 0 auto;
  align-items: center;
  padding: 0 9px;
  border: 1px solid currentColor;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 400;
  line-height: 20px;
}

.status-in_progress {
  color: #fa8c16;
}

.status-pending {
  color: #7a8699;
}

.status-blocked,
.priority-高 {
  color: #fb363f;
}

.status-done {
  color: #52c41a;
}

.priority-中 {
  color: #fa8c16;
}

.priority-低 {
  color: #52c41a;
}

.meta-divider {
  width: 1px;
  height: 12px;
  flex: 0 0 auto;
  border-radius: 1px;
  background: #f0f0f0;
}

.deadline {
  overflow: hidden;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-desc-markdown {
  color: #262626;
  font-size: 14px;
  font-weight: 400;
  line-height: 28px;
  overflow-wrap: anywhere;
}

.detail-desc-markdown :deep(p),
.detail-desc-markdown :deep(ul),
.detail-desc-markdown :deep(ol),
.detail-desc-markdown :deep(blockquote),
.detail-desc-markdown :deep(pre) {
  margin: 0;
}

.detail-desc-markdown :deep(ul),
.detail-desc-markdown :deep(ol) {
  padding-left: 21px;
}

.detail-desc-markdown :deep(h1),
.detail-desc-markdown :deep(h2),
.detail-desc-markdown :deep(h3),
.detail-desc-markdown :deep(h4),
.detail-desc-markdown :deep(h5),
.detail-desc-markdown :deep(h6) {
  margin: 12px 0 4px;
  color: inherit;
  font-size: inherit;
  font-weight: 600;
  line-height: inherit;
}

.detail-desc-markdown :deep(pre),
.detail-desc-markdown :deep(table) {
  max-width: 100%;
  overflow: auto;
}

.detail-desc-markdown :deep(img) {
  max-width: 100%;
  max-height: 320px;
  object-fit: contain;
  border: 1px solid #f0f0f0;
  border-radius: 4px;
  cursor: pointer;
  vertical-align: middle;
}

.detail-desc-markdown :deep(img):hover {
  border-color: #3157e2;
  box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.15);
}

.task-info-pane {
  min-width: 0;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 24px;
  scrollbar-color: #a1a7b2 transparent;
  scrollbar-width: thin;
}

.task-info-pane h3 {
  margin-bottom: 24px;
}

.task-info-list {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.task-info-item {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 8px;
}

.info-icon {
  display: flex;
  width: 26px;
  height: 26px;
  flex: 0 0 26px;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  background: #f2f4f7;
}

.info-icon img {
  width: 16px;
  height: 16px;
}

.info-content {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 4px;
}

.info-label {
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}

.info-value {
  display: block;
  min-width: 0;
  overflow: hidden;
  color: #242933;
  font-size: 14px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.info-value-with-avatar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.info-value-with-avatar > img {
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  border-radius: 50%;
  object-fit: cover;
}

.project-symbol {
  display: flex;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  font-size: 9px;
  font-weight: 700;
  line-height: 20px;
}

.info-value-with-avatar > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.modal-empty {
  display: flex;
  height: 360px;
  align-items: center;
  justify-content: center;
  color: #8c8c8c;
  font-size: 14px;
}

:global(.task-detail-info-modal .ant-modal) {
  max-width: calc(100vw - 32px);
  padding-bottom: 0;
}

:global(.task-detail-info-modal .ant-modal-content) {
  overflow: hidden;
  padding: 0;
  border-radius: 16px;
}

:global(.task-detail-info-modal .ant-modal-body) {
  padding: 0;
}

@media (max-width: 760px) {
  .modal-body {
    display: block;
    height: calc(100vh - 88px);
    min-height: 0;
    overflow-y: auto;
  }

  .requirement-pane,
  .task-info-pane {
    overflow: visible;
  }

  .requirement-pane {
    border-right: 0;
  }

  .task-info-pane {
    border-top: 1px solid #f0f0f0;
  }

  .requirement-meta {
    flex-wrap: wrap;
  }
}
</style>
