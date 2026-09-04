<template>
  <div class="projects-page">
    <header class="page-bar">
      <div>
        <h1>项目</h1>
        <i aria-hidden="true" />
        <small>管理本地可用项目</small>
      </div>
    </header>

    <main class="projects-content">
      <div class="content-title">
        <div class="content-heading">
          <h2>本地项目</h2>
          <span class="project-count">{{ projects.length }}</span>
        </div>
        <a-button
          class="add-project-button"
          type="primary"
          @click="openModal()"
        >
          <template #icon>
            <img
              :src="projectAddIcon"
              alt=""
              aria-hidden="true"
              class="add-project-icon"
            />
          </template>
          添加
        </a-button>
      </div>

      <div class="project-guide">
        <div class="project-guide-title">
          <img
            :src="projectGuideInfoIcon"
            alt=""
            aria-hidden="true"
          />
          <strong>项目用于绑定本地目录作为项目目录</strong>
        </div>
        <p>
          新建任务时选择「所属项目」，会自动将该项目目录作为任务的工作目录；勾选「关联项目」也会自动把对应目录加入任务的关联目录，方便
          Agent 在正确的代码目录中工作。
        </p>
      </div>

      <a-spin :spinning="loading">
        <div class="project-grid">
          <article
            v-for="project in projects"
            :key="project.uuid"
            class="project-card"
          >
            <div class="project-heading">
              <div class="project-identity">
                <img
                  v-if="project.icon_type === 'custom' && project.icon_url"
                  :src="project.icon_url"
                  alt=""
                  class="project-image"
                />
                <span
                  v-else
                  class="project-symbol"
                  :style="{ background: projectCardBackground(project.icon_type) }"
                >
                  <img
                    v-if="projectCardIcon(project.icon_type)"
                    :src="projectCardIcon(project.icon_type)"
                    alt=""
                    aria-hidden="true"
                    class="project-preset-icon"
                  />
                  <span
                    v-else
                    :style="{ color: projectIconPreset(project.icon_type).color }"
                  >
                    {{ projectIconPreset(project.icon_type).symbol }}
                  </span>
                </span>
                <strong :title="project.name">{{ project.name }}</strong>
              </div>

              <div class="card-actions">
                <a-dropdown
                  :open="activeMenuUuid === project.uuid"
                  :trigger="['click']"
                  placement="bottomRight"
                  @update:open="updateMenuOpen(project.uuid, $event)"
                >
                  <a-button
                    type="text"
                    size="small"
                    :aria-label="`管理项目“${project.name}”`"
                    aria-haspopup="menu"
                    :aria-expanded="activeMenuUuid === project.uuid"
                  >
                    <MoreOutlined />
                  </a-button>
                  <template #overlay>
                    <a-menu>
                      <a-menu-item @click="openModal(project)">
                        <EditOutlined />
                        编辑
                      </a-menu-item>
                      <a-menu-item
                        danger
                        @click="remove(project)"
                      >
                        <DeleteOutlined />
                        删除
                      </a-menu-item>
                    </a-menu>
                  </template>
                </a-dropdown>
              </div>
            </div>

            <p class="project-description">绑定本地目录，任务关联时作为工作目录</p>

            <div class="directory">
              <img
                :src="projectDirectoryIcon"
                alt=""
                aria-hidden="true"
                class="directory-icon"
              />
              <span :title="project.local_dir">{{ project.local_dir }}</span>
              <em>项目目录</em>
            </div>

            <footer>
              <span class="project-source">
                <img
                  :src="projectSourceIcon"
                  alt=""
                  aria-hidden="true"
                />
                <span>本地项目</span>
              </span>
            </footer>
          </article>
        </div>

        <a-empty
          v-if="!loading && !projects.length"
          description="暂无项目，点击右上角“添加”创建"
        />
      </a-spin>
    </main>

    <ProjectFormModal
      v-model:open="modalOpen"
      :project="editing"
      @saved="load"
    />
  </div>
</template>

<script setup lang="ts">
import { onActivated, onDeactivated, onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  DeleteOutlined,
  EditOutlined,
  MoreOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import projectAddIcon from '@/assets/icons/project-add.svg'
import projectDatabaseIcon from '@/assets/icons/project-card-database.svg'
import projectDirectoryIcon from '@/assets/icons/project-card-directory.svg'
import projectFolderIcon from '@/assets/icons/project-card-folder.svg'
import projectGlobeIcon from '@/assets/icons/project-card-globe.svg'
import projectSourceIcon from '@/assets/icons/project-card-source.svg'
import projectGuideInfoIcon from '@/assets/icons/project-guide-info.svg'
import projectModalImageIcon from '@/assets/icons/project-modal-icon-image.svg'
import { projectIconPreset } from '@/constants/project-icons'
import ProjectFormModal from '@/views/projects/components/ProjectFormModal.vue'
import type { LocalProject, ProjectIconKind } from '@/types/project'

const PROJECT_CARD_ICONS: Partial<Record<ProjectIconKind, string>> = {
  folder: projectFolderIcon,
  database: projectDatabaseIcon,
  globe: projectGlobeIcon,
  image: projectModalImageIcon,
}

const PROJECT_CARD_BACKGROUNDS: Partial<Record<ProjectIconKind, string>> = {
  folder: '#e5efff',
  code: '#ebfbf1',
  database: '#f6efff',
  globe: '#eafcff',
  image: '#ffe6f6',
}

const loading = ref(false)
const projects = ref<LocalProject[]>([])
const modalOpen = ref(false)
const editing = ref<LocalProject>()
const activeMenuUuid = ref('')
let deleteConfirm: ReturnType<typeof Modal.confirm> | undefined
let hasActivatedOnce = false

function projectCardIcon(type: ProjectIconKind) {
  return PROJECT_CARD_ICONS[type]
}

function projectCardBackground(type: ProjectIconKind) {
  return PROJECT_CARD_BACKGROUNDS[type] || projectIconPreset(type).background
}

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    projects.value = (await apiClient.get<{ items: LocalProject[] }>('/projects')).items || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '项目加载失败')
  } finally {
    if (showLoading) loading.value = false
  }
}

function openModal(project?: LocalProject) {
  activeMenuUuid.value = ''
  editing.value = project
  modalOpen.value = true
}

function remove(project: LocalProject) {
  activeMenuUuid.value = ''
  deleteConfirm?.destroy()
  deleteConfirm = Modal.confirm({
    title: `删除项目“${project.name}”？`,
    content: '仅删除项目配置，不会删除本地目录和已有任务。',
    okType: 'danger',
    onOk: async () => {
      await apiClient.delete(`/projects/${project.uuid}`)
      await load()
    },
    afterClose: () => {
      deleteConfirm = undefined
    },
  })
}

function updateMenuOpen(uuid: string, open: boolean) {
  activeMenuUuid.value = open ? uuid : ''
}

onMounted(load)

onActivated(() => {
  // KeepAlive 首次挂载也会触发激活钩子，首轮请求继续只由 onMounted 负责。
  if (!hasActivatedOnce) {
    hasActivatedOnce = true
    return
  }
  void load(false)
})

onDeactivated(() => {
  modalOpen.value = false
  editing.value = undefined
  activeMenuUuid.value = ''
  deleteConfirm?.destroy()
  deleteConfirm = undefined
})
</script>

<style scoped>
.projects-page {
  display: flex;
  height: calc(100% + 48px);
  margin: -24px;
  flex-direction: column;
  color: #1f2937;
  background: #f7f8fa;
}

.page-bar {
  display: flex;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}

.page-bar > div {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}

.page-bar h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.page-bar i {
  width: 4px;
  height: 4px;
  margin-left: 4px;
  border-radius: 50%;
  background: #d1d5db;
}

.page-bar small {
  color: #9ca3af;
  font-size: 12px;
  font-weight: 400;
}

.projects-content {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 0 24px;
  background: #fff;
}

.content-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 0;
  background: #fff;
}

.content-heading {
  display: flex;
  align-items: center;
  gap: 4px;
}

.content-title h2 {
  margin: 0;
  color: #262626;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.project-count {
  display: flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #8c95a8;
  background: #edeff2;
  font-size: 12px;
  font-weight: 400;
  line-height: 14px;
}

.add-project-button {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 4px;
  padding: 5px 16px;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
}

.add-project-button :deep(.ant-btn-icon) {
  display: flex;
  margin-inline-end: 0;
}

.add-project-icon {
  display: block;
  width: 12px;
  height: 12px;
}

.project-guide {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
  overflow: hidden;
  margin-bottom: 24px;
  padding: 12px 16px;
  border-radius: 12px;
  background: linear-gradient(90deg, #f0f2fd 0%, #dee4ff 100%);
}

.project-guide-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.project-guide-title img {
  display: block;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
}

.project-guide strong {
  color: #000;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.project-guide p {
  margin: 0;
  padding-left: 24px;
  color: #3a4559;
  font-size: 12px;
  font-weight: 400;
  line-height: 20px;
}

.project-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.project-card {
  position: relative;
  display: flex;
  height: 217px;
  min-width: 0;
  overflow: hidden;
  box-sizing: border-box;
  flex-direction: column;
  padding: 20px;
  border: 1px solid #d9d9d9;
  border-radius: 12px;
  background: #fff;
  transition:
    border-color 180ms ease,
    box-shadow 180ms ease;
}

.project-card:hover,
.project-card:focus-within {
  border-color: #b9c0cb;
  box-shadow: 0 4px 13px rgba(15, 23, 42, 0.08);
}

.project-heading,
.project-identity {
  display: flex;
  min-width: 0;
  align-items: center;
}

.project-heading {
  justify-content: space-between;
}

.project-identity {
  flex: 1;
  gap: 12px;
  padding-right: 4px;
}

.project-identity strong {
  overflow: hidden;
  min-width: 0;
  color: #1d1d1f;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-symbol,
.project-image {
  display: flex;
  width: 40px;
  height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 12px;
  object-fit: cover;
  font-size: 15px;
  font-weight: 700;
}

.project-preset-icon {
  display: block;
  width: 20px;
  height: 20px;
}

.card-actions {
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  opacity: 0;
  transition: opacity 180ms ease;
}

.project-card:hover .card-actions,
.project-card:focus-within .card-actions {
  opacity: 1;
}

.card-actions :deep(.ant-btn) {
  width: 28px;
  height: 28px;
  padding: 0;
  color: #969ba5;
}

.project-description {
  margin: 10px 0 0;
  color: #595959;
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
}

.directory {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  margin-top: 20px;
}

.directory-icon {
  display: block;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
}

.directory span {
  overflow: hidden;
  min-width: 0;
  flex: 1;
  color: #595959;
  font-size: 14px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.directory em {
  display: flex;
  min-height: 24px;
  flex: 0 0 auto;
  align-items: center;
  padding: 0 8px;
  border-radius: 6px;
  color: #3157e2;
  background: #e5efff;
  font-size: 12px;
  font-style: normal;
  font-weight: 600;
  line-height: 20px;
}

.project-card footer {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: auto;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
  color: #969ba5;
  font-size: 12px;
  line-height: 18px;
}

.project-source {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.project-source img {
  display: block;
  width: 18px;
  height: 18px;
  flex: 0 0 18px;
}

@media (max-width: 1100px) {
  .project-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .projects-content {
    padding: 0 16px;
  }

  .content-title {
    padding: 16px;
  }

  .project-grid {
    grid-template-columns: 1fr;
  }

  .page-bar small,
  .page-bar i {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .project-card,
  .card-actions {
    transition: none;
  }
}
</style>
